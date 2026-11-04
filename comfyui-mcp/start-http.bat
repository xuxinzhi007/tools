@echo off
rem 启动 comfyui-mcp 的 HTTP 模式（网页版 Grok / 手机等 URL 接入的客户端用）。
rem 地址: http://127.0.0.1:8189/mcp   （局域网设备用本机 IP 替换 127.0.0.1 时，
rem                                     先把下面一行改成 --http :8189）
title comfyui-mcp (http://127.0.0.1:8189/mcp)
"D:\xuxinzhi\ComfyUI_windows_portable_nvidia\ComfyUI_windows_portable\python_embeded\python.exe" "%~dp0server.py" --http 127.0.0.1:8189
pause
