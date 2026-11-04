#!/usr/bin/python3
"""临时仓库上的外部行为。

编码步骤的 done：没有检查入口、package.json 的 test、Makefile 的 test、
只有 go.mod、显式 verify 盖过 package.json。
会话钩子：没有运行、只有已完成运行时不注入。有未完成运行时注入 NOTICE，不注入 next 的操作指令，也不改 run.json。
"""

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

ME = os.path.abspath(__file__)
SKILL_ROOT = os.path.dirname(os.path.dirname(ME))
REPO_ROOT = os.path.dirname(os.path.dirname(SKILL_ROOT))
RUNNER = os.path.join(SKILL_ROOT, "scripts", "workflow_next.py")
HOOK = os.path.join(SKILL_ROOT, "hooks", "session-start")
GATE = os.path.join(SKILL_ROOT, "hooks", "gate-edit")
INSTALL = os.path.join(REPO_ROOT, "scripts", "install.sh")
FEATURE = "probe"

# 本套工具链的入口都是 bash 壳，python 由壳自己探测。
# 不直接执行 .py：Windows 上 subprocess 会报 WinError 193（shebang 不可执行）。
BASH = shutil.which("bash") or "bash"
PY = sys.executable


def with_store(root, env=None):
    base = dict(env or os.environ)
    base["DATA_WORKFLOW_STORE"] = root
    return base


def run(root, *args):
    return subprocess.run(
        [PY, RUNNER, *args, "--repo", root],
        capture_output=True,
        text=True,
        encoding="utf-8",
        env=with_store(root),
    )


def read_run(root):
    path = os.path.join(root, "specs", FEATURE, "run.json")
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


def reach_code(root):
    start = run(root, "start", "change", FEATURE)
    if start.returncode != 0:
        raise SystemExit(start.stdout + start.stderr)
    folder = os.path.join(root, "specs", FEATURE)
    os.makedirs(folder, exist_ok=True)
    with open(os.path.join(folder, "decision.md"), "w", encoding="utf-8") as fh:
        fh.write("- 原话（一字不改）：探测用\n- 我理解你要的是：改一处\n- 可能理解错的地方：无歧义\n- 本质：改一处\n- 怎样算满足：跑 next 返回 DO\n- 未知：无\n")
    done = run(root, "done", FEATURE)
    if done.returncode != 0 or read_run(root).get("stage") != 2:
        raise SystemExit(done.stdout + done.stderr)
    return done


def finish_code(root):
    return run(root, "done", FEATURE)


def log_text(root):
    path = os.path.join(root, "specs", FEATURE, "verify.log")
    with open(path, encoding="utf-8") as fh:
        return fh.read()


def expect(ok, label, detail):
    if ok:
        print("PASS " + label)
        return
    print("FAIL " + label)
    print(detail)
    raise SystemExit(1)


def case_none():
    """完全没测试入口时：RED + 说清怎么补。"""
    root = tempfile.mkdtemp(prefix="dw-none-")
    try:
        reach_code(root)
        done = finish_code(root)
        out = done.stdout + done.stderr
        expect(
            done.returncode != 0
            and read_run(root).get("stage") == 2
            and "RED" in done.stdout,
            "none stays RED",
            out,
        )
        expect(
            "config.yaml" in out and "verify:" in out,
            "RED names where to configure verify",
            out,
        )
        expect(
            "go.mod" in out and "package.json" in out,
            "RED lists the auto-detected entry files",
            out,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_npm():
    root = tempfile.mkdtemp(prefix="dw-npm-")
    try:
        with open(os.path.join(root, "package.json"), "w", encoding="utf-8") as fh:
            json.dump({"scripts": {"test": "true"}}, fh)
        with open(os.path.join(root, "package-lock.json"), "w", encoding="utf-8") as fh:
            fh.write("{}\n")
        reach_code(root)
        done = finish_code(root)
        log = log_text(root) if os.path.isfile(os.path.join(root, "specs", FEATURE, "verify.log")) else ""
        expect(
            done.returncode == 0
            and read_run(root).get("stage") == 3
            and "cmd npm test" in log,
            "npm test advances",
            done.stdout + done.stderr + log,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_make():
    root = tempfile.mkdtemp(prefix="dw-make-")
    try:
        with open(os.path.join(root, "Makefile"), "w", encoding="utf-8") as fh:
            fh.write("test:\n\ttrue\n")
        reach_code(root)
        done = finish_code(root)
        log = log_text(root) if os.path.isfile(os.path.join(root, "specs", FEATURE, "verify.log")) else ""
        expect(
            done.returncode == 0
            and read_run(root).get("stage") == 3
            and "cmd make test" in log,
            "make test advances",
            done.stdout + done.stderr + log,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_gomod():
    """go.mod 就是 Go 项目的声明入口，不再要求手配 verify。"""
    root = tempfile.mkdtemp(prefix="dw-go-")
    try:
        with open(os.path.join(root, "go.mod"), "w", encoding="utf-8") as fh:
            fh.write("module example.com/probe\n\ngo 1.22\n")
        with open(os.path.join(root, "probe.go"), "w", encoding="utf-8") as fh:
            fh.write("package probe\n")
        reach_code(root)
        done = finish_code(root)
        # 不能只看 exit 0：没探测到时会停在 RED、根本没跑测试。
        # 必须确认真的走了 go test 且产生 verify.log。
        log = (
            log_text(root)
            if os.path.isfile(os.path.join(root, "specs", FEATURE, "verify.log"))
            else ""
        )
        expect(
            "go test" in log and "verify.log" not in log.split("\n")[0],
            "go.mod discovered as the verify entry",
            done.stdout + done.stderr + "\n--- verify.log ---\n" + log,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_install_makes_tool_dir():
    """新仓库连 .cursor 都没有时，--project 应该自己建，而不是直接拒。"""
    root = tempfile.mkdtemp(prefix="dw-nodir-")
    home = tempfile.mkdtemp(prefix="dw-nodirhome-")
    try:
        git_init(root)
        with open(os.path.join(root, "README.md"), "w", encoding="utf-8") as fh:
            fh.write("probe\n")
        env = clean_env()
        env["HOME"] = home
        proc = subprocess.run(
            ["bash", INSTALL, "--project"],
            cwd=root,
            capture_output=True,
            text=True,
            env=env,
        )
        out = proc.stdout + proc.stderr
        expect(
            proc.returncode == 0 and "NOTHING" not in out,
            "--project on a repo without tool dirs does not refuse",
            out,
        )
        expect(
            os.path.isdir(os.path.join(root, ".cursor", "skills", "data-workflow")),
            "--project creates the Cursor skill dir when none exists",
            out,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)
        shutil.rmtree(home, ignore_errors=True)


def case_verify_wins():
    root = tempfile.mkdtemp(prefix="dw-verify-")
    try:
        with open(os.path.join(root, "package.json"), "w", encoding="utf-8") as fh:
            json.dump({"scripts": {"test": "true"}}, fh)
        with open(os.path.join(root, "config.yaml"), "w", encoding="utf-8") as fh:
            fh.write('verify: "false"\n')
        reach_code(root)
        done = finish_code(root)
        log = log_text(root) if os.path.isfile(os.path.join(root, "specs", FEATURE, "verify.log")) else ""
        expect(
            done.returncode != 0
            and read_run(root).get("stage") == 2
            and "RED" in done.stdout
            and "cmd false" in log,
            "explicit verify stops",
            done.stdout + done.stderr + log,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def git_init(root):
    subprocess.run(["git", "init", "-q", root], check=True)


def git_commit_all(root, message="probe"):
    subprocess.run(["git", "-C", root, "add", "-A"], check=True)
    subprocess.run(
        [
            "git",
            "-C",
            root,
            "-c",
            "user.name=probe",
            "-c",
            "user.email=probe@example.com",
            "commit",
            "-qm",
            message,
        ],
        check=True,
    )


def hook_text(root):
    proc = subprocess.run(
        [BASH, HOOK],
        cwd=root,
        capture_output=True,
        text=True,
        encoding="utf-8",
        env=with_store(root),
        check=True,
    )
    data = json.loads(proc.stdout)
    return data.get("additional_context") or ""


def case_hook_quiet():
    empty = tempfile.mkdtemp(prefix="dw-hook-empty-")
    done = tempfile.mkdtemp(prefix="dw-hook-done-")
    open_root = tempfile.mkdtemp(prefix="dw-hook-open-")
    try:
        git_init(empty)
        expect(hook_text(empty) == "", "no run injects nothing", hook_text(empty))

        git_init(done)
        folder = os.path.join(done, "specs", "finished")
        os.makedirs(folder)
        with open(os.path.join(folder, "run.json"), "w", encoding="utf-8") as fh:
            json.dump(
                {
                    "workflow": "change",
                    "feature": "finished",
                    "stage": 3,
                    "status": "done",
                    "question": "",
                    "reply": "",
                },
                fh,
            )
        expect(hook_text(done) == "", "finished run injects nothing", hook_text(done))

        left = tempfile.mkdtemp(prefix="dw-hook-left-")
        try:
            git_init(left)
            folder = os.path.join(left, "specs", "old")
            os.makedirs(folder)
            with open(os.path.join(folder, "run.json"), "w", encoding="utf-8") as fh:
                json.dump(
                    {
                        "workflow": "change",
                        "feature": "old",
                        "stage": 2,
                        "status": "working",
                        "question": "",
                        "reply": "",
                    },
                    fh,
                )
            dropped = run(left, "abandon", "old")
            status = json.load(open(os.path.join(folder, "run.json"), encoding="utf-8")).get("status")
            injected = hook_text(left)
            expect(
                dropped.returncode == 0 and injected == "" and status == "abandoned",
                "abandon stops injection",
                dropped.stdout + injected,
            )
        finally:
            shutil.rmtree(left, ignore_errors=True)

        git_init(open_root)
        started = run(open_root, "start", "change", FEATURE)
        text = hook_text(open_root)
        status = json.load(open(os.path.join(open_root, "specs", FEATURE, "run.json"), encoding="utf-8")).get("status")
        nxt = run(open_root, "next")
        expect(
            started.returncode == 0
            and text.startswith("NOTICE %s working" % FEATURE)
            and "NEED" not in text
            and "写完后运行" not in text
            and status == "working"
            and "NEED" in nxt.stdout,
            "open run injects a notice and next still prints the step",
            text + nxt.stdout,
        )
        waited = run(open_root, "wait", FEATURE, "继续吗？", "继续")
        noticed = hook_text(open_root)
        record = json.load(open(os.path.join(open_root, "specs", FEATURE, "run.json"), encoding="utf-8"))
        resumed = run(open_root, "next")
        expect(
            waited.returncode == 0
            and "NOTICE %s waiting" % FEATURE in noticed
            and "摘要：继续吗？" in noticed
            and "answer " not in noticed
            and record.get("status") == "waiting"
            and "WAIT" in resumed.stdout
            and "REPLY 继续" in resumed.stdout,
            "waiting notice does not resume or clear the run",
            noticed + resumed.stdout,
        )
    finally:
        shutil.rmtree(empty, ignore_errors=True)
        shutil.rmtree(done, ignore_errors=True)
        shutil.rmtree(open_root, ignore_errors=True)


def tool(name):
    return shutil.which(name) is not None


def module(name):
    proc = subprocess.run(
        ["python3", "-c", "import %s" % name],
        capture_output=True,
        text=True,
    )
    return proc.returncode == 0


def case_selected(label, write_files, command, advances):
    root = tempfile.mkdtemp(prefix="dw-sel-")
    try:
        write_files(root)
        reach_code(root)
        done = finish_code(root)
        log = log_text(root) if os.path.isfile(os.path.join(root, "specs", FEATURE, "verify.log")) else ""
        chosen = ("cmd %s" % command) in log
        if advances:
            ok = done.returncode == 0 and read_run(root).get("stage") == 3 and chosen
        else:
            ok = (
                done.returncode != 0
                and read_run(root).get("stage") == 2
                and "RED" in done.stdout
                and chosen
            )
        expect(ok, label, done.stdout + done.stderr + log)
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_pnpm():
    def write_files(root):
        with open(os.path.join(root, "package.json"), "w", encoding="utf-8") as fh:
            json.dump({"scripts": {"test": "true"}}, fh)
        with open(os.path.join(root, "pnpm-lock.yaml"), "w", encoding="utf-8") as fh:
            fh.write("lockfileVersion: 9\n")

    case_selected("pnpm test", write_files, "pnpm test", tool("pnpm"))


def case_yarn():
    def write_files(root):
        with open(os.path.join(root, "package.json"), "w", encoding="utf-8") as fh:
            json.dump({"scripts": {"test": "true"}}, fh)
        with open(os.path.join(root, "yarn.lock"), "w", encoding="utf-8") as fh:
            fh.write("# yarn lock\n")

    case_selected("yarn test", write_files, "yarn test", tool("yarn"))


def case_pytest():
    def write_files(root):
        with open(os.path.join(root, "pytest.ini"), "w", encoding="utf-8") as fh:
            fh.write("[pytest]\n")
        with open(os.path.join(root, "test_probe.py"), "w", encoding="utf-8") as fh:
            fh.write("def test_ok():\n    assert True\n")

    case_selected("pytest", write_files, "python3 -m pytest -q", module("pytest"))


def case_cargo():
    def write_files(root):
        with open(os.path.join(root, "Cargo.toml"), "w", encoding="utf-8") as fh:
            fh.write('[package]\nname = "probe"\nversion = "0.1.0"\nedition = "2021"\n')
        os.makedirs(os.path.join(root, "src"))
        with open(os.path.join(root, "src", "lib.rs"), "w", encoding="utf-8") as fh:
            fh.write("#[cfg(test)]\nmod tests {\n    #[test]\n    fn ok() {}\n}\n")

    case_selected("cargo test", write_files, "cargo test", tool("cargo"))


def case_answer():
    root = tempfile.mkdtemp(prefix="dw-answer-")
    try:
        start = run(root, "start", "change", "ask")
        folder = os.path.join(root, "specs", "ask")
        os.makedirs(folder, exist_ok=True)
        with open(os.path.join(folder, "decision.md"), "w", encoding="utf-8") as fh:
            fh.write("- 待拍板：用全量还是三态？\n")
        nxt = run(root, "next", "ask")
        waited = run(root, "wait", "ask", "用全量还是三态？", "全量")
        record = json.load(open(os.path.join(folder, "run.json"), encoding="utf-8"))
        mismatch = run(root, "answer", "ask", "三态")
        answered = run(root, "answer", "ask", "全量")
        text = open(os.path.join(folder, "decision.md"), encoding="utf-8").read()
        expect(
            start.returncode == 0
            and "OPEN" in nxt.stdout
            and waited.returncode == 0
            and record.get("reply") == "全量"
            and mismatch.returncode != 0
            and "MISMATCH" in mismatch.stdout
            and answered.returncode == 0
            and "已拍板：全量" in text,
            "reply is recorded",
            nxt.stdout + waited.stdout + mismatch.stdout + answered.stdout + text,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def clean_env():
    env = os.environ.copy()
    for key in list(env):
        if key.startswith("DATA_WORKFLOW_"):
            del env[key]
    return env


def gate_permission(hook, root, path, tool_name="edit_file", env=None, raw=None):
    payload = raw if raw is not None else json.dumps(
        {"cwd": root, "tool_name": tool_name, "tool_input": {"path": path}}
    )
    proc = subprocess.run(
        [BASH, hook],
        input=payload,
        cwd=root,
        capture_output=True,
        text=True,
        encoding="utf-8",
        env=env or with_store(root),
        check=True,
    )
    return json.loads(proc.stdout).get("permission")


def case_foreign():
    root = tempfile.mkdtemp(prefix="dw-foreign-")
    try:
        git_init(root)
        os.makedirs(os.path.join(root, ".cursor", "skills", "keep-me"))
        with open(os.path.join(root, ".cursor", "skills", "keep-me", "SKILL.md"), "w", encoding="utf-8") as fh:
            fh.write("keep\n")
        with open(os.path.join(root, "package.json"), "w", encoding="utf-8") as fh:
            json.dump({"scripts": {"test": "true"}}, fh)
        with open(os.path.join(root, "package-lock.json"), "w", encoding="utf-8") as fh:
            fh.write("{}\n")
        installed = subprocess.run(
            [BASH, INSTALL, "--project", "--with-hooks", root],
            cwd=root,
            capture_output=True,
            text=True,
            encoding="utf-8",
            env=clean_env(),
        )
        copy = os.path.join(root, ".cursor", "skills", "data-workflow")
        expect(
            installed.returncode == 0 and os.path.isdir(copy) and not os.path.islink(copy),
            "foreign install copies the skill",
            installed.stdout + installed.stderr,
        )
        script = os.path.realpath(os.path.join(copy, "scripts", "workflow_next.py"))
        expect(
            script.startswith(os.path.realpath(root) + os.sep)
            and os.path.isfile(os.path.join(root, ".cursor", "skills", "keep-me", "SKILL.md")),
            "copy stays in the foreign repo and keeps the other skill",
            script,
        )
        hooks = json.load(open(os.path.join(root, ".cursor", "hooks.json"), encoding="utf-8"))
        commands = []
        for entries in (hooks.get("hooks") or {}).values():
            for entry in entries:
                commands.append(entry.get("command") or "")
        expect(
            any(cmd.endswith("data-workflow/hooks/session-start") for cmd in commands)
            and all(REPO_ROOT not in cmd for cmd in commands),
            "foreign hooks point at the copy",
            "\n".join(commands),
        )
        expect(
            all(entry.get("failClosed") is not True for entries in (hooks.get("hooks") or {}).values() for entry in entries),
            "hooks do not fail closed (strictness is gate_strict's job)",
            json.dumps(hooks, ensure_ascii=False),
        )
        config_path = os.path.join(copy, "config.yaml")
        config_text = open(config_path, encoding="utf-8").read()
        paths = subprocess.run(
            [BASH, os.path.join(copy, "scripts", "paths.sh")],
            cwd=root,
            capture_output=True,
            text=True,
            encoding="utf-8",
            env=clean_env(),
        )
        expect(
            "check_runner" not in config_text
            and "KNOW=''" in paths.stdout
            and "TEMPLATES=''" in paths.stdout
            and "REPO=''" in paths.stdout,
            "foreign knowledge path stays empty",
            paths.stdout + paths.stderr,
        )
        gate = os.path.join(copy, "hooks", "gate-edit")
        expect(
            gate_permission(gate, root, "api/foo.proto") == "allow"
            and gate_permission(gate, root, "internal/x.go") == "allow"
            and gate_permission(gate, root, "README.md") == "allow",
            "foreign repo defaults to no write gate until protected_paths declared",
            "",
        )
        with open(config_path, "a", encoding="utf-8") as fh:
            fh.write('protected_paths: "*.proto"\n')
        expect(
            gate_permission(gate, root, "api/foo.proto") == "deny"
            and gate_permission(gate, root, "README.md") == "allow",
            "foreign repo write gate activates on declared protected_paths",
            "",
        )
        runner = os.path.join(copy, "scripts", "workflow_next.py")

        def copy_run(*args):
            return subprocess.run(
                [PY, runner, *args, "--repo", root],
                cwd=root,
                capture_output=True,
                text=True,
                encoding="utf-8",
                env=with_store(root, clean_env()),
            )

        started = copy_run("start", "change", FEATURE)
        folder = os.path.join(root, "specs", FEATURE)
        os.makedirs(folder, exist_ok=True)
        with open(os.path.join(folder, "decision.md"), "w", encoding="utf-8") as fh:
            fh.write("- 原话（一字不改）：探测用\n- 我理解你要的是：改一处\n- 可能理解错的地方：无歧义\n- 本质：改一处\n- 怎样算满足：跑 next 返回 DO\n- 未知：无\n")
        stepped = copy_run("done", FEATURE)
        finished = copy_run("done", FEATURE)
        record = json.load(open(os.path.join(folder, "run.json"), encoding="utf-8"))
        log = open(os.path.join(folder, "verify.log"), encoding="utf-8").read()
        expect(
            started.returncode == 0
            and stepped.returncode == 0
            and finished.returncode == 0
            and record.get("stage") == 3
            and "cmd npm test" in log
            and "check_runner" not in log,
            "foreign code step uses the copy and npm test",
            started.stdout + stepped.stdout + finished.stdout + log,
        )
        hook = subprocess.run(
            [BASH, os.path.join(copy, "hooks", "session-start")],
            cwd=root,
            capture_output=True,
            text=True,
            encoding="utf-8",
            env=with_store(root, clean_env()),
            check=True,
        )
        injected = json.loads(hook.stdout).get("additional_context") or ""
        after = json.load(open(os.path.join(folder, "run.json"), encoding="utf-8"))
        expect(
            "NOTICE" in injected
            and FEATURE in injected
            and "NEED" not in injected
            and "写完后运行" not in injected
            and after.get("status") == "working"
            and after.get("stage") == record.get("stage"),
            "foreign session hook injects a notice and leaves the run",
            injected,
        )
        feature = copy_run("start", "feature", "chain")
        expect(
            "NEED specs/chain/spec.md" in feature.stdout
            and "stages/requirement-analysis/SKILL.md" in feature.stdout,
            "foreign copy still starts the feature chain",
            feature.stdout,
        )
        review = copy_run("start", "review-only", "look")
        expect(
            review.returncode == 0 and gate_permission(gate, root, "README.md") == "deny",
            "foreign review run blocks product writes",
            review.stdout,
        )
        commands = os.path.join(root, ".cursor", "commands")
        next_cmd = open(os.path.join(commands, "dw-next.md"), encoding="utf-8").read()
        with open(os.path.join(commands, "mine.md"), "w", encoding="utf-8") as fh:
            fh.write("mine\n")
        expect(
            "workflow_next.py next" in next_cmd
            and os.path.isfile(os.path.join(commands, "dw-done.md"))
            and os.path.isfile(os.path.join(commands, "dw-wait.md"))
            and os.path.isfile(os.path.join(commands, "dw-start.md")),
            "foreign install adds slash commands",
            next_cmd,
        )
        with open(config_path, "a", encoding="utf-8") as fh:
            fh.write('verify: "echo foreign-marker"\n')
        again = subprocess.run(
            [BASH, INSTALL, root],
            cwd=root,
            capture_output=True,
            text=True,
            encoding="utf-8",
            env=clean_env(),
        )
        kept = open(config_path, encoding="utf-8").read()
        other = open(os.path.join(root, ".cursor", "skills", "keep-me", "SKILL.md"), encoding="utf-8").read()
        mine = open(os.path.join(commands, "mine.md"), encoding="utf-8").read()
        expect(
            again.returncode == 0
            and 'verify: "echo foreign-marker"' in kept
            and other == "keep\n"
            and mine == "mine\n"
            and "COMMAND " in again.stdout,
            "reinstall keeps foreign config, the other skill, and other commands",
            again.stdout + again.stderr + kept,
        )
        removed = subprocess.run(
            [BASH, INSTALL, "--unlink", root],
            cwd=root,
            capture_output=True,
            text=True,
            encoding="utf-8",
            env=clean_env(),
        )
        expect(
            removed.returncode == 0
            and not os.path.isfile(os.path.join(commands, "dw-next.md"))
            and open(os.path.join(commands, "mine.md"), encoding="utf-8").read() == "mine\n"
            and open(os.path.join(root, ".cursor", "skills", "keep-me", "SKILL.md"), encoding="utf-8").read() == "keep\n",
            "unlink removes only the dw slash commands",
            removed.stdout + removed.stderr,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_default_no_hooks():
    """默认安装（不加 --with-hooks）不写 hooks.json，也不把副本写进项目。

    原因：hooks.json 写的是相对路径，指向项目内副本。副本没提交时 hook 文件缺失，
    那套门禁实际上不存在。宁可没有门禁，也不要给团队一个坏门。
    """
    root = tempfile.mkdtemp(prefix="dw-defaulthooks-")
    home = tempfile.mkdtemp(prefix="dw-home-")
    try:
        git_init(root)
        os.makedirs(os.path.join(root, ".cursor"))
        os.makedirs(os.path.join(home, ".cursor"))
        env = clean_env()
        env["HOME"] = home
        proc = subprocess.run(
            [BASH, INSTALL],
            cwd=root,
            capture_output=True,
            text=True,
            encoding="utf-8",
            env=env,
        )
        expect(
            proc.returncode == 0
            and not os.path.exists(os.path.join(root, ".cursor", "hooks.json")),
            "default install writes no hooks.json",
            proc.stdout + proc.stderr,
        )
        expect(
            not os.path.isdir(
                os.path.join(root, ".cursor", "skills", "data-workflow")
            ),
            "default install leaves no skill copy in the project",
            proc.stdout + proc.stderr,
        )
        expect(
            os.path.isdir(
                os.path.join(home, ".cursor", "skills", "data-workflow", "stages")
            ),
            "default install puts the full skill in HOME",
            proc.stdout + proc.stderr,
        )
        expect(
            os.path.isfile(
                os.path.join(root, ".cursor", "commands", "dw-next.md")
            ),
            "default install still writes slash commands into the project",
            proc.stdout + proc.stderr,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)
        shutil.rmtree(home, ignore_errors=True)


def case_record_follows_install():
    """装进哪个仓库，运行记录就写在那个仓库。

    不回到技能源仓库的 runs/。这是用户要的：记录跟着 install 的地方走。
    """
    root = tempfile.mkdtemp(prefix="dw-record-")
    home = tempfile.mkdtemp(prefix="dw-rechome-")
    try:
        git_init(root)
        os.makedirs(os.path.join(root, ".cursor"))
        os.makedirs(os.path.join(home, ".cursor"))
        env = clean_env()
        env["HOME"] = home
        subprocess.run(
            [BASH, INSTALL, "--project"],
            cwd=root,
            capture_output=True,
            text=True,
            encoding="utf-8",
            env=env,
        )
        runner = os.path.join(root, ".cursor", "skills", "data-workflow", "scripts", "workflow_next.py")
        # 关键：不传 DATA_WORKFLOW_STORE，走默认推断。
        proc = subprocess.run(
            [PY, runner, "start", "change", FEATURE, "--repo", root],
            cwd=root,
            capture_output=True,
            text=True,
            encoding="utf-8",
            env=env,
        )
        expect(
            os.path.isfile(os.path.join(root, "specs", FEATURE, "run.json")),
            "record lands in the installed repo",
            proc.stdout + proc.stderr,
        )
        expect(
            not os.path.isdir(os.path.join(REPO_ROOT, "runs")),
            "record does not leak back into the skill source repo",
            lambda: "",
        )
        ignore = os.path.join(root, ".gitignore")
        text = open(ignore, encoding="utf-8").read() if os.path.isfile(ignore) else ""
        expect(
            "specs/" in text,
            "install keeps specs/ out of git status",
            text,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)
        shutil.rmtree(home, ignore_errors=True)


def write_config(root, text):
    with open(os.path.join(root, "config.yaml"), "w", encoding="utf-8") as fh:
        fh.write(text)


def case_gate_tool_case():
    """各宿主给的工具名大小写不一，门禁必须都认得。

    Cursor 给 Write，Claude Code 给 Write，有的宿主给 write / multiEdit。
    不归一化的话受保护路径会默默失效——不报错，只是放行。这是我们
t    在五个平台上实测到的真问题。
    """
    root = tempfile.mkdtemp(prefix="dw-gate-case-")
    try:
        git_init(root)
        os.makedirs(os.path.join(root, "api"))
        with open(os.path.join(root, "api", "a.proto"), "w", encoding="utf-8") as fh:
            fh.write("syntax = \"proto3\";\n")
        with open(os.path.join(root, "README.md"), "w", encoding="utf-8") as fh:
            fh.write("probe\n")
        write_config(root, 'protected_paths: "api/**"\n')
        for tool in ("Write", "write", "Edit", "edit_file", "MultiEdit", "multiEdit", "apply_patch"):
            expect(
                gate_permission(GATE, root, "api/a.proto", tool) == "deny",
                "protected write is denied for tool name %r" % tool,
                "",
            )
        for tool in ("Read", "read_file", "grep", "list_dir", "Grep"):
            expect(
                gate_permission(GATE, root, "api/a.proto", tool) == "allow",
                "reading stays allowed for tool name %r" % tool,
                "",
            )
        expect(
            gate_permission(GATE, root, "README.md", "Write") == "allow",
            "unprotected write is still allowed",
            "",
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_gate_read_write():
    """读不能当写拦。

    受保护路径上的 read_file / grep 必须放行：业务分析要读的正是这些文件。
    旧版 destinations() 不看 tool_name，把这些读也当写拒了。
    """
    root = tempfile.mkdtemp(prefix="dw-gate-rw-")
    try:
        git_init(root)
        os.makedirs(os.path.join(root, "api"))
        with open(os.path.join(root, "api", "a.proto"), "w", encoding="utf-8") as fh:
            fh.write("syntax = \"proto3\";\n")
        write_config(root, 'protected_paths: "api/**"\nwaiver_globs: "*.go"\n')
        expect(
            gate_permission(GATE, root, "api/a.proto", "read_file") == "allow"
            and gate_permission(GATE, root, "api/a.proto", "grep" ) == "allow"
            and gate_permission(GATE, root, "api/a.proto", "list_dir") == "allow",
            "reading protected paths is allowed",
            "",
        )
        expect(
            gate_permission(GATE, root, "api/a.proto", "edit_file") == "deny"
            and gate_permission(GATE, root, "api/a.proto", "write") == "deny",
            "writing protected paths is denied",
            "",
        )
        expect(
            gate_permission(GATE, root, "api/a.proto", "some_future_tool") == "allow",
            "unknown tool name fails open instead of silently blocking",
            "",
        )
        expect(
            gate_permission(GATE, root, "README.md", "edit_file") == "allow",
            "unprotected paths stay writable",
            "",
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_gate_strict():
    """坏输入默认放行；gate_strict 开着才拒绝。"""
    root = tempfile.mkdtemp(prefix="dw-gate-strict-")
    try:
        git_init(root)
        loose = gate_permission(GATE, root, "README.md", raw="{not json")
        write_config(root, 'gate_strict: "true"\n')
        tight = gate_permission(GATE, root, "README.md", raw="{not json")
        expect(
            loose == "allow" and tight == "deny",
            "bad input fails open unless gate_strict is on",
            "loose=%s tight=%s" % (loose, tight),
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_gate_no_python():
    """解释器缺失时不制造坏门：输出合法 JSON 并放行。"""
    root = tempfile.mkdtemp(prefix="dw-gate-nopy-")
    empty = tempfile.mkdtemp(prefix="dw-nopath-")
    try:
        git_init(root)
        env = with_store(root)
        env["PATH"] = empty
        proc = subprocess.run(
            [BASH, GATE],
            input="{}",
            cwd=root,
            capture_output=True,
            text=True,
            encoding="utf-8",
            env=env,
        )
        data = json.loads(proc.stdout)
        expect(
            proc.returncode == 0
            and data.get("permission") == "allow"
            and "python" in (data.get("agent_message") or ""),
            "missing interpreter allows instead of blocking",
            proc.stdout + proc.stderr,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)
        shutil.rmtree(empty, ignore_errors=True)


def case_utf8_output():
    """中文输出在 UTF-8 下可读（Windows 默认 GBK 会变成乱码）。"""
    root = tempfile.mkdtemp(prefix="dw-utf8-")
    try:
        git_init(root)
        run(root, "start", "change", FEATURE)
        nxt = run(root, "next")
        expect(
            "写完后运行" in nxt.stdout and "\ufffd" not in nxt.stdout,
            "runner prints readable utf-8",
            nxt.stdout + nxt.stderr,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_no_phantom():
    """不造 stage-N.md 幻影文件名。"""
    root = tempfile.mkdtemp(prefix="dw-phantom-")
    try:
        git_init(root)
        hot = run(root, "start", "hotfix", "h1")
        expect(
            "NEED specs/h1/triage.md" in hot.stdout and "stage-1.md" not in hot.stdout,
            "hotfix asks for triage.md, not a phantom stage file",
            hot.stdout,
        )
        run(root, "start", "review-only", "r1")
        folder = os.path.join(root, "specs", "r1")
        os.makedirs(os.path.join(folder, "review"), exist_ok=True)
        with open(os.path.join(folder, "diff.patch"), "w", encoding="utf-8") as fh:
            fh.write("diff\n")
        for name in ("spec-compliance.md", "code-quality.md", "interface-impact.md"):
            with open(os.path.join(folder, "review", name), "w", encoding="utf-8") as fh:
                fh.write("x\n")
        with open(os.path.join(folder, "interface-test.md"), "w", encoding="utf-8") as fh:
            fh.write("x\n")
        nxt = run(root, "next", "r1")
        expect(
            "NEED specs/r1/review/verdict.md" in nxt.stdout
            and "stage-4.md" not in nxt.stdout,
            "review-only asks for review/verdict.md",
            nxt.stdout,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_intake():
    """原话进记录，缺「未知」停在内容门。"""
    root = tempfile.mkdtemp(prefix="dw-intake-")
    try:
        git_init(root)
        raw = "把导出的默认集改成三态，别再问我「根据项目规则」"
        run(root, "start", "change", FEATURE, raw)
        record = read_run(root)
        expect(
            record.get("raw_request") == raw,
            "start stores the raw request verbatim",
            json.dumps(record, ensure_ascii=False),
        )
        folder = os.path.join(root, "specs", FEATURE)
        with open(os.path.join(folder, "decision.md"), "w", encoding="utf-8") as fh:
            fh.write("- 原话（一字不改）：%s\n- 本质：改默认集\n" % raw)
        blocked = run(root, "next")
        expect(
            "GATE" in blocked.stdout
            and "未知" in blocked.stdout
            and read_run(root).get("stage") == 1,
            "missing 未知 stops at the gate",
            blocked.stdout,
        )
        expect(
            "我理解你要的是" in blocked.stdout and "可能理解错" in blocked.stdout,
            "gate also demands the understanding lines",
            blocked.stdout,
        )
        with open(os.path.join(folder, "decision.md"), "a", encoding="utf-8") as fh:
            fh.write("\n- 未知：无\n")
        still = run(root, "next")
        expect(
            "GATE" in still.stdout and "我理解你要的是" in still.stdout,
            "understanding lines alone do not slip past the gate",
            still.stdout,
        )
        with open(os.path.join(folder, "decision.md"), "a", encoding="utf-8") as fh:
            fh.write("- 我理解你要的是：把导出默认集改成三态，不再追问\n")
            fh.write("- 可能理解错的地方：无歧义\n")
            fh.write("- 怎样算满足：跑 next 返回 DO\n")
        nxt = run(root, "next")
        expect(
            "DO" in nxt.stdout and "RAW %s" % raw in nxt.stdout,
            "complete intake advances and echoes the raw request",
            nxt.stdout,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_restated_raw():
    """「我理解你要的是」照抄原话时给一句 NOTE，但不拦。

    判断权在用户：写得对不对靠人看，不靠脚本算。这里只抓最低级的偷懒。
    """
    root = tempfile.mkdtemp(prefix="dw-restate-")
    try:
        git_init(root)
        raw = "把导出的默认集改成三态，别再问我根据项目规则"
        run(root, "start", "change", FEATURE, raw)
        folder = os.path.join(root, "specs", FEATURE)
        with open(os.path.join(folder, "decision.md"), "w", encoding="utf-8") as fh:
            fh.write(
                "- 原话（一字不改）：%s\n"
                "- 我理解你要的是：%s\n"
                "- 可能理解错的地方：无歧义\n"
                "- 本质：改默认集\n"
                "- 怎样算满足：跑 next 返回 DO\n"
                "- 未知：无\n" % (raw, raw)
            )
        copied = run(root, "next")
        expect(
            "NOTE" in copied.stdout and "原话的复制" in copied.stdout,
            "copying the raw line raises a note",
            copied.stdout,
        )
        expect(
            "DO" in copied.stdout,
            "the note does not block progress",
            copied.stdout,
        )
        with open(os.path.join(folder, "decision.md"), "w", encoding="utf-8") as fh:
            fh.write(
                "- 原话（一字不改）：%s\n"
                "- 我理解你要的是：导出默认集固定三态，不再交互追问\n"
                "- 可能理解错的地方：三态具体是哪三个尚未确认\n"
                "- 本质：改默认集\n"
                "- 未知：无\n" % raw
            )
        own = run(root, "next")
        expect(
            "NOTE" not in own.stdout and "DO" in own.stdout,
            "rephrasing in own words raises no note",
            own.stdout,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_domains():
    """domains 配置键真的有读取方。"""
    root = tempfile.mkdtemp(prefix="dw-domains-")
    try:
        git_init(root)
        write_config(root, 'domains: "采集任务,质检管理"\n')
        proc = subprocess.run(
            [BASH, os.path.join(SKILL_ROOT, "scripts", "paths.sh")],
            cwd=root,
            capture_output=True,
            text=True,
            encoding="utf-8",
            env=clean_env(),
        )
        expect(
            "DOMAINS='采集任务,质检管理'" in proc.stdout,
            "paths.sh exposes the configured domains",
            proc.stdout + proc.stderr,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_domain_docs():
    """仓库内域文档：paths.sh 找得到，drift 能判级。"""
    root = tempfile.mkdtemp(prefix="dw-domain-")
    try:
        git_init(root)
        with open(os.path.join(root, "README.md"), "w", encoding="utf-8") as fh:
            fh.write("probe\n")
        os.makedirs(os.path.join(root, "docs", "domain"))
        git_commit_all(root)
        head = subprocess.run(
            ["git", "-C", root, "rev-parse", "HEAD"],
            capture_output=True,
            text=True,
            encoding="utf-8",
        ).stdout.strip()
        with open(os.path.join(root, "docs", "domain", "order.md"), "w", encoding="utf-8") as fh:
            fh.write(
                "---\nid: domain-order\nobject: 订单\nverified_commit: %s\n"
                "watches:\n  - README.md\n---\n\n# 订单\n" % head
            )
        git_commit_all(root, "doc")
        paths = subprocess.run(
            [BASH, os.path.join(SKILL_ROOT, "scripts", "paths.sh")],
            cwd=root,
            capture_output=True,
            text=True,
            encoding="utf-8",
            env=clean_env(),
        )
        drift = subprocess.run(
            [BASH, os.path.join(SKILL_ROOT, "scripts", "knowledge_drift.sh")],
            cwd=root,
            capture_output=True,
            text=True,
            encoding="utf-8",
            env=clean_env(),
        )
        expect(
            "DOMAIN_DOCS='" in paths.stdout and "docs/domain" in paths.stdout,
            "paths.sh exposes the repo domain docs",
            paths.stdout + paths.stderr,
        )
        expect(
            "FRESH" in drift.stdout and "order.md" in drift.stdout,
            "drift grades the repo domain doc",
            drift.stdout + drift.stderr,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_approve():
    """放行行只由 approve 写；手写行在 gate_strict 下不作数。"""
    root = tempfile.mkdtemp(prefix="dw-approve-")
    try:
        git_init(root)
        os.makedirs(os.path.join(root, "api"))
        write_config(root, 'protected_paths: "api/**"\ngate_strict: "true"\n')
        run(root, "start", "change", FEATURE)
        folder = os.path.join(root, "specs", FEATURE)
        with open(os.path.join(folder, "decision.md"), "w", encoding="utf-8") as fh:
            fh.write("- 原话（一字不改）：探测用\n- 我理解你要的是：改一处\n- 可能理解错的地方：无歧义\n- 本质：改一处\n- 怎样算满足：跑 next 返回 DO\n- 未知：无\n")
        blocked = run(root, "approve", FEATURE, "design", "api")
        expect(
            "NEED_ANSWER" in blocked.stdout,
            "approve refuses without an answered question",
            blocked.stdout,
        )
        with open(os.path.join(root, "specs", "passes.log"), "w", encoding="utf-8") as fh:
            fh.write("design %s api\n" % FEATURE)
        with open(os.path.join(folder, "plan.md"), "w", encoding="utf-8") as fh:
            fh.write("plan\n")
        with open(os.path.join(folder, "gate"), "w", encoding="utf-8") as fh:
            fh.write("plan pass\n")
        expect(
            gate_permission(GATE, root, "api/a.proto") == "deny",
            "hand-written pass line is ignored under gate_strict",
            "",
        )
        run(root, "wait", FEATURE, "可以把 api/ 放行吗？", "可以")
        run(root, "answer", FEATURE, "可以")
        approved = run(root, "approve", FEATURE, "design", "api")
        expect(
            "APPROVED" in approved.stdout
            and gate_permission(GATE, root, "api/a.proto") == "allow",
            "approve after a real reply unlocks the write",
            approved.stdout,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_gate_check():
    """gate_check 跑命令，退出码非 0 就停。"""
    root = tempfile.mkdtemp(prefix="dw-gatecheck-")
    try:
        git_init(root)
        write_config(root, 'verify: "true"\n')
        run(root, "start", "change", FEATURE)
        folder = os.path.join(root, "specs", FEATURE)
        with open(os.path.join(folder, "decision.md"), "w", encoding="utf-8") as fh:
            fh.write("- 原话（一字不改）：探测用\n- 我理解你要的是：改一处\n- 可能理解错的地方：无歧义\n- 本质：改一处\n- 怎样算满足：跑 next 返回 DO\n- 未知：无\n")
        run(root, "done", FEATURE)
        git_commit_all(root)
        blocked = run(root, "done", FEATURE)
        expect(
            "GATE" in blocked.stdout
            and "CHECK" in blocked.stdout
            and read_run(root).get("stage") == 2,
            "gate_check blocks a clean working tree",
            blocked.stdout,
        )
        with open(os.path.join(root, "app.txt"), "w", encoding="utf-8") as fh:
            fh.write("x\n")
        passed = run(root, "done", FEATURE)
        expect(
            passed.returncode == 0 and read_run(root).get("stage") == 3,
            "gate_check passes when the tree changed",
            passed.stdout,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_evidence_log():
    """推进时落一行证据（含 head）；HEAD 变了 next 要提醒。"""
    root = tempfile.mkdtemp(prefix="dw-evidence-")
    try:
        git_init(root)
        write_config(root, 'verify: "true"\n')
        git_commit_all(root)
        run(root, "start", "change", FEATURE)
        folder = os.path.join(root, "specs", FEATURE)
        with open(os.path.join(folder, "decision.md"), "w", encoding="utf-8") as fh:
            fh.write("- 原话（一字不改）：探测用\n- 我理解你要的是：改一处\n- 可能理解错的地方：无歧义\n- 本质：改一处\n- 怎样算满足：跑 next 返回 DO\n- 未知：无\n")
        run(root, "next")
        log = open(os.path.join(folder, "evidence.log"), encoding="utf-8").read()
        expect(
            "stage=1" in log and "head=" in log and "by=runner" in log,
            "advancing appends an evidence line",
            log,
        )
        git_commit_all(root)
        with open(os.path.join(root, "later.txt"), "w", encoding="utf-8") as fh:
            fh.write("x\n")
        git_commit_all(root, "later")
        nxt = run(root, "next")
        expect(
            "NOTE 工作树已变" in nxt.stdout,
            "head drift shows up as a note",
            nxt.stdout,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_decided():
    """拍板过的话由 next 带出来（不另存一份）。"""
    root = tempfile.mkdtemp(prefix="dw-decided-")
    try:
        git_init(root)
        write_config(root, 'verify: "true"\n')
        run(root, "start", "change", FEATURE)
        folder = os.path.join(root, "specs", FEATURE)
        with open(os.path.join(folder, "decision.md"), "w", encoding="utf-8") as fh:
            fh.write("- 原话（一字不改）：探测\n- 我理解你要的是：改一处\n- 可能理解错的地方：无歧义\n- 本质：改一处\n- 怎样算满足：跑 next 返回 DO\n- 未知：无\n- 待拍板：范围多大？\n")
        run(root, "next")
        run(root, "wait", FEATURE, "用全量还是三态？", "只改判定那一行")
        run(root, "answer", FEATURE, "只改判定那一行")
        nxt = run(root, "next")
        expect(
            "DECIDED 只改判定那一行" in nxt.stdout,
            "next echoes the last decision",
            nxt.stdout,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)


def case_docs_match_reality():
    """文档里的脚本清单必须跟实现一致。

    断言数量这类数字靠人记必失；但用「内层跑一遍 verify」来自检会递归、
    而且内外层并发会互相干扰。所以只检查稳定的东西：脚本清单。
    断言数量交给 README 一句话说明，不写死具体数字。
    """
    scripts_md = open(
        os.path.join(REPO_ROOT, "docs", "scripts.md"), encoding="utf-8"
    ).read()
    script_dir = os.path.join(SKILL_ROOT, "scripts")
    missing = [
        name
        for name in sorted(os.listdir(script_dir))
        if not name.startswith(".") and name not in scripts_md
    ]
    expect(
        not missing,
        "docs/scripts.md lists every skill script (missing: %s)" % missing,
        "",
    )
    # competitive.md 不应写死断言数量——那个数每加一条测试就过期
    comp = open(
        os.path.join(REPO_ROOT, "docs", "competitive.md"), encoding="utf-8"
    ).read()
    expect(
        not re.search(r"verify\.sh\s+#\s*\d+\s*条", comp),
        "competitive.md does not hardcode the assertion count",
        "",
    )


def case_readme_matches_requires():
    """README 里的流程示例必须与 change.json 的 requires 一致。

    这类漂移最容易发生：改了 requires 忘了改 README，新人照 README 写产物
    就会被 GATE 拦住，而且不知道是文档错还是自己错。实战里真的撞到过。
    """
    wf = json.load(
        open(
            os.path.join(SKILL_ROOT, "workflows", "change.json"), encoding="utf-8"
        )
    )
    req = wf["stages"][0].get("requires") or []
    readme = open(os.path.join(REPO_ROOT, "README.md"), encoding="utf-8").read()
    # 第一屏可能在 README 里跨行，逐行收集以「# 写 decision.md 第一屏」开头的注释
    first_screen = []
    for line in readme.splitlines():
        stripped = line[line.find("#"):] if "#" in line else ""
        if "写 decision.md 第一屏" in stripped or first_screen:
            if stripped.startswith("#"):
                first_screen.append(stripped)
            elif first_screen:
                break
    block_text = "\n".join(first_screen)
    expect(
        bool(block_text),
        "README documents the decision.md first screen",
        "",
    )
    text = block_text
    missing = [k for k in req if k not in text]
    expect(
        not missing,
        "README first-screen matches change.json requires (missing: %s)" % missing,
        text,
    )


def case_intent_confirm_wired():
    """两个工作流的第 1 步都要求理解确认，且模板措辞与 requires 逐字一致。

    措辞不一致 = 字面量门永远匹配不上。这条把它钉住。
    """
    pairs = [
        ("change.json", "decision-template.md"),
        ("feature.json", "spec-template.md"),
    ]
    lines = ["我理解你要的是", "可能理解错"]
    for wf_name, tpl_name in pairs:
        wf_path = os.path.join(SKILL_ROOT, "workflows", wf_name)
        wf = json.load(open(wf_path, encoding="utf-8"))
        first = wf["stages"][0]
        req = first.get("requires") or []
        for line in lines:
            expect(
                line in req,
                "%s stage 1 requires %s" % (wf_name, line),
                json.dumps(req, ensure_ascii=False),
            )
        tpl = open(
            os.path.join(SKILL_ROOT, "templates", tpl_name), encoding="utf-8"
        ).read()
        for line in lines:
            expect(
                line in tpl,
                "%s contains %s" % (tpl_name, line),
                "",
            )
    # 模板里不能有“待拍板”以外的裸关键字干扰门；这里只查原话行还在
    for _, tpl_name in pairs:
        tpl = open(
            os.path.join(SKILL_ROOT, "templates", tpl_name), encoding="utf-8"
        ).read()
        expect("原话（一字不改）" in tpl, "%s keeps the raw line" % tpl_name, "")


def case_skill_routing():
    """阶段→文件只留一份（next）；SKILL.md 不再维护那张表。"""
    text = open(os.path.join(SKILL_ROOT, "SKILL.md"), encoding="utf-8").read()
    expect(
        "| 需求分析 |" not in text
        and "| 业务梳理 |" in text
        and "| 画图 |" in text,
        "SKILL.md keeps only the four non-workflow entries",
        text,
    )


def case_card_writeback():
    """卡片回写：写下去的东西能被脚本验证，而不是只看起来像。

    对应人工样例 #5：卡片出现、verified_commit 等于 HEAD、检索面可 grep、drift 报 FRESH。
    """
    root = tempfile.mkdtemp(prefix="dw-card-")
    cards = tempfile.mkdtemp(prefix="dw-cards-")
    try:
        git_init(root)
        with open(os.path.join(root, "README.md"), "w", encoding="utf-8") as fh:
            fh.write("probe\n")
        git_commit_all(root)
        head = subprocess.run(
            ["git", "-C", root, "rev-parse", "HEAD"],
            capture_output=True, text=True, encoding="utf-8",
        ).stdout.strip()
        env = clean_env()
        env["DATA_WORKFLOW_CARDS_DIR"] = cards
        # 模拟按 capture.md 写的一张卡
        body = (
            "---\n"
            "id: support-20260101-mqtt-false\n"
            "title: MQTT \u63a8\u9001\u88ab\u8bef\u8ba4\u4e3a\u4e0d\u5b58\u5728\n"
            "kind: fact\n"
            "status: verified-static\n"
            "domains: [job]\n"
            "object: job\n"
            "symptoms: [mqtt, \u6ca1\u6536\u5230\u901a\u77e5, push]\n"
            "source_paths: [internal/biz/job_dispatch_notification.go]\n"
            "invalidates_when: \u8be5\u6587\u4ef6\u88ab\u5220\u9664\u6216\u91cd\u547d\u540d\n"
            "verified_branch: main\n"
            "verified_commit: %s\n"
            "verified_at: 2026-01-01T00:00:00+08:00\n"
            "verification: static\n"
            "---\n\n# MQTT\n\u7684\u786e\u6709\u63a8\u9001\u3002\n" % head
        )
        path = os.path.join(cards, "2026-01-01-mqtt.md")
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(body)
        text = open(path, encoding="utf-8").read()
        expect(
            ("verified_commit: %s" % head) in text,
            "card pins verified_commit to current HEAD",
            text,
        )
        for field in ("source_paths", "invalidates_when", "symptoms"):
            expect(
                re.search(r"^%s:\s*\S" % field, text, re.M) is not None,
                "card has non-empty %s" % field,
                text,
            )
        hit = subprocess.run(
            ["grep", "-rl", "mqtt", cards],
            capture_output=True, text=True, encoding="utf-8",
        )
        expect(
            hit.returncode == 0 and "2026-01-01-mqtt.md" in hit.stdout,
            "card is findable by symptom keyword",
            hit.stdout,
        )
        drift = subprocess.run(
            [BASH, os.path.join(SKILL_ROOT, "scripts", "knowledge_drift.sh"), path],
            cwd=root, capture_output=True, text=True, encoding="utf-8", env=env,
        )
        expect(
            "FRESH" in drift.stdout,
            "drift grades the freshly written card as FRESH",
            drift.stdout + drift.stderr,
        )
    finally:
        shutil.rmtree(root, ignore_errors=True)
        shutil.rmtree(cards, ignore_errors=True)


def case_lazy_load():
    """常驻成本：SKILL.md 短，references 不被无条件加载。

    对应人工样例 #6。只要有人把内容往 SKILL.md 里塞，或者让阶段正文互相
    import 全部 reference，这条会碎。
    """
    skill = open(os.path.join(SKILL_ROOT, "SKILL.md"), encoding="utf-8").read()
    lines = len(skill.splitlines())
    expect(
        lines <= 200,
        "SKILL.md stays under 200 lines (got %d)" % lines,
        "",
    )
    # SKILL.md 里不能出现「无条件读全部 references」这类指令
    expect(
        not re.search(r"references/\*", skill),
        "SKILL.md does not glob-load references",
        "",
    )
    # description 是常驻成本，不许回流成长段规则
    m = re.search(r"^description:\s*(.*)$", skill, re.M)
    desc = m.group(1) if m else ""
    expect(
        len(desc) <= 260,
        "description stays short (got %d chars)" % len(desc),
        desc,
    )


def case_evals_are_runnable():
    """evals.json 里只许放能被脚本判定的用例。

    判不了的（要读 agent 自然语言回答的）应该去 docs/review-cases.md。
    把它们塞回这里，就会变成一份没人执行的文档——而 README 又把它当质量证明。
    """
    path = os.path.join(REPO_ROOT, "evals", "evals.json")
    data = json.load(open(path, encoding="utf-8"))
    evals = data.get("evals") or []
    expect(
        bool(evals),
        "evals.json is not empty",
        "",
    )
    expect(
        data.get("policy", {}).get("executed_by"),
        "evals.json declares who executes it",
        "",
    )
    missing = [
        e.get("id") for e in evals if not (e.get("machine_case") or "").strip()
    ]
    expect(
        not missing,
        "every eval names the check_runner case that covers it (missing: %s)" % missing,
        "",
    )
    src = open(
        os.path.join(SKILL_ROOT, "scripts", "check_runner.py"), encoding="utf-8"
    ).read()
    unbacked = []
    for e in evals:
        names = re.findall(r"\bcase_[a-z_]+", e.get("machine_case") or "")
        for n in names:
            if "def %s(" % n not in src:
                unbacked.append(n)
    expect(
        not unbacked,
        "every named case actually exists in check_runner.py (missing: %s)" % unbacked,
        "",
    )
    # 人工用例必须有落点，不能凭空消失
    expect(
        os.path.isfile(os.path.join(REPO_ROOT, "docs", "review-cases.md")),
        "human review cases live in docs/review-cases.md",
        "",
    )


def case_open_marker():
    """待拍板只认行首标记；正文里提到不算。"""
    sys.path.insert(0, os.path.join(SKILL_ROOT, "scripts"))
    import workflow_next as w

    cases = [
        ("正文提到就不算:\n- 现有能力：`open_decision` 检「待拍板」标记后打 OPEN", False),
        ("待拍板", True),
        ("- 待拍板", True),
        ("- 待拍板：用全量还是三态？", True),
        ("> 待拍板", True),
        ("# 决定\n\n- 原话：改一处\n- 待拍板\n", True),
        ("- 待拍板\n已拍板：用全量\n", False),
        ("- 待拍板", True),
        ("没有这个标记\n", False),
    ]
    for text, want in cases:
        got = w.file_is_open(text)
        expect(
            got == want,
            "file_is_open(%r) should be %s" % (text[:30], want),
            "got %s" % got,
        )
    # 回复关闭后不能再被同一行打开
    text = "- 待拍板\n已拍板：A\n- 待拍板\n已拍板：B\n"
    expect(
        w.file_is_open(text) is False,
        "closed markers do not reopen the file",
        text,
    )


def case_evidence_first():
    """查证义务必须在 scope.md 里，且带可判定边界与反例。

    这条防的是：把能自己查的事实反问用户。一旦删掉，来回拉抷的成本就回来了。
    """
    scope = open(
        os.path.join(SKILL_ROOT, "references", "scope.md"), encoding="utf-8"
    ).read()
    expect(
        "查证义务" in scope,
        "scope.md keeps the evidence-first section",
        "",
    )
    # 边界必须可判定：列出「本机可查」与「该问人」两类，而不是一句劝告
    for needle in ("本机文件", "业务口径", "卡在哪", "谁去补"):
        expect(
            needle in scope,
            "scope.md names the boundary marker %r" % needle,
            "",
        )
    expect(
        "不通过的样子" in scope,
        "scope.md carries a failing example",
        "",
    )
    agents = open(os.path.join(REPO_ROOT, "AGENTS.md"), encoding="utf-8").read()
    expect(
        "先查完再给方案" in agents,
        "AGENTS.md points at the evidence-first rule",
        "",
    )


def main():
    case_none()
    case_npm()
    case_make()
    case_gomod()
    case_install_makes_tool_dir()
    case_verify_wins()
    case_pnpm()
    case_yarn()
    case_pytest()
    case_cargo()
    case_answer()
    case_hook_quiet()
    case_gate_read_write()
    case_gate_tool_case()
    case_gate_strict()
    case_gate_no_python()
    case_utf8_output()
    case_no_phantom()
    case_intake()
    case_restated_raw()
    case_domains()
    case_domain_docs()
    case_approve()
    case_gate_check()
    case_evidence_log()
    case_decided()
    case_skill_routing()
    case_intent_confirm_wired()
    case_readme_matches_requires()
    case_docs_match_reality()
    case_open_marker()
    case_evidence_first()
    case_card_writeback()
    case_lazy_load()
    case_evals_are_runnable()
    case_foreign()
    case_default_no_hooks()
    case_record_follows_install()
    print("OK")


if __name__ == "__main__":
    main()
