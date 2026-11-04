# 工作流与门禁

`workflows/*.json` 是阶段顺序。本文件是 data-service（Go 栈）主链上、进具体阶段之前要知道的规则。`change` 不读这份。

## 主链

| 步 | 产出 |
|---|---|
| 环境探测 + 查重 | branch、commit、是否有未提交改动、知识新鲜度 |
| 需求分析 | `specs/<feature>/spec.md` |
| 影响面 | `specs/<feature>/impact.md` |
| 技术方案 + 回滚 | `plan.md`、`rollback.md` |
| 编码 | 代码 |
| 改动审核 | `specs/<feature>/review/*.md` |
| 质量门 | `specs/<feature>/quality.md` |
| 接口门 | `specs/<feature>/interface-test.md` |
| 文档审计 + 提交 | `specs/<feature>/change-note.md` |

`bugfix` / `hotfix` 的第 ① 步是排查。`oncall` 不插入 feature 主链。进这条链之前先读 `references/scope.md`。L1 不进这里。门禁命中就停。每步用一句话汇报工件路径和门的结论。

公司模板只承接已确认的内容，空章节写「无」。用户没要求发工单时不发布 issue。

## 环境探测

整个会话一次：

```bash
"$PLUGIN_ROOT/scripts/repo_snapshot.sh"
"$PLUGIN_ROOT/scripts/knowledge_drift.sh"
```

新鲜度三层同时成立：

1. 脚本只标注。`knowledge_drift.sh` 恒 exit 0（exit 2 仅表示文档无法解析）。缺 frontmatter 算 STALE。
2. STALE 不停工。过期文档用来选搜索目标，取值回代码重新推导，产出里写明哪份文档已过期。FRESH 可当证据引用。
3. 产出引用了 STALE 取值却没回代码重新推导，新鲜度门未过，停。

查重：`specs/` 与 `$CARDS/` 里是否已有覆盖此事的工件；是否与已钉住的决定冲突。正在继续同一次运行、且 `run.json` 的 status 不是 done 时，先 `next`，只做它打印的那一步。

## 门

任一未过就停下等用户。运行器不给这六道门打分。缺文件时 `next` 打 `NEED`，还有待拍板时打 `OPEN`，编码步骤退出码不是 0 时打 `RED` 并停住。

| 门 | 在哪之后 | 看什么 | 未过时 |
|---|---|---|---|
| 负债 | 需求分析 | `spec.md` 里的九项和方案数 | 停，不写技术方案 |
| 影响面 | 影响面评估 | `impact.md` 四轴 | 停，视为评估未做 |
| 改动审核 | 改动审核 | review 指纹与工作树 | STALE，重跑，不进质量门 |
| 接口 | 接口测试 | `interface-test.md` | FAIL。跑不了只能记 WAIVED |
| 文档审计 | 提交之前 | `change-note.md` | 停，不许提交 |
| 新鲜度 | 任何引用了知识文档的产出 | drift 标成 STALE 的取值 | 回代码再写，否则停 |

编码一旦入选，后面按改动审核、质量、接口、文档审计做完。提交留到最后。

写入放行的路径表在 `references/edit-gate.md`。派子代理的判据在 `references/subagent-protocol.md`。工件落点在 `references/artifact-tree.md`。
