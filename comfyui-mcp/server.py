#!/usr/bin/env python3
"""ComfyUI MCP 服务（stdio，纯标准库）。

设计目标：直接用 ComfyUI 便携包自带的 `python_embeded\\python.exe` 跑，不装任何依赖，
不吃掉 ComfyUI 环境的包版本、不需要 comfy-cli。

    D:\\...\\python_embeded\\python.exe server.py

工具一览见 README.md / tools_list。日志一律走 stderr，stdout 只留 MCP 协议报文。
"""

from __future__ import annotations

import base64
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)

import comfy_client  # noqa: E402
import workflows  # noqa: E402
from comfy_client import ComfyClient, ComfyError  # noqa: E402
from workflows import WorkflowError, apply_overrides, build_txt2img, is_api_graph, load_workflow, ui_to_api  # noqa: E402

SERVER_NAME = "comfyui-mcp"
SERVER_VERSION = "0.1.0"
PROTOCOL_VERSIONS = ("2025-06-18", "2025-03-26", "2024-11-05")
INLINE_IMAGE_LIMIT = 6 * 1024 * 1024  # 超过 6MB 不塞进 MCP 响应，只给路径


# --------------------------------------------------------------------- 日志


def log(*parts):
    print("[comfyui-mcp]", *parts, file=sys.stderr, flush=True)


# --------------------------------------------------------------------- 配置

DEFAULTS = {
    "base_url": "http://127.0.0.1:8188",
    "portable_root": "",
    "comfy_dir": "",
    "python": "",
    "default_checkpoint": "",
    "workflow_dirs": [],
    "output_dir": "",
    "launch_args": ["--windows-standalone-build"],
    "startup_timeout": 240,
    "http_host": "127.0.0.1",
    "http_port": 8189,
    "http_token": "",
}


def load_config():
    cfg = dict(DEFAULTS)
    for name in ("config.json", "config.local.json"):
        path = os.path.join(HERE, name)
        if os.path.isfile(path):
            with open(path, "r", encoding="utf-8") as fh:
                cfg.update(json.load(fh))
            break

    # 环境变量优先，方便换机 / toolbox 注入
    if os.environ.get("COMFYUI_URL"):
        cfg["base_url"] = os.environ["COMFYUI_URL"]
    if os.environ.get("COMFYUI_PORTABLE_ROOT"):
        cfg["portable_root"] = os.environ["COMFYUI_PORTABLE_ROOT"]
    if os.environ.get("COMFYUI_DIR"):
        cfg["comfy_dir"] = os.environ["COMFYUI_DIR"]
    if os.environ.get("COMFYUI_PYTHON"):
        cfg["python"] = os.environ["COMFYUI_PYTHON"]
    if os.environ.get("COMFYUI_CHECKPOINT"):
        cfg["default_checkpoint"] = os.environ["COMFYUI_CHECKPOINT"]
    if os.environ.get("COMFYUI_MCP_OUTPUT_DIR"):
        cfg["output_dir"] = os.environ["COMFYUI_MCP_OUTPUT_DIR"]

    root = cfg.get("portable_root") or ""
    if root and not cfg.get("comfy_dir"):
        cfg["comfy_dir"] = os.path.join(root, "ComfyUI")
    if root and not cfg.get("python"):
        cfg["python"] = os.path.join(root, "python_embeded", "python.exe")
    return cfg


CONFIG = load_config()
CLIENT = ComfyClient(CONFIG["base_url"])
RUN_DIR = os.path.join(HERE, ".run")
PID_FILE = os.path.join(RUN_DIR, "comfyui.pid")
LOG_FILE = os.path.join(RUN_DIR, "comfyui.log")


def workflow_dirs():
    dirs = []
    if CONFIG.get("comfy_dir"):
        dirs.append(os.path.join(CONFIG["comfy_dir"], "user", "default", "workflows"))
    dirs.extend(CONFIG.get("workflow_dirs") or [])
    dirs.append(os.path.join(HERE, "workflows"))
    return [d for d in dirs if os.path.isdir(d)]


def ensure_output_dir(explicit=None):
    out = explicit or CONFIG.get("output_dir") or ""
    if not out and CONFIG.get("comfy_dir"):
        out = os.path.join(CONFIG["comfy_dir"], "output", "mcp")
    out = os.path.expanduser(out or os.path.join(RUN_DIR, "outputs"))
    if not os.path.isabs(out):
        out = os.path.join(HERE, out)
    os.makedirs(out, exist_ok=True)
    return out


# ------------------------------------------------------------------- 工具表

TOOLS = {}


def tool(name, description, schema=None):
    def wrap(fn):
        TOOLS[name] = {
            "name": name,
            "description": description,
            "inputSchema": schema or {"type": "object", "properties": {}, "additionalProperties": False},
            "fn": fn,
        }
        return fn

    return wrap


def _obj(props, required=()):
    return {
        "type": "object",
        "properties": props,
        "required": list(required),
        "additionalProperties": False,
    }


# ------------------------------------------------------------------- 通用块


def _call(fn, *args, **kwargs):
    """工具统一入口：把异常翻译成 MCP 的 isError 内容。"""
    try:
        return fn(*args, **kwargs)
    except (ComfyError, WorkflowError) as exc:
        return {"error": str(exc)}
    except Exception as exc:  # noqa: BLE001 - 兜底，避免整个 stdio 循环挂掉
        log("tool error:", repr(exc))
        return {"error": f"{type(exc).__name__}: {exc}"}


def _image_contents(files, inline=True):
    """把输出图片读成 MCP image 内容（供 agent 直接看），非图片/超大文件只留路径。"""
    if not inline:
        return []
    out = []
    for item in files:
        local = item.get("local_path")
        if not local or not os.path.isfile(local):
            continue
        ext = os.path.splitext(local)[1].lower()
        mime = {".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp", ".gif": "image/gif"}.get(ext)
        if not mime or os.path.getsize(local) > INLINE_IMAGE_LIMIT:
            continue
        with open(local, "rb") as fh:
            out.append({"type": "image", "data": base64.b64encode(fh.read()).decode("ascii"), "mimeType": mime})
    return out


def _submit(graph, wait=True, timeout=600, save_to=None, extra_data=None):
    """提交工作流，按需等结果、下载输出，返回可直接给 agent 的 dict。"""
    resp = CLIENT.queue_prompt(graph, extra_data=extra_data)
    prompt_id = resp.get("prompt_id") or (resp.get("data") or {}).get("prompt_id")
    if not prompt_id:
        return {"error": f"提交没拿到 prompt_id：{json.dumps(resp, ensure_ascii=False)[:800]}"}

    result = {"prompt_id": prompt_id, "queued": True}
    started = time.time()

    if wait:
        job, timed_out = CLIENT.wait_job(prompt_id, timeout=timeout)
        status = (job or {}).get("status", "unknown")
        result.update({"status": status, "elapsed_seconds": round(time.time() - started, 1)})
        if timed_out:
            result["note"] = f"等了 {timeout}s 还没跑完，任务仍在队列里。用 job(action='wait') 继续等。"
            return result
        if status in ("failed", "cancelled"):
            info = (job or {}).get("execution_error")
            result["error"] = _summarize_error(info) if info else f"任务 {status}"
            return result
    else:
        result["note"] = f"已入队。用 job(prompt_id='{prompt_id}', action='wait') 收结果。"
        return result

    out_dir = ensure_output_dir(save_to)
    files = CLIENT.collect_outputs(prompt_id)
    for item in files:
        try:
            blob = CLIENT.view_bytes(item["filename"], item["subfolder"], item["type"])
            dest = os.path.join(out_dir, os.path.basename(item["filename"]))
            with open(dest, "wb") as fh:
                fh.write(blob)
            item["local_path"] = dest
        except ComfyError as exc:
            item["error"] = str(exc)
    result["outputs"] = files
    result["output_dir"] = out_dir
    if not files:
        result["note"] = "任务成功但没解析到输出文件，看看 workflow 里是否没有 SaveImage/PreviewImage 节点。"
    result["_images"] = _image_contents(files)
    return result


def _summarize_error(info):
    if not isinstance(info, dict):
        return str(info)
    bits = [info.get("exception_type"), info.get("exception_message")]
    node = info.get("node_id")
    text = ": ".join(b for b in bits if b) or "执行失败"
    if node:
        text = f"节点 {node}({info.get('node_type')}) 报错 — {text}"
    return text


def _split_result(result):
    """把带 _images 的结果拆成 (json_payload, extra_content)。"""
    images = result.pop("_images", []) if isinstance(result, dict) else []
    return result, images


def _dump(payload):
    return json.dumps(payload, ensure_ascii=False, indent=2, default=str)


def _resolve_checkpoint(name):
    if name:
        return name
    if CONFIG.get("default_checkpoint"):
        return CONFIG["default_checkpoint"]
    try:
        items = CLIENT.models("checkpoints")
    except ComfyError:
        items = []
    if isinstance(items, list) and items:
        return items[0]
    raise ComfyError("没有可用 checkpoint：先在 ComfyUI 里放模型，或传 checkpoint 参数")


def _resolve_input_image(image):
    """支持：本地路径（自动上传）/ 已在 input 里的文件名。返回可用于 LoadImage 的名字。"""
    if not image:
        raise WorkflowError("少了 image 参数")
    image = os.path.expanduser(image)
    if os.path.isfile(image):
        info = CLIENT.upload_image(image)
        name = info.get("name") or os.path.basename(image)
        sub = info.get("subfolder") or ""
        return f"{sub}/{name}" if sub else name
    return image


# --------------------------------------------------------------------- 工具


@tool(
    "comfy_status",
    "看 ComfyUI 是否在跑：版本、显存、内存、队列长度。任何生成操作前先调它。",
    _obj({}),
)
def comfy_status():
    if not CLIENT.is_up():
        return {
            "up": False,
            "base_url": CONFIG["base_url"],
            "hint": "ComfyUI 没在跑。调用 launch_comfy 拉起来（会用便携包的 python_embeded + run_nvidia_gpu）。",
        }
    stats = CLIENT.system_stats()
    queue = CLIENT.queue()
    devices = []
    for dev in stats.get("devices") or []:
        devices.append(
            {
                "name": dev.get("name"),
                "type": dev.get("type"),
                "vram_total_mb": round((dev.get("vram_total") or 0) / 1048576),
                "vram_free_mb": round((dev.get("vram_free") or 0) / 1048576),
            }
        )
    system = stats.get("system") or {}
    return {
        "up": True,
        "base_url": CONFIG["base_url"],
        "comfyui_version": system.get("comfyui_version"),
        "python": system.get("python_version"),
        "ram_free_mb": round((system.get("ram_free") or 0) / 1048576),
        "ram_total_mb": round((system.get("ram_total") or 0) / 1048576),
        "devices": devices,
        "queue": {
            "running": len(queue.get("queue_running") or []),
            "pending": len(queue.get("queue_pending") or []),
        },
    }


@tool(
    "launch_comfy",
    "启动本地 ComfyUI（便携包：python_embeded\\python.exe -s ComfyUI\\main.py，日志写到 .run/comfyui.log）。已在跑则直接返回。",
    _obj(
        {
            "wait": {"type": "boolean", "default": True, "description": "是否等 /system_stats 通"},
            "timeout": {"type": "number", "default": 240, "description": "等待秒数，冷启动加载模型可能 1-3 分钟"},
            "extra_args": {"type": "array", "items": {"type": "string"}, "description": "额外命令行参数，如 --lowvram"},
        }
    ),
)
def launch_comfy(wait=True, timeout=None, extra_args=None):
    if CLIENT.is_up():
        return {"up": True, "already_running": True, "base_url": CONFIG["base_url"]}

    py, main = CONFIG.get("python"), CONFIG.get("comfy_dir")
    if not py or not os.path.isfile(py):
        return {"error": f"找不到便携包 python：{py}。检查 config.json 的 portable_root / python。"}
    if not main or not os.path.isdir(main):
        return {"error": f"找不到 ComfyUI 目录：{main}。检查 config.json 的 comfy_dir。"}

    os.makedirs(RUN_DIR, exist_ok=True)
    args = list(CONFIG.get("launch_args") or []) + list(extra_args or [])
    cmd = [py, "-s", os.path.join(main, "main.py")] + args

    # Windows：脱离父进程独立跑，输出写文件，等不了就超时
    creationflags = 0
    if os.name == "nt":
        creationflags = 0x00000008 | 0x00000200  # DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP

    logfile = open(LOG_FILE, "ab", buffering=0)
    logfile.write(f"\n=== {time.strftime('%Y-%m-%d %H:%M:%S')} launch: {' '.join(cmd)}\n".encode("utf-8"))
    proc = subprocess.Popen(
        cmd,
        cwd=CONFIG.get("portable_root") or main,
        stdout=logfile,
        stderr=subprocess.STDOUT,
        stdin=subprocess.DEVNULL,
        creationflags=creationflags,
        close_fds=True,
    )
    with open(PID_FILE, "w", encoding="utf-8") as fh:
        fh.write(str(proc.pid))

    if not wait:
        return {"started": True, "pid": proc.pid, "log": LOG_FILE, "note": "还没确认端口起来，稍后用 comfy_status 探。"}

    deadline = time.time() + float(timeout or CONFIG.get("startup_timeout") or 240)
    while time.time() < deadline:
        if CLIENT.is_up(timeout=2):
            return {"up": True, "pid": proc.pid, "log": LOG_FILE, "waited_seconds": round(time.time() - (deadline - float(timeout or 240)), 1)}
        if proc.poll() is not None:
            return {"up": False, "pid": proc.pid, "exit_code": proc.returncode, "log_tail": _tail(LOG_FILE, 40), "error": "进程退出了，看 log_tail"}
        time.sleep(2)

    return {"up": False, "pid": proc.pid, "log_tail": _tail(LOG_FILE, 40), "error": f"等了 {timeout}s 端口还没通"}


@tool(
    "stop_comfy",
    "停掉由 launch_comfy 启动的 ComfyUI（按 .run/comfyui.pid 结束进程）。",
    _obj({}),
)
def stop_comfy():
    if not os.path.isfile(PID_FILE):
        return {"stopped": False, "note": "没有记录的 pid（可能不是本 MCP 启动的），请手动关窗口。"}
    with open(PID_FILE, "r", encoding="utf-8") as fh:
        pid = int((fh.read() or "0").strip() or 0)
    if not pid:
        return {"stopped": False, "error": "pid 文件是空的"}
    try:
        if os.name == "nt":
            subprocess.run(["taskkill", "/PID", str(pid), "/T", "/F"], capture_output=True, check=False)
        else:
            os.kill(pid, 15)
    except Exception as exc:  # noqa: BLE001
        return {"stopped": False, "error": f"{type(exc).__name__}: {exc}"}
    try:
        os.remove(PID_FILE)
    except OSError:
        pass
    return {"stopped": True, "pid": pid}


@tool(
    "generate_image",
    "一句话出图：内置拼一个 SD1.5/SDXL 的 txt2img 图（可选 LoRA、CLIP skip），跑完把图片直接返回。",
    _obj(
        {
            "prompt": {"type": "string", "description": "正向提示词（英文标签效果最好）"},
            "negative": {"type": "string", "description": "负向提示词"},
            "checkpoint": {"type": "string", "description": "模型文件名，默认取 config 里配的/列表第一个"},
            "width": {"type": "integer", "default": 512},
            "height": {"type": "integer", "default": 768},
            "steps": {"type": "integer", "default": 28},
            "cfg": {"type": "number", "default": 7.0},
            "sampler": {"type": "string", "default": "dpmpp_2m"},
            "scheduler": {"type": "string", "default": "karras"},
            "seed": {"type": "integer", "description": "-1 或省略 = 随机"},
            "batch_size": {"type": "integer", "default": 1},
            "clip_skip": {"type": "integer", "description": "动漫模型常用 -2"},
            "loras": {
                "type": "array",
                "items": {"type": "object"},
                "description": '[{"name": "xiaoyan.safetensors", "strength_model": 0.8, "strength_clip": 0.8}]',
            },
            "wait": {"type": "boolean", "default": True},
            "timeout": {"type": "number", "default": 600},
        },
        required=["prompt"],
    ),
)
def generate_image(prompt, negative=None, checkpoint=None, width=512, height=768, steps=28, cfg=7.0,
                   sampler="dpmpp_2m", scheduler="karras", seed=None, batch_size=1, clip_skip=None,
                   loras=None, wait=True, timeout=600):
    ckpt = _resolve_checkpoint(checkpoint)
    graph, slots = build_txt2img(
        checkpoint=ckpt,
        positive=prompt,
        negative=negative if negative is not None else "lowres, bad anatomy, bad hands, worst quality, low quality, jpeg artifacts, watermark",
        width=width,
        height=height,
        steps=steps,
        cfg=cfg,
        sampler=sampler,
        scheduler=scheduler,
        seed=seed,
        batch_size=batch_size,
        clip_skip=clip_skip,
        loras=loras,
    )
    out = _submit(graph, wait=wait, timeout=timeout)
    if "error" not in out:
        out["request"] = {
            "checkpoint": ckpt,
            "seed": graph["3"]["inputs"]["seed"],
            "size": [width, height],
            "steps": steps,
            "cfg": cfg,
            "sampler": f"{sampler}/{scheduler}",
            "clip_skip": clip_skip,
            "loras": loras,
        }
    return out


@tool(
    "img2img",
    "图生图：给一张本地图片路径（自动上传到 input/）或已在 input 里的文件名，按 denoise 重绘。",
    _obj(
        {
            "image": {"type": "string", "description": "本地绝对路径，或 input 目录里的文件名"},
            "prompt": {"type": "string"},
            "negative": {"type": "string"},
            "checkpoint": {"type": "string"},
            "denoise": {"type": "number", "default": 0.55, "description": "0.4~0.55 保形，0.65+ 大改"},
            "steps": {"type": "integer", "default": 28},
            "cfg": {"type": "number", "default": 7.0},
            "sampler": {"type": "string", "default": "dpmpp_2m"},
            "scheduler": {"type": "string", "default": "karras"},
            "seed": {"type": "integer"},
            "clip_skip": {"type": "integer"},
            "loras": {"type": "array", "items": {"type": "object"}},
            "wait": {"type": "boolean", "default": True},
            "timeout": {"type": "number", "default": 600},
        },
        required=["image", "prompt"],
    ),
)
def img2img(image, prompt, negative=None, checkpoint=None, denoise=0.55, steps=28, cfg=7.0,
            sampler="dpmpp_2m", scheduler="karras", seed=None, clip_skip=None, loras=None,
            wait=True, timeout=600):
    ckpt = _resolve_checkpoint(checkpoint)
    image_name = _resolve_input_image(image)
    graph, _ = build_txt2img(
        checkpoint=ckpt,
        positive=prompt,
        negative=negative if negative is not None else "lowres, bad anatomy, bad hands, worst quality, low quality",
        steps=steps,
        cfg=cfg,
        sampler=sampler,
        scheduler=scheduler,
        seed=seed,
        clip_skip=clip_skip,
        loras=loras,
        denoise=denoise,
        init_image=image_name,
    )
    out = _submit(graph, wait=wait, timeout=timeout)
    if "error" not in out:
        out["request"] = {"checkpoint": ckpt, "init_image": image_name, "denoise": denoise, "seed": graph["3"]["inputs"]["seed"]}
    return out


@tool(
    "list_workflows",
    "列出可跑的工作流：ComfyUI 界面里保存的（user/default/workflows）+ 本仓库 workflows/ 下的。",
    _obj({}),
)
def list_workflows():
    items = []
    for d in workflow_dirs():
        for name in sorted(os.listdir(d)):
            if not name.lower().endswith(".json"):
                continue
            path = os.path.join(d, name)
            kind = "?"
            try:
                with open(path, "r", encoding="utf-8") as fh:
                    data = json.load(fh)
                kind = "api" if is_api_graph(data) else ("ui" if "nodes" in data else "unknown")
            except Exception as exc:  # noqa: BLE001
                kind = f"unreadable: {exc}"
            items.append({"name": os.path.splitext(name)[0], "file": path, "format": kind, "dir": d})
    return {"count": len(items), "workflows": items}


@tool(
    "workflow_slots",
    "看一个工作流里哪些参数能改（节点id.输入名 / 标题.输入名），跑之前先调它拿可覆盖的槽位。",
    _obj({"workflow": {"type": "string", "description": "工作流名（可省 .json）或绝对路径"}}, required=["workflow"]),
)
def workflow_slots(workflow):
    data, path = load_workflow(workflow, workflow_dirs())
    warnings = []
    if is_api_graph(data):
        slots = []
        for node_id, node in data.items():
            for name, value in (node.get("inputs") or {}).items():
                if isinstance(value, list):  # 连线
                    continue
                title = (node.get("_meta") or {}).get("title") or node.get("class_type")
                slots.append(
                    {
                        "ref": f"{node_id}.{name}",
                        "node": node_id,
                        "title": title,
                        "class_type": node.get("class_type"),
                        "input": name,
                        "value": value if not isinstance(value, str) or len(value) < 80 else value[:77] + "...",
                    }
                )
    else:
        _, slots = ui_to_api(data, CLIENT.object_info(), warn=warnings.append)
    return {"workflow": path, "slots": slots, "warnings": warnings}


@tool(
    "run_workflow",
    "跑一个已有工作流：UI 格式自动转 API 格式。overrides 用 '节点id.输入名' 或 '标题.输入名' 覆盖参数。",
    _obj(
        {
            "workflow": {"type": "string", "description": "工作流名（可省 .json）或绝对路径"},
            "overrides": {"type": "object", "description": '{"6.text": "1girl", "3.seed": 12345, "正向提示词.text": "..."}'},
            "wait": {"type": "boolean", "default": True},
            "timeout": {"type": "number", "default": 900},
            "save_to": {"type": "string", "description": "输出另存目录，默认 ComfyUI/output/mcp"},
        },
        required=["workflow"],
    ),
)
def run_workflow(workflow, overrides=None, wait=True, timeout=900, save_to=None):
    data, path = load_workflow(workflow, workflow_dirs())
    warnings = []

    if is_api_graph(data):
        graph, slots = data, None
    else:
        graph, slots = ui_to_api(data, CLIENT.object_info(), warn=warnings.append)

    applied = []
    if overrides:
        if slots is None:  # API 格式：直接按 节点id.输入名 落值
            for key, value in overrides.items():
                node_id, _, input_name = key.partition(".")
                if node_id not in graph or input_name not in graph[node_id].get("inputs", {}):
                    raise WorkflowError(f"覆盖项 {key!r} 在 API 工作流里找不到")
                graph[node_id]["inputs"][input_name] = value
                applied.append({"ref": key, "value": value})
        else:
            graph, applied = apply_overrides(graph, slots, overrides)
            graph = {str(k): v for k, v in graph.items()}

    out = _submit(graph, wait=wait, timeout=timeout, save_to=save_to)
    out["workflow"] = path
    if applied:
        out["overrides_applied"] = applied
    if warnings:
        out["warnings"] = warnings
    return out


@tool(
    "job",
    "任务查询/等待/取消/列表。action: status | wait | cancel | list。",
    _obj(
        {
            "action": {"type": "string", "enum": ["status", "wait", "cancel", "list"], "default": "status"},
            "prompt_id": {"type": "string"},
            "timeout": {"type": "number", "default": 900},
            "save_to": {"type": "string", "description": "action=wait 时把输出下载到该目录"},
        },
        required=["action"],
    ),
)
def job(action="status", prompt_id=None, timeout=900, save_to=None):
    if action == "list":
        return CLIENT.jobs(limit=20)

    if not prompt_id:
        raise ComfyError("这个 action 需要 prompt_id")

    if action == "cancel":
        return {"prompt_id": prompt_id, "result": CLIENT.cancel(prompt_id)}

    if action == "wait":
        job_info, timed_out = CLIENT.wait_job(prompt_id, timeout=timeout)
        out = {"prompt_id": prompt_id, "status": (job_info or {}).get("status", "unknown"), "timed_out": timed_out}
        if not timed_out and out["status"] == "completed":
            out_dir = ensure_output_dir(save_to)
            files = CLIENT.collect_outputs(prompt_id)
            for item in files:
                blob = CLIENT.view_bytes(item["filename"], item["subfolder"], item["type"])
                dest = os.path.join(out_dir, os.path.basename(item["filename"]))
                with open(dest, "wb") as fh:
                    fh.write(blob)
                item["local_path"] = dest
            out["outputs"] = files
            out["output_dir"] = out_dir
            out["_images"] = _image_contents(files)
        elif out["status"] in ("failed",):
            out["error"] = _summarize_error((job_info or {}).get("execution_error"))
        return out

    info = CLIENT.job(prompt_id)
    if info is None:
        return {"prompt_id": prompt_id, "status": "unknown", "note": "队列和历史里都没有这个 id（可能被清空过）"}
    return {
        "prompt_id": prompt_id,
        "status": info.get("status"),
        "outputs_count": info.get("outputs_count"),
        "preview_output": info.get("preview_output"),
        "execution_error": info.get("execution_error"),
    }


@tool(
    "queue",
    "看当前队列：正在跑的和排队的 prompt_id。",
    _obj({}),
)
def queue():
    data = CLIENT.queue()
    running = [{"prompt_id": item[1], "number": item[0]} for item in data.get("queue_running") or []]
    pending = [{"prompt_id": item[1], "number": item[0]} for item in data.get("queue_pending") or []]
    return {"running": running, "pending": pending}


@tool(
    "fetch_outputs",
    "把某个任务的输出文件下载到指定目录（默认 ComfyUI/output/mcp），可选直接回传图片。",
    _obj(
        {
            "prompt_id": {"type": "string"},
            "out_dir": {"type": "string"},
            "inline": {"type": "boolean", "default": True},
        },
        required=["prompt_id"],
    ),
)
def fetch_outputs(prompt_id, out_dir=None, inline=True):
    target = ensure_output_dir(out_dir)
    files = CLIENT.collect_outputs(prompt_id)
    for item in files:
        blob = CLIENT.view_bytes(item["filename"], item["subfolder"], item["type"])
        dest = os.path.join(target, os.path.basename(item["filename"]))
        with open(dest, "wb") as fh:
            fh.write(blob)
        item["local_path"] = dest
    return {"prompt_id": prompt_id, "out_dir": target, "count": len(files), "outputs": files, "_images": _image_contents(files, inline)}


@tool(
    "upload_image",
    "把本地图片放进 ComfyUI 的 input 目录（图生图 / 蒙版前置步骤），返回可直接给 LoadImage 用的名字。",
    _obj({"path": {"type": "string"}, "subfolder": {"type": "string"}}, required=["path"]),
)
def upload_image(path, subfolder=""):
    info = CLIENT.upload_image(path, subfolder=subfolder or "")
    name = info.get("name") or os.path.basename(path)
    sub = info.get("subfolder") or ""
    return {"uploaded": info, "load_image_value": f"{sub}/{name}" if sub else name}


@tool(
    "list_models",
    "列模型文件。folder: checkpoints | loras | vae | controlnet | ...（不给则列出所有分类）。",
    _obj({"folder": {"type": "string"}}),
)
def list_models(folder=None):
    if not folder:
        return {"folders": CLIENT.models()}
    items = CLIENT.models(folder)
    return {"folder": folder, "count": len(items) if isinstance(items, list) else None, "files": items}


@tool(
    "search_nodes",
    "在 /object_info 里搜节点（含自定义节点），返回输入/输出签名，用来手写或改工作流。",
    _obj(
        {
            "keyword": {"type": "string", "description": "节点类名或关键词，如 KSampler / lora / SeeThrough"},
            "limit": {"type": "integer", "default": 15},
            "detail": {"type": "boolean", "default": False, "description": "true 时带上完整输入定义（较长）"},
        },
        required=["keyword"],
    ),
)
def search_nodes(keyword, limit=15, detail=False):
    info = CLIENT.object_info()
    kw = (keyword or "").lower()
    hits = []
    for name, spec in info.items():
        hay = " ".join([name, str(spec.get("display_name") or ""), str(spec.get("category") or ""), str(spec.get("description") or "")]).lower()
        if kw and kw not in hay:
            continue
        entry = {
            "class_type": name,
            "display_name": spec.get("display_name"),
            "category": spec.get("category"),
            "output": spec.get("output"),
            "output_name": spec.get("output_name"),
        }
        if detail:
            entry["input"] = spec.get("input")
        else:
            req = list((spec.get("input", {}).get("required") or {}).keys())
            opt = list((spec.get("input", {}).get("optional") or {}).keys())
            entry["required_inputs"] = req
            entry["optional_inputs"] = opt
        hits.append(entry)
        if len(hits) >= int(limit):
            break
    return {"keyword": keyword, "total_nodes": len(info), "matches": hits}


@tool("free_memory", "让 ComfyUI 卸载模型、释放显存（换模型 / 显存不够时用）。", _obj({}))
def free_memory():
    CLIENT.free_memory()
    return {"freed": True}


@tool("interrupt", "打断当前正在跑的任务（不动排队中的）。", _obj({}))
def interrupt():
    CLIENT.interrupt()
    return {"interrupted": True}


def _tail(path, lines=40):
    try:
        with open(path, "r", encoding="utf-8", errors="replace") as fh:
            return "".join(fh.readlines()[-lines:])
    except OSError:
        return ""


# ----------------------------------------------------------------- MCP 协议


def handle(msg):
    method = msg.get("method")
    msg_id = msg.get("id")
    params = msg.get("params") or {}

    if method == "initialize":
        asked = params.get("protocolVersion")
        version = asked if asked in PROTOCOL_VERSIONS else PROTOCOL_VERSIONS[0]
        return {
            "protocolVersion": version,
            "capabilities": {"tools": {"listChanged": False}},
            "serverInfo": {"name": SERVER_NAME, "version": SERVER_VERSION},
            "instructions": (
                "本地 ComfyUI 控制台。先 comfy_status 确认在跑（没跑就 launch_comfy）；"
                "出图用 generate_image（最快）或 run_workflow（跑已有工作流，先 workflow_slots 看可改参数）。"
            ),
        }

    if method in ("notifications/initialized", "notifications/cancelled", "notifications/roots/list_changed"):
        return None

    if method == "ping":
        return {}

    if method == "tools/list":
        return {
            "tools": [
                {"name": t["name"], "description": t["description"], "inputSchema": t["inputSchema"]}
                for t in TOOLS.values()
            ]
        }

    if method == "tools/call":
        name = params.get("name")
        args = params.get("arguments") or {}
        entry = TOOLS.get(name)
        if not entry:
            raise McpError(-32602, f"没有这个工具：{name}")
        payload = _call(entry["fn"], **args)
        content = []
        if isinstance(payload, dict):
            payload, images = _split_result(payload)
            content.extend(images)
        content.insert(0, {"type": "text", "text": _dump(payload)})
        is_error = isinstance(payload, dict) and "error" in payload
        return {"content": content, "isError": bool(is_error)}

    if method == "resources/list":
        return {"resources": []}

    if method == "prompts/list":
        return {"prompts": []}

    raise McpError(-32601, f"未实现的方法：{method}")


class McpError(Exception):
    def __init__(self, code, message):
        super().__init__(message)
        self.code = code


def rpc_response(msg):
    """处理单条消息 → 完整 JSON-RPC 响应报文；纯通知返回 None。

    stdio 和 HTTP 两种传输共用这一层；单个请求出错只进响应，不炸会话。
    """
    msg_id = msg.get("id") if isinstance(msg, dict) else None
    try:
        reply = handle(msg)
    except McpError as exc:
        return {"jsonrpc": "2.0", "id": msg_id, "error": {"code": exc.code, "message": str(exc)}}
    except Exception as exc:  # noqa: BLE001
        log("handle error:", repr(exc))
        return {"jsonrpc": "2.0", "id": msg_id, "error": {"code": -32603, "message": f"{type(exc).__name__}: {exc}"}}
    if reply is None:
        return None
    return {"jsonrpc": "2.0", "id": msg_id, "result": reply}


def _utf8_stdio():
    # Windows 控制台默认 GBK，MCP 报文必须是 UTF-8
    for stream in (sys.stdin, sys.stdout, sys.stderr):
        try:
            stream.reconfigure(encoding="utf-8", newline="\n")
        except Exception:  # noqa: BLE001
            pass


def main():
    """stdio 模式：逐行读 JSON-RPC（本地客户端：Cursor / Qoder / Claude）。"""
    _utf8_stdio()
    log(f"v{SERVER_VERSION} 启动（stdio），ComfyUI = {CONFIG['base_url']}，工作流目录 = {workflow_dirs()}")

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            msg = json.loads(line)
        except json.JSONDecodeError as exc:
            log("坏报文:", exc)
            continue

        if isinstance(msg, list):  # 兼容 batch
            outs = [rpc_response(m) for m in msg]
            outs = [o for o in outs if o is not None]
            if outs:
                _send(outs[0] if len(outs) == 1 else outs)
        else:
            out = rpc_response(msg)
            if out is not None:
                _send(out)


def _send(obj):
    sys.stdout.write(json.dumps(obj, ensure_ascii=False) + "\n")
    sys.stdout.flush()


# ----------------------------------------------------------------- HTTP 模式
# Streamable HTTP 传输（MCP 2025-03-26+）：POST /mcp 收 JSON-RPC，回 application/json。
# 给只支持 URL 接入的客户端用 —— 网页版 Grok、手机端、或像 mt-apk-mcp 那样挂局域网。
# 无会话状态（stateless），CORS 全开，可选 Bearer / X-API-Key 令牌。


def http_bind(cli_value=""):
    """解析监听地址：'127.0.0.1:8189'；':8189' = 0.0.0.0:8189；空则用 config。"""
    value = cli_value or os.environ.get("COMFYUI_MCP_HTTP") or ""
    if value:
        host, _, port = value.rpartition(":")
        if not host:
            return "0.0.0.0", int(port or 8189)
        return host, int(port or 8189)
    return (CONFIG.get("http_host") or "127.0.0.1"), int(CONFIG.get("http_port") or 8189)


def _bind_token():
    """取访问令牌：config / 环境变量；隧道模式下没配则自动生成一个。"""
    token = CONFIG.get("http_token") or os.environ.get("COMFYUI_MCP_HTTP_TOKEN") or ""
    return token


# ------------------------------------------------------------- ngrok 隧道
# 移植自 quality-gate/ngrok.go 的玩法：拉 ngrok http <port>，
# 轮询本机 4040..4050 的 /api/tunnels 拿公网 https 地址，打印可直接粘到
# grok.com/connectors → Custom 的连接器 URL。


def _fetch_ngrok_url(api_url, port):
    try:
        with urllib.request.urlopen(api_url, timeout=2) as resp:
            data = json.loads(resp.read().decode("utf-8"))
    except Exception:  # noqa: BLE001
        return ""
    tunnels = data.get("tunnels") or []
    for t in tunnels:
        if t.get("proto") != "https":
            continue
        addr = str((t.get("config") or {}).get("addr") or "")
        if f":{port}" in addr:
            return str(t.get("public_url") or "").rstrip("/")
    for t in tunnels:  # 没匹配到端口的，退而取第一个 https
        if t.get("proto") == "https":
            return str(t.get("public_url") or "").rstrip("/")
    return ""


def _discover_ngrok(port):
    for p in range(4040, 4051):
        url = _fetch_ngrok_url(f"http://127.0.0.1:{p}/api/tunnels", port)
        if url:
            return url
    return ""


def _launch_ngrok(port):
    """启动 ngrok 子进程；已存在可用隧道则复用。返回 (public_url, proc_or_None)。"""
    existing = _discover_ngrok(port)
    if existing:
        log(f"ngrok 已在跑，复用隧道：{existing}")
        return existing, None

    import shutil
    import subprocess

    exe = shutil.which("ngrok")
    if not exe:
        raise RuntimeError("找不到 ngrok。去 https://ngrok.com 下载并 ngrok config add-authtoken <token>")
    proc = subprocess.Popen(
        [exe, "http", str(port)],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        stdin=subprocess.DEVNULL,
    )

    import time

    deadline = time.time() + 30
    while time.time() < deadline:
        url = _discover_ngrok(port)
        if url:
            log(f"ngrok 隧道就绪：{url}")
            return url, proc
        if proc.poll() is not None:
            raise RuntimeError(f"ngrok 进程退出了（code={proc.returncode}），检查 authtoken：ngrok config add-authtoken <token>")
        time.sleep(0.5)
    proc.kill()
    raise RuntimeError("等了 30s 还没拿到 ngrok 公网地址")


def serve_http(host, port, token="", tunnel=False):
    from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

    token = token or _bind_token()

    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"
        server_version = f"comfyui-mcp/{SERVER_VERSION}"

        def _reply(self, status, body=b"", content_type="application/json"):
            self.send_response(status)
            self.send_header("Access-Control-Allow-Origin", self.headers.get("Origin") or "*")
            self.send_header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
            self.send_header(
                "Access-Control-Allow-Headers",
                "Content-Type, Accept, Authorization, Mcp-Session-Id, "
                "Mcp-Protocol-Version, Last-Event-ID, X-API-Key",
            )
            self.send_header("Access-Control-Expose-Headers", "Mcp-Session-Id")
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            if body:
                self.wfile.write(body)

        def _authorized(self):
            if not token:
                return True
            # 1) URL 里带 ?key=<token>（给没有认证配置入口的客户端用，地址自带钥匙）
            try:
                import urllib.parse as _up
                qs = _up.parse_qs(_up.urlparse(self.path).query)
                if (qs.get("key") or [""])[0] == token:
                    return True
            except Exception:  # noqa: BLE001
                pass
            # 2) 标准头：Authorization: Bearer <token> 或 X-API-Key: <token>
            auth = (self.headers.get("Authorization") or "").strip()
            supplied = auth[7:].strip() if auth.lower().startswith("bearer ") else auth
            supplied = supplied or (self.headers.get("X-API-Key") or "").strip()
            return supplied == token

        def do_OPTIONS(self):  # CORS 预检（204 不能带 Content-Length）
            self.send_response(204)
            self.send_header("Access-Control-Allow-Origin", self.headers.get("Origin") or "*")
            self.send_header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
            self.send_header(
                "Access-Control-Allow-Headers",
                "Content-Type, Accept, Authorization, Mcp-Session-Id, "
                "Mcp-Protocol-Version, Last-Event-ID, X-API-Key",
            )
            self.send_header("Access-Control-Max-Age", "86400")
            self.end_headers()

        def do_POST(self):
            path = self.path.split("?")[0].rstrip("/") or "/"
            if path not in ("/", "/mcp"):
                self._reply(404, b'{"error":"not found"}')
                return
            if not self._authorized():
                self._reply(401, b'{"error":"unauthorized"}')
                return
            try:
                length = int(self.headers.get("Content-Length") or 0)
            except ValueError:
                length = 0
            raw = self.rfile.read(length) if length > 0 else b""

            payload = None
            parse_err = None
            if raw:
                try:
                    payload = json.loads(raw.decode("utf-8"))
                except (json.JSONDecodeError, UnicodeDecodeError) as exc:
                    parse_err = str(exc)
            if parse_err or payload is None:
                self._reply(
                    400,
                    json.dumps(
                        {"jsonrpc": "2.0", "id": None,
                         "error": {"code": -32700, "message": f"parse error: {parse_err or 'empty body'}"}},
                        ensure_ascii=False,
                    ).encode("utf-8"),
                )
                return

            batch = isinstance(payload, list)
            outs = [rpc_response(m) for m in (payload if batch else [payload])]
            outs = [o for o in outs if o is not None]
            if not outs:  # 全是通知：规范要求 202 + 空 body
                self._reply(202, b"")
                return
            body = outs if batch else outs[0]
            self._reply(200, json.dumps(body, ensure_ascii=False).encode("utf-8"))

        def do_GET(self):
            # 不做服务端推送流（SSE）；规范允许 GET 返回 405
            path = self.path.split("?")[0].rstrip("/") or "/"
            self._reply(405 if path in ("/", "/mcp") else 404, b'{"error":"GET not supported"}')

        def do_DELETE(self):  # 无会话状态可删
            self._reply(405, b'{"error":"no session"}')

        def log_message(self, fmt, *args):
            log("http:", fmt % args)

    class Server(ThreadingHTTPServer):
        daemon_threads = True
        # Windows 下 SO_REUSEADDR 会允许两个进程同时绑同一端口、静默抢流量
        # （实测踩过：旧实例无令牌却接住了隧道请求）。关掉它，占用时直接报错。
        allow_reuse_address = False

    try:
        httpd = Server((host, port), Handler)
    except OSError as exc:
        print(f"[comfyui-mcp] 端口 {port} 绑定失败：{exc}\n"
              f"可能已有一个实例在跑。查一下：netstat -ano | findstr :{port}\n"
              f"或换个端口：--http 127.0.0.1:{port + 1}", file=sys.stderr, flush=True)
        raise SystemExit(1)

    ngrok_proc = None
    if tunnel:
        if not token:
            import secrets

            token = secrets.token_hex(16)  # 公网暴露必须带钥匙
        public_url, ngrok_proc = _launch_ngrok(port)
        mcp_url = f"{public_url}/mcp"
        os.makedirs(RUN_DIR, exist_ok=True)
        with open(os.path.join(RUN_DIR, "tunnel.url"), "w", encoding="utf-8") as fh:
            fh.write(f"{mcp_url}?key={token}\n{mcp_url}\ntoken={token}\n")
        banner = [
            "",
            "=" * 68,
            " 隧道已就绪 —— 网页版 Grok 接入信息（grok.com → 连接器/MCP → Custom）",
            "=" * 68,
            f" 连接器 URL：  {mcp_url}",
            f" 认证 token：  {token}",
            "",
            " · 认证方式三选一：",
            f"   1) URL 直接带钥匙（粘贴这个）：{mcp_url}?key={token}",
            "   2) 客户端认证配置里填 Bearer token / API key",
            "   3) 表头 X-API-Key",
            f" · 已写入 {os.path.join(RUN_DIR, 'tunnel.url')}，Ctrl+C 退出会一并收掉 ngrok",
            "=" * 68,
            "",
        ]
        for line in banner:
            print(line, file=sys.stderr, flush=True)
    else:
        shown = "127.0.0.1" if host in ("0.0.0.0", "") else host
        log(f"HTTP 模式监听 http://{host}:{port}/mcp （令牌：{'开启' if token else '未设置'}）")
        log(f"网页客户端填这个地址：http://{shown}:{port}/mcp；局域网设备把 host 换成本机 IP")
    try:
        httpd.serve_forever()
    finally:
        httpd.server_close()
        if ngrok_proc is not None:
            try:
                ngrok_proc.kill()
            except Exception:  # noqa: BLE001
                pass


if __name__ == "__main__":
    try:
        argv = sys.argv[1:]
        if argv and argv[0] in ("--http", "--tunnel"):
            # python server.py --http 0.0.0.0:8189   局域网直连
            # python server.py --tunnel              自动起 ngrok 公网隧道（给网页版 Grok）
            _utf8_stdio()
            tunnel = argv[0] == "--tunnel"
            host, port = http_bind(argv[1] if len(argv) > 1 else "")
            serve_http(host, port, tunnel=tunnel)
        else:
            main()  # 默认 stdio
    except KeyboardInterrupt:
        pass
    except BrokenPipeError:
        pass
