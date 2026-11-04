"""qodercli 无头调用封装：干净环境 + 会话映射 + 白名单 + 超时。

核心约束：
- 绝不允许传 --dangerously-skip-permissions；
- 固定 --permission-mode default；
- 子进程剥离 Qoder.app / SDK 注入的环境变量，否则 `qodercli -p` 会
  强制走 stream-json SDK 协议而报错；
- `--session-id` 必须是合法 UUID；首次调用用它建会话，后续用
  `--resume <uuid>` 续接（重复传 --session-id 会报 already in use）；
- 会话查找按 cwd 归属项目，故 cwd 需保持一致，否则续接失败。
"""

import json
import os
import queue
import re
import shutil
import subprocess
import threading
import time
import uuid
from pathlib import Path

# Qoder.app / SDK 会注入这些前缀的变量（如 QODER_AGENT_SDK_ENTRYPOINT=sdk-ts），
# 残留时 `qodercli -p` 会误判为 SDK 模式。子进程启动前统一剥离。
_SDK_ENV_PREFIXES = (
    "QODER_AGENT_SDK_",
    "QODER_SDK_",
    "QODER_SESSION_",
    "QODER_WORKER_",
    "QODERCLI_",
    "__QODER",
)

_ANSI_RE = re.compile(r"\x1b\[[0-9;?]*[ -/]*[@-~]")

# --resume 会话已失效时的错误特征（据此触发重建）
_MISSING_SESSION_MARKERS = (
    "Invalid session identifier",
    "No previous sessions",
)


def clean_env() -> dict:
    """返回剥离 SDK 注入变量后的环境副本，其余变量（PATH/HOME/代理等）保留。"""
    env = dict(os.environ)
    for key in list(env):
        if key.upper().startswith(_SDK_ENV_PREFIXES):
            env.pop(key, None)
    return env


def _find_qodercli(path_override: str = "") -> str:
    if path_override:
        p = os.path.expanduser(path_override)
        if os.path.exists(p):
            return p
        raise SystemExit(f"QODERCLI_PATH 指向的 qodercli 不存在: {p}")
    for cand in ("~/.local/bin/qodercli", "~/.qoder/entry/qoder"):
        p = os.path.expanduser(cand)
        if os.path.exists(p):
            return p
    found = shutil.which("qodercli") or shutil.which("qoder")
    if not found:
        raise SystemExit("找不到 qodercli，请设置 QODERCLI_PATH 指向可执行文件")
    return found


class SessionStore:
    """用户 key ↔ qodercli session-id(UUID) 的映射，落盘 JSON。"""

    def __init__(self, path: str):
        self._path = os.path.expanduser(path) if path else ""
        self._lock = threading.Lock()
        self._sessions: dict[str, str] = {}
        if self._path:
            self._load()

    def _load(self) -> None:
        try:
            data = json.loads(Path(self._path).read_text(encoding="utf-8"))
            if isinstance(data, dict):
                self._sessions = {str(k): str(v) for k, v in data.items()}
        except (OSError, ValueError):
            pass

    def _save(self) -> None:
        if not self._path:
            return
        try:
            p = Path(self._path)
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text(
                json.dumps(self._sessions, indent=2, ensure_ascii=False),
                encoding="utf-8",
            )
        except OSError:
            pass

    def get(self, user_key: str) -> str | None:
        with self._lock:
            return self._sessions.get(user_key)

    def set(self, user_key: str, session_id: str) -> None:
        with self._lock:
            self._sessions[user_key] = session_id
            self._save()

    def reset(self, user_key: str) -> None:
        with self._lock:
            self._sessions.pop(user_key, None)
            self._save()

    def list(self) -> dict:
        with self._lock:
            return dict(self._sessions)


class QoderCliRunner:
    """把一次「跑编码任务」翻译成 qodercli -p 调用。"""

    def __init__(
        self,
        qodercli_path: str,
        cwd: str,
        allowed_tools: list[str],
        timeout_seconds: int,
        sessions: SessionStore,
    ):
        self.qodercli = _find_qodercli(qodercli_path)
        self.cwd = os.path.expanduser(cwd) if cwd else ""
        # --allowed-tools 是单值参数，多个工具用逗号拼接（重复传只取第一个）。
        self.allowed_tools = ",".join(t for t in allowed_tools if t)
        self.timeout = int(timeout_seconds)
        self.sessions = sessions
        self._locks: dict[str, threading.Lock] = {}
        self._locks_guard = threading.Lock()

    def _user_lock(self, user_key: str) -> threading.Lock:
        with self._locks_guard:
            lk = self._locks.get(user_key)
            if lk is None:
                lk = threading.Lock()
                self._locks[user_key] = lk
            return lk

    def chat(self, prompt: str, user_key: str = "default") -> dict:
        with self._user_lock(user_key):
            return self._chat_locked(prompt, user_key)

    def _chat_locked(self, prompt: str, user_key: str) -> dict:
        sid = self.sessions.get(user_key)
        result = None
        if sid:
            result = self._run(sid, prompt, resume=True)
            if any(m in result["output"] for m in _MISSING_SESSION_MARKERS):
                self.sessions.reset(user_key)
                sid = None
        if sid is None:
            result = self._run(str(uuid.uuid4()), prompt, resume=False)
            self.sessions.set(user_key, result["session_id"])
        return result

    def _build_args(self, sid: str, prompt: str, resume: bool, stream: bool) -> list[str]:
        args = [self.qodercli, "-p", "--permission-mode", "default"]
        if stream:
            args += ["-o", "stream-json"]
        if resume:
            args += ["--resume", sid]
        else:
            args += ["--session-id", sid]
        if self.cwd:
            args += ["--cwd", self.cwd]
        if self.allowed_tools:
            args += ["--allowed-tools", self.allowed_tools]
        args.append(prompt)
        return args

    def _run(self, sid: str, prompt: str, resume: bool) -> dict:
        args = self._build_args(sid, prompt, resume, stream=False)
        try:
            proc = subprocess.run(
                args,
                capture_output=True,
                text=True,
                timeout=self.timeout,
                env=clean_env(),
            )
        except subprocess.TimeoutExpired:
            return {
                "output": f"任务超过 {self.timeout} 秒被中止。",
                "session_id": sid,
                "exit_code": None,
            }
        output = (proc.stdout or "") + (f"\n{proc.stderr}" if proc.stderr else "")
        output = _ANSI_RE.sub("", output).strip()
        return {
            "output": output,
            "session_id": sid,
            "exit_code": proc.returncode,
        }

    def chat_stream(self, prompt: str, user_key: str, on_event) -> dict:
        """流式执行：`-o stream-json` 逐事件回调 on_event，返回最终结果。

        on_event 收到规范化事件 dict，type 取值：
        thinking / tool_use / tool_result / text / result。
        会话续接失败时静默重建（与 chat 一致）。
        """
        with self._user_lock(user_key):
            sid = self.sessions.get(user_key)
            if sid:
                result, missing = self._run_stream(sid, prompt, resume=True, on_event=on_event)
                if missing:
                    self.sessions.reset(user_key)
                    sid = None
            if sid is None:
                sid = str(uuid.uuid4())
                result, _ = self._run_stream(sid, prompt, resume=False, on_event=on_event)
                self.sessions.set(user_key, sid)
            return result

    def _run_stream(self, sid: str, prompt: str, resume: bool, on_event) -> tuple[dict, bool]:
        args = self._build_args(sid, prompt, resume, stream=True)
        proc = subprocess.Popen(
            args,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            env=clean_env(),
        )

        stderr_lines: list[str] = []

        def _drain_stderr() -> None:
            for line in proc.stderr:
                stderr_lines.append(line)

        def _drain_stdout(q: queue.Queue) -> None:
            for line in proc.stdout:
                for ev in self._parse_line(line):
                    q.put(ev)
            q.put(None)

        q: queue.Queue = queue.Queue()
        threading.Thread(target=_drain_stderr, daemon=True).start()
        threading.Thread(target=_drain_stdout, args=(q,), daemon=True).start()

        deadline = time.monotonic() + self.timeout
        final_text = ""
        timed_out = False
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                proc.kill()
                timed_out = True
                break
            try:
                item = q.get(timeout=min(remaining, 1.0))
            except queue.Empty:
                continue
            if item is None:
                break
            on_event(item)
            if item["type"] == "result":
                final_text = item["text"]

        proc.wait()

        stderr_text = "".join(stderr_lines)
        missing = any(m in stderr_text for m in _MISSING_SESSION_MARKERS)

        if timed_out:
            final_text = f"任务超过 {self.timeout} 秒被中止。"
            try:
                on_event({"type": "result", "text": final_text, "is_error": True})
            except Exception:  # noqa: BLE001 - 回调异常不应影响收尾
                pass
        elif not final_text:
            final_text = _ANSI_RE.sub("", stderr_text).strip()

        return {
            "output": final_text,
            "session_id": sid,
            "exit_code": proc.returncode,
        }, missing

    @staticmethod
    def _parse_line(line: str) -> list[dict]:
        line = line.strip()
        if not line:
            return []
        try:
            obj = json.loads(line)
        except ValueError:
            return []
        if not isinstance(obj, dict):
            return []
        events: list[dict] = []
        t = obj.get("type")
        if t == "assistant":
            for c in obj.get("message", {}).get("content") or []:
                if not isinstance(c, dict):
                    continue
                ct = c.get("type")
                if ct == "thinking":
                    events.append({"type": "thinking", "text": str(c.get("thinking") or "")})
                elif ct == "tool_use":
                    events.append(
                        {"type": "tool_use", "name": str(c.get("name") or ""), "input": c.get("input") or {}}
                    )
                elif ct == "text":
                    events.append({"type": "text", "text": str(c.get("text") or "")})
        elif t == "user":
            for c in obj.get("message", {}).get("content") or []:
                if isinstance(c, dict) and c.get("type") == "tool_result":
                    events.append(
                        {"type": "tool_result", "content": QoderCliRunner._fmt_tool_result(c.get("content"))}
                    )
        elif t == "result":
            events.append(
                {
                    "type": "result",
                    "text": str(obj.get("result") or ""),
                    "is_error": bool(obj.get("is_error")),
                }
            )
        return events

    @staticmethod
    def _fmt_tool_result(content) -> str:
        if isinstance(content, str):
            return content
        if isinstance(content, list):
            parts = []
            for c in content:
                if isinstance(c, dict):
                    parts.append(c.get("text") or "")
                else:
                    parts.append(str(c))
            return "\n".join(p for p in parts if p)
        if content is None:
            return ""
        return json.dumps(content, ensure_ascii=False)
