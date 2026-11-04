"""统一配置：环境变量 > QODER_MCP_CONFIG 指向的 JSON > 脚本同目录 config.json > 当前目录 config.json。"""

import json
import os
from pathlib import Path

_DEFAULTS = {
    # qodercli 调用（server.py / feishu_bot.py 共用）
    "qodercli_path": "",
    "cwd": "",
    "allowed_tools": [],
    "timeout_seconds": 600,
    "session_db": "~/.qoder/qoder-mcp/sessions.json",
    # MCP server（server.py）
    "transport": "stdio",
    "host": "127.0.0.1",
    "port": 8000,
    "token": "",
    "allowed_users": [],
    # 飞书机器人（feishu_bot.py）
    "app_id": "",
    "app_secret": "",
}

_ENV_MAP = {
    "QODERCLI_PATH": "qodercli_path",
    "QODERCLI_CWD": "cwd",
    "QODERCLI_ALLOWED_TOOLS": "allowed_tools",
    "QODER_MCP_TIMEOUT_SECONDS": "timeout_seconds",
    "QODER_MCP_SESSION_DB": "session_db",
    "QODER_MCP_TRANSPORT": "transport",
    "QODER_MCP_HOST": "host",
    "QODER_MCP_PORT": "port",
    "QODER_MCP_TOKEN": "token",
    "QODER_MCP_ALLOWED_USERS": "allowed_users",
    "FEISHU_APP_ID": "app_id",
    "FEISHU_APP_SECRET": "app_secret",
}

_LIST_KEYS = {"allowed_tools", "allowed_users"}
_INT_KEYS = {"timeout_seconds", "port"}


def load_config() -> dict:
    cfg = dict(_DEFAULTS)

    path = os.environ.get("QODER_MCP_CONFIG", "").strip()
    candidates = [p for p in (path,) if p]
    candidates += [
        str(Path(__file__).with_name("config.json")),
        str(Path.cwd() / "config.json"),
    ]
    for p in candidates:
        try:
            data = json.loads(Path(p).read_text(encoding="utf-8"))
        except (OSError, ValueError):
            continue
        if isinstance(data, dict):
            for k in cfg:
                if data.get(k) is not None:
                    cfg[k] = data[k]
            break

    for env, key in _ENV_MAP.items():
        v = os.environ.get(env, "").strip()
        if not v:
            continue
        if key in _LIST_KEYS:
            cfg[key] = [x.strip() for x in v.split(",") if x.strip()]
        elif key in _INT_KEYS:
            cfg[key] = int(v)
        else:
            cfg[key] = v
    return cfg
