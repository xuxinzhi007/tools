#!/usr/bin/env python3
"""飞书机器人：把群聊/单聊消息转给 qodercli，再把结果回给用户。

接入方式走「自建应用 + 机器人 + 长连接(WebSocket)」，无需公网回调 URL。
前置条件（飞书开放平台）：
1. 自建应用已开启「机器人」能力；
2. 事件与回调订阅 im.message.receive_v1（长连接方式无需填回调地址）；
3. 权限含 im:message 收发（获取与发送消息）。

会话按 chat_id 隔离（每个群/单聊一个独立 qoder 会话）；发「新会话」可重置。
"""

import json
import re
import time
from concurrent.futures import ThreadPoolExecutor

import lark_oapi as lark
import lark_oapi.api.im.v1 as im
from lark_oapi.event.dispatcher_handler import EventDispatcherHandler
from lark_oapi.ws import Client as WSClient

from config import load_config
from qodercli_runner import QoderCliRunner, SessionStore

# 触发重置会话的指令（精确匹配）
_RESET_KEYWORDS = ("新会话", "重置会话", "清空会话", "重新开始")

# 回复文本上限（字符），超出截断，避免撞到消息长度限制
_MAX_REPLY_CHARS = 4000

# 时间线里最多保留的步骤行数（超出折叠最旧的）
_MAX_STEPS = 30

# update 更新同一消息的最小间隔（秒），避开飞书回复频控
_UPDATE_INTERVAL = 0.8

_HEADER = "🔍 qoder 处理中…"


def _extract_text(message) -> str:
    if message.message_type != "text":
        return ""
    try:
        data = json.loads(message.content or "{}")
    except ValueError:
        return ""
    return str(data.get("text", "")).strip()


def _clip(s: str, n: int) -> str:
    """压平空白并按长度截断，保证时间线每步一行。"""
    s = re.sub(r"\s+", " ", s).strip()
    if len(s) <= n:
        return s
    return s[:n] + "…"


class _LiveTimeline:
    """边跑边 update 同一条消息，最后替换为完整时间线 + 最终回答。"""

    def __init__(self, bot: "FeishuBot", mid: str, header: str):
        self._bot = bot
        self._mid = mid
        self._header = header
        self._steps: list[str] = []
        self._final = ""
        self._dirty = False
        self._last_flush = 0.0

    def add(self, line: str) -> None:
        self._steps.append(line)
        if len(self._steps) > _MAX_STEPS:
            self._steps = ["…（中间省略若干步）"] + self._steps[-(_MAX_STEPS - 1):]
        self._dirty = True
        self._flush(force=False)

    def set_final(self, text: str) -> None:
        self._final = text
        self._flush(force=True)

    def _flush(self, force: bool) -> None:
        if not self._dirty:
            return
        if not force and time.monotonic() - self._last_flush < _UPDATE_INTERVAL:
            return
        body = self._header
        if self._steps:
            body += "\n" + "\n".join(self._steps)
        if self._final:
            body += "\n\n──────\n" + self._final
        if len(body) > _MAX_REPLY_CHARS:
            body = body[:_MAX_REPLY_CHARS] + "\n…（已截断）"
        self._bot._update_text(self._mid, body)
        self._dirty = False
        self._last_flush = time.monotonic()


class FeishuBot:
    def __init__(self, app_id: str, app_secret: str, runner: QoderCliRunner):
        self._app_id = app_id
        self._app_secret = app_secret
        self.runner = runner
        self.api = lark.Client.builder().app_id(app_id).app_secret(app_secret).build()
        self.executor = ThreadPoolExecutor(max_workers=4)

    def on_message(self, data) -> None:
        msg = getattr(data, "event", None) and data.event.message
        if msg is None or msg.message_type != "text":
            return
        if msg.chat_type not in ("p2p", "group"):
            return
        sender = data.event.sender
        if sender is not None and sender.sender_type == "bot":
            return
        # 群里只响应 @ 机器人的消息，避免刷屏
        if msg.chat_type == "group" and not (msg.mentions or []):
            return
        text = _extract_text(msg)
        if not text:
            return
        self.executor.submit(self._handle, msg, text)

    def _handle(self, msg, text: str) -> None:
        chat_id = msg.chat_id or "unknown"
        message_id = msg.message_id
        if text in _RESET_KEYWORDS:
            self.runner.sessions.reset(chat_id)
            self._reply(message_id, "已开启新会话。")
            return
        mid = self._reply(message_id, _HEADER)
        if not mid:
            # 没拿到可编辑的消息 id，退化为一次性回复
            result = self.runner.chat(text, user_key=chat_id)
            self._reply(message_id, (result.get("output") or "").strip() or "(无输出)")
            return
        tl = _LiveTimeline(self, mid, _HEADER)

        def on_event(ev: dict) -> None:
            self._on_event(tl, ev)

        try:
            result = self.runner.chat_stream(text, user_key=chat_id, on_event=on_event)
        except Exception as exc:  # noqa: BLE001 - 兜底，避免静默丢失回复
            result = {"output": f"运行异常：{exc}"}
        final = (result.get("output") or "").strip() or "(无输出)"
        tl.set_final(final)

    def _on_event(self, tl: _LiveTimeline, ev: dict) -> None:
        t = ev.get("type")
        if t == "thinking":
            txt = _clip(ev.get("text") or "", 60)
            if txt:
                tl.add(f"💭 {txt}")
        elif t == "tool_use":
            name = ev.get("name") or "工具"
            try:
                inp = json.dumps(ev.get("input") or {}, ensure_ascii=False)
            except (TypeError, ValueError):
                inp = str(ev.get("input") or "")
            tl.add(f"🔧 {name}\n   {_clip(inp, 120)}")
        elif t == "tool_result":
            txt = _clip(ev.get("content") or "", 120)
            if txt:
                tl.add(f"↩ {txt}")

    def _reply(self, message_id: str, text: str) -> str | None:
        request = (
            im.ReplyMessageRequest.builder()
            .message_id(message_id)
            .request_body(
                im.ReplyMessageRequestBody.builder()
                .msg_type("text")
                .content(json.dumps({"text": text}, ensure_ascii=False))
                .build()
            )
            .build()
        )
        try:
            resp = self.api.im.v1.message.reply(request)
            if not resp.success():
                print(f"[reply] failed: code={resp.code} msg={resp.msg}")
                return None
            return resp.data.message_id
        except Exception as exc:  # noqa: BLE001 - 上报通道异常不应拖垮机器人
            print(f"[reply] exception: {exc}")
            return None

    def _update_text(self, mid: str, text: str) -> None:
        if not mid:
            return
        request = (
            im.UpdateMessageRequest.builder()
            .message_id(mid)
            .request_body(
                im.UpdateMessageRequestBody.builder()
                .msg_type("text")
                .content(json.dumps({"text": text}, ensure_ascii=False))
                .build()
            )
            .build()
        )
        try:
            resp = self.api.im.v1.message.update(request)
            if not resp.success():
                print(f"[update] failed: code={resp.code} msg={resp.msg}")
        except Exception as exc:  # noqa: BLE001
            print(f"[update] exception: {exc}")

    def start(self) -> None:
        handler = (
            EventDispatcherHandler.builder("", "")
            .register_p2_im_message_receive_v1(self.on_message)
            .build()
        )
        ws = WSClient(self._app_id, self._app_secret, event_handler=handler)
        print("qoder 飞书机器人已启动，等待消息…")
        ws.start()


def main() -> None:
    cfg = load_config()
    app_id = cfg["app_id"]
    app_secret = cfg["app_secret"]
    if not app_id or not app_secret:
        raise SystemExit(
            "缺少飞书应用凭据。请设置环境变量 FEISHU_APP_ID / FEISHU_APP_SECRET，"
            "或在 config.json 中填写 app_id / app_secret（见 config.example.json）。"
        )
    sessions = SessionStore(cfg["session_db"])
    runner = QoderCliRunner(
        qodercli_path=cfg["qodercli_path"],
        cwd=cfg["cwd"],
        allowed_tools=cfg["allowed_tools"],
        timeout_seconds=cfg["timeout_seconds"],
        sessions=sessions,
    )
    bot = FeishuBot(app_id, app_secret, runner)
    bot.start()


if __name__ == "__main__":
    main()
