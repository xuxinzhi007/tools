# 安装

在目标仓库根执行。data-service 和其他仓库是同一条命令。

```bash
bash ~/skills/data-workflow/scripts/install.sh                      # 默认：技能进 $HOME
bash ~/skills/data-workflow/scripts/install.sh --project            # 副本也进项目
bash ~/skills/data-workflow/scripts/install.sh --project --with-hooks  # 加上强制门禁
bash ~/skills/data-workflow/scripts/install.sh --project --unlink   # 撤掉
```

## 三种落点，能力不同

先看清代价再选。差别不在技能，在门禁。

| 模式 | 项目里新增 | 技能能力 | 强制门禁 |
|---|---|---|---|
| 默认（`$HOME`） | 4 个 `/` 命令 | 完整 | 无 |
| `--project` | 4 个命令 + 整包副本 | 完整 | 无 |
| `--project --with-hooks` | 4 个命令 + 副本 + `hooks.json` + `.gitignore` 块 | 完整 | Cursor 里生效 |

**技能能力在三种模式里都是完整的。** 只差门禁。副本装在 `$HOME` 和装在项目里，`SKILL.md`、`stages/`、`workflows/`、`scripts/` 一模一样。

## 为什么默认不装进项目

`merge_cursor_hooks` 往 `.cursor/hooks.json` 写的是相对项目根的命令，带 `bash` 前缀：

```
bash .cursor/skills/data-workflow/hooks/gate-edit
```

这条是硬绑定，没有回退。如果副本被 `.gitignore` 忽略、而 `hooks.json` 被提交，队友 clone 下来会拿到一个指向不存在文件的 hook。它现在不会把仓库变成只读：`failClosed` 不写 `true`，hook 跑不起来就等于没有门禁。

所以只有两个自洽的选择：

- 副本提交 → 才写 hooks（`--project --with-hooks`）
- 副本不提交 → 不写 hooks（默认 / `--project`）

没有第三种。脚本不会替你制造那个坏门。

## 默认模式

技能装到 `$HOME/.cursor/skills/data-workflow`（工具目录已存在就装进去，不存在就建这个）。项目里只写 `.cursor/commands/dw-*.md` 四个文件，可以提交给团队共享入口。

改动技能正文后，重新跑默认安装即可更新 `$HOME` 里的副本，不动项目。

## `--project --with-hooks`

副本装进项目的 `.<工具>/skills/data-workflow/`，**要提交**。同时写两个东西：

- `.cursor/hooks.json`：`sessionStart` 注入 NOTICE，`preToolUse` / `beforeShellExecution` 走 `gate-edit`
- `.gitignore` 里一个带标记的块：

```gitignore
# >>> data-workflow >>>
# 本机路径，不随仓库提交。技能副本其余部分可以提交。
.*/skills/data-workflow/config.yaml
.*/skills/data-workflow/origin
# <<< data-workflow <<<
```

`config.yaml` 里是各人的绝对路径（比如 Obsidian vault），`origin` 是本机技能仓库路径。这两个不该跟着提交，其余副本文件该提交。

## 运行记录去哪了

开一次工作流（`start change <feature>`）会产生 `specs/<feature>/` 下的 `run.json`、`decision.md`、`check.md`、`verify.log`。

**这些记录写在你当前干活的仓库里，不回到技能源仓库。** 在哪个仓库 install、在哪干活，记录就落在哪里。默认模式（技能在 `$HOME`）也一样——记录照样进当前仓库的 `specs/`。

安装时会把 `specs/` 写进该仓库 `.gitignore` 的标记块，所以它**不会出现在 `git status`**，也不进版本库。想让团队看到进度，删掉那一行就行。

想换个落点，在该仓库的 `config.yaml` 里设 `artifacts_root`。

这个块用标记圈起来，`--unlink` 时整块撤掉，**不碰 `.gitignore` 里你自己的其他行**。

## `--global`

同默认。显式写出来只是为了让意图清楚。

## `--unlink`

撤掉本脚本写过的东西：`hooks.json` 里的条目、4 个命令、`.gitignore` 标记块、项目内副本。保留 `config.yaml`。

## 其它行为

脚本只改已经存在的 `.qoder`、`.claude`、`.cursor`、`.codex`、`.agents`，不替你新建（默认模式的 `$HOME` 除外）。再跑一次更新正文，不覆盖已有的 `config.yaml`。

在本技能仓库里执行时，不另拷一份技能。`.cursor/hooks.json` 指向 `skills/data-workflow/hooks/`。

Qoder、Claude、Codex 的副本没有 `gate-edit` 那道拦截，那些环境里门禁只在技能被读到之后生效。**门禁本来就只有一个工具真的强制。**

有未完成运行时，`sessionStart` 注入一条 NOTICE：运行名、状态、一行摘要。它不代替当前需求。用户要继续那条时再跑 `next`。

Agent 输入框输入 `/`，可选 `dw-next`、`dw-done`、`dw-wait`、`dw-start`。

## 这个仓库带来什么

装完之后真正在起作用的是三件事，每件都能自己验：

| 能力 | 怎么验 |
|---|---|
| 没跑检查就声称做完时被拦住 | `verify` 退出码不是 0，`workflow_next.py next` 不进下一步，打印 RED |
| 未完成的运行不会因为换了话题就消失 | 新会话拿到 `NOTICE <feature> <status>`，`next` 仍打到原来那一步 |
| 一个词的含义回代码取，不从记忆编 | `resolve.sh knowledge_root` 空时打印 `MISSING`，技能转去问用户，不猜 |

第二件是分水岭。普通的对话助手在长会话里丢状态；这个把状态写在 `specs/<feature>/run.json`，会话开始时读回来。

## 故障排查

新会话没看到 NOTICE：

1. 看 `.cursor/hooks.json` 是否存在、是否被 Cursor 加载
2. 看它指向的脚本是否真的在：`ls .cursor/skills/data-workflow/hooks/`
3. 若脚本缺失而 hook 存在——这就是上面说的坏门，跑 `--unlink` 然后改用默认模式

写入被无故拒绝：先看有没有开 `gate_strict`（项目 `config.yaml`）。不开时 hook 坏了自己让路，不会拦写入；开了才会把「解释器缺失 / 坏输入 / 脚本崩溃」都当拒绝。
