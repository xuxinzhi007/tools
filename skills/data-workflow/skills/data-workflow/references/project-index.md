# 项目索引

本文件只有指针。**任何会被一个 commit 变成假话的内容（状态取值、枚举清单、文件职责、构建命令）都不写在这里**，写了对它就会腐烂，而且腐烂时没人会听见。

业务口径以 `$KNOW/**` + 当前代码为准。本文件是**通用骨架**：data-service 的具体取值在「data-service 实例」一节，装进别的项目时替换为该项目的取值。

## 去哪读（通用）

知识层根路径由 `$PLUGIN_ROOT/scripts/paths.sh` 解析出 `$KNOW` / `$TEMPLATES` / `$CARDS` / `$REPO`（路径可能含空格，命令必须引号包裹）。为空 → 问用户，不猜绝对路径。

| 要查什么 | 去哪（通用） |
|---|---|
| 技术栈、项目全貌、常用命令 | `$KNOW` 里该项目速查文件 |
| 状态定义、语义、聚合口径 | `$KNOW` 里该项目状态/枚举文件 → 回代码核对取值 |
| 线上症状、排查入口 | `$KNOW` 里该项目的线上排查文件 |
| 构建与本地运行 | `$KNOW` 里该项目的运行/构建文件 → 命令以该项目构建入口（Makefile / package.json…）为准 |
| 单个业务域 | `$DOMAIN_DOCS/<域>.md`（仓库内，优先）→ `$KNOW/<域>/README.md`（补充源），一次只读一个域。看什么维度见 `business-model.md` |
| 公司模板（技术方案、MR、提测、代码规范、研发流程） | `$TEMPLATES/` |
| 工程约束、分层规则、生成物清单、构建命令 | 该项目成文的工程约束文件（落点因 IDE 而异，见下） |
| 术语 | 该项目的术语文件 |
| 个人调查卡片（症状索引） | `$CARDS/`，检索方式见该目录 `README.md` |
| 接口/契约 | 该项目的事实源 |
| 任何域的当前真实行为 | 该项目当前代码，以代码为准 |

工程约束文件的落点因 IDE 而异：仓库里没有 `.cursor/rules/` 时，依次找 `$REPO/.qoder/rules/`、`$REPO/AGENTS.md`、`$REPO/CLAUDE.md`。都没有就明说「本仓无成文工程约束」，不要把某个 IDE 的目录名当成事实。

## 找代码入口的方法（通用）

不维护「域 → 文件」对照表——那种表每次重命名就腐烂，而且腐烂时是**安静地骗人**。改用不会失效的定位规则：从已知的 URL / RPC / 表字段 / 状态数字，按该项目的工程约束（分层、目录）进入。data-service 的具体规则见「data-service 实例」与 `stages/oncall/references/routing.md` step 2。

## data-service 实例（示例）

装进 data-service 时，上面的通用取值落到这份：

- 技术栈 / 状态 / 线上排查 / 构建：`$KNOW/` 下的 `data-service项目速查.md`、`状态机.md`、`线上排查.md`、`运行与构建.md`（`状态机.md` 取值回 `internal/biz/status.go` 核对）。
- 业务域：`$KNOW/采集任务|审批流程|任务分发|采集批次|数据条目|设备管理|质检管理|标签管理|小组管理|多样性模板|任务配置|飞书配置`，一次只读一个域。
- 工程约束 / Go 细则 R01–R18：`$REPO/.cursor/rules/data-service.mdc`、`data-service-go.mdc`。
- 接口字段：`api/**/*.proto`（生成物 `*.pb.go` 同源）。
- 任何域的当前真实行为：`internal/{service,biz,data}` 对应文件。
- 术语：`CONTEXT.md`。
- 降级区：`docs/` 与 `docs/support/` 已被证实在 MQTT 推送链路与 Episode 终态上说谎，只可当搜索线索，不可当结论（清单见 `stages/oncall/references/knowledge-sources.md`）。
