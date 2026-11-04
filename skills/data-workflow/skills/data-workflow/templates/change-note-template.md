# <主题> 变更说明（change note）

> 产出者：`stages/git-commit/SKILL.md`（提交前的文档审计门产物）。落 `specs/<feature>/change-note.md`。
> 用途：让下一个人（和下一个 agent）不必读 diff 就知道**这次改了什么、为什么、代价是什么、文档同步了没有**。
> 形状：`references/doc-shape.md`。主活是速查。第一屏是 §1 一句话，加上文档同步了没有。
> 规则：只写会被 diff 之外的东西证伪的内容。不复述代码。

- feature：`<name>` ／ 分支：`<branch>` ／ commit 范围：`<base>..<head>`
- 日期：`YYYY-MM-DD`

## 1 一句话

这次改动做了什么（动词开头，不带形容词）。

## 2 为什么

需求出处（spec 路径 / 工单 / 用户原话），以及**为什么现有能力不够用**（复用门禁的结论）。

## 3 改了什么

| 层 | 文件 | 变更 |
|---|---|---|
| proto | | |
| service | | |
| biz | | |
| data | | |
| 配置 / 迁移 | | |
| 测试 | | |

## 4 契约影响

`scripts/contract_diff.sh <base>` 的结论：`OK` / `BREAKING ×N`。有 BREAKING 时逐行写裁决：

| 类别 | 对象 | 谁需要跟着改 | 已通知 | 可接受理由 |
|---|---|---|---|---|

## 5 验证记录

| 门 | 命令 | 结果 |
|---|---|---|
| 构建 | | PASS / FAIL |
| 单测 | | |
| 契约 ① `go test ./api/...` | | |
| 契约 ② `contract_diff.sh` | | |
| 接口 ③ 场景回放 | | PASS / **WAIVED（缺什么、影响哪些端点、谁补跑）** |
| 审核三轴 | | PASS / CONCERNS / FAIL |

> **GATE：WAIVED 必须写清补跑条件。空着 = 谎报通过。**

## 6 文档审计（提交前门）

对照 `git diff --name-only <base>..HEAD` 逐条核对：

| 文档 | 是否需要同步 | 状态 | 备注 |
|---|---|---|---|
| `specs/<feature>/spec.md` | | 已同步 / 不适用 | |
| `specs/<feature>/plan.md` | | | |
| `specs/<feature>/impact.md` | | | |
| `README.md`（仓库根） | | | |
| `CONTEXT.md`（术语） | | | |
| 知识层 `$KNOW/<域>/`、`$CARDS/` | | | 归档见 `references/feature-archive.md` |
| 仓库 `docs/` | | | 降级区：**不为它改口径**，只在自己产出里标注它已失效 |

> **GATE：有「需要同步但没同步」的项时，不能直接提交。**
> 要么补文档，要么在下面「接受的文档风险」里显式记账（写清风险内容与补的计划），二选一，不许沉默。

### 接受的文档风险

| 未同步项 | 为什么现在不补 | 谁在什么时候补 |
|---|---|---|

## 7 已知代价与欠债

| 债 | 在哪 | 怎么还 | 何时还 |
|---|---|---|---|

## 8 回滚

见 `specs/<feature>/rollback.md`，此处只写一行：可否只回滚代码（是/否）、数据是否可逆（是/否）。
