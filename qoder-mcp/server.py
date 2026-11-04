#!/usr/bin/env python3
"""qoder-mcp：把 qodercli 暴露给飞书智能体的 MCP 服务。

方向与 feishu-mcp 相反：

    飞书智能体(MCP client) ──▶ 本服务 ──▶ qodercli -p / remote-control

传输方式（transport）：
- stdio（默认）：给本机 qoder 当 MCP 用；
- streamable-http / sse：给飞书智能体连（host/port 生效，建议仅绑 127.0.0.1）。

说明：这是「飞书智能体(Aily) → MCP」的接入方式；若你的智能体是
「自建应用 + 机器人」形态，用 feishu_bot.py（长连接收消息）更直接。
"""

import os

from mcp.server.fastmcp import FastMCP
from mcp.server.auth.provider import AccessToken

from config import load_config
from qodercli_runner import QoderCliRunner, SessionStore


class StaticBearerVerifier:
    """静态 Bearer token 校验（非 OAuth）。token 为空时不启用。"""

    def __init__(self, token: str):
        self._token = token

    def verify_token(self, token: str):
        if token and token == self._token:
            return AccessToken(token=token, client_id="qoder-mcp", scopes=[])
        return None


def main() -> None:
    cfg = load_config()
    sessions = SessionStore(cfg["session_db"])
    runner = QoderCliRunner(
        qodercli_path=cfg["qodercli_path"],
        cwd=cfg["cwd"],
        allowed_tools=cfg["allowed_tools"],
        timeout_seconds=cfg["timeout_seconds"],
        sessions=sessions,
    )
    allowed_users = {u for u in cfg["allowed_users"] if u}

    kwargs = {"host": cfg["host"], "port": cfg["port"]}
    if cfg["token"]:
        kwargs["token_verifier"] = StaticBearerVerifier(cfg["token"])
    mcp = FastMCP("qoder-mcp", **kwargs)

    def _user_key(user_id: str) -> str:
        return user_id.strip() or "default"

    def _deny_reason(user_key: str) -> str | None:
        if not allowed_users:
            return None
        if user_key not in allowed_users:
            return f"用户 {user_key} 不在允许名单内。"
        return None

    @mcp.tool()
    def qoder_chat(prompt: str, user_id: str = "") -> dict:
        """让 qodercli 在当前会话里执行一次编码任务（多轮对话自动续用同一会话）。

        Args:
            prompt: 用自然语言描述要 qoder 做的事，例如「帮我在 data-service 里查 X 的调用链」。
            user_id: 可选。调用者标识（如飞书 open_id）；不传则共用默认会话。
        """
        user_key = _user_key(user_id)
        deny = _deny_reason(user_key)
        if deny:
            return {"error": deny}
        return runner.chat(prompt, user_key)

    @mcp.tool()
    def qoder_reset(user_id: str = "") -> dict:
        """清除某个调用者的会话，下次 qoder_chat 将从全新会话开始。

        Args:
            user_id: 可选。与 qoder_chat 传的值一致；不传则重置默认会话。
        """
        user_key = _user_key(user_id)
        deny = _deny_reason(user_key)
        if deny:
            return {"error": deny}
        sessions.reset(user_key)
        return {"message": f"已清除 {user_key} 的会话。"}

    @mcp.tool()
    def qoder_sessions() -> dict:
        """列出当前已建立的 用户 → qodercli 会话 映射（运维排查用）。"""
        return {"sessions": sessions.list()}

    transport = cfg["transport"]
    if transport == "streamable-http":
        mcp.run(transport="streamable-http")
    elif transport == "sse":
        mcp.run(transport="sse")
    else:
        mcp.run(transport="stdio")


if __name__ == "__main__":
    main()
