# qoder-mcp

让**飞书**驱动 `qodercli` 跑编码任务。方向与同目录的 `feishu-mcp` 相反（那个是 qoder → 飞书）。

```
飞书 ──▶ qoder-mcp(本目录) ──▶ qodercli -p
              │
              └─ 干完活后，qoder 走现有 feishu-mcp 回写文档/发消息
```

本目录提供**两个入口**，按你的智能体形态二选一：

| 入口 | 适用 | 说明 |
| --- | --- | --- |
| `feishu_bot.py` | 自建应用 + 机器人（长连接） | 收 `im.message.receive_v1` 事件 → 调 qodercli → 回复；**无需公网 URL** |
| `server.py` | 飞书智能体 / 智能伙伴 (Aily) | MCP server（stdio / sse / streamable-http） |

## 安装

```bash
cd /Users/xuxinzhi/Documents/gowork/tools/qoder-mcp
uv sync
```

依赖：`mcp`（MCP server）、`lark-oapi`（飞书 SDK），无其它第三方运行时依赖。

## 配置

优先级：环境变量 > `QODER_MCP_CONFIG` 指向的 JSON > 脚本同目录 `config.json` > 当前目录 `config.json`。

复制 `config.example.json` 为 `config.json` 后按需修改。常用项：

| 键 | 环境变量 | 默认 | 说明 |
| --- | --- | --- | --- |
| `qodercli_path` | `QODERCLI_PATH` | 自动探测 | qodercli 可执行文件路径 |
| `cwd` | `QODERCLI_CWD` | 空 | 固定 qoder 工作目录（强烈建议指定） |
| `allowed_tools` | `QODERCLI_ALLOWED_TOOLS` | 空=不限制 | 允许的工具名，逗号分隔（如 `Read,Edit,Write,Grep,Glob,Bash`） |
| `allowed_users` | `QODER_MCP_ALLOWED_USERS` | 空=不限制 | 允许的调用者标识白名单 |
| `token` | `QODER_MCP_TOKEN` | 空=不启用 | streamable-http 模式的 Bearer 鉴权 |
| `timeout_seconds` | `QODER_MCP_TIMEOUT_SECONDS` | 600 | 单次任务超时秒数 |
| `transport` | `QODER_MCP_TRANSPORT` | `stdio` | `stdio` / `sse` / `streamable-http` |
| `host` / `port` | `QODER_MCP_HOST` / `QODER_MCP_PORT` | `127.0.0.1:8000` | HTTP 模式监听地址 |
| `session_db` | `QODER_MCP_SESSION_DB` | `~/.qoder/qoder-mcp/sessions.json` | 会话映射落盘文件 |
| `app_id` / `app_secret` | `FEISHU_APP_ID` / `FEISHU_APP_SECRET` | 空 | 飞书自建应用凭据（仅 `feishu_bot.py` 需要） |

## 运行

**方式一：飞书机器人（自建应用 + 长连接，推荐）**

```bash
uv run python feishu_bot.py
```

前置条件（飞书开放平台，对你的自建应用）：

1. 开启「机器人」能力；
2. 事件与回调 → 订阅 `im.message.receive_v1`（选**长连接**方式，不用填回调 URL）；
3. 权限添加 im:message 的「获取/发送消息」。

启动后给机器人发消息即可。会话按**每个群/单聊**隔离，发「新会话」可重置。

机器人会把 `qodercli -o stream-json` 的完整链路实时转发成**步骤时间线**：先回一条「处理中」，随后边跑边更新同一条消息（💭 思考 / 🔧 工具名+参数 / ↩ 返回摘要），跑完替换为完整时间线 + 最终回答。流程图（mermaid）以源码形式原样贴出，可在 mermaid.live 查看。

**方式二：本机给 qoder 当 MCP 用（stdio）：**

```bash
uv run python server.py
```

在 qoder 的 MCP 配置里加一条 stdio 命令即可（`command: uv`, `args: ["run", "python", "server.py"]`）。

**方式三：给飞书智能体(Aily)连（streamable-http）：**

```bash
QODER_MCP_TRANSPORT=streamable-http QODER_MCP_PORT=8000 uv run python server.py
# MCP endpoint: http://127.0.0.1:8000/mcp
```

## 接入飞书智能体(Aily)（方式三）

1. 在本机（或可被飞书访问的服务器）以 `streamable-http` 启动本服务；
2. 飞书智能体 → MCP 插件/自定义连接器 → 添加 `http://<host>:8000/mcp`；
3. 若设置了 `token`，在连接器里配置 `Authorization: Bearer <token>` 请求头。

## 暴露的工具

| 工具 | 说明 |
| --- | --- |
| `qoder_chat(prompt, user_id?)` | 让 qodercli 执行一次编码任务，同一 user_id 多轮自动续用同一会话 |
| `qoder_reset(user_id?)` | 清除某调用者的会话，下次从新会话开始 |
| `qoder_sessions()` | 列出当前 用户 → 会话 映射（运维排查） |

## 安全说明

- 子进程**永不**传 `--dangerously-skip-permissions`，固定 `--permission-mode default`；
- 启动 qodercli 前会剥离 Qoder.app/SDK 注入的环境变量（否则会误入 SDK 流式模式）；
- 建议始终指定 `cwd` 把 qoder 锁在单个仓库内，并用 `allowed_tools` 收窄可用工具；
- HTTP 模式建议只绑 `127.0.0.1`，跨网络时用反向代理/隧道 + `token` 鉴权；
- `user_id` 由调用方传入，本服务不做身份核验，多用户隔离依赖上层平台注入真实身份；单机自用时用默认会话即可。
