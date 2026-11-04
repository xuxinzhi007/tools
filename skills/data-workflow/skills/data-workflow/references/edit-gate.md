# 编辑门禁

Cursor 项目安装会把 hook 写进该仓库的 `.cursor/hooks.json`。会话开始就注入本技能，不等触发词。`preToolUse` 和 `beforeShellExecution` 在写入前查产物目录里的 `specs/passes.log`。产物目录一律是**当前工作仓库的仓库根**——谁 install 了技能，记录就跟着谁，不回到技能源仓库。install.sh 会把这个 `specs/` 写进 `.gitignore`。

`--global` 不写这道 hook。Qoder、Claude、Codex 的副本没有这道拦截，那些环境里门禁仍只在技能被读到之后生效。

## 只拦写，不拦读

hook 按 `tool_name` 判断这次调用会不会写文件：`write`、`edit_file`、`multi_edit`、`apply_patch`、`delete_file` 一类才进门禁；`read_file`、`grep`、`list_dir` 和没见过的工具名一律放行。

理由：业务分析要读的正是 `api/**`、`*.proto`、`status.go` 这些受保护路径。把读也当写拦，会让排查和需求分析一起哑掉。没见过的工具名选择放行而不是拒绝——把「新工具悄悄被绕过」记在文档里，比让模型每次看到一条看不懂的拒绝理由再重试要好。

`apply_patch` 的目标路径在补丁正文里（`*** Update File: <path>`），脚本从正文抠。

## 坏了让路：fail-open 与 `gate_strict`

默认 fail-open：脚本出错、输入不是合法 JSON、找不到可用的 python 时，hook 输出合法 JSON 并 `permission: allow`，同时附一句说明。

理由：门禁的价值是拦「已知危险的写」，不是拦「hook 自己崩了」。旧版靠 Cursor 的 `failClosed: true` 兜底，结果是 Windows 上 `python3` 解析到 Microsoft Store 的占位程序（exit 49）→ hook 没有 stdout → 所有写入被拒，连诊断都做不了。

要恢复严格，在项目 `config.yaml` 写：

```yaml
gate_strict: "true"
```

此时坏输入一律拒绝。**注意代价**：解释器缺失、hook 崩溃也会变成拒绝写入。

## 入口是壳，不是 python 文件

`hooks/gate-edit`（POSIX / git-bash）与 `hooks/gate-edit.cmd`（原生 Windows）都是壳：先找一台机器上真正能跑的 python，再把 stdin 交给 `hooks/gate_edit.py`。探测逻辑在 `scripts/find_python.sh`：`$DATA_WORKFLOW_PYTHON` > `py -3` > `python3` > `python` > PATH 目录里的 `python.exe`，每个候选都实跑一次 `-c "import sys"`。

`install.sh` 写进 `.cursor/hooks.json` 的命令因此带 `bash` 前缀，且不写 `failClosed: true`。代价：团队里有人没装 bash 或 python 时，门禁静默失效而不是拦死。这是有意的取舍，兜底仍是提交前看 diff。

hook 看得见的是写入工具的目标路径（`path`、`file_path`、`filePath`），以及命令文本里带有路径的写入（重定向、`tee`、`sed -i`、`rm`、`mv`、`cp`）。从补丁文件间接写入时，命令文本里没有目标路径，hook 看不到。

shell 这条是尽力而为。挡不住 `python -c` 里打开文件、变量拼出来的路径、heredoc，以及 `&&` 或管道后面的 `sed -i`。提交前和 CI 里的 diff 才是兜底。

`failClosed` 由本技能自己控制：`.cursor/hooks.json` 不写 `failClosed: true`，严格度看 `gate_strict`。并发调用偶尔会拿到空 stdout，界面只说 hook 没有输出；这不是拒绝。

## 谁要记录

默认（两个键都未配置时）**没有受保护路径**，`specs/` 之外的写入直接放行——技能不猜任何项目的核心文件。data-service 实例通过它的 `config.yaml` 声明受保护路径：

| 写入 | 放行 |
|---|---|
| `specs/` 下的工件，含 `passes.log` 本身 | 直接写 |
| data-service 声明的 `protected_paths`（如 `api/`、`*.proto`、`*.pb.go`、`internal/biz/status.go`） | `design` 行，且该 feature 的方案门已落盘 |
| data-service 声明的 `waiver_globs`（如其它 `*.go`） | `waiver` 行，或覆盖该文件的 `design` 行 |
| 其余文件 | 直接写 |

调用方在别的仓库里，这道 hook 拦不到。

## 记录格式

`specs/passes.log`，一行一条，`#` 开头是注释。**只由 `workflow_next.py approve` 写**，不要手写：手写的行在 `gate_strict` 下不作数。

```text
design <feature> <前缀> approved <时间> "<那次拍板的回复>"
waiver <前缀> <原因> approved <时间> "<那次拍板的回复>"
```

为什么要带 `approved`：放行等于「有人同意改这个文件」。如果写代码的手可以自己追加一行就放行，那道门就只是“写一行字”。`approve` 要求 `run.json` 里已经有 `last_reply`（先 `wait`，对方回那句，再 `answer`），所以一行放行背后至少有一次真实的拍板对话。

```bash
# 先记下要问的一句，停下来等对方回原句
workflow_next.py wait <feature> "可以把 api/ 放行吗？" "可以"
workflow_next.py answer <feature> "可以"
workflow_next.py approve <feature> design api
workflow_next.py approve <feature> waiver internal/biz 只改一处判空
```

`<feature>` 只含字母、数字、`.`、`_`、`-`，对应目录 `specs/<feature>/`。前缀是相对仓库根的文件或目录，不含空格。`api/order` 覆盖 `api/order/a.proto`，不覆盖 `api/order2`。

`design` 生效还要同时有：

- `specs/<feature>/plan.md` 非空
- `specs/<feature>/gate` 第一条非空行是 `plan pass`

`plan pass` 只在技术方案门过了之后写。hook 只认这个文件，不重新给九项负债打分。

内部小改动用 `waiver`，原因写在同一行前缀后面。先 `approve` 这一行，再改受保护文件。

**`gate_strict` 下的差别**：严格模式只认末尾带 `approved` 的行；审计模式（默认）兼容旧格式，但仍然建议只走 `approve`。

## 项目自己的路径

`config.yaml` 里可以写两个键，逗号分隔，相对仓库根（示例是 data-service 实例的取值）：

```yaml
protected_paths: "api/**,*.proto,*.pb.go,internal/biz/status.go,*/biz/status.go"
waiver_globs: "*.go"
```

写了就用写的。`protected_paths` 为空表示没有路径要 `design`。`waiver_globs` 为空表示没有路径要 `waiver`。两个键都不出现时，默认没有受保护路径（不拦），data-service 需要在它的 `config.yaml` 里写上表取值才启用门禁。

## 和一次运行对上

`specs/<feature>/run.json` 的 `status` 是 `working` 或 `waiting` 时：

- 工作流 `kind` 是 `review`：除 `specs/` 以外的写入都拒绝。
- 工作流 `kind` 是 `execute`：`design` 行里的 feature 必须是这次运行的名字。没有这份运行记录时，仍接受任意合法 feature。

下一步读哪个文件、要不要停下来问一句，由 `scripts/workflow_next.py` 打印。本文件只规定写入能不能发生。
