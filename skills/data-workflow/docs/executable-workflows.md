# 怎么把工作流改成能跑的

核对日期：2026-10-28。跑起来的入口是 `skills/data-workflow/scripts/workflow_next.py`。它不写 spec，只打印下一步。

我自己用这套东西时，真正卡住的是：问了一个选择，收尾却是「根据项目规则进行调整」，下一句没有落在文件里。飞书上的 [AI技能栈](https://x2-robot.feishu.cn/wiki/QleIw3bx7ip0tAk4OWacJ50LnUb) 写的是日常自己选下一步，不要两套总路由一起接活。所以这里不把 Matt、pstack、Osmani 再包进来。data-service 的七段阶段表继续只服务那个仓库；任意仓库用 `workflows/change.json`。

上游是 `obra/superpowers` `2ce8aa15`（v7.0.1）和 `mattpocock/skills` `59db609e`。两边都是模型在推进。本仓库借的是「检查通过才往下」，不搬 worktree，也不并行派实现子代理。

## 现在有什么

四份工作流是给模型读的状态机，没有程序按它往下走：

| 文件 | 类型 | 会不会改业务代码 |
|---|---|---|
| `workflows/feature.json` | 执行 | 会，从「编码实现」起 |
| `workflows/bugfix.json` | 执行 | 会，从「回归测试先行」起 |
| `workflows/hotfix.json` | 执行 | 会，从「最小修复」起 |
| `workflows/review-only.json` | 审阅 | 不会 |

`SKILL.md` 步骤 3 让模型自己读 JSON，自己决定进哪一阶段。门禁是一段中文。没有「当前停在第几步」的文件。hook 只认 `specs/passes.log`、`plan.md` 和 `gate` 里的 `plan pass`，不认工作流走到了哪一步。

官方任务导出那一轮就是这样停的：需求分析按说明停下来问三态还是全量，但没有把「你回这一句」写成下一步。人看不到该接什么。

上游的执行也是模型在推进，脚本只做单次记录：

- Superpowers 先把计划写成 `docs/superpowers/plans/YYYY-MM-DD-<feature>.md`，等人选定 `executing-plans`（同一会话逐项做）或 `subagent-driven-development`（同一会话当调度，一次只派一个实现子代理，再派一个审查；技能写明不要并行派多个实现子代理）。进度账本在 `<repo>/.superpowers/sdd/<plan>/progress.md`。`task-done` 只有测试命令成功才追加 `Task N: complete`。中途只为四件事停下：不可逆操作、安全敏感操作、工作树外的副作用（合并、推共享分支、发布）、计划坏到每条路都是猜。
- Matt 的 `implement` 在当前分支做完一件事：调 `tdd`，收尾调 `code-review`，然后提交。没有账本，也不取下一张票。`implement-spec` 让模型自己读票的阻塞关系，算出当前能做的前沿，再为每张票开 worktree。没有脚本负责选前沿或合并。

本仓库要借的是账本加「检查通过才记完成」。不搬 worktree，也不并行派实现子代理。

## 后续要分成两类

执行类：允许改业务代码。进入第一段会改代码的阶段之前，`specs/<feature>/` 里要有已经写明的决定。feature、bugfix、hotfix 属于这一类。

审阅类：不改业务代码，不提交。`review-only` 属于这一类。hook 对这类运行拒绝产品代码写入。

通用项目和 data-service 共用一个跑法，不共用同一份阶段表。

- 任意项目：`workflows/change.json`，三步。决定写入 `specs/<feature>/decision.md`，再改文件，再写 `check.md`。要换掉默认的 api、proto、status.go 门禁，在该项目 `config.yaml` 写上 `protected_paths` 和 `waiver_globs`。两个键都不写时，默认门禁还在。写空值才放开。
- data-service：继续用现在的 feature / bugfix / hotfix / review-only。九项负债和 proto 门只在这份配置被点名时加载。

## 跑起来的脚本

`skills/data-workflow/scripts/workflow_next.py`。它不实现业务，只回答「下一步读哪个文件，或者停下来问哪一句」。

每次只看一份运行记录 `specs/<feature>/run.json`：

```json
{
  "workflow": "feature",
  "stage": 1,
  "status": "waiting",
  "question": "未传状态继续三态，还是改为全部状态？",
  "reply": "未传状态继续三态，不改默认集。"
}
```

脚本的规则：

1. 当前阶段要求的产物文件不在，就打印该阶段的 `file`，让模型只读那一个阶段文件。
2. 产物在，但门还开着（`run.json` 的 `status` 是 `waiting`），就打印 `question` 和可原样回的 `reply`，不进入下一阶段。
3. 人已经按 `reply` 回答，或门对应的文件已经出现，`stage` 加一。
4. 执行类在第一段会改代码的阶段之前，检查决定文件。没有就不打印实现阶段。
5. 审阅类的任何阶段都不打印实现阶段。

hook 继续拦没有放行记录的写入。放行记录要和 `run.json` 对得上：执行类改受保护路径时，`design` 行里的 feature 必须是当前这次运行。

会话开始只通知还有未完成运行。用户要继续它时，再跑 `next`，只做它打印的那一步。

这样官方任务导出停住时，记录里会留下那句推荐回复。人回那一句，脚本才放行后面的修改。

## 现在怎么跑

`. {脚本根}/scripts/find_python.sh` 拿一个真能跑的解释器，再 `$py skills/data-workflow/scripts/workflow_next.py next`（不要直接执行 `.py`：Windows 上 shebang 不可用）。产物路径取工作流里已经写过的 `artifacts`：像路径的才检查，像「代码改动」这种句子不检查。`code: true` 的阶段要 `done` 才往下。没有可判定产物、也不是改代码的阶段，打印 `NO_ARTIFACT` 并要求 `done`，**不编一个 `stage-<id>.md`**。文件里还有「待拍板」就打印 `OPEN`，不进入下一阶段。

会话开始会跑一次 `next`，注入的是 NOTICE，不是那段操作指令。做完的运行留在 `specs/<feature>/run.json`，不注入。用户要继续时再跑 `next`，才会看到 `WAIT` 或 `NEED`。技能仓库对自己安装时，hook 指向 `skills/data-workflow/hooks/`，不另拷一份。

`config.yaml` 没写 `protected_paths` 时，hook 不拦任何路径（技能不猜哪个文件是核心文件）。data-service 实例在它的 `config.yaml` 里声明 `api/**`、`*.proto`、`*.pb.go`、`internal/biz/status.go` 才启用拦截。**读不受影响**：受保护路径上的 `read_file` / `grep` 一律放行。

不在这一步做的：把阶段正文改成空模板、并行子代理、git worktree、用脚本代替模型去写 spec。
