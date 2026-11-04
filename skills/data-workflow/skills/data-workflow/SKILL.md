---
name: data-workflow
description: 用户说 @data-workflow、开始开发、架构设计、梳理业务、这个词什么意思、需求分析、影响面、技术方案、编码实现、改动审核、审一下、质量检查、接口测试、排查、状态含义、提交、提 MR、画架构图时读本文件。它是选型入口：只决定读哪一个阶段文件；业务事实运行时从知识层读，产物落 specs/<feature>/。不用于：纯 git 操作、解决合并冲突。
---

# 开发工作台（data-workflow）

本文件只做选型。具体步骤在点名的那一个 `stages/` 文件里。业务事实运行时从知识层读。安装后工具目录里只有这一个 `data-workflow`。装进别的仓库后，那个仓库里的代理读这份副本。

`$PLUGIN_ROOT` 是本文件所在目录。命令表在 `references/commands.md`。Agent 输入框输入 `/`，选 `dw-next`、`dw-done`、`dw-wait`、`dw-start`。

Go/Kratos 实例的取值（分层目录、`api/**`、状态族、九项负债、契约门）集中在 `profiles/go-kratos/profile.md`；本文件与阶段正文不写实例取值，只在需要时指过去。

会话开头解析一次路径。空值就问，或显式声明降级后继续。不猜绝对路径。

```bash
eval "$(bash "$PLUGIN_ROOT/scripts/paths.sh")"
```

## 绑定

Cursor 项目安装后，没有 `specs/passes.log` 里覆盖该路径的记录，hook 拒绝写入。放行记录**只由 `workflow_next.py approve` 写**（要求已有一次拍板回话），格式与判定在 `references/edit-gate.md`；`gate_strict` 开着时手写的行不作数。`protected_paths` 和 `waiver_globs` 都没写时，**默认没有受保护路径**（技能不猜任何项目的核心文件）；data-service 实例通过它的 `config.yaml` 注入门禁路径。有未完成运行时，会话钩子注入一条 NOTICE：运行名、状态、一行摘要。NOTICE 不代替当前需求。用户要继续那条时，再跑 `next`，只做它打印的那一件。这次注入依赖 Cursor 加载了 `.cursor/hooks.json` 里的 `sessionStart`。新会话没有看到 NOTICE 时，先确认这份钩子文件被加载。

## 继续一次运行

未完成的运行在 `specs/<feature>/run.json`。产物目录就是当前工作仓库根——在哪个仓库 install、在哪干活，记录就落在哪里，不回到技能源仓库，也不写进别人的工作树。`install.sh` 已把 `specs/` 写进该仓库的 `.gitignore`。会话注入是 NOTICE 时，先做当前这句话。用户要继续那条，或那条和当前需求是同一件事时，再跑 `next`。没有注入，或输出是 `LAST` / `NO_RUN` 时，回答当前的问题。`next` 打出的 NEED、WAIT、DO、OPEN 只约束正在继续的那条运行。

| 看到 | 做 |
|---|---|
| `READ` / `NEED` | 只读打印出来的那一个阶段文件，把缺的文件写上 |
| `OPEN` | `wait <feature> "<问题>" "<对方能原样粘贴的一句>"`，然后停止 |
| `WAIT` | 把 `REPLY` 交给对方。对方原样回了再 `answer`。对不上就停 |
| `DO` | 做完再 `done <feature>`。这一步跑该仓库自己的测试入口，退出码不是 0 就停 |
| `DONE` | 这次运行结束。证实或推翻了一条口径时，按 capture.md 写一张卡 |
| `LAST` | 这次没有未完成步骤。回答当前的问题 |

`start feature|bugfix|hotfix|review-only|change <feature>` 新开一次。任意仓库的一次改动或定架构用 `change`（形状写进 `decision.md`）。data-service（Go 栈）实例的 L3 变更——落在其 `api/`、proto、pb.go、`internal/biz/status.go`，或涉及状态机、租户权限、回滚——不用 `change`，用 `feature` 或 `hotfix`。

这轮不做了就 `abandon <feature>`。状态变成 `abandoned` 之后，钩子不再注入。不按天数、也不按 HEAD 变化自动放弃。停在 `working` 的运行下次仍会注入，直到 `abandon`。编码步骤的退出码是完成信号的下界。状态和契约（data-service 实例是 proto）的语义是否正确，看 `plan pass`、契约 diff 和审核。

## 选工作流

用户这句话已经点明某个阶段（例：「帮我写个技术方案」）：没开运行就先 `start`，然后按 `next` 打印的 `READ` 读那一个阶段文件；已有运行直接 `next`。用户只说开始干活：先读 `references/scope.md`，按 L1 / L2 / L3 定这一次的范围，再选工作流。不先弹出九步菜单。

进入 feature / bugfix / hotfix（data-service Go 栈实例）之前，先读 `references/workflows.md`；之后每一步读什么由 `next` 打印的 `READ` 决定。

| 场景 | 工作流 |
|---|---|
| data-service（Go 栈）新能力 / 扩展 / 新字段 / 新端点 | `workflows/feature.json` |
| data-service 已知错误行为要修 | `workflows/bugfix.json` |
| data-service 线上正在出事 | `workflows/hotfix.json` |
| 只审不改 | `workflows/review-only.json` |
| 任意仓库的一次改动，或给任意仓库定架构 | `workflows/change.json` |

新业务场景是新的工作流模板，不是再装一个技能。

只梳理、不改代码时，不 `start`。读 `stages/oncall/references/routing.md`：先 grep `$CARDS/`，再读相关的一个域文件，取值回代码。证据指向另一个域时再读那一个。`status: superseded` 的卡跳过。关键结论标已证实、推断、待确认或存在冲突。没有出处的词写成待确认，然后 `wait`。不影响方案的细节不清楚，写明假设后继续。用户要留下这条时，再读 `stages/oncall/references/capture.md` 写一张卡。不先写一篇领域百科。

动手前读 `references/scope.md`。先写用户真正要解决的问题和怎样算满足，再选方案。已有 `文件:符号` 能做到就复用。L1 走 `change`。碰到契约、持久化、权限、状态或回滚，即使用户把它叫成小改动，也走 `feature` 或 `hotfix`。与这次无关的债不进这次 diff。

梳理和需求判断发给用户的第一份，按 `references/doc-shape.md` 的结论层：要不要改、现在能不能改、下一步。依据和实施细节等用户追问再写。

## 阶段文件从哪来

阶段 → 文件的映射**不在本文件维护**，三份描述会漂：`next` 会打印 `READ <文件>`，按它读那一个。没开运行时先 `start`，别照着记忆选文件。

不经任何工作流的只有四个入口（用户点名时才用）：

| 入口 | 读 | 产出 |
|---|---|---|
| 业务梳理 | `stages/oncall/references/routing.md`（维度见 `references/business-model.md`） | 待确认，或一张卡，或 `docs/domain/<域>.md` |
| 排查确认 | `stages/oncall/SKILL.md` | 只读结论 |
| 提交 | `stages/git-commit/SKILL.md` | `specs/<feature>/change-note.md` |
| 画图 | `stages/excalidraw-diagram/SKILL.md` | 图 |

一次只读一个文件。`$KNOW` 一次只加载一个域文件。

点名才读的全局技能：`grilling`、`diagnosing-bugs`、`ponytail-review`、`security-best-practices`、`security-threat-model`、`gh-fix-ci`、`gh-address-comments`、`domain-modeling`。阶段文件按路径读，不用技能名去调用。

写 spec、impact、plan、rollback、review、change-note 之前读 `references/doc-shape.md`。归档见 `references/feature-archive.md`。

## 三条硬规则

1. 答案与产出里引用读过的文件，不复述知识层。
2. 一次只加载一个业务域文件。
3. 阶段是本技能内的文件。全局技能仍按名字调用。
