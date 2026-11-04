# comfyui-mcp

本地 ComfyUI 出图 MCP（stdio），专为你这台机器的便携包定制：
`D:\xuxinzhi\ComfyUI_windows_portable_nvidia\ComfyUI_windows_portable`。

**零依赖、零安装**：纯 Python 标准库实现，直接用 ComfyUI 便携包自带的
`python_embeded\python.exe` 跑 —— 不需要 uv、不需要 pip install、不动 ComfyUI 环境的任何包。
相比官方 `comfy-mcp`（要 pip install + comfy-cli 壳一层）和第三方的 npx 方案，
这个版本就是"手搓的内嵌直连"：所有工具直接打 ComfyUI 0.28 的 HTTP API
（`/prompt`、`/api/jobs/*`、`/object_info`、`/upload/image`、`/view`…），
还能直接调用你 ComfyUI 界面里保存的工作流。

## 启动 / 挂载

配置在 `config.json`（从 `config.example.json` 复制，已 gitignore）：

```json
{
  "base_url": "http://127.0.0.1:8188",
  "portable_root": "D:\\xuxinzhi\\ComfyUI_windows_portable_nvidia\\ComfyUI_windows_portable",
  "default_checkpoint": "meinamix_v12Final.safetensors"
}
```

`portable_root` 推导出 `python`（python_embeded）和 `comfy_dir`，`launch_comfy` 靠它们拉起服务。

MCP 客户端（Cursor / Qoder / Claude）里：

```json
{
  "mcpServers": {
    "comfyui": {
      "command": "D:\\xuxinzhi\\ComfyUI_windows_portable_nvidia\\ComfyUI_windows_portable\\python_embeded\\python.exe",
      "args": ["C:\\Users\\ZhuanZ1\\Desktop\\tools\\comfyui-mcp\\server.py"]
    }
  }
}
```

## HTTP 模式（网页版 Grok 等只能填 URL 的客户端）

像 `mt-apk-mcp`（`http://192.168.124.50:8787/mcp`）那种 URL 接入方式，本服务也支持 ——
用 MCP 的 **Streamable HTTP** 传输起一个常驻 HTTP 端点：

```bash
# 本机浏览器用（网页版 Grok 开在同一台机器）
D:\...\python_embeded\python.exe server.py --http 127.0.0.1:8189

# 局域网可连（手机 / 其他设备，写法同 ：端口）
D:\...\python_embeded\python.exe server.py --http :8189
```

在网页客户端里添加 MCP 连接，地址填：

```
http://127.0.0.1:8189/mcp        ← 浏览器在同一台机器
http://192.168.x.x:8189/mcp      ← 局域网其他设备（--http :8189 启动时）
```

说明：
- 无状态实现，POST JSON-RPC 直接回 `application/json`，不带 SSE 推送流，符合规范里服务器可选的简化形态
- CORS 全开（浏览器跨域直连没问题），首次绑定全网卡时 Windows 防火墙会弹一次允许
- 公网/不可信网络下设置令牌：`config.json` 里 `http_token`（或环境变量 `COMFYUI_MCP_HTTP_TOKEN`），
  客户端用 `Authorization: Bearer <token>` 或 `X-API-Key` 头
- 默认端口可在 `config.json` 改（`http_host` / `http_port`）；stdio 与 HTTP 两种模式可同时分开跑（各起一个进程即可）

## 工具（16 个）

| 工具 | 用途 |
|---|---|
| `comfy_status` | 版本 / 显存 / 队列。干活前先调它 |
| `launch_comfy` / `stop_comfy` | 用便携包拉起 / 停掉 ComfyUI（日志 `.run/comfyui.log`） |
| `generate_image` | 一句话出图：内置拼 txt2img 图（LoRA / CLIP skip 可选），图片直接回传 |
| `img2img` | 给本地图片路径（自动上传）或 input 文件名，按 denoise 重绘 |
| `list_workflows` | 列出 ComfyUI 界面保存的工作流（`user/default/workflows`）+ 本目录 `workflows/` |
| `workflow_slots` | 看一个工作流里哪些参数能改（`节点id.输入名` / `标题.输入名`） |
| `run_workflow` | 跑已有工作流：**UI 格式自动转 API 格式**，overrides 改参数 |
| `job` | 任务 status / wait / cancel / list（0.28 的 `/api/jobs`） |
| `queue` | 当前队列 |
| `fetch_outputs` | 把任务输出下载到指定目录（默认 `ComfyUI/output/mcp`），可选回传图片 |
| `upload_image` | 本地图片 → ComfyUI `input/`，返回 LoadImage 可用的名字 |
| `list_models` | checkpoints / loras / vae … |
| `search_nodes` | 在 `/object_info` 里搜节点（含自定义节点），手写工作流用 |
| `free_memory` / `interrupt` | 卸载模型释显存 / 打断当前任务 |

出图结果里图片直接以 MCP image content 回传（>6MB 只给本地路径），
`outputs[].local_path` 是落盘位置。

## 典型用法（给 agent 的提示）

- 没起服务：`launch_comfy`（冷启动 1-3 分钟，会等端口通）
- 快速出图：`generate_image(prompt="1girl, chibi, ...", width=512, height=768, clip_skip=-2)`
- 跑你调好的图：`run_workflow("MeinaMix_文生图", {"正向提示词.text": "...", "3.steps": 28})`
  先 `workflow_slots("MeinaMix_文生图")` 看槽位名
- 图生图：`img2img(image="D:\\path\\x.jpg", prompt="...", denoise=0.5)`

## UI → API 转换注意

ComfyUI 界面保存的是 UI 格式（nodes/links/widgets_values），`/prompt` 只吃 API 格式。
转换器做了：Reroute 穿线、bypass 节点透传、`control_after_generate` 跳列、
COMBO 值校验（拼错模型名直接报候选）、required 缺省值补齐。
特殊节点（子图入口、PrimitiveNode）若报错，让 agent 走 `generate_image` / `search_nodes` 手拼。

## 自检

```bash
D:\...\python_embeded\python.exe selftest.py          # 协议握手（stdio + HTTP）+ 转换
D:\...\python_embeded\python.exe selftest.py --live   # 额外拿真实 /object_info 校验全部工作流
```

## 为什么不直接用官方 comfy-mcp？

调研结论（2026-11）：
- 官方 [`Comfy-Org/comfy-mcp`](https://github.com/Comfy-Org/comfy-mcp)（beta，325★）：stdio，每个工具壳一层 comfy-cli，**便携包（embedded python）不在它的支持矩阵里**（`comfy install` 需要 uv 建 venv）
- [`artokun/comfyui-mcp`](https://github.com/artokun/comfyui-mcp)（890★，npm）：功能全但重，node>=22，全家桶安装
- [`joenorton/comfyui-mcp-server`](https://github.com/joenorton/comfyui-mcp-server)（411★）：HTTP 传输 + 自己维护模板，跟本地工作流脱节

便携包方案上它们都得先解决"给 embedded python 装包 / 双环境"的问题，而手搓版
零依赖直连 + 原生读你 ComfyUI 里保存的工作流，反而最贴合本地使用。
