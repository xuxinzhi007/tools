# 命令

Agent 输入框里输入 `/`，选 `dw-next`、`dw-done`、`dw-wait`、`dw-start`。`install.sh` 把这四个文件写到**当前项目**的 `.cursor/commands/`，默认安装也一样——技能副本在 `$HOME`，命令在项目里，可提交给团队共享入口。

脚本只有一份，相对技能根。不要直接执行 `.py`：Windows 上 shebang 指向的 `/usr/bin/python3` 常常不存在，要先用探测脚本拿一个真能跑的解释器：

```bash
. "$PLUGIN_ROOT/scripts/find_python.sh"
py="$(dw_python)"          # → py -3 / python3 / python / 绝对路径
$py "$PLUGIN_ROOT/scripts/workflow_next.py" <子命令>
```

| 子命令 | 何时 | 成功时看到 |
|---|---|---|
| `next` | 用户要继续那条未完成运行 | 只一件：`NEED`、`GATE`、`WAIT`、`DO`、`OPEN` 或 `DONE` |
| `next` | 没有未完成运行 | `NO_RUN`，或 `LAST` 加记录路径 |
| `start <workflow> <feature>` | 新开。workflow 为 feature、bugfix、hotfix、review-only、change | 停在第一阶段缺的文件 |
| `wait <feature> "<问题>" "<一句回复>"` | `OPEN`，或还要问一句 | `run.json` 里有这句 reply |
| `answer <feature> <回复原文>` | 对方回的就是 reply | 仍开着的产物末尾有 `已拍板：` |
| `done <feature>` | `DO` 做完 | 编码步骤先过 `gate_check`（工作树真动过），再跑该仓库的测试入口。任一不过就停；通过后往 `specs/<f>/evidence.log` 追一行（含 `head`）。退出码 0 只说明命令通过 |
| 证据行 | 任何一次推进 | `stage=<id> requires=ok gate_check=ok\|none verify=exit0\|none head=<sha> at=<iso> by=runner`。只由 runner 写；手写的行没有 `by=runner`。`head` 与当前 HEAD 不一致时 `next` 会打一行 `NOTE`（不阻塞） |
| `next` 的 `DECIDED` | 这条运行拍过板 | 最近一条 `已拍板：`，从产物里读（不另存一份）。新会话继续一条旧运行时靠它拿回“为什么这么定” |
| `approve <feature> <design|waiver> <前缀> [原因]` | 要放行受保护路径 | `specs/passes.log` 多一行带 `approved` 的记录。没有已答复的拍板时打 `NEED_ANSWER`，不写 |
| `abandon <feature>` | 这轮不做了 | `status` 成为 `abandoned`。`next` 把它当已结束，钩子不再注入 |
