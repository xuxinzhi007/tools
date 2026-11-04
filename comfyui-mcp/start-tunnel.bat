@echo off
rem 启动 comfyui-mcp 的隧道模式：本地 HTTP + ngrok 公网隧道（网页版 Grok 用）。
rem 启动后看窗口里打印的「连接器 URL」和「认证 token」，粘到 grok.com 连接器配置里。
rem 需要 ngrok 在 PATH 且已 ngrok config add-authtoken（同 quality-gate 的用法）。
title comfyui-mcp tunnel (ngrok)
"D:\xuxinzhi\ComfyUI_windows_portable_nvidia\ComfyUI_windows_portable\python_embeded\python.exe" "%~dp0server.py" --tunnel
pause
