# profile: go-kratos

> 一个 **profile** 是"某个技术栈的实例取值"。它只描述**这个栈**：分层目录、契约事实源、生成物、构建命令、专属门。
> 通用流程（`references/`、`workflows/`、`references/business-model.md`）不写实例取值——那些一次重命名就会变成假话。

## 什么时候生效

项目 `config.yaml` 写 `stack: "go"` 时。目前这个键只影响两处：

- `workflow_next.py start feature|bugfix|hotfix` 会提示（未声明时提示改走 `change`）。
- 本文件以及下表列出的实例文件是"给 Go 项目看的"，非 Go 项目等价于跳过。

**尚未生效**（欠账，写在文末）：阶段文件不会因为 `stack` 不是 go 就自动切分支，所以非 Go 项目读到本表的路径时要按「非 Go 替代」换掉。

## 这个 profile 拥有什么

| 类别 | 取值 | 落在哪 |
|---|---|---|
| 分层目录 | `internal/service`（校验/协议适配）、`internal/biz`（规则/编排）、`internal/data`（持久化） | `stages/requirement-analysis/SKILL.md`、`stages/change-review/references/standards-spec.md`、`templates/plan-template.md` |
| 契约事实源 | `api/**/*.proto`；生成物 `*.pb.go` / `*_http.pb.go` / `*_errors.pb.go` | `stages/impact-assessment/SKILL.md`、`references/edit-gate.md` |
| 契约门 | `go test ./api/...`（进程内 httptest）、`scripts/contract_diff.sh <base>` | `stages/interface-test/SKILL.md`、`stages/interface-test/references/contract-gates.md` |
| 状态族 | 状态常量集中在 `internal/biz/status.go`，按族隔离 | `stages/oncall/references/state-protocol.md` |
| 九项负债评估 | 维护性 / 可运营性 / 可信性 | `stages/requirement-analysis/SKILL.md`、`stages/requirement-analysis/references/debt-gate.md` |
| 复用门禁与风格 | 跨层依赖 = 0、重复逻辑必须复用、Go 细则 | `stages/implementation/references/{go-style,reuse-gate}.md` |
| 生成命令 | `make api` / `make api-one PROTO=...` | `stages/quality-gate/SKILL.md`、`stages/git-commit/SKILL.md` |
| 影响面检索配方 | 按 Go 目录分层的 grep | `stages/impact-assessment/references/blast-radius.md` |

## 非 Go 项目怎么换

| Go 实例取值 | 换成 |
|---|---|
| `internal/{service,biz,data}` | 该项目成文的工程约束（`.cursor/rules/` → `.qoder/rules/` → `AGENTS.md` → `CLAUDE.md`；都没有就明说"本仓无成文工程约束"）。定位方法见 `references/project-index.md` |
| `api/**/*.proto` | 该项目的契约事实源（openapi / 类型定义 / schema），以代码为准 |
| `go test ./api/...` + `contract_diff.sh` | 该仓库自己的接口测试入口与 `verify`；没有就写 `UNVERIFIED`，不许假装跑过 |
| `internal/biz/status.go` 状态族 | `business-model.md` 维度 2：先定族，再找该族的权威 const/枚举位置 |
| 九项负债 | **不套用**。用 `references/scope.md` 的分级与 `references/business-model.md` 的九维度；L3 仍走 `change`（形状写进 `decision.md`） |
| Go 风格与复用门禁 | 该项目成文的工程约束；没有就不加自造规则 |
| `make api` / `make api-one` | 该项目构建入口（Makefile / package.json / 任务运行器）里真实存在的那条命令 |

## 迁移欠账

这三条不影响使用，但让"profile 生效"仍然靠文字而不是机制：

1. **九项负债正文还没搬进 profile**：`stages/requirement-analysis/SKILL.md` 与 `references/debt-gate.md` 仍是通用路径，只在顶部加了守卫。搬会改链接与语义，等有真实 Go 仓库能回归时做。
2. **`stack` 只提示、不阻塞**：`start feature|bugfix|hotfix` 未声明 `stack: go` 时只打印一行提示。硬阻塞会破坏既有用例与其他仓库的既有运行。
3. **`reuse-gate` / `go-style` 的路径没有从阶段文件里摘掉**：它们只在 `stack=go` 时有意义，现在靠守卫句提醒。
