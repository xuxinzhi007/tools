# apifoxmcp

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

`whoami`、`list_projects`、`list_teams`、`list_branches`、`create_automation_branch`、`search_endpoints`、`get_endpoint`、`pick_endpoints`、`list_scenarios`、`get_scenario`、`create_scenario`、`update_scenario`。

个人令牌不能直接在 `main` 上新建场景。先 `create_automation_branch`，需要的接口用 `pick_endpoints` 挑进该分支，再用 `create_scenario` / `update_scenario` 写步骤。
