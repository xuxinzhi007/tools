#!/usr/bin/env python3
"""Cursor preToolUse / beforeShellExecution 门禁。

P0 之后的三条边界，改之前先读这里：

1. **只拦写工具。** 读（read_file / grep / list_dir …）一律放行——业务分析
   要读受保护路径，把读当写拦会把排查和需求分析一起锁死。
2. **默认 fail-open。** 脚本出错、输入不是 JSON、解释器缺失时放行并说明。
   只有该项目在 config.yaml 写 `gate_strict: "true"` 时才改成拒绝。
3. **只读、无副作用。** 不再调用 `layout.store_root()`（它会 mkdir specs/）。

放行记录的格式在 references/edit-gate.md。本脚本只认 specs/passes.log。
"""

import fnmatch
import json
import os
import re
import subprocess
import sys


def utf8_stdout():
    for stream in (sys.stdout, sys.stderr):
        try:
            stream.reconfigure(encoding="utf-8", errors="replace")
        except (AttributeError, ValueError):
            pass


SKILL_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, os.path.join(SKILL_ROOT, "scripts"))
import layout  # noqa: E402

# 会写文件的工具。没列到的工具按「不写」处理（fail-open），并在放行时说明，
# 免得将来某个新工具名悄悄绕过门禁而没人知道。
WRITE_TOOLS = {
    "write",
    "write_file",
    "create_file",
    "edit",
    "edit_file",
    "multi_edit",
    "multiedit",
    "str_replace_editor",
    "strreplaceeditor",
    "str_replace",
    "apply_patch",
    "search_replace",
    "insert_edit_into_file",
    "replace_string_in_file",
    "delete_file",
    "move_file",
    "rename_file",
    "notebook_edit",
}

SHELL_TOOLS = {
    "run_terminal_cmd",
    "run_terminal_command",
    "execute_command",
    "shell",
    "bash",
    "terminal",
}


def emit(permission, agent_message="", user_message=""):
    out = {"permission": permission}
    if agent_message:
        out["agent_message"] = agent_message
    if user_message:
        out["user_message"] = user_message
    sys.stdout.write(json.dumps(out, ensure_ascii=False) + "\n")


def repo_root(cwd):
    try:
        out = subprocess.check_output(
            ["git", "-C", cwd or ".", "rev-parse", "--show-toplevel"],
            stderr=subprocess.DEVNULL,
            text=True,
        ).strip()
        return os.path.realpath(out) if out else None
    except (subprocess.CalledProcessError, FileNotFoundError, OSError):
        return None


def rel_path(root, raw):
    if not raw or not isinstance(raw, str):
        return None
    raw = raw.strip().strip("'\"")
    if not raw or raw.startswith("-"):
        return None
    root_real = os.path.realpath(root)
    if os.path.isabs(raw):
        path = os.path.realpath(raw)
    else:
        path = os.path.realpath(os.path.normpath(os.path.join(root_real, raw)))
    try:
        rel = os.path.relpath(path, root_real)
    except ValueError:
        return None
    if rel.startswith(".."):
        return None
    return rel.replace("\\", "/")


def protected(rel):
    # 彻底解耦：默认不再硬编码 data-service / Go 路径。
    # 受保护路径由该项目 config.yaml 的 protected_paths 声明。
    # 未配置时默认不拦，交由 install 时 config.example.yaml 引导填写。
    return False


def needs_waiver(rel):
    # 同上：默认不按 *.go 要求 waiver。技术栈路径由 config.yaml 的 waiver_globs 声明。
    return False


def read_yaml_key(path, key):
    if not os.path.isfile(path):
        return None
    prefix = key + ":"
    try:
        with open(path, encoding="utf-8", errors="replace") as fh:
            lines = fh.readlines()
    except OSError:
        return None
    for line in lines:
        if line.startswith("#") or not line.strip():
            continue
        if line.startswith(prefix):
            val = line.split(":", 1)[1].strip()
            if len(val) >= 2 and val[0] == val[-1] and val[0] in "\"'":
                val = val[1:-1]
            val = re.sub(r"\s+#.*$", "", val).strip()
            return val
    return None


def split_patterns(val):
    if val is None:
        return None
    if not val.strip():
        return []
    return [part.strip() for part in val.split(",") if part.strip()]


def policy(root):
    protected_val = None
    waiver_val = None
    candidates = []
    for tool in (".cursor", ".qoder", ".agents", ".claude", ".codex"):
        candidates.append(os.path.join(root, tool, "skills", "data-workflow", "config.yaml"))
    candidates.append(os.path.join(root, "config.yaml"))
    home = os.environ.get("HOME") or ""
    if home:
        candidates.append(os.path.join(home, ".config", "data-workflow", "config.yaml"))
    for path in candidates:
        if protected_val is None:
            protected_val = read_yaml_key(path, "protected_paths")
        if waiver_val is None:
            waiver_val = read_yaml_key(path, "waiver_globs")
    return split_patterns(protected_val), split_patterns(waiver_val)


STRICT_HINT = "gate_strict 开着时只认 approve 写的放行行（末尾带 approved）。"


def strict(root):
    for key in ("gate_strict",):
        for path in (
            os.path.join(root, "config.yaml"),
            os.path.join(root, ".cursor", "skills", "data-workflow", "config.yaml"),
        ):
            val = read_yaml_key(path, key)
            if val is not None:
                return val.strip().lower() in ("1", "true", "yes", "on")
    return False


def match_any(rel, patterns):
    for pattern in patterns:
        if pattern.endswith("/**"):
            prefix = pattern[:-3].rstrip("/")
            if prefix and (rel == prefix or rel.startswith(prefix + "/")):
                return True
            continue
        if pattern.endswith("/"):
            if rel.startswith(pattern) or rel.rstrip("/") == pattern.rstrip("/"):
                return True
            continue
        if rel == pattern or fnmatch.fnmatch(rel, pattern) or fnmatch.fnmatch(os.path.basename(rel), pattern):
            return True
    return False


def is_protected(root, rel):
    patterns, _ = policy(root)
    if patterns is None:
        return protected(rel)
    return match_any(rel, patterns)


def is_waiver(root, rel):
    _, patterns = policy(root)
    if patterns is None:
        return needs_waiver(rel)
    return match_any(rel, patterns)


def workflow_kind(name):
    if not re.fullmatch(r"[A-Za-z0-9._-]+", name or ""):
        return ""
    path = os.path.join(SKILL_ROOT, "workflows", name + ".json")
    if not os.path.isfile(path):
        return ""
    try:
        with open(path, encoding="utf-8") as fh:
            data = json.load(fh)
    except (OSError, json.JSONDecodeError):
        return ""
    if isinstance(data, dict):
        return data.get("kind") or "execute"
    return ""


def active_runs(store):
    specs = os.path.join(store, "specs")
    found = []
    if not os.path.isdir(specs):
        return found
    for name in sorted(os.listdir(specs)):
        if not re.fullmatch(r"[A-Za-z0-9._-]+", name):
            continue
        path = os.path.join(specs, name, "run.json")
        if not os.path.isfile(path):
            continue
        try:
            with open(path, encoding="utf-8") as fh:
                data = json.load(fh)
        except (OSError, json.JSONDecodeError):
            continue
        if not isinstance(data, dict):
            continue
        if data.get("status") not in ("working", "waiting"):
            continue
        workflow = data.get("workflow") or ""
        found.append((name, workflow_kind(workflow)))
    return found


def covers(prefix, rel):
    prefix = prefix.strip().strip("/")
    if not prefix or ".." in prefix.split("/"):
        return False
    return rel == prefix or rel.startswith(prefix + "/")


def read_passes(store):
    path = os.path.join(store, "specs", "passes.log")
    rows = []
    if not os.path.isfile(path):
        return rows
    with open(path, encoding="utf-8", errors="replace") as fh:
        for line in fh:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            parts = line.split()
            if len(parts) < 3:
                continue
            kind = parts[0]
            if kind == "design":
                rows.append(("design", parts[1], parts[2], "approved" in line))
            elif kind == "waiver":
                rows.append(("waiver", parts[1], " ".join(parts[2:]), "approved" in line))
    return rows


def design_ok(store, feature):
    if not re.fullmatch(r"[A-Za-z0-9._-]+", feature or ""):
        return False
    plan = os.path.join(store, "specs", feature, "plan.md")
    gate = os.path.join(store, "specs", feature, "gate")
    if not os.path.isfile(plan) or os.path.getsize(plan) == 0:
        return False
    if not os.path.isfile(gate):
        return False
    with open(gate, encoding="utf-8", errors="replace") as fh:
        for line in fh:
            if line.strip():
                return line.strip() == "plan pass"
    return False


def store_dir(root):
    """产物目录。只读地取，不建目录（旧版调 studio_root 会 mkdir）。"""
    forced = layout.configured_store(root)
    if forced:
        return os.path.realpath(forced)
    return os.path.realpath(root)


def decision(root, rel):
    store = store_dir(root)
    if rel is None:
        return "allow", ""
    if rel == "specs" or rel.startswith("specs/"):
        if os.path.realpath(store) == os.path.realpath(root):
            return "allow", ""
        return (
            "deny",
            "拒绝写入 %s。运行记录写在 %s ，不写进被测仓库。"
            % (rel, os.path.join(store, "specs")),
        )
    runs = active_runs(store)
    if any(kind == "review" for _, kind in runs):
        return (
            "deny",
            "拒绝写入 %s。当前有未完成的审阅运行，只写 specs/。要改产品代码，等这次审阅结束或另开一次执行运行。"
            % rel,
        )
    execute_features = {feature for feature, kind in runs if kind == "execute"}
    rows = read_passes(store)
    strict_mode = strict(root)
    if is_protected(root, rel):
        for kind, feature, prefix, approved in rows:
            if strict_mode and not approved:
                continue
            if execute_features and feature not in execute_features:
                continue
            if kind == "design" and covers(prefix, rel) and design_ok(store, feature):
                return "allow", ""
        if execute_features:
            return (
                "deny",
                "拒绝写入 %s。design 行的 feature 要是当前运行 %s，并且 specs/<feature>/plan.md 与 gate 首行 plan pass 都在。%s"
                % (rel, "、".join(sorted(execute_features)), STRICT_HINT),
            )
        return (
            "deny",
            "拒绝写入 %s。先完成 specs/<feature>/plan.md，把 specs/<feature>/gate 首行写成 plan pass，再运行 workflow_next.py approve <feature> design <前缀>。%s"
            % (rel, STRICT_HINT),
        )
    if is_waiver(root, rel):
        for kind, a, b, approved in rows:
            if strict_mode and not approved:
                continue
            if kind == "waiver" and covers(a, rel):
                return "allow", ""
            if execute_features and a not in execute_features:
                continue
            if kind == "design" and covers(b, rel) and design_ok(store, a):
                return "allow", ""
        return (
            "deny",
            "拒绝写入 %s。内部改动运行 workflow_next.py approve <feature> waiver <前缀> <原因>（要已有一次拍板回话）。%s"
            % (rel, STRICT_HINT),
        )
    return "allow", ""


def patch_destinations(text, root):
    """apply_patch 的目标路径只在补丁正文里。"""
    found = []
    for match in re.finditer(r"^\*\*\*\s*(?:Add|Update|Delete)\s+File:\s*(.+)$", text or "", re.M):
        rel = rel_path(root, match.group(1))
        if rel:
            found.append(rel)
    return found


def destinations(data, root):
    found = []
    tool_input = data.get("tool_input") or data.get("toolInput") or {}
    if isinstance(tool_input, dict):
        for key in ("path", "file_path", "filePath", "target_file", "target_notebook"):
            rel = rel_path(root, tool_input.get(key))
            if rel:
                found.append(rel)
        edits = tool_input.get("edits") or tool_input.get("files") or []
        if isinstance(edits, list):
            for item in edits:
                if isinstance(item, dict):
                    for key in ("path", "file_path", "filePath"):
                        rel = rel_path(root, item.get(key))
                        if rel:
                            found.append(rel)
        for key in ("input", "patch", "diff"):
            value = tool_input.get(key)
            if isinstance(value, str) and "*** " in value:
                found.extend(patch_destinations(value, root))
    return found


def shell_destinations(command, root):
    """只取写入目标，不把被搜索的路径当成写入。"""
    found = []
    patterns = [
        r"(?:>>|>)\s*([^\s;&|]+)",
        r"\btee\s+(?:-a\s+)?([^\s;&|]+)",
        r"\bsed\s+-\S*i\S*\s+.*?([^\s;&|]+)\s*$",
        r"\bperl\s+-\S*i\S*\s+.*?([^\s;&|]+)\s*$",
    ]
    for pattern in patterns:
        for match in re.finditer(pattern, command):
            rel = rel_path(root, match.group(1))
            if rel:
                found.append(rel)
    if re.search(r"\b(?:rm|mv|cp)\b", command):
        for token in re.findall(r"[A-Za-z0-9_./~-]+\.[A-Za-z0-9]+|[A-Za-z0-9_./~-]*api/[A-Za-z0-9_./~-]*", command):
            rel = rel_path(root, token)
            if rel:
                found.append(rel)
    return found


def targets(data, root):
    """只返回这次调用可能写入的仓库内路径。读工具返回 None。

    工具名大小写不一：Cursor 给 Write，Claude Code 给 Write，有的宿主给 write、
    multiEdit。集合是小写的，所以先归一化再比。不归一化的话受保护路径在
    多种宿主上直接失效——它不会报错，只会默默放行。
    """
    tool = (data.get("tool_name") or data.get("toolName") or "").strip().lower()
    command = data.get("command")
    if isinstance(command, str) and command.strip():
        return shell_destinations(command, root)
    if tool in SHELL_TOOLS:
        tool_input = data.get("tool_input") or data.get("toolInput") or {}
        cmd = tool_input.get("command") if isinstance(tool_input, dict) else ""
        return shell_destinations(cmd, root) if isinstance(cmd, str) else []
    if tool in WRITE_TOOLS:
        return destinations(data, root)
    return None


def main():
    raw = sys.stdin.read()
    try:
        data = json.loads(raw) if raw.strip() else {}
        if not isinstance(data, dict):
            data = {}
    except json.JSONDecodeError:
        cwd = os.getcwd()
        root = repo_root(cwd) or cwd
        if strict(root):
            emit("deny", "编辑门禁没有读到合法 JSON，gate_strict 开着，拒绝这次操作。")
        else:
            emit("allow", "编辑门禁没有读到合法 JSON，按 fail-open 放行（gate_strict 未开）。")
        return 0
    cwd = data.get("cwd") or os.getcwd()
    root = repo_root(cwd)
    if not root:
        emit("allow")
        return 0
    paths = targets(data, root)
    if paths is None:
        emit("allow")
        return 0
    if not paths:
        emit("allow")
        return 0
    for rel in paths:
        permission, message = decision(root, rel)
        if permission == "deny":
            emit("deny", message, message)
            return 0
    emit("allow")
    return 0


if __name__ == "__main__":
    utf8_stdout()
    try:
        raise SystemExit(main())
    except SystemExit:
        raise
    except Exception as exc:  # noqa: BLE001 — fail-open 是默认；严格模式见 gate_strict
        emit("allow", "编辑门禁脚本出错，按 fail-open 放行：%s" % exc)
        raise SystemExit(0)
