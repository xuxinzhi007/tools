#!/usr/bin/env python3
"""工作流处理：UI 导出 → API 格式转换、内置文生图/图生图拼装、参数覆盖。

ComfyUI 的 `/prompt` 只吃 API 格式（{"节点id": {"class_type": ..., "inputs": ...}}），
而 ComfyUI 界面里「保存」出来的是 UI 格式（nodes / links / widgets_values）。
本模块负责把用户 `user/default/workflows/*.json` 里的 UI 格式翻成 API 格式，
并且用 `/object_info` 校正输入名与顺序，保证老前端、自定义节点也能跑。
"""

from __future__ import annotations

import glob
import json
import os
import random

# 这些 INT 控件在前面板里会多带一个 control_after_generate（fixed/randomize/...）
SEED_WIDGETS = {"seed", "noise_seed"}

# 纯 UI 用的虚拟节点，不进 API 图
VIRTUAL_NODES = {"Reroute", "PrimitiveNode", "Note", "MarkdownNote", "Note Plus (mtb)"}

# 文本类控件，用于 workflow_slots 里做截断显示
TEXT_WIDGETS = {"text", "prompt", "positive", "negative", "string", "value"}


class WorkflowError(RuntimeError):
    pass


# --------------------------------------------------------------------- 载入


def load_workflow(ref, search_dirs, cwd=None):
    """ref 可以是绝对路径、相对路径，或工作流目录里的文件名（可省 .json）。"""
    candidates = []
    if os.path.isabs(ref) and os.path.isfile(ref):
        candidates.append(ref)
    else:
        if os.path.isfile(ref):
            candidates.append(os.path.abspath(ref))
        if cwd:
            p = os.path.join(cwd, ref)
            if os.path.isfile(p):
                candidates.append(p)
        for d in search_dirs:
            for pat in (ref, ref + ".json"):
                for hit in glob.glob(os.path.join(d, pat)):
                    if os.path.isfile(hit):
                        candidates.append(hit)
                        break

    if not candidates:
        raise WorkflowError(
            f"找不到工作流 {ref!r}。搜索目录：{', '.join(search_dirs)}"
        )
    path = candidates[0]
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    return data, path


def is_api_graph(data):
    if not isinstance(data, dict) or not data:
        return False
    if "nodes" in data and "links" in data:
        return False
    values = list(data.values())
    return all(isinstance(v, dict) and "class_type" in v for v in values)


# --------------------------------------------------------------- UI → API


def ui_to_api(ui, object_info=None, warn=None):
    """UI 格式工作流转 API 格式。

    返回 (api_graph, slots)。slots 描述可覆盖的控件，供 agent 改参数。
    """
    if "nodes" not in ui:
        raise WorkflowError("这不是 UI 格式工作流（缺 nodes）")

    nodes = {n["id"]: n for n in ui.get("nodes", []) if isinstance(n, dict)}
    links = {}
    for link in ui.get("links") or []:
        if isinstance(link, dict):  # 新格式也允许 dict
            links[link["id"]] = (link["origin_id"], link["origin_slot"])
        elif isinstance(link, (list, tuple)) and len(link) >= 3:
            links[link[0]] = (link[1], link[2])

    def resolve(link_id, want_type=None, depth=0):
        """把 link id 解析成 [上游节点id, slot]，穿过 Reroute / 旁路节点。"""
        if link_id is None:
            return None
        if depth > 32:
            raise WorkflowError("工作流连线出现环（Reroute 递归过深）")
        origin = links.get(link_id)
        if not origin:
            return None
        origin_id, origin_slot = origin
        node = nodes.get(origin_id)
        if node is None:
            return None

        ntype = node.get("type")
        mode = node.get("mode", 0)

        if ntype == "Reroute":
            up = None
            for inp in node.get("inputs") or []:
                if inp.get("link") is not None:
                    up = resolve(inp["link"], want_type, depth + 1)
                    break
            return up

        if ntype == "PrimitiveNode":
            # 虚拟节点只提供控件值，本身不进 API 图：让调用方退回 widget 值
            return None

        if mode == 2:  # muted：断开
            return None

        if mode == 4:  # bypassed：按类型透传
            for inp in node.get("inputs") or []:
                if want_type and inp.get("type") not in (want_type, "*"):
                    continue
                if inp.get("link") is not None:
                    got = resolve(inp["link"], want_type, depth + 1)
                    if got:
                        return got
            if warn:
                warn(f"节点 {origin_id}({ntype}) 被 bypass 但找不到同类型上游，已断开")
            return None

        return [str(origin_id), origin_slot]

    api = {}
    slots = []

    for node_id, node in nodes.items():
        ntype = node.get("type") or ""
        if ntype in VIRTUAL_NODES or ntype.startswith("workflow>"):
            continue
        if node.get("mode", 0) in (2, 4):  # mute / bypass 本身不执行
            continue

        spec = (object_info or {}).get(ntype)
        inputs = {}

        # 1) 连线输入
        for inp in node.get("inputs") or []:
            name = inp.get("name")
            if inp.get("link") is None or not name:
                continue
            got = resolve(inp["link"], inp.get("type"))
            if got:
                inputs[name] = got

        # 2) 控件值（按 inputs 里 widget 声明的顺序对 widgets_values 顺次取；
        #    老导出 / 自定义节点不声明时，退回 /object_info 的入参顺序）
        widget_inputs = [i.get("name") for i in (node.get("inputs") or []) if i.get("widget") and i.get("name")]
        if not widget_inputs:
            widget_inputs = _widget_names_from_spec(spec)
        widgets = _map_widgets(node, widget_inputs)

        for name, value in widgets.items():
            if name in inputs:  # 已被连线覆盖
                continue
            inputs[name] = value

        # 3) 用 object_info 校正：只留真实输入名，并补齐缺省
        inputs = _conform(ntype, inputs, spec, node_id, warn)

        api[str(node_id)] = {
            "class_type": ntype,
            "inputs": inputs,
            "_meta": {"title": node.get("title") or ntype},
        }

        for name, value in widgets.items():
            if name not in inputs:
                continue
            shown = value
            if isinstance(shown, str) and len(shown) > 80:
                shown = shown[:77] + "..."
            slots.append(
                {
                    "ref": f"{node_id}.{name}",
                    "node": str(node_id),
                    "title": node.get("title") or ntype,
                    "class_type": ntype,
                    "input": name,
                    "value": shown,
                }
            )

    if not api:
        raise WorkflowError("UI 工作流里没有可执行节点")
    return api, slots


def _widget_names_from_spec(spec):
    """从 /object_info 推控件入参顺序：非连线类型、且没标 forceInput 的那些。"""
    if not spec:
        return []
    declared = spec.get("input") or {}
    order = spec.get("input_order") or {}
    names = []
    for group in ("required", "optional"):
        block = declared.get(group) or {}
        keys = list(order.get(group) or block.keys())
        for name in keys:
            cfg = block.get(name)
            if _is_widget_cfg(cfg):
                names.append(name)
    return names


def _is_widget_cfg(cfg):
    """INT/FLOAT/STRING/BOOLEAN/COMBO 且不是强制连线，就是面板上的控件。"""
    if not isinstance(cfg, (list, tuple)) or not cfg:
        return False
    head = cfg[0]
    options = cfg[1] if len(cfg) > 1 and isinstance(cfg[1], dict) else {}
    if options.get("forceInput"):
        return False
    if isinstance(head, list):
        return True
    if isinstance(head, dict):  # 新版可能把类型写成 dict
        return head.get("type") in ("INT", "FLOAT", "STRING", "BOOLEAN") and not head.get("forceInput")
    return head in ("INT", "FLOAT", "STRING", "BOOLEAN")


def _map_widgets(node, widget_inputs):
    """widgets_values → {输入名: 值}，处理 seed 后面的 control_after_generate。"""
    raw = node.get("widgets_values")
    if raw is None:
        return {}
    if isinstance(raw, dict):
        return dict(raw)
    if not isinstance(raw, list):
        return {}

    names = widget_inputs or []
    seed_extras = sum(1 for n in names if n in SEED_WIDGETS)

    if len(raw) == len(names) + seed_extras and seed_extras:
        out, cursor = {}, 0
        for name in names:
            if cursor >= len(raw):
                break
            out[name] = raw[cursor]
            cursor += 1
            if name in SEED_WIDGETS and cursor < len(raw) and isinstance(raw[cursor], str):
                cursor += 1  # 跳过 control_after_generate
        return out

    # 位置对齐（LoadImage 的 upload 之类也在这里一并落到输入里，后面 _conform 会滤掉）
    return {name: raw[i] for i, name in enumerate(names) if i < len(raw)}


def _conform(ntype, inputs, spec, node_id, warn):
    """按 object_info 的 required/optional 过滤未知键、补默认值、校验 COMBO。"""
    if not spec:
        if warn:
            warn(f"节点 {node_id} 的类 {ntype} 不在 /object_info 里（可能自定义节点未加载），原样提交")
        return inputs

    declared = {}
    for group in ("required", "optional"):
        for name, cfg in (spec.get("input", {}).get(group) or {}).items():
            declared[name] = (group, cfg)

    out = {}
    for name, value in inputs.items():
        if name in declared:
            out[name] = value
        # 其余键（upload / control_after_generate 等 UI 专有）直接丢掉

    for name, (group, cfg) in declared.items():
        if name in out or group != "required":
            continue
        default = _default_of(cfg)
        if default is not None:
            out[name] = default

    # COMBO 值校验：避免拼错模型名时只在服务端报一堆 node_errors
    for name, (_, cfg) in declared.items():
        options = _options_of(cfg)
        if not options or name not in out:
            continue
        value = out[name]
        if isinstance(value, str) and value not in options:
            near = difflib_close(value, options)
            raise WorkflowError(
                f"节点 {node_id}({ntype}) 的 {name}={value!r} 不在候选里。可能是：" + "、".join(near)
            )

    return out


def _default_of(cfg):
    if not isinstance(cfg, (list, tuple)) or not cfg:
        return None
    spec = cfg[0]
    if isinstance(spec, (list, tuple)):
        return spec[0] if spec else None
    if isinstance(spec, dict) and "default" in spec:
        return spec["default"]
    return None


def _options_of(cfg):
    if not isinstance(cfg, (list, tuple)) or not cfg:
        return None
    spec = cfg[0]
    if isinstance(spec, list):
        return spec
    if isinstance(spec, dict) and isinstance(spec.get("options"), list):
        return spec["options"]
    return None


def difflib_close(value, options, n=3):
    import difflib

    hits = difflib.get_close_matches(str(value), [str(o) for o in options], n=n, cutoff=0.3)
    return hits or [str(o) for o in options[:3]]


# ------------------------------------------------------------------- 覆盖


def apply_overrides(api, slots, overrides):
    """按 "节点id.输入名" 或 "标题.输入名" 覆盖控件值。"""
    if not overrides:
        return api, []

    by_ref, by_title = {}, {}
    for slot in slots:
        by_ref.setdefault(slot["ref"].lower(), slot)
        by_title.setdefault(f"{str(slot['title']).lower()}.{slot['input'].lower()}", slot)

    applied = []
    for key, value in overrides.items():
        slot = by_ref.get(key.lower()) or by_title.get(key.lower())
        if not slot:
            raise WorkflowError(
                f"覆盖项 {key!r} 没匹配上。可用 ref 例如：" + "、".join(s["ref"] for s in slots[:12])
            )
        api[slot["node"]]["inputs"][slot["input"]] = value
        applied.append({"ref": slot["ref"], "title": slot["title"], "value": _short(value)})

    return api, applied


# --------------------------------------------------------------- 内置图拼装


def build_txt2img(
    checkpoint,
    positive,
    negative="lowres, bad anatomy, bad hands, worst quality, low quality",
    width=512,
    height=768,
    steps=28,
    cfg=7.0,
    sampler="dpmpp_2m",
    scheduler="karras",
    seed=None,
    batch_size=1,
    clip_skip=None,
    loras=None,
    filename_prefix="comfyui-mcp",
    denoise=1.0,
    init_image=None,
):
    """拼一个 SD1.5 / SDXL 通用的图（txt2img；给了 init_image 就是 img2img）。"""
    seed = int(seed) if seed not in (None, -1) else random.randint(0, 2**32 - 1)
    loras = loras or []

    g = {}
    g["4"] = {"class_type": "CheckpointLoaderSimple", "inputs": {"ckpt_name": checkpoint}, "_meta": {"title": "加载模型"}}

    model_src = ["4", 0]
    clip_src = ["4", 1]
    for i, lora in enumerate(loras):
        nid = str(20 + i)
        name = lora.get("name") if isinstance(lora, dict) else lora
        sm = float(lora.get("strength_model", 1.0)) if isinstance(lora, dict) else 1.0
        sc = float(lora.get("strength_clip", sm)) if isinstance(lora, dict) else 1.0
        g[nid] = {
            "class_type": "LoraLoader",
            "inputs": {
                "model": model_src,
                "clip": clip_src,
                "lora_name": name,
                "strength_model": sm,
                "strength_clip": sc,
            },
            "_meta": {"title": f"LoRA {name}"},
        }
        model_src, clip_src = [nid, 0], [nid, 1]

    if clip_skip:
        g["11"] = {
            "class_type": "CLIPSetLastLayer",
            "inputs": {"clip": clip_src, "stop_at_clip_layer": int(clip_skip)},
            "_meta": {"title": "CLIP skip"},
        }
        clip_src = ["11", 0]

    g["6"] = {"class_type": "CLIPTextEncode", "inputs": {"clip": clip_src, "text": positive}, "_meta": {"title": "正向提示词"}}
    g["7"] = {"class_type": "CLIPTextEncode", "inputs": {"clip": clip_src, "text": negative}, "_meta": {"title": "负向提示词"}}

    if init_image:
        g["12"] = {"class_type": "LoadImage", "inputs": {"image": init_image}, "_meta": {"title": "输入图片"}}
        g["14"] = {"class_type": "VAEEncode", "inputs": {"pixels": ["12", 0], "vae": ["4", 2]}, "_meta": {"title": "编码"}}
        latent = ["14", 0]
    else:
        g["5"] = {
            "class_type": "EmptyLatentImage",
            "inputs": {"width": int(width), "height": int(height), "batch_size": int(batch_size)},
            "_meta": {"title": "画布"},
        }
        latent = ["5", 0]

    g["3"] = {
        "class_type": "KSampler",
        "inputs": {
            "model": model_src,
            "positive": ["6", 0],
            "negative": ["7", 0],
            "latent_image": latent,
            "seed": seed,
            "steps": int(steps),
            "cfg": float(cfg),
            "sampler_name": sampler,
            "scheduler": scheduler,
            "denoise": float(denoise),
        },
        "_meta": {"title": "采样器"},
    }
    g["8"] = {"class_type": "VAEDecode", "inputs": {"samples": ["3", 0], "vae": ["4", 2]}, "_meta": {"title": "解码"}}
    g["9"] = {"class_type": "SaveImage", "inputs": {"images": ["8", 0], "filename_prefix": filename_prefix}, "_meta": {"title": "保存"}}

    slots = [
        {"ref": "6.text", "node": "6", "title": "正向提示词", "class_type": "CLIPTextEncode", "input": "text", "value": _short(positive)},
        {"ref": "7.text", "node": "7", "title": "负向提示词", "class_type": "CLIPTextEncode", "input": "text", "value": _short(negative)},
        {"ref": "3.seed", "node": "3", "title": "采样器", "class_type": "KSampler", "input": "seed", "value": seed},
        {"ref": "3.steps", "node": "3", "title": "采样器", "class_type": "KSampler", "input": "steps", "value": int(steps)},
        {"ref": "3.cfg", "node": "3", "title": "采样器", "class_type": "KSampler", "input": "cfg", "value": float(cfg)},
        {"ref": "3.sampler_name", "node": "3", "title": "采样器", "class_type": "KSampler", "input": "sampler_name", "value": sampler},
        {"ref": "3.scheduler", "node": "3", "title": "采样器", "class_type": "KSampler", "input": "scheduler", "value": scheduler},
        {"ref": "4.ckpt_name", "node": "4", "title": "加载模型", "class_type": "CheckpointLoaderSimple", "input": "ckpt_name", "value": checkpoint},
    ]
    if not init_image:
        slots += [
            {"ref": "5.width", "node": "5", "title": "画布", "class_type": "EmptyLatentImage", "input": "width", "value": int(width)},
            {"ref": "5.height", "node": "5", "title": "画布", "class_type": "EmptyLatentImage", "input": "height", "value": int(height)},
        ]
    return g, slots


def _short(value, limit=80):
    if isinstance(value, str) and len(value) > limit:
        return value[: limit - 3] + "..."
    return value
