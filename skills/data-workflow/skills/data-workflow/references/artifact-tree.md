# 工件树：产出落在哪、叫什么

**工件是流程的记忆。** 对话会被压缩，工件不会。每一步的产出都落盘，下一步才有输入，事后才可复盘。

---

## 目录结构

```
$REPO/specs/<feature>/
├── spec.md              需求规格：事实清单 + 决定表 + 九项评估      ← requirement-analysis
├── triage.md            止血决策：症状 / 最可能链路 / 三个选项的代价  ← hotfix 第 1 步
├── impact.md            影响面：四轴 + 维护性三查                   ← impact-assessment
├── plan.md              技术方案：接口/数据/状态/权限/验证/回滚      ← tech-design
├── rollback.md          回滚预案：可逆性判定 + 步骤 + 触发条件       ← tech-design / impact-assessment
├── diff.patch           固定下来的 diff（审核三轴共读同一份）        ← change-review
├── review/
│   ├── spec-compliance.md    是否符合 spec 与需求口径              ← 子 agent
│   ├── code-quality.md       规范与可维护性                        ← 子 agent
│   ├── interface-impact.md   契约与调用方影响                      ← 子 agent
│   └── verdict.md            合并裁决：风险等级 + 逐条问题          ← 主 agent（review-only 第 4 步）
├── interface-test.md    三级门结论 + 四态裁决 + 补跑条件            ← interface-test
├── quality.md           构建/单测/覆盖率证据                        ← quality-gate
├── change-note.md       变更说明 + 文档审计门结果                    ← git-commit
├── run.json             当前停在第几步（进度，不存理由）            ← workflow_next.py
├── evidence.log         每次推进一行：stage/门/退出码/head/时间   ← workflow_next.py（只由 runner 写）
├── verify.log           测试入口的原始输出（第一行是 exit）        ← workflow_next.py
├── gate                 方案门：第一条非空行是 plan pass              ← 人 / tech-design
└── agents/
    ├── ledger.md             子 agent 台账（唯一可靠状态源）
    ├── brief-<axis>.md       派活书
    └── report-<axis>.md      回信
```

模板在 `$PLUGIN_ROOT/templates/`：`spec-template.md`、`plan-template.md`、`impact-template.md`、`change-note-template.md`、`rollback-template.md`。**模板里的 GATE 段落不要删** —— 删掉就等于假装那一维不存在；没内容就写「无」。

每份工件的主活和第一屏见 `doc-shape.md`。后一步写「见某文件某节」，不把上一份抄进来。

---

## `<feature>` 怎么命名

`kebab-case`，动词开头或对象开头，能让人不看内容就知道范围：

- `job-label-tags-batch-limit`（对象 + 具体点）
- `fix-episode-status-stuck`（bugfix 用 `fix-` 前缀）
- `hotfix-2026-10-27-dispatch-timeout`（hotfix 带日期）

不用 `test`、`tmp`、`new-feature`、`wip` —— 三个月后没人知道那是什么。

---

## 落在哪个仓库

| 产出 | 落点 | 理由 |
|---|---|---|
| `specs/<feature>/**`（feature / bugfix / hotfix，以及审自己的分支） | 当前工作仓库根 `specs/`。install 到哪个仓库就写哪个，不回技能源仓库 | 记录跟着这一仓开发走；install.sh 已把它写进 `.gitignore` |
| review-only 审别人的工作树 | `$CARDS/reviews/<feature>/` | 不改对方工作树。`$CARDS` 为空就问用户要一个该工作树之外的目录 |
| 排查卡片 | `$CARDS/`（个人知识层） | 结论跨需求复用，按症状检索；**不进仓库** |
| 已稳定的领域知识 | `$KNOW/<域>/` | 归档走 `references/feature-archive.md` |
| 决策前的临时工作记忆 | `$REPO/.cursor/plans/`（或该 IDE 的等价目录） | 临时，不升级为工件 |

`specs/` 若被仓库 `.gitignore` 忽略，就**不提交**，只在本地留着当工作记忆，并在 change-note 里写清产物路径。先核一眼：`git check-ignore -v specs/`。

---

## 三条纪律

1. **一步一产物。** 阶段结束时产物必须已落盘，不要「等最后一起写」—— 最后你就不会写了。
2. **产物只引用，不复述。** 写 `见 impact.md 轴 2`，不要把清单抄一遍。抄出来的副本会各自腐烂，而且腐烂时没有声音。
3. **产物里的每条事实都要能被复现。** 带 `文件:符号`、命令、或输出摘录。做不到的写 `UNVERIFIED <缺什么证据>`。

---

## 与工作流状态机的关系

`$PLUGIN_ROOT/workflows/*.json` 里每个 stage 的 `artifacts` 字段就是上表的机器可读版：

| 工作流 | 必产工件 |
|---|---|
| `feature.json` | spec → impact → plan + rollback → review/×3 → interface-test → quality → change-note |
| `bugfix.json` | spec(精简) → impact → review/×3 → quality → change-note + 卡片 |
| `hotfix.json` | rollback → quality → change-note（**跳过的阶段必须记账**） |
| `review-only.json` | diff.patch → review/×3 → interface-test（不改业务代码；别人的工作树不写 `$REPO/specs/`） |

新业务场景 = **新工作流模板**（重排现有技能与工件），不是新技能。技能池是复用的，模板才是扩展点。
