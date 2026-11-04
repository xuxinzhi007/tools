# <主题> 技术方案（plan）

> 产出者：`stages/tech-design/SKILL.md`。落 `specs/<feature>/plan.md`。
> **实例取值守卫**：§2 的 `internal/{service,biz,data}` 是 Go/Kratos 实例（`stack: "go"`）的取值。未声明 `stack: go` 的项目按 `../../profiles/go-kratos/profile.md` 换成本项目的分层。
> 前置：`specs/<feature>/spec.md` 的九项评估无阻塞项、决定表已拍板。
> 形状：`references/doc-shape.md`。主活是怎么做。第一屏是 §1 接口和 §3 的 3–6 步。
> 规则：**本文件不重开需求分析**。公司模板（`$TECH_DESIGN_TEMPLATE`）存在时以公司模板为准，本文件只补工程细节；公司模板缺失时才用它，并在开头注明「未使用公司模板」。

- feature：`<name>` ／ 分支：`<branch>` ／ 基线：`<sha>`
- 一页方案：`<路径>`（评审正文是它，本文件是落地细节）

## 1 接口变更（先写这一节）

| proto 文件 | rpc | 变更 | 兼容性 |
|---|---|---|---|
| `api/<module>/<version>/*.proto` | | 新增 / 加字段 / 改语义 | 非 breaking / breaking（谁跟着改） |

> **GATE：改动 proto 必须写「谁需要跟着改」；`scripts/contract_diff.sh` 报 BREAKING 而本节没有对应裁决 → 方案不完整。**
> 字段先从 proto 定义，不从 service/data 倒推。

## 2 数据变更

| 表 | 列 | 类型 | 索引 | 迁移方式 | 可逆 |
|---|---|---|---|---|---|

> **GATE：新增查询必须写走哪个索引；不可逆迁移必须写备份与回滚脚本。**

## 3 后端流程（service → biz → data，3–6 步）

1. `internal/service/<file>.go:<Method>` —— 校验与协议适配，不含业务规则。
2. `internal/biz/<file>.go:<Method>` —— 规则、编排、事务边界、副作用。
3. `internal/data/<file>.go:<Method>` —— 持久化、筛选、排序、分页、聚合。

> 跨层依赖 = 0；业务规则不进 SQL；能复用就写「复用 `<文件:符号>`」。

## 4 状态与枚举

新增/修改的状态属于哪一族、写入方是谁、**读取方消费点清单**（switch / 白名单 / SQL 过滤 / 展示映射 / 资格条件）逐项列出，引用 `specs/<feature>/impact.md` 轴 2。

> **GATE：新增枚举值而没有全库消费点追踪 → 方案不完整。**

## 5 权限与租户

权限码（贴 proto option 原文）、数据范围复用哪个 `apply*Scope`、是否有跨租户分支、租户缺失时的行为与错误码。

## 6 验证（DoD，Go 栈，仅当 stack=go）

| 门 | 命令 | 期望输出 |
|---|---|---|
| 构建 | `make build` / `go build ./...` | 无错误 |
| 单测 | `go test ./internal/...` | PASS |
| 契约 ① | `go test ./api/...` | PASS |
| 契约 ② | `bash "$PLUGIN_ROOT/scripts/contract_diff.sh" <base>` | OK 或逐行裁决 |
| 接口 ③ | `bash scripts/test-api.sh -u <base>` | PASS 或如实 WAIVED |

> **GATE：DoD 里没有可执行命令 = 没有 DoD。**

## 7 发布与回滚

发布顺序（先后端/前端/配置）、开关名与默认值、回滚步骤（编号）、数据回滚可行性。详见 `specs/<feature>/rollback.md`。

## 8 已知代价与欠债

| 债 | 在哪 | 怎么还 | 何时还 |
|---|---|---|---|

> **GATE：欠债三要素缺一即阻塞。**

## 9 风险

| 风险 | 触发条件 | 影响面 | 缓解 |
|---|---|---|---|
