# Matt Pocock / Addy Osmani / Superpowers

核对日期：2026-10-28，对过各仓库默认分支：`mattpocock/skills` `59db609`，`addyosmani/agent-skills` `aeac41d`（插件 0.6.13），`obra/superpowers` `2ce8aa1`（插件 7.0.1）。技能名以目录里的 `SKILL.md` 为准。

- Matt Pocock Skills：<https://github.com/mattpocock/skills>
- Addy Osmani agent-skills：<https://github.com/addyosmani/agent-skills> ，站点 <https://skills.addy.ie>
- obra Superpowers：<https://github.com/obra/superpowers>

口语里的「Pstack」在 Superpowers 仓库里没有这个名字。下文用仓库名 Superpowers。

## 结论

日常写 Go / TS、人要留在回路里：用已经装好的 **Matt Pocock**。data-workflow 继续做数据服务这一条链（六道门禁、`specs/<feature>/`、契约 diff），不要把它改写成另外两套。

Addy 是一条完整生命周期路由，适合上线前要统一检查清单的时候再单独装。Superpowers 是设计确认后让子代理在 git worktree 里长时间自己跑，和本仓库「实现类禁止并行」是相反的机制，不并进 data-workflow。

三套不要同时常驻。description 会一起进上下文，路由会抢同一句话。

## 对照

| | Matt Pocock | Addy Osmani | Superpowers |
| --- | --- | --- | --- |
| 作者自己怎么说 | 小、可改、可组合。点名 GSD、BMAD、Spec-Kit 把流程收走、出了错难修 | 把高级工程师的流程、门禁、做法打成技能，覆盖开发各阶段 | 一套完整方法论，外加让 agent 一定会用这些技能的初始指令 |
| 今天仓库里有什么 | 插件发出 27 个：`engineering` 20 + `productivity` 7。另有 `in-progress` 7、`misc` 4 不进插件。`deprecated/` 是空的 | `skills/` 25 个。入口 9 个：`/spec` `/plan` `/build` `/test` `/constraints` `/review` `/webperf` `/code-simplify` `/ship` | `skills/` 15 个。没有单独的 setup 命令 |
| 谁在开车 | 你点名才会跑的技能，和 agent 自己可以选的技能，是两份名单。`ask-matt` 只负责告诉你该用哪个 | 你点阶段命令，命令再去配技能；做着 API / UI 时技能也会自己亮 | `using-superpowers` 要求先加载相关技能。设计文档你点头之前，`brainstorming` 不让写实现 |
| 强项 | 需求对齐（grill）、术语表和 ADR、红绿重构、分诊 | 阶段固定，安全、性能、发布、可观测都有对应技能。另有四个评审角色：`code-reviewer`、`test-engineer`、`security-auditor`、`web-performance-auditor` | 计划写到能交给初级工程师；`using-git-worktrees` 把改动留在隔离工作树；`subagent-driven-development` 可以连续做很久 |
| 不适合 | 想丢一句需求就自己跑完一整条流水线 | 小改动、个人仓库。规则多，单技能安装还带不走仓库级 `references/`（上游 issue 361） | 要中途改一行、查一个小 bug。链路长，工作树要自己收 |
| 本机 | 已在 `~/.agents/skills`，并链到 `~/.cursor/skills`，Cursor 新开会话能看到 | 未装 | 未装 |

## 本机已经能用的 Matt

这些目录是本机已有副本，`~/.cursor/skills/<name>` 指向 `~/.agents/skills/<name>`。新开一个 Cursor Agent 会话后能看到。

要你打出斜杠才会跑：`/ask-matt`、`/grill-with-docs`、`/grill-me`、`/triage`、`/improve-codebase-architecture`、`/setup-matt-pocock-skills`、`/to-spec`、`/to-tickets`、`/implement`、`/wayfinder`、`/handoff`、`/teach`、`/to-questionnaire`、`/wait-what`。上游还有 `/implement-spec`、`/retro`，本机副本里没有。

agent 自己也可以选上：`grilling`、`tdd`、`diagnosing-bugs`、`domain-modeling`、`codebase-design`、`code-review`、`prototype`、`research`、`wizard`、`writing-for-agents`。上游还有 `pr`，本机没有。你点名它们同样有效。

你点名才会跑的技能不去调用另一个同类技能。`/implement` 会用到 `tdd`，收尾用 `code-review`，这两个都在 agent 可选的那份名单里。

每个**业务仓库**第一次用工程技能前，在那个仓库里跑一次 `/setup-matt-pocock-skills`。它是一段提问，不是脚本。确认之后写入：

- `docs/agents/issue-tracker.md`：issue 放 GitHub、GitLab，还是本地 `.scratch/<feature>/`
- `docs/agents/domain.md`：领域文档放哪
- `docs/agents/triage-labels.md`：只有同时装了 `triage` 才写。默认标签 `needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`
- 已有的 `CLAUDE.md` 或 `AGENTS.md` 里加一节 `## Agent skills`；两个都没有就先问你建哪个

它不创建 `GLOSSARY.md`。术语表和 ADR 留给之后的 `/domain-modeling`。仓库里如果还留着旧的 `CONTEXT.md`，setup 可以提议 `git mv` 成 `GLOSSARY.md`。不要在 data-workflow 技能仓库里跑。不要反复跑。

`in-progress/` 和 `misc/` 没有装。本机 `~/.agents/skills/resolving-merge-conflicts` 不是这个仓库里的技能。

### 需求对齐

| 技能 | 什么时候用 |
| --- | --- |
| `grill-with-docs` | 要改代码、想法还没对齐。边问边写 ADR 和术语。长期项目用这个 |
| `grill-me` | 同样的追问，不写文档。非代码、一次性的方案 |
| `grilling` | 你说出 grill，或要压测一个决定。本仓库 data-workflow 的入口已经把「压测方案」指到这里 |
| `to-spec` | 对话里已经谈拢，只把共识收成 spec，写到该仓库配置的 issue tracker。它不负责再采访你 |
| `to-tickets` | 把 spec 拆成带阻塞边的垂直切片，写到 tracker |
| `wayfinder` | 一次会话装不下的大块工作。在 tracker 上铺决策票，一次解开一张 |
| `ask-matt` | 不知道该用上面哪一个 |

没有 `/to-prd` 这个名字，对应的是 `to-spec`。没有 `/diagnose`，对应的是 `diagnosing-bugs`。

### 领域和模块

| 技能 | 什么时候用 |
| --- | --- |
| `domain-modeling` | 写术语表、记 ADR、统一业务名词。文件位置以 `docs/agents/domain.md` 为准 |
| `codebase-design` | 设计模块接口、缝（测试边界）、依赖方向 |
| `improve-codebase-architecture` | 扫现有代码，出一份可点的架构机会报告，再针对一项继续 grill |
| `research` | 要查权威来源并落成仓库里的一篇 markdown |

`grill-with-docs` 会建立共享语言。Matt 仓库 README 里的旧例子还把词表叫做 `CONTEXT.md`；当前 setup 把这份约定记在 `docs/agents/domain.md`，新文件是术语表和 ADR。

### 实现和排障

| 技能 | 什么时候用 |
| --- | --- |
| `tdd` | 先写失败测试，再实现，再重构。一次一个切片 |
| `implement` | 已经有 spec 或一组 ticket，按它们实现 |
| `diagnosing-bugs` | 坏了、抛了、慢了。先复现再假设再验证。data-workflow 的「排查」也指到这个名字 |
| `triage` | issue / 外部 PR 走分诊状态机，写出 agent 能执行的 brief |
| `prototype` | 一次性原型，用来确认状态模型或界面感觉，不留成长期代码 |
| `wizard` | 只有人能做的步骤（控制台、密钥、CI secret），生成一个交互式 bash 向导 |
| `handoff` | 把当前会话压成另一位 agent 能接着读的交接 |

### 评审和其它

| 技能 | 什么时候用 |
| --- | --- |
| `code-review` | 从某个 commit / 分支 / merge-base 看起。一条轴对仓库里写明的规范，一条轴对当初的 spec |
| `teach` | 在当前仓库里讲一个概念 |
| `to-questionnaire` | 你答不全的决定，做成问卷给别人填 |
| `wait-what` | 上一句没接住，让它重说 |
| `writing-for-agents` | 改 SKILL.md、AGENTS.md、CLAUDE.md |

### 日常怎么串

小改动：`grill-me` → `tdd` → `code-review`。

要留下来的功能：`grill-with-docs` → `to-spec` → `to-tickets` → 每张票 `tdd`（或有 spec 之后 `implement`）→ `code-review`。

线上 bug：`diagnosing-bugs` → 根因说得清之后 `tdd` 补回归 → `code-review`。

跨很多天：`wayfinder` 先铺决策票，再对每一张走上面的短链。

这些技能写出来的术语表、ADR、`docs/agents/`、issue，和 data-workflow 的 `specs/<feature>/` 不是同一份东西。术语和「为什么这样选」留在 Matt 的文档里；契约、影响面、回滚、门禁留在 `specs/`。同一句话不要写进两处。

## Addy，需要时再装

九个命令对应 Define → Plan → Build → Verify → Review → Ship，中间多了 `/constraints`、`/webperf`、`/code-simplify`。没有 setup 技能。`/spec` 写 `SPEC.md`，`/plan` 写 `tasks/plan.md` 和 `tasks/todo.md`，`/constraints` 写根上的 `CONSTRAINTS.md`。`/webperf` 叫的是 `web-performance-auditor` 这个角色，不是一个同名技能。Claude Code 里如果和内置命令重名（例如 `/review`），用 `/agent-skills:review`。Gemini 里计划命令是 `/planning`，因为 `/plan` 和内置命令撞名。`/build auto` 是你批准计划之后，按任务自己做完，每任务仍是测试驱动并单独提交，失败或有风险的步骤会停。

Cursor 的装法是把 `skills/` 放进项目的 `.cursor/skills/`，短政策放 `.cursor/rules/`，不要把整份技能贴进 rules。见上游 `docs/cursor-setup.md`。

```bash
npx skills add addyosmani/agent-skills --list
npx skills add addyosmani/agent-skills --skill code-review-and-quality
```

先装单个技能看手感。整仓 25 个一起进 data-service，会和 data-workflow、Matt 抢「审一下 / 写个方案」这类话。

## Superpowers，需要时再装

15 个技能都会自己触发，仓库没有给它们单独的斜杠命令。固定顺序是 `brainstorming` → `using-git-worktrees` → `writing-plans` → `subagent-driven-development` 或 `executing-plans`，中间配 `test-driven-development`、`requesting-code-review`，结束用 `finishing-a-development-branch`。设计文档落在 `docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md`，你点头之前不写实现；点头后的下一步只是 `writing-plans`。你说开始之后，子代理按计划做，README 里写它可以连续做上一两个小时。

其余技能：`dispatching-parallel-agents`、`receiving-code-review`、`systematic-debugging`、`verification-before-completion`、`using-superpowers`、`writing-skills`、`diagnosing-superpowers`。会话里要查这套自己哪里走歪了，说 “figure out what went wrong with superpowers”。

Cursor 里：

```text
/add-plugin superpowers
```

用完看 `git worktree list`，不用的工作树删掉。它和 Matt、Addy 不要在同一个仓库常驻。

## 和 data-workflow 的关系

可以借的是 Matt 的形状：技能小、你来点名、通用工程动作不写进业务流水线。本仓库已经是这个形状——入口只有 `data-workflow`，阶段是它里面的文件，`grilling` 和 `diagnosing-bugs` 按名字调用，不复制进阶段目录。

不要做的三件事：

1. 把 Matt 那 20 个工程技能搬进 `stages/`。它们解决的是任何仓库的对齐、测试、评审；data-workflow 解决的是数据服务的知识漂移、影响面、proto 契约、六道门禁。
2. 把入口改成 Addy 的九段命令。`workflows/feature.json` 已经是这条业务的顺序，新场景加新 JSON，不加新技能。
3. 把实现改成 Superpowers 的并行子代理。本仓库写明实现类禁止并行，是因为同一批文件、共享工作区会互相覆盖。Superpowers 用独立 worktree 换并行，那是另一套隔离，不塞进现有门禁。

新业务仓库要 Matt：技能已经在用户级 `~/.cursor/skills`，进那个仓库跑一次 `/setup-matt-pocock-skills` 即可。要 data-workflow：在那个仓库执行本仓库的 `bash scripts/install.sh <项目根>`，配置留在安装副本里的 `config.yaml`。
