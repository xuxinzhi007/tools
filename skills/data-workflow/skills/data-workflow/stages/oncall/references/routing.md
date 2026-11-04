# 定位与路由

按 step 0 → 1 → 2 顺序执行。**先查记忆，再查知识库，最后才读代码**；反过来会把已经付过代价的调查重做一遍。

## step 0 记忆检索（必做，在碰任何代码之前）

```bash
if [ -z "${CARDS:-}" ]; then
  echo "无卡片目录（config.yaml: cards_dir / knowledge_root 未配）——跳过 step 0，直接进 step 1"
else
  grep -rl "<症状词>|<状态值>|<RPC名>|<表名>" "$CARDS/"
fi
```

`$CARDS` 为空时必须走上面那个分支。**不要写裸的 `grep -rl … $CARDS/`**：空变量未加引号会变成 `grep -rl … /`，从文件系统根开始扫。

命中就打开卡片，读 `verified_commit` / `watches` / `status`。然后跑新鲜度门禁：

```bash
"$PLUGIN_ROOT/scripts/knowledge_drift.sh" <卡片路径>
```

- `FRESH` → 打开 `watches` / `source_paths`，对照当前 `文件:符号`。仍符合才可引用。代码已经不是卡上写的那样，标存在冲突，当前行为以代码为准。
- `kind: decision` 的卡是当时的选择，不是永远的业务规则。被 `supersedes` 接上的新卡才是当前这条。
- `STALE` → **只当线索**：用它选搜索目标，然后从 `watches` 列出的文件重新推导每个取值，并在答案里说明该卡已过期。
- `SUPERSEDED` → 跳过，看 `supersedes` 指向的那张卡。旧卡留着查历史。
- `CONFLICT` → 同一个 `object` 落在不同的 `applies_to`。只用当前服务这一张，不把另一张的规则拿来用。
- 无 frontmatter → 按 `STALE` 处理。
- 卡片写了 `applies_to`，且不是当前这次问的服务或仓库 → 跳过。

索引表在 `$CARDS/README.md`（每卡约 1 行，比逐卡打开便宜）。

## step 1 域路由：只加载一个知识库文件

知识库按业务域拆分，先只读一个域。读错域比不读更糟。证据指向另一个域、出现 `CONFLICT`、或这条规则写明跨域时，再读那一个，并写明原因。不要把整个知识库读进来。

先按**通用**问题类型，从 `$KNOW` 结构发现该项目对应的域文件；`$DOMAINS` 非空时优先用它（`printf '%s\n' "$DOMAINS" | tr ',' '\n'` 拿到域名清单，挑一个读）。data-service 的域表见下方「示例」。

| 通用问题类型 | 去哪发现 |
|---|---|
| 这个域的整体骨架（对象/状态族/写入口/可见性/角色） | 先看 `$DOMAIN_DOCS/<域>.md`（`paths.sh` 解析，目录不存在就是空）；没有就按 `../../../references/business-model.md` 的九个维度现场取证 |
| 状态 / 枚举 / 迁移 / 终态 / 展示态 | `$KNOW` 里该项目声明状态/枚举的文件（data-service 是 `状态机.md`） |
| 线上症状、卡住、看不到、进度不对 | 该项目的线上排查文件（data-service 是 `线上排查.md`） |
| 项目全貌、分层与常用命令 | 该项目的速查文件（data-service 是 `data-service项目速查.md`） |
| 构建、环境、本地启动 | 该项目的运行/构建文件（data-service 是 `运行与构建.md`） |
| 具体业务域（建单、审批、分发、质检…） | `$DOMAINS`（`paths.sh` 从 `config.yaml` 的 `domains` 解析，逗号分隔）；为空时看 `$KNOW` 下的域目录 / 索引。一次只读一个域 |

> `$KNOW` 未配置，或该项目没写成文的域结构时 → 只用代码与仓库内 `docs/`（降级区），并在结论里声明知识源缺失，不猜域表。

**加载顺序（优先级从高到低）**：`$DOMAIN_DOCS/<域>.md`（仓库内、随代码提交、能 drift）→ `$KNOW/<域>`（补充源，引用时标「未核对」）→ 仓库 `docs/`（降级区，只当搜索线索）。两处冲突时以仓库内 + 当前代码为准，并在结论里点名冲突在哪。

### data-service 实例的域表（示例）

技能默认不再内置 data-service 业务域；下表是 data-service 实例通过其 `$KNOW` 呈现的取值，装进别的项目时替换为该项目自己的 `$KNOW` 结构。

| 问题类型 | 加载这一个 |
|---|---|
| 状态数字 / 名称 / 迁移 / 终态 / 展示态 | `knowledge/状态机.md` |
| 线上症状、卡住、看不到、进度不对 | `knowledge/线上排查.md` |
| 项目全貌、新人上下文、分层与常用命令 | `knowledge/data-service项目速查.md` |
| 构建、环境、本地启动 | `knowledge/运行与构建.md` |
| Job 建单（APP/平台）、任务生命周期 | `knowledge/采集任务/README.md` |
| 审批流、节点、规则 | `knowledge/审批流程/README.md` |
| 分发、改派、抢单、工作台 | `knowledge/任务分发/README.md` |
| Collection 采集批次 | `knowledge/采集批次/README.md` |
| Episode 数据条目、事件上报 | `knowledge/数据条目/README.md` |
| QC 质检（列表/领取/提交/智能预检） | `knowledge/质检管理/README.md` |
| 标签、Tag code | `knowledge/标签管理/README.md` |
| 小组、数据范围 | `knowledge/小组管理/README.md` |
| 设备、设备型号、在线状态 | `knowledge/设备管理/README.md` |
| 多样性模板 | `knowledge/多样性模板/README.md` |
| 任务配置 | `knowledge/任务配置/README.md` |
| 飞书通知 | `knowledge/飞书配置/README.md` |

根路径：`$KNOW`（`paths.sh` 解析，**可能含空格，命令里必须引号包裹**）。上表的 `knowledge/...` 都相对它。`$KNOW` 为空 → 只用代码与仓库内 `docs/`（降级区），并在结论里声明知识源缺失。

各域 `README.md` 是入口，需要细节时再顺着它进同目录的具体文件（如 `质检管理/人工质检.md`）。仍然一次一个文件。

信任级别与例外见 [knowledge-sources.md](./knowledge-sources.md)。**知识库文件只给线索和口径，取值一律回 L3 代码核对**——尤其是状态数字、字段名、SQL 条件。

## step 2 快速定位规则（纯流程，不随 commit 失效）

**通用原则**：定位入口取决于该项目的技术栈与成文工程约束。data-service（Go 栈）用下方的定位规则；装进别的项目时，按该项目 `project-index` / 工程约束声明的结构定位（`internal/{service,biz,data}` 分层是 data-service 的，不套用）。

### data-service（Go 栈）实例的定位规则

- 已知 URL：先在 `api/**/*.proto` 搜 HTTP path，再找同名 Service 方法。
- 已知 RPC：从 `internal/service` 同名方法进入，向 Biz 和 Data 追踪。
- 已知表/字段：从 `internal/data` 模型与 `Updates/Where/Joins` 进入，再反查 Biz 调用者。
- 已知状态数字：**先定族**（见 [state-protocol.md](./state-protocol.md)），再只 grep `internal/biz/status.go` 里该族的 const 块，然后搜全部写入口与读侧筛选。
- 已知错误码/文案：搜 Proto error enum、`internal/pkg/errors`、Biz guard 和外部错误映射。
- 已知日志：搜稳定 message/key，而不是完整动态日志行。
- “列表里看不到”：同时检查 RBAC、tenant/group scope、candidate scope、状态/软删过滤、关联表、分页前后过滤。
- “卡住/进度不对”：同时检查持久化状态、并行状态字段、聚合计数、CAS 是否命中、队列/调度器和外部回调。
- “没收到通知/没推送”：不要先信文档结论，先 grep `internal/biz/*notification*.go`、`internal/data/mqtt.go`、`internal/server/scheduler/`，确认是否存在主动推送通道（已有一张卡专门记录这里文档说谎）。

## 完成判定

定位阶段结束的条件：已经能说出「这个问题属于哪个族/哪个域」+「下一步该打开哪 1–3 个代码文件」。若还需要打开第 4 个文档才能决定，说明路由做错了，回到 step 1 重选域。
