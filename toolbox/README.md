# toolbox

本地工具控制台。一个页面列出所有工具，能启动、能进入、能看日志。

不代理、不改子工具代码 —— 每个工具保留自己的端口和进程，toolbox 只负责托管生命周期。

## 用法

```bash
cd tools/toolbox
go build -o ~/bin/toolbox .
toolbox              # 打开 http://127.0.0.1:7700
toolbox -stop        # 停掉所有子工具
toolbox -no-autostart
toolbox -host 0.0.0.0   # 局域网可访问（有风险）
```

进程退出（Ctrl-C / SIGTERM）会级联收掉所有子进程，不会留下占端口的孤儿。

## 配置在 `~/.toolbox/`

| 文件 | 内容 |
|---|---|
| `config.yaml` | 凭证、路径、监听地址。`chmod 600`，**不进 git** |
| `tools.yaml` | 工具清单：命令、端口、分组、凭证依赖 |
| `logs/` | 子工具日志 |

### 加一个工具

在 `tools.yaml` 的 `tools:` 下加一段：

```yaml
- id: my-tool
  name: 我的工具
  desc: 一句话说明
  group: web          # web / mcp / script
  dir: my-tool        # 相对 tools 仓库根，或绝对路径
  cmd: ["go", "run", ".", "--port", "9001"]
  port: 9001
  url: http://127.0.0.1:9001
  autostart: false
  needs: [secrets.feishu.app_secret]   # 缺了会在首页标黄
```

命令里可用 `${paths.xxx}` / `${secrets.xxx}` 引用 `config.yaml` 的值，启动前替换。

### 凭证集中

同一个飞书 `app_secret` 原本散在 `feishu-mcp/`、`qoder-mcp/`、`sql-monitor/`、`feishu-message/` 四处。
现在只写 `~/.toolbox/config.yaml` 一处，子工具启动时通过环境变量拿到：

- `FEISHU_APP_ID`
- `FEISHU_APP_SECRET`

`needs:` 里声明了的凭证缺失，首页卡片会直接标出来缺哪一项，不用等启动失败才发现。

## 换机器恢复

首页底部点「生成换机恢复脚本」，下载 `toolbox-bootstrap.sh`：

```bash
bash toolbox-bootstrap.sh
```

做三件事：clone 仓库、`go build` 二进制、`uv sync` 各个 MCP 依赖。
唯一要手动补的是 `~/.toolbox/config.yaml`（含凭证，不进 git）—— 建议放 1Password 或 iCloud。

## 状态含义

| 状态 | 含义 |
|---|---|
| `运行中` | toolbox 启动的（或重启后接管的孤儿），端口在监听 |
| `外部进程` | 端口通着，但不是 toolbox 起的。点「清理」杀掉 |
| `启动中` | 已 fork，等端口就绪（`go run` 首次编译最多 60s） |
| `启动失败` | 看卡片里的错误或点「日志」 |

状态靠**探端口**判断而非记 PID，所以手动起的进程也能被认出来。
唯一例外：toolbox 会把自己拉起的子进程 PID 写进 `~/.toolbox/children.json`，
重启时据此认领上次 `kill -9` 留下的孤儿（标为「运行中」，能正常停止，不会误当外部进程）。

## 点击「进入」

工具用 `window.open` 在新标签页打开，标签页名固定为 `toolbox-<id>` —— 同一个工具重复点不会开一堆。

> 页面内没用内联 `onclick="open(...)"`。内联 handler 的作用域链是 `element → document → window`，
> `open` 会优先命中 `document.open()`，直接把整个页面清空 —— 这是之前「点进入没反应」的根因。
> 现在全部走 `addEventListener` + `data-act` 事件委托。

## 为什么不内嵌 iframe

`sql-monitor` 的 CSP 带 `frame-ancestors 'none'`（`internal/web/handler.go:34`），
`system_tags_editor` 有 loopback Host 校验 —— 内嵌必然白屏。
新标签页方案零改造、零风险，也是 Grafana / Portainer / Jenkins 插件的通行做法。

## 常见问题

**`bind: address already in use`**

工具箱已经在跑了。直接开浏览器，或者先 `toolbox -stop` 再启动。

**卡片显示「外部进程」**

端口被非 toolbox 的进程占着。点「清理」，或手动 `lsof -nP -iTCP:8765 -sTCP:LISTEN` 后 kill。
