#!/usr/bin/env python3
"""Apifox MCP 服务（官方 mcp SDK + FastMCP，stdio）。"""

from mcp.server.fastmcp import FastMCP

import apifox_api as a

mcp = FastMCP("apifox")


@mcp.tool()
def whoami() -> dict:
    """查看当前个人令牌对应的 Apifox 账号。"""
    return a.whoami()


@mcp.tool()
def list_projects() -> dict:
    """列出令牌能看到的项目。"""
    return a.list_projects()


@mcp.tool()
def list_teams() -> dict:
    """列出令牌所在团队。"""
    return a.list_teams()


@mcp.tool()
def list_branches(project_id: int) -> dict:
    """列出项目的迭代分支。个人令牌不能直接写 main，写入前先找自动化分支。"""
    return a.list_branches(project_id)


@mcp.tool()
def create_automation_branch(project_id: int, name: str, fork_from_branch_id: int) -> dict:
    """从指定分支拉一条 createAutomation 分支。后续建场景必须带返回的分支 id。"""
    return a.create_automation_branch(project_id, name, fork_from_branch_id)


@mcp.tool()
def search_endpoints(project_id: int, keyword: str, branch_id: int | None = None) -> dict:
    """按路径或名称在项目接口里搜索，返回 id、method、path、name。"""
    return a.search_endpoints(project_id, keyword, branch_id)


@mcp.tool()
def get_endpoint(project_id: int, endpoint_id: int, branch_id: int | None = None) -> dict:
    """查看一个 HTTP 接口的 method、path、name。"""
    return a.get_endpoint(project_id, endpoint_id, branch_id)


@mcp.tool()
def pick_endpoints(project_id: int, source_branch_id: int, target_branch_id: int, endpoint_ids: list) -> dict:
    """把 main 上的接口挑进自动化分支。分支上没有这些接口时，场景步骤绑不上。"""
    return a.pick_endpoints(project_id, source_branch_id, target_branch_id, endpoint_ids)


@mcp.tool()
def list_scenarios(project_id: int, branch_id: int) -> dict:
    """列出某个分支上的场景用例。"""
    return a.list_scenarios(project_id, branch_id)


@mcp.tool()
def get_scenario(project_id: int, branch_id: int, scenario_id: int) -> dict:
    """读取场景步骤：方法、路径、请求体摘要、后置操作类型。"""
    return a.get_scenario(project_id, branch_id, scenario_id)


@mcp.tool()
def create_scenario(
    project_id: int,
    branch_id: int,
    name: str,
    steps: list,
    description: str = "",
    priority: int = 2,
    folder_id: int = 0,
    reuse_existing: bool = True,
) -> dict:
    """在自动化分支上创建场景并写入 HTTP 步骤。

    同名场景默认直接返回、不复制一份。步骤会带上现有环境的 x-xr-auth-* 和 {{access_token}}。不会运行场景。

    Args:
        steps: 每项形如 {"name": str, "endpoint_id": int, "path": str, "method": str, "body": str, "script": str, "assertions": list}。
    """
    return a.create_scenario(project_id, branch_id, name, description, steps, priority, folder_id, reuse_existing)


@mcp.tool()
def update_scenario(
    project_id: int,
    branch_id: int,
    scenario_id: int,
    name: str,
    steps: list,
    description: str = "",
    priority: int = 2,
    folder_id: int = 0,
) -> dict:
    """覆盖已有场景的步骤。用于修正请求体、断言和后置脚本。不会运行场景。"""
    return a.update_scenario(project_id, branch_id, scenario_id, name, description, steps, priority, folder_id)


def main():
    mcp.run()


if __name__ == "__main__":
    main()
