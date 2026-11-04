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

## HTTP / 隧道模式（网页版 Grok 等只能填 URL 的客户端）

像 `mt-apk-mcp`（`http://192.168.124.50:8787/mcp`）那种 URL 接入方式，本服务也支持 ——
用 MCP 的 **Streamable HTTP** 传输；网页版 Grok 的连接是从**它的服务器**发起的，
本机/局域网地址都够不着，得用 `--tunnel` 挂公网隧道（同 quality-gate 的 ngrok 玩法）。

### 局域网 HTTP（手机 / 同网段设备）

```bash
D:\...\python_embeded\python.exe server.py --http 127.0.0.1:8189   # 只本机
D:\...\python_embeded\python.exe server.py --http :8189          # 局域网可连
```

### 公网隧道（网页版 Grok：grok.com → 连接器 → Custom）

双击 `start-tunnel.bat`，或：

```bash
D:\...\python_embeded\python.exe server.py --tunnel
```

它会：起本地 HTTP(8189) → 自动拉 `ngrok http 8189`（已在跑则复用）→
轮询 4040 拿公网地址 → 打印可直接粘贴的连接器 URL 和认证 token，并写入 `.run/tunnel.url`：

```
 连接器 URL：  https://xxx.ngrok-free.dev/mcp
 认证 token：  <32位hex，config.json 里 http_token 固定，没配则每次自动生成>
 粘这个即可：  https://xxx.ngrok-free.dev/mcp?key=<token>
```

在 grok.com 的连接器配置里：URL 填连接器地址，认证方式填 Bearer token / API key，
值为 token；不支持认证配置的客户端直接用带 `?key=` 的完整地址。

隧道模式的认证是**强制**的（公网暴露没钥匙等于裸奔）：三种方式任选 ——
`?key=<token>` 查询参数、`Authorization: Bearer <token>`、`X-API-Key: <token>`。

注意：
- ngrok 免费版每次重启地址会变，重启后去 grok.com 改连接器 URL 即可（token 不变）；
  想固定地址需要在 ngrok 控制台建静态域，再加进 `config.json` 的 `ngrok_domain`
- 无状态实现，POST JSON-RPC 回 `application/json`，CORS 全开；Ctrl+C 退出会一并收掉 ngrok
- 端口占用会直接报错退出（不会像之前那样静默抢流量）

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
