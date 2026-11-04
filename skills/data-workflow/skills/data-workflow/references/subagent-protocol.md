# 子 agent 协议：brief / report / ledger

什么时候派子 agent、怎么派、派完怎么收。**协议用文件承载，不用对话记忆承载** —— 上下文压缩（compaction）会吃掉对话，吃不掉落盘的文件。

---

## 1 fan-out 判据（先判该不该派）

派子 agent 只在同时满足这三条时才划算：

1. **2 个以上彼此独立的问题** —— 一个问题的答案不依赖另一个。
2. **无共享写状态** —— 不会两个子 agent 改同一份文件、同一个工作树。
3. **互不需要对方的上下文** —— A 的结论不是 B 的输入。

外加一条成本判据：子 agent 会重复加载一遍环境，**任务太小就别派**（一次 grep 能解决的事，派 agent 是纯亏）。

### 禁止并行的场景

| 场景 | 为什么 | 改成 |
|---|---|---|
| **实现类任务（写代码）** | 共享工作树，并行必然互相覆盖；且改动之间有语义依赖 | 串行，一次一个实现子 agent |
| 探索性 debug | 需要连续假设与全局状态，拆开就断线 | 主 agent 自己做，或全局技能 `diagnosing-bugs` |
| 相关联的失败（同一根因的多个症状） | 分开查会各自得出片面的结论 | 主 agent 合并查 |
| 事故期（hotfix） | 上下文共享成本高，需要单一决策者 | 主 agent 直接做 |
| 裁决类任务（判 breaking、判风险等级） | 责任不能分散给子 agent | 子 agent 只取证，主 agent 裁决 |

**取证可并行，裁决不可并行。** 这是本协议的核心一句。

---

## 2 三个文件（都落 `specs/<feature>/agents/`）

| 文件 | 谁写 | 内容 |
|---|---|---|
| `brief-<axis>.md` | 主 agent | 派活书：目标、边界、必须回答的问题、证据格式、禁止事项 |
| `report-<axis>.md` | 子 agent | 回信：结论、证据（`文件:行号` / 命令 + 输出摘录）、UNVERIFIED 清单 |
| `ledger.md` | 主 agent | 台账：每个子 agent 的状态（running / done / parked）、fix loop 轮次、最终裁决 |

为什么必须落盘：

- 主 agent 的上下文会被压缩，**台账是唯一可靠的状态源**。
- 子 agent 之间不共享上下文，`brief` 是它唯一知道的东西 —— 写不清就是它的错，不是它的错。
- 用户回来问「刚才那个结论怎么来的」，`report` 就是答案。

### brief 的四要素（缺一不可）

```markdown
# brief-<axis>
目标        一句话：要回答什么问题（不是「去看看代码」）
边界        只读哪些路径 / 不改任何东西 / 不评其他轴的事
必答        逐条列出必须回答的问题，每条要求什么形式的证据
禁止        不许猜符号名（引用前必须 grep 到）；不许下 breaking / 风险等级裁决；
            不许贴 token、真实租户 ID、客户数据
产出        写到 specs/<feature>/agents/report-<axis>.md，格式：<结论 / 证据 / UNVERIFIED>
```

**brief 里不许塞业务事实**（状态取值、字段含义）。子 agent 自己去 `$KNOW` 与代码里查 —— 塞给它就等于把主 agent 的偏见传下去，这正是 Meta 那篇《When Context Hurts》说的锚定效应。

---

## 3 三轴审核的标准派法

`stages/change-review/SKILL.md` 与 `workflows/review-only.json` 用这套：

1. 主 agent 先把 diff 落盘：`git diff <base>..HEAD > <artifacts_root>/diff.patch`（feature 链的 `artifacts_root` 是 `specs/<feature>/`；review-only 审别人的工作树时用该工作流写明的目录），记指纹（base sha / head sha / 文件数 / 行数）。
2. 写三份 brief：`spec-compliance`（是否符合 spec 与需求口径）、`code-quality`（规范与可维护性）、`interface-impact`（契约与调用方）。
3. 三个子 agent **并行**，都只读同一份 `diff.patch`，各写自己的 `report-*.md`。
4. 主 agent 读三份 report，合并成一份裁决（风险等级 + 逐条问题）。**合并这一步不外包。**

### freshness：审完就别再动工作树

review 结论绑定指纹。**被审 diff 变了（新 commit、新的未提交业务改动），指纹失效 → 结论 STALE → 必须重跑。** 写入评审工件不算。

产物里写一行：`review 指纹: base=<sha> head=<sha> files=N lines=M`。对不上就是过期结论，不许拿来当依据。

---

## 4 fix loop 上限

子 agent 报告问题 → 主 agent（或实现子 agent）修 → 重跑审核。**最多 2 轮。**

第 3 轮还没收敛，说明不是「再改一次」能解决的：

```markdown
## ledger.md 里记 parked
| 轴 | 问题 | 轮次 | 状态 | 裁决 |
|---|---|---|---|---|
| code-quality | <一句话> | 2 | parked | 主 agent 裁决：<接受并记账 / 阻塞并要求人工介入> |
```

`parked with ruling` = 停下、把问题原样交给用户、附上主 agent 的裁决建议。**不许无限重试**（烧 token 且结论会来回漂），也**不许静默放弃**（问题会跟着上线）。

---

## 5 台账格式

```markdown
# ledger <feature>
指纹     base=<sha> head=<sha> files=N lines=M
| 子 agent | brief | report | 状态 | 轮次 | 结论 |
|---|---|---|---|---|---|
| spec-compliance | brief-spec-compliance.md | report-spec-compliance.md | done | 1 | CONCERNS ×2 |
| code-quality    | ... | ... | parked | 2 | 见裁决 |
| interface-impact| ... | ... | done | 1 | PASS |
主 agent 裁决   <一段话：合并不合入、为什么、谁拍板>
```

---

## 6 自查

- 派之前是否过了 fan-out 三判据？有没有把「实现」或「裁决」并行出去？
- brief 四要素齐了吗？有没有把业务事实塞进 brief（应该让子 agent 自己查）？
- report 里每条结论都有证据吗？没证据的是否写成 UNVERIFIED？
- 台账是否反映了当前真实状态（compaction 之后只信台账）？
- fix loop 是否超了 2 轮？超了有没有 parked with ruling？
