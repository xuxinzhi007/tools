# 竞品对照：这套技能在同类里处于什么位置

核对方式与边界先说清楚，否则下面每一格都会被读成"我觉得"：

- **本仓自己的能力**：可复现。每一条都能用 `bash scripts/verify.sh`（45+ 条回归）或 `specs/*/check.md` 里的实测输出核对，标 **已证实**。
- **同类技能包**：来自本仓 `docs/external-skill-packs.md`（2026-10-28 的记录，钉了上游 commit：`mattpocock/skills@59db609`、`addyosmani/agent-skills@aeac41d`（插件 0.6.13）、`obra/superpowers@2ce8aa1`（v7.0.1））。那一份是逐仓库读过的；本文对它的引用标 **已记录**。
- **没有钉 commit 的部分**（Spec Kit、BMAD、GSD、Anthropic 官方 skill 规范等）：标 **推断**，只写结构性判断，不写具体文件与数量。
- **本文档随时可能过期**：上游在动。过期后就按 `docs/agent-pain.md` 的说法——"过期后要漏得响"，请重新读上游而不是信这份表。

---

## 1 比什么：只看两件事

文风、抽象程度、命名好不好听都不进表。只比两件会直接影响 agent 干活的：

1. **能不能被验证**：说"做完了"这件事，有没有一条命令的退出码，或者一个能被另一个会话核对的文件。
2. **跨会话会不会散**：长任务被压缩、换人接手、换 IDE，决定和进度还在不在。

其余能力（需求对齐、代码规范、评审清单）不是这套的强项，不进核心对比，只在 §4 里点名。

---

## 2 矩阵

`●` 有且可执行 · `◐` 有但是纯约定（靠模型自觉） · `○` 没有

| 能力 | 本仓 | Superpowers | Matt Pocock | Addy Osmani | Spec Kit / BMAD（推断） |
|---|---|---|---|---|---|
| 运行状态机（记录"停在第几步"） | ● `run.json`，`next` 只打一件事 | ◐ `progress.md` 台账，约定式 | ○ 无账本 | ◐ 阶段清单 | ◐ 任务清单，无恢复语义 |
| 未完成任务跨会话续跑 | ● sessionStart 注入 NOTICE + `next` 门 | ◐ 靠人读台账 | ○ | ○ | ○ |
| 编码步骤过门要退出码 | ● `done` 跑仓库声明的 `verify`，非 0 停 `RED` | ● `task-done` 要测试成功 | ◐ 靠 tdd 技能自觉 | ◐ 靠 `/test` | ◐ 靠阶段自述 |
| 产物内容门（不是文件名门） | ● `requires` 字面量检查（本轮补） | ○ | ○ | ○ | ◐ 模板占位符 |
| 写入拦截（改文件前拒绝） | ● Cursor hook + `passes.log` | ○ | ○ | ○ | ○ |
| hook 坏了自己的降级策略 | ● fail-open，`gate_strict` 才能转严格 | — | — | — | — |
| 证据分级强制（已证实/推断/待确认/冲突） | ● `scope.md` + `UNVERIFIED` 不许当已知条件 | ○ | ◐ 术语与 ADR 冲突提示 | ○ | ○ |
| 知识新鲜度门禁 | ● `knowledge_drift.sh`（FRESH/STALE/SUPERSEDED/CONFLICT） | ○ | ○ | ○ | ○ |
| 知识落点在仓库内、随代码提交 | ○ **缺**（`$KNOW` 在仓外，`specs/` 被 gitignore） | ◐ 计划在仓库 | ● `GLOSSARY.md` + `docs/adr/` | ○ | ◐ |
| 需求对齐（拷问式提问） | ◐ `scope.md` 五问 + `wait` 机制 | ◐ brainstorming | ● `/grill-me`、`/grill-with-docs` | ◐ `/spec` | ◐ |
| 多 agent 编排纪律 | ● `subagent-protocol.md`：取证可并行、裁决不可并行、fix loop ≤2 | ● `subagent-driven-development`（实现类串行） | ◐ 无 | ◐ 四类评审角色 | ○ |
| 单源分发到多个 IDE | ● `install.sh` 一份源 → 5 个工具目录，幂等，可 `--unlink` | ◐ 官方插件 | ● 插件 27 个技能 | ◐ 插件 | ○ |
| 跨平台（Windows / 无 rsync / 无 python3） | ● 本轮修完并有回归 | ? 未记录 | ? 未记录 | ? 未记录 | ? 未记录 |
| 生态规模（技能数、文档量） | ○ 1 个技能 / 45 个 md | ● 15 个 | ● 27 个 | ● 25 个 | ● 完整方法论 |

> `?` 不是我判断它不行，是**没有证据**。上一轮我犯过一次同类错误（按记忆写符号名），这里不重犯。

---

## 3 结论：三个真的差异点，三个真不如人的地方

### 真的差异点（别人没有同类机制，且我们可自证）

**① 「做完」的下界是可执行退出码，不是一段说明。**
`done` → 该仓库声明的 `verify` → `specs/<f>/verify.log` 里第一行是 `exit N`。没有入口就停在 `RED`，不许自己写"通过"。同类里只有 Superpowers 的 `task-done` 是同级别的；其余是"技能让模型记得跑测试"。

**② 门禁有明确的失效策略，而不是"要么全拦要么全不拦"。**
写入前按 `tool_name` 只拦写工具；读一律放行；hook 崩了/没有解释器时 fail-open 并说明；项目写 `gate_strict: "true"` 才转严格。这类"自己的故障模式"在同类技能包里我没有找到对应设计（推断：多数技能包不管 hook 层）。

**③ 知识是分级的：口径可信、取值必须回代码。**
`已证实 / 推断 / 待确认 / 存在冲突` 四态 + `UNVERIFIED` 不许在方案里当已知条件 + `knowledge_drift.sh` 判新鲜度。Matt 的 GLOSSARY/ADR 更擅长"术语和决定"，但没有"这条知识什么时候过期"的机制。

### 真不如人的地方

**① 知识不落仓库，所以业务分析不会越用越好。**
我们把业务事实放 `$KNOW`（个人 vault，仓外），把分析产物写 `specs/` 并 gitignore。Matt 的 `GLOSSARY.md` + `docs/adr/` 在仓库里、随代码评审。这是本轮**没有**解决的（P2）。结果是：**我们能证明"这次改对了"，不能证明"下次会更快"。**

**② 门是字面量门，不是语义门。**
本轮加的 `requires` 只能检查"产物里有没有出现『未知』这两个字"，不能检查"未知是不是写对了"。Superpowers 的 brainstorming 与 Matt 的 grilling 在"把需求问清楚"这件事上比我们的 `scope.md` 五问更强。

**③ 生态规模差一到两个数量级。**
1 个技能 vs 15–27 个。我们的 `feature` 链只服务 Go/Kratos 实例，通用性靠 `change` 三步兜。别人是全生命周期覆盖。

---

## 4 什么场景该用这套，什么场景别用

| 场景 | 建议 |
|---|---|
| 在一个**已有业务仓库**里改一处、要能说清"为什么这么改、验证了什么" | 用这套（`change` 三步 + `verify.log` + `check.md`） |
| 高风险改动（契约 / 迁移 / 权限 / 状态机） | 用这套的 `feature` 链，但要接受它是 Go/Kratos 实例模板 |
| 想让人和 agent 对齐术语、做架构决定 | 别用这套做主入口，用 `/grill-with-docs` + `GLOSSARY.md` + `docs/adr/` |
| 想让 agent 长时间无人值守跑一批任务 | 这套不行（`subagent-protocol.md` 明确禁止并行实现）；用 Superpowers 的 worktree + 子代理 |
| 新项目从零做产品 | 这套不适用（没有需求探索、没有 UI） |

一句话：**这套的定位是"改动的证据链"，不是"开发全流程"。** 它管的是"凭什么说这次改对了"，不管"该不该做、做成什么样"。

---

## 5 这两轮补了哪几格（可复核）

| 矩阵格 | 之前 | 现在 | 证据 |
|---|---|---|---|
| 写入拦截 | ◐ 有脚本但默认不拦、读也被拦 | ● 只拦写、读放行 | `specs/hook-hardening/check.md` §2 |
| hook 降级策略 | ○ 崩了就拦死全部写入 | ● fail-open + `gate_strict` | 同上 §3 |
| 跨平台 | ○ Windows 上 hook 无输出、`verify` 跑不起来 | ● 解释器探测 + bash 壳 + 无 rsync 安装 | `specs/hook-hardening/verify.log` |
| 产物内容门 | ○ 文件名门 | ● `requires`（本轮） | `specs/evidence-gates/check.md` |
| 未完成任务续跑 | ● 已有 | ● 加 `raw_request`，原话进记录 | `specs/evidence-gates/check.md` |
| 知识落点在仓库 | ○ 缺 | ○ **仍缺**（P2） | 未做，别当成已做 |

复核命令：

```bash
bash scripts/verify.sh                        # 回归集，末行打印 OK
bash skills/data-workflow/hooks/session-start # 会话注入
```
