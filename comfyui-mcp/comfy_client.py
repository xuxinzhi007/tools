#!/usr/bin/env python3
"""ComfyUI HTTP 客户端（纯标准库）。

只覆盖本 MCP 需要的接口，全部打的是 ComfyUI 自带的 HTTP API：

    GET  /system_stats          显存 / 内存 / 版本
    GET  /queue                 运行中 + 排队中
    GET  /history/{prompt_id}   出图结果
    GET  /api/jobs/{prompt_id}  0.28 起的新任务视图（带 status / outputs）
    GET  /object_info[/{node}]  节点定义（含自定义节点）
    GET  /models[/{folder}]     模型文件列表
    POST /prompt                提交 API 格式工作流
    POST /upload/image          把本地图片塞进 input/
    POST /interrupt /free       打断 / 释放显存
    GET  /view                  取回输出文件字节

不引入任何第三方依赖，以便直接用 ComfyUI 便携包里的 python_embeded 跑。
"""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.parse
import urllib.request
import uuid

# ComfyUI 会把该 header 透传给 API 节点，便于在服务端日志里认出调用方
USAGE_SOURCE = "comfyui-mcp"


class ComfyError(RuntimeError):
    """ComfyUI 请求失败（连接失败 / 4xx / 5xx / 校验失败）。"""

    def __init__(self, message, status=None, payload=None):
        super().__init__(message)
        self.status = status
        self.payload = payload


class ComfyClient:
    def __init__(self, base_url="http://127.0.0.1:8188", timeout=30.0):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self.client_id = uuid.uuid4().hex

    # ---------------------------------------------------------------- 底层

    def _request(self, method, path, body=None, params=None, timeout=None):
        url = self.base_url + path
        if params:
            url += "?" + urllib.parse.urlencode(params)

        data = None
        headers = {
            "Accept": "application/json",
            "Comfy-Usage-Source": USAGE_SOURCE,
        }
        if body is not None:
            data = json.dumps(body, ensure_ascii=False).encode("utf-8")
            headers["Content-Type"] = "application/json"

        req = urllib.request.Request(url, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=timeout or self.timeout) as resp:
                return resp.read()
        except urllib.error.HTTPError as exc:
            raw = exc.read()
            payload = None
            try:
                payload = json.loads(raw.decode("utf-8", "replace"))
            except Exception:
                payload = raw.decode("utf-8", "replace")[:2000]
            raise ComfyError(_http_message(payload, exc.code), exc.code, payload)
        except urllib.error.URLError as exc:
            raise ComfyError(
                f"连不上 ComfyUI ({url})：{exc.reason}。先用 comfy_status 看它是否在跑，"
                f"或 launch_comfy 拉起来。"
            )
        except OSError as exc:  # 超时 / 连接被重置
            raise ComfyError(f"请求 {url} 失败：{exc}")

    def get_json(self, path, params=None, timeout=None):
        raw = self._request("GET", path, params=params, timeout=timeout)
        return _json(raw)

    def post_json(self, path, body=None, timeout=None):
        raw = self._request("POST", path, body=body if body is not None else {}, timeout=timeout)
        return _json(raw) if raw else {}

    # ---------------------------------------------------------------- 只读

    def system_stats(self):
        return self.get_json("/system_stats")

    def queue(self):
        return self.get_json("/queue")

    def history(self, prompt_id):
        return self.get_json(f"/history/{prompt_id}")

    def job(self, prompt_id):
        """0.28 的 /api/jobs/{id}：pending/in_progress/completed/failed/cancelled。"""
        try:
            return self.get_json(f"/api/jobs/{prompt_id}")
        except ComfyError as exc:
            if exc.status == 404:
                return None
            raise

    def jobs(self, status=None, limit=20):
        params = {"limit": limit}
        if status:
            params["status"] = status
        return self.get_json("/api/jobs", params=params)

    def object_info(self, node_class=None, timeout=120):
        path = "/object_info" if not node_class else f"/object_info/{urllib.parse.quote(node_class)}"
        return self.get_json(path, timeout=timeout)

    def models(self, folder=None):
        if folder:
            return self.get_json(f"/models/{urllib.parse.quote(folder)}")
        return self.get_json("/models")

    def view_bytes(self, filename, subfolder="", type="output"):
        return self._request(
            "GET",
            "/view",
            params={"filename": filename, "subfolder": subfolder or "", "type": type or "output"},
            timeout=120,
        )

    # ---------------------------------------------------------------- 写入

    def queue_prompt(self, graph, extra_data=None, prompt_id=None):
        body = {
            "prompt": graph,
            "client_id": self.client_id,
            "extra_data": extra_data or {},
        }
        if prompt_id:
            body["prompt_id"] = prompt_id
        return self.post_json("/prompt", body)

    def upload_image(self, path, subfolder="", overwrite=True, kind="image"):
        """把本地文件传进 ComfyUI 的 input 目录，返回 {"name": ..., "subfolder": ...}。"""
        path = os.path.expanduser(path)
        if not os.path.isfile(path):
            raise ComfyError(f"上传失败，文件不存在：{path}")
        filename = os.path.basename(path)
        with open(path, "rb") as fh:
            blob = fh.read()

        boundary = "----comfymcp" + uuid.uuid4().hex
        parts = []
        for key, value in (
            ("type", kind),
            ("subfolder", subfolder or ""),
            ("overwrite", "true" if overwrite else "false"),
        ):
            parts.append(
                f"--{boundary}\r\nContent-Disposition: form-data; name=\"{key}\"\r\n\r\n{value}\r\n".encode("utf-8")
            )
        parts.append(
            f"--{boundary}\r\nContent-Disposition: form-data; name=\"image\"; filename=\"{filename}\"\r\n"
            f"Content-Type: application/octet-stream\r\n\r\n".encode("utf-8")
        )
        parts.append(blob)
        parts.append(f"\r\n--{boundary}--\r\n".encode("utf-8"))
        body = b"".join(parts)

        req = urllib.request.Request(
            self.base_url + "/upload/image",
            data=body,
            headers={
                "Content-Type": f"multipart/form-data; boundary={boundary}",
                "Comfy-Usage-Source": USAGE_SOURCE,
            },
            method="POST",
        )
        try:
            with urllib.request.urlopen(req, timeout=300) as resp:
                return _json(resp.read())
        except urllib.error.HTTPError as exc:
            raise ComfyError(f"上传被拒绝（HTTP {exc.code}）：{exc.read()[:500]!r}", exc.code)
        except urllib.error.URLError as exc:
            raise ComfyError(f"上传失败，连不上 ComfyUI：{exc.reason}")

    def interrupt(self):
        return self.post_json("/interrupt", {})

    def free_memory(self, unload_models=True, free_memory=True):
        return self.post_json("/free", {"unload_models": unload_models, "free_memory": free_memory})

    def cancel(self, prompt_id):
        return self.post_json(f"/api/jobs/{prompt_id}/cancel", {})

    # ---------------------------------------------------------------- 组合

    def is_up(self, timeout=2.0):
        try:
            self._request("GET", "/system_stats", timeout=timeout)
            return True
        except ComfyError:
            return False

    def wait_job(self, prompt_id, timeout=600.0, interval=1.0, on_tick=None):
        """轮询到任务进入终态。返回 (job_dict, timed_out)。"""
        import time

        deadline = time.time() + float(timeout)
        while True:
            job = self.job(prompt_id)
            if job and job.get("status") in ("completed", "failed", "cancelled"):
                return job, False
            if time.time() >= deadline:
                return job, True
            if on_tick:
                on_tick(job)
            time.sleep(interval)

    def collect_outputs(self, prompt_id):
        """把一次任务的输出整理成 [{filename, subfolder, type, kind}] 列表。"""
        job = self.job(prompt_id) or {}
        outputs = job.get("outputs")
        if outputs is None:
            outputs = (self.history(prompt_id) or {}).get(prompt_id, {}).get("outputs", {})

        files = []
        for node_id, node_out in (outputs or {}).items():
            if not isinstance(node_out, dict):
                continue
            for key, items in node_out.items():
                if key == "animated" or not isinstance(items, list):
                    continue
                for item in items:
                    if isinstance(item, dict) and item.get("filename"):
                        files.append(
                            {
                                "node": node_id,
                                "filename": item["filename"],
                                "subfolder": item.get("subfolder", ""),
                                "type": item.get("type", "output"),
                                "kind": item.get("format") or key,
                            }
                        )
        return files

    def fetch_to_dir(self, prompt_id, out_dir):
        """下载一次任务的全部输出到 out_dir，返回本地路径列表。"""
        os.makedirs(out_dir, exist_ok=True)
        saved = []
        for item in self.collect_outputs(prompt_id):
            blob = self.view_bytes(item["filename"], item["subfolder"], item["type"])
            dest = os.path.join(out_dir, os.path.basename(item["filename"]))
            with open(dest, "wb") as fh:
                fh.write(blob)
            saved.append(dest)
        return saved


def _json(raw):
    try:
        return json.loads(raw.decode("utf-8"))
    except Exception:
        return {"raw": raw[:4000].decode("utf-8", "replace")}


def _http_message(payload, status):
    """把 ComfyUI 的 {"error": {...}, "node_errors": {...}} 压成一句人话。"""
    if isinstance(payload, dict):
        err = payload.get("error")
        if isinstance(err, dict):
            msg = err.get("message") or err.get("type") or "unknown error"
            details = err.get("details")
            node_errors = payload.get("node_errors") or {}
            text = f"ComfyUI 拒绝工作流（HTTP {status}）：{msg}"
            if details:
                text += f" — {details}"
            if node_errors:
                bits = []
                for node_id, info in list(node_errors.items())[:4]:
                    errs = info.get("errors") if isinstance(info, dict) else None
                    if errs:
                        for e in errs:
                            bits.append(f"节点 {node_id}({info.get('class_type')}): {e.get('message')} {e.get('details') or ''}".strip())
                    else:
                        bits.append(f"节点 {node_id}: {info}")
                text += "\n  " + "\n  ".join(bits)
            return text
        if isinstance(err, str):
            return f"ComfyUI 返回 HTTP {status}：{err}"
    return f"ComfyUI 返回 HTTP {status}：{str(payload)[:800]}"
