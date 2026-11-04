# 知识源清单与信任级别

L2 = 知识层。**永远向下指向 L3（代码、生成物、聚焦测试）**，任何 L2 文件都不是结论。

## 主源（按域拆分，一次只加载一个）

根路径 `$KNOW`（由 `$PLUGIN_ROOT/scripts/paths.sh` 解析；可能含空格，命令必须引号包裹）。下表里的 `knowledge/...` 都相对它。

| 源 | 内容类别 | 信任级别 |
|---|---|---|
| `$REPO/docs/domain/*.md`（`paths.sh` 的 `$DOMAIN_DOCS`） | 仓库内业务骨架：对象、状态族名、写入口、可见性、角色范围 | **高**（团队共识 + 随代码提交 + 能 drift）。**与 `$KNOW` 冲突时以它和当前代码为准** |
| `knowledge/状态机.md` | F2 全部状态族的定义、语义、聚合口径 | **高**。状态问题的首选知识源；比仓库 `docs/support/state-machines.md` 新且更完整 |
| `knowledge/线上排查.md` | F2 症状 → 排查入口，含外部通道（数据管线/MQTT/Redis/Kafka） | **高**。事件类问题首选 |
| `knowledge/data-service项目速查.md` | F1+F2 混合：技术栈、分层、关键文件、常用命令、错误码范围 | **中**。适合建立上下文；**其 Episode 生命周期图缺 92/93**，状态取值一律改走 `状态机.md` + 代码 |
| `knowledge/运行与构建.md` | F1 构建与本地运行 | 中。命令以 `Makefile` 目标为准 |
| `knowledge/<业务域>/README.md` | F2 单域业务规则 | **高**（域内）。见 [routing.md](./routing.md) step 1 的域表 |
| `knowledge/接口总览.md` | F2 API 汇总 | 中。以 `api/**/v1/*.proto` 为准 |
| `knowledge/待优化.md`、`归档/**`、`未命名.md` | 工作记忆 / 历史 | **低**。只作背景，不作证据 |
| `开发规范化/**`（同 vault） | F1 组织模板：`研发技术方案-模板.md`、`MR模板.md`、`代码规范.md`、`开发提测文档-模板.md`、`研发流程.md` | 高（模板本身）。模板是事实，技能里只放指针 |

**已知缺口**：整个 知识层没有 baseline commit 与核实日期，所以它自己无法被 `knowledge_drift.sh` 门禁。读它时按「口径可信、取值待核」处理。

## 工程约定源（F1）

落点因 IDE 而异。下表是 data-service 当前 checkout 的实际落点；换仓库时按 `../../../references/project-index.md`「工程约束落点」重解析（`.cursor/rules/` → `.qoder/rules/` → `AGENTS.md` → `CLAUDE.md`），都不存在就明说「本仓无成文工程约束」。

| 源 | 内容 | 注意 |
|---|---|---|
| `.cursor/rules/data-service.mdc` | 分层规则、生成物清单、构建命令的**唯一副本** | Cursor `alwaysApply: true`；**Qoder 不自动加载 `.cursor/rules/`，需要时显式 Read** |
| `.cursor/rules/data-service-go.mdc` | Go/Proto 细则，仓内编号 R01–R18 | 同上，仅 `globs` 命中时由 Cursor 加载。与技能清单 REV-01–REV-09 不是同一套，覆盖关系见 `project-index.md` |
| `.cursor/rules/reuse-gate.mdc` | 复用门禁 | 同上 |
| `CONTEXT.md` | 通用语言术语表 | 只取术语，不取业务规则 |

## 调查卡片（F2，个人，不进 git）

`$CARDS/` —— 检索方式与索引见该目录 `README.md`，流程见 [capture.md](./capture.md)。这是**唯一由本技能写入**的知识源。

## 降级区：只作搜索线索，不可当结论

以下文件已被证实说谎，**不得引用其结论**；与主源或代码冲突时一律以后者为准，并在答案里点名该文档已失效。

| 文件 | 已证实的假话 | 记录卡片 |
|---|---|---|
| `docs/README.md`、`docs/architecture/**`（含 `README.md`、`architecture.md`、`worker-data-collection.md`、`task-mode-design.md`、`dc-phase2-architecture.md`、`dc-phase2-api-reference.md`） | 断言「服务内无 MQTT / 主链路只有 HTTP」。实际 `internal/data/mqtt.go`、`internal/biz/mqtt_client.go`、`internal/biz/thing_client.go` 存在，`cmd/data-service/wire_gen.go` 已注入，`internal/biz/job_dispatch_notification.go` 真实发送且有单测 | `$CARDS/2026-10-27-mqtt-推送链路-docs-失效.md` |
| `docs/support/state-machines.md` | ① 基线钉在 `main@145829cf`（2026-08-16），HEAD 已领先近千 commit；② EpisodeStatus 缺 92/93；③ 断言 Episode 终态只有 90/91；④ **把上面的假话洗成「不要信其他文档」的规则** —— 引用它时会连带采信它的偏见 | `$CARDS/2026-10-27-episode-终态-92-93.md` |
| `docs/support/domain-map.md` | 基线同上。域路由表可当**搜索起点**，不可当当前链路结论。其中「快速定位规则」是流程（P），已迁入 [routing.md](./routing.md) step 2 | 同上两张卡 |
| `docs/adr/**`、`docs/API/**` | 未逐条核验，按「设计意图可信、当前行为待核」处理 | — |

## 永不作为权威引用

- `.qoder/repowiki/knowledge/` —— **机器生成的仓库缓存**，不是第 4 层证据。它随工具重跑而变，无核实人、无 commit 钉，且会把降级区的假话再抄一遍。可当 grep 加速，不可当结论。
- `.cursor/plans/` —— 决策前的工作记忆，记录「当时怎么想」，不记录「现在是什么」。
- 仓库内 `configs/` 默认值 —— 不等于线上有效配置。
