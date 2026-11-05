# apifoxmcp

> **已降级为兜底工具。** 主力已切到 Apifox 官方远程 MCP（`https://api.apifox.com/mcp`），
> Cursor / Qoder / pi 均指向官方端点。本地这套保留两个官方没有的能力：
> `snapshot_scenario` / `restore_scenario`（文件级场景备份回滚）、以及 `whoami` / 查询类快捷入口。
>
> **不要用本工具的 `create_scenario` / `update_scenario` 写场景**：已知两个缺陷——
> 重名时静默丢弃 `steps`（返回 `created:false` 却什么都不写）、`assertions` 缺 `subject`
> 时抛 KeyError 而非可读校验错误。写场景请用官方 MCP。
>
> 令牌单一来源：`~/.config/apifox/token`（600）。本目录的 `config.yaml` 仅作 toolbox 启动时注入用。

Apifox 场景用例的 stdio MCP，基于官方 [mcp SDK](https://github.com/modelcontextprotocol/python-sdk) 的 FastMCP 实现。令牌放在本地 `config.yaml`，不要提交。

```yaml
api_fox_tokens: <个人访问令牌>
```

## 启动

```bash
uv run --directory . python server.py
```

MCP 客户端（Qoder / Cursor）里：

```json
{
  "mcpServers": {
    "apifox": {
      "command": "uv",
      "args": ["run", "--directory", "/绝对路径/apifoxmcp", "python", "server.py"]
    }
  }
}
```

## 工具

`whoami`、`list_projects`、`list_teams`、`list_branches`、`create_automation_branch`、`search_endpoints`、`get_endpoint`、`pick_endpoints`、`list_scenarios`、`get_scenario`、`snapshot_scenario`、`restore_scenario`、`create_scenario`、`update_scenario`。

个人令牌不能直接在 `main` 上新建场景。先 `create_automation_branch`，需要的接口用 `pick_endpoints` 挑进该分支，再用 `create_scenario` / `update_scenario` 写步骤。

## 写场景的注意事项

- **步骤数组顺序就是执行顺序**，`number` 只是展示编号。引用 `{{$.N.response.body.xxx}}` 时，N 指执行链上第 N 个执行的步骤。
- **改已有步骤的请求体必须让服务端重建用例**。Apifox 会沿用 case 里已保存的请求体，直接改 `httpApiCase.requestBody.data` 然后 PUT 上去，值会被丢掉且不报错。用 `{"raw": 原 step, "rebuild": True}` 包一层才会生效。
- **改前先 `snapshot_scenario` 存一份**，改坏了用 `restore_scenario` 回滚。
- `get_scenario(full=True)` 才会返回 database 步骤和 customScript 全文，默认只给摘要。

## 踩过的坑

- `create_automation_branch` 超项目分支上限时报 `403 Project branches limit exceeded`，先复用空分支。
- 环境 baseUrl 已经带了服务前缀（如 `https://dev-api.x2robot.com/core`），场景里的相对 path 会拼在它后面。要调其他服务（如 robot 的 `/robot/...`）得在 path 里写绝对 URL。
- 接口响应统一包在 `data` 里，脚本取数要写 `pm.response.json().data.jobs`，断言 jsonPath 写 `$.data.jobs`。
