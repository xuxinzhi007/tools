#!/usr/bin/env python3
"""离线自检：MCP 协议握手 + 工具表 + 工作流 UI→API 转换。

    python_embeded\\python.exe selftest.py            # 不依赖 ComfyUI 在跑
    python_embeded\\python.exe selftest.py --live     # 额外拿真实 /object_info 校验转换

只读，不会提交任务、不会出图。
"""

from __future__ import annotations

import json
import os
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)

import workflows  # noqa: E402

OK, FAIL = "  ok  ", " FAIL "


def check(name, cond, detail=""):
    print(f"[{OK if cond else FAIL}] {name}" + (f" — {detail}" if detail else ""))
    return bool(cond)


def mcp_handshake():
    """起一个真进程，走真实 stdio 报文，确认协议层没写错。"""
    proc = subprocess.Popen(
        [sys.executable, os.path.join(HERE, "server.py")],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        cwd=HERE,
        text=True,
        encoding="utf-8",
        bufsize=1,
    )

    def send(msg):
        proc.stdin.write(json.dumps(msg, ensure_ascii=False) + "\n")
        proc.stdin.flush()
        line = proc.stdout.readline()
        return json.loads(line) if line.strip() else None

    try:
        init = send(
            {
                "jsonrpc": "2.0",
                "id": 1,
                "method": "initialize",
                "params": {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "selftest", "version": "0"}},
            }
        )
        info = (init or {}).get("result", {})
        check("initialize 返回 serverInfo", info.get("serverInfo", {}).get("name") == "comfyui-mcp", json.dumps(info.get("serverInfo"), ensure_ascii=False))

        proc.stdin.write(json.dumps({"jsonrpc": "2.0", "method": "notifications/initialized"}) + "\n")
        proc.stdin.flush()

        listed = send({"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
        tools = (listed or {}).get("result", {}).get("tools", [])
        names = [t["name"] for t in tools]
        check("tools/list 有工具", len(tools) >= 10, f"{len(tools)} 个：{', '.join(names)}")
        missing_schema = [t["name"] for t in tools if not t.get("inputSchema")]
        check("每个工具都有 inputSchema", not missing_schema, str(missing_schema))

        called = send({"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "nope", "arguments": {}}})
        check("未知工具返回错误", (called or {}).get("error") is not None or (called or {}).get("result", {}).get("isError"), json.dumps(called, ensure_ascii=False)[:160])
    finally:
        proc.stdin.close()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()


def workflow_conversion(live):
    sys.path.insert(0, HERE)
    import server  # noqa: F401  —— 复用配置与工作流目录解析

    dirs = server.workflow_dirs()
    check("找到工作流目录", bool(dirs), str(dirs))

    object_info = None
    if live:
        client = server.CLIENT
        if client.is_up():
            object_info = client.object_info()
            check("拿到 /object_info", bool(object_info), f"{len(object_info)} 个节点")
        else:
            print("[ skip ] ComfyUI 没在跑，跳过 live 校验（UI 转换仍然会跑一遍）")

    total = 0
    for d in dirs:
        for name in sorted(os.listdir(d)):
            if name.startswith(".") or not name.endswith(".json"):
                continue
            path = os.path.join(d, name)
            try:
                data, _ = workflows.load_workflow(path, dirs)
            except Exception as exc:  # noqa: BLE001
                check(f"读取 {name}", False, str(exc))
                continue
            if workflows.is_api_graph(data):
                check(f"{name} 已是 API 格式", True)
                continue
            warn = []
            try:
                api, slots = workflows.ui_to_api(data, object_info, warn=warn.append)
            except Exception as exc:  # noqa: BLE001
                check(f"转换 {name}", False, str(exc))
                continue
            total += 1
            empties = [nid for nid, n in api.items() if not n["inputs"]]
            check(
                f"转换 {name}",
                len(api) > 0,
                f"{len(api)} 个节点，{len(slots)} 个可改槽位，空输入节点={empties or '无'}，warn={len(warn)}",
            )
            for w in warn[:3]:
                print("         warn:", w)
    check("至少转换了一个目录里的工作流", total > 0, f"{total} 个")


def graph_builder():
    import server  # noqa: F401

    graph, slots = workflows.build_txt2img("meinamix_v12Final.safetensors", "1girl, chibi", width=512, height=768)
    check("build_txt2img 节点齐全", set(graph) >= {"3", "4", "5", "6", "7", "8", "9"}, str(sorted(graph)))
    check("build_txt2img 有可改槽位", len(slots) >= 8, f"{len(slots)} 个")
    graph2, _ = workflows.build_txt2img("m.safetensors", "x", init_image="a.png", denoise=0.5)
    check("img2img 用 VAEEncode", "14" in graph2 and graph2["3"]["inputs"]["denoise"] == 0.5)


def http_mode():
    """起一个 --http 服务，用真实 POST 走一遍 MCP 握手 / 列工具 / 调工具 / batch / 通知。"""
    import socket
    import time
    import urllib.error
    import urllib.request

    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()

    proc = subprocess.Popen(
        [sys.executable, os.path.join(HERE, "server.py"), "--http", f"127.0.0.1:{port}"],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        cwd=HERE,
    )
    base = f"http://127.0.0.1:{port}/mcp"

    def post(obj):
        req = urllib.request.Request(
            base,
            data=json.dumps(obj, ensure_ascii=False).encode("utf-8"),
            headers={"Content-Type": "application/json", "Accept": "application/json, text/event-stream"},
            method="POST",
        )
        with urllib.request.urlopen(req, timeout=60) as resp:
            body = resp.read().decode("utf-8")
            return resp.status, (json.loads(body) if body else None)

    try:
        up = False
        for _ in range(60):
            try:
                post({"jsonrpc": "2.0", "id": 0, "method": "ping"})
                up = True
                break
            except Exception:  # noqa: BLE001
                time.sleep(0.2)
        check("HTTP 服务起来", up, base)
        if not up:
            return

        status, data = post(
            {
                "jsonrpc": "2.0",
                "id": 1,
                "method": "initialize",
                "params": {"protocolVersion": "2025-06-18", "capabilities": {},
                           "clientInfo": {"name": "selftest-http", "version": "0"}},
            }
        )
        info = (data or {}).get("result", {}).get("serverInfo", {})
        check("HTTP initialize", status == 200 and info.get("name") == "comfyui-mcp", json.dumps(info, ensure_ascii=False))

        status, data = post({"jsonrpc": "2.0", "method": "notifications/initialized"})
        check("HTTP 纯通知返 202 空响应", status == 202 and data is None, f"status={status}")

        status, data = post({"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
        tools = (data or {}).get("result", {}).get("tools", [])
        check("HTTP tools/list", len(tools) >= 10, f"{len(tools)} 个")

        status, data = post(
            {"jsonrpc": "2.0", "id": 3, "method": "tools/call",
             "params": {"name": "comfy_status", "arguments": {}}}
        )
        text = (data or {}).get("result", {}).get("content", [{}])[0].get("text", "")
        check("HTTP tools/call comfy_status", status == 200 and '"up"' in text, text[:120])

        status, data = post([{"jsonrpc": "2.0", "id": 4, "method": "ping"},
                              {"jsonrpc": "2.0", "id": 5, "method": "ping"}])
        check("HTTP batch 返数组", status == 200 and isinstance(data, list) and len(data) == 2, str(data)[:120])

        status, data = post({"jsonrpc": "2.0", "id": 6, "method": "tools/call",
                             "params": {"name": "nope", "arguments": {}}})
        check("HTTP 未知工具返 JSON-RPC error", (data or {}).get("error", {}).get("code") == -32602, str(data)[:140])
    finally:
        proc.kill()
        proc.wait()


def main():
    live = "--live" in sys.argv
    print(f"== comfyui-mcp selftest (python {sys.version.split()[0]}) ==")
    mcp_handshake()
    http_mode()
    graph_builder()
    workflow_conversion(live)


if __name__ == "__main__":
    main()
