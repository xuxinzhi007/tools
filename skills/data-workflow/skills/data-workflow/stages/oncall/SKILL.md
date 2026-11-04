# data-service on-call

像资深维护者一样回答支持类问题：每个结论都能在当前 checkout 上指认证据。目标是**现在给一个正确有用的答案**，并且**留下可复用的记忆**。

默认只读。诊断请求不授权改代码。

`$PLUGIN_ROOT` 是入口 SKILL.md 所在目录（含 `scripts/paths.sh`）。会话已解析过就直接用。单独打开本文件时，从本文件向上找到该目录。

知识源路径运行时解析，不写死：

```bash
eval "$(bash "$PLUGIN_ROOT/scripts/paths.sh")"   # → $KNOW / $CARDS / $TEMPLATES / $REPO
```

路径可能含空格，命令必须引号包裹。`$KNOW` 为空 → 声明「知识源未配置，只用代码与仓库内 docs（降级区）」，不许猜绝对路径。

## 环境快照

若本次会话已由工作台 `data-workflow` 探测过环境，直接复用其结果，不要重复跑。否则执行一次：

```bash
"$PLUGIN_ROOT/scripts/repo_snapshot.sh"
```

记下 branch、commit、是否有未提交改动、核实时间。**不要求工作区干净，但绝不覆盖无关改动。**

工程约定源（不是业务事实源）：仓库根的 `CONTEXT.md`（术语）与该仓库成文的工程约束文件。约束文件的落点因 IDE 而异，按 `../../references/project-index.md`「工程约束落点」解析（`.cursor/rules/` → `.qoder/rules/` → `AGENTS.md` → `CLAUDE.md`）；**大多数 agent 不会自动加载别的 IDE 的规则目录**，需要时显式 Read。解析不到就明说「本仓无成文工程约束」，不要凭记忆补一份。

## 先分类，再决定加载什么

| 分类 | 触发信号 | 追加加载 |
|---|---|---|
| Contract | 请求/响应字段、错误码、HTTP binding、兼容性 | [trace.md](references/trace.md) |
| Behavior | 服务当前做什么、在哪些守卫下做 | [trace.md](references/trace.md) |
| State | 状态数字/名称、迁移、卡住、进度不符、UI 状态 | [state-protocol.md](references/state-protocol.md) + [trace.md](references/trace.md) |
| Visibility | JWT/RBAC、tenant/group scope、候选数据范围、绕过 | [trace.md](references/trace.md) |
| Incident | 线上症状，需要代码事实 + 运行时证据 | [trace.md](references/trace.md) + [evidence.md](references/evidence.md) |
| Change | 用户明确要求的实现工作 | 交给 `../implementation/SKILL.md`，本技能不做 |

**所有分类都先走 [routing.md](references/routing.md)**：step 0 grep 已有卡片 → step 1 只加载一个 知识层域文件 → step 2 按快速定位规则选代码入口。

定位到域之后，**按 [../../references/business-model.md](../../references/business-model.md) 的九个维度逐维度取证**：对象与身份 / 状态族 / 迁移与守卫 / 读侧可见性 / 角色与数据范围 / 外部通道 / 幂等与重试 / 审计与通知 / 契约。一次只用一个维度——九维同时铺开会互相污染结论。知识落点优先 `$DOMAIN_DOCS`（仓库内），其次 `$KNOW`（补充源）。

非平凡回答前加载 [answer.md](references/answer.md)。要沉淀知识、或发现降级区文档失效时加载 [capture.md](references/capture.md)。

知识源的信任级别与**降级区清单**见 [knowledge-sources.md](references/knowledge-sources.md)。**不要无条件加载 references**——按需取一个。

## 三条硬规则

1. **状态问题先定族再解释数字。**用户只给一个裸数字、没给表/字段/页面时，反问，不要枚举候选含义当答案。
2. **答案中不复述知识层内容，只引用你亲自读过的文件。**取值必须能指到代码或运行时证据。
3. **发现降级区文档（`docs/`、`docs/support/`）与代码冲突时，按 [capture.md](references/capture.md) 的强制规则写卡片**，并在答案里点名该文档已失效。

## 完成标准

- 已标明当前 checkout（branch + commit + 是否有未提交改动）。
- 相关端到端链路与状态维度都覆盖到了。
- 代码事实、知识库口径、假设、运行时证据 visibly 分开。
- 冲突或失效的信源已披露。
- 提出的下一个检查动作能区分剩余假设。
- 若写了卡片，含 `verified_commit`、`source_paths`、`watches`、`invalidates_when`，且索引已更新。
