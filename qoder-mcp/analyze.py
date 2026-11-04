#!/usr/bin/env python3
"""只读代码分析 CLI，供 sql-monitor 调用。

从 stdin（或 argv 拼接）读取一段自然语言分析提示词，交给 qodercli 以只读工具
（Read / Grep / Glob）执行，并把结果打印到 stdout。sql-monitor 把它当作子进程
运行，再把输出转发到飞书或网页。

约束：只读工具，永不传 --dangerously-skip-permissions，复用 qodercli_runner 的
干净环境与会话管理；会话键固定为 "sql-monitor-analysis"。
"""

import sys

from config import load_config
from qodercli_runner import QoderCliRunner, SessionStore

READ_ONLY_TOOLS = ["Read", "Grep", "Glob"]
ANALYSIS_SESSION = "sql-monitor-analysis"


def main() -> int:
    cfg = load_config()
    sessions = SessionStore(cfg["session_db"])
    runner = QoderCliRunner(
        qodercli_path=cfg["qodercli_path"],
        cwd=cfg["cwd"],
        allowed_tools=READ_ONLY_TOOLS,
        timeout_seconds=cfg["timeout_seconds"],
        sessions=sessions,
    )
    prompt = sys.stdin.read() if len(sys.argv) == 1 else " ".join(sys.argv[1:])
    prompt = prompt.strip()
    if not prompt:
        print("empty analysis prompt", file=sys.stderr)
        return 2
    result = runner.chat(prompt, ANALYSIS_SESSION)
    print(result["output"])
    return 0


if __name__ == "__main__":
    sys.exit(main())
