# Git 分支、提交与 MR 规范

> data-service 实例的分支规范（feature 九步链的阶段）。装进别的项目时，按该项目的分支与 MR 约定，不套用这里的版本节奏。

## 新建分支

从最新的目标分支（默认 `main` 或对应 `release/x.y.z.0`）切出，分支名格式：

```text
<前缀>/<版本号>/<功能>
release/<版本号>

# 示例（以当前版本 26.10.1.0 为例）
feature/26.10.1.0/episode-qc-visibility
fix/26.10.1.0/task-dispatch-retry
hotfix/26.9.4.0/batch-approve-gap-lock
release/26.10.1.0
```

- 版本号为四位：`主版本.次版本.修订号.迭代号`，如 `26.10.1.0`，不带 `v` 前缀
- 版本号必须与当前开发中的 release 分支对应，新功能跟随下一个版本号
- hotfix 使用线上正在运行的版本号
- 功能描述用小写英文单词，中划线分隔，不超过 4 个词

## 分支前缀

| 前缀 | 用途 |
| --- | --- |
| `feature` | 新功能开发 |
| `fix` | 缺陷修复（跟随版本节奏） |
| `hotfix` | 线上紧急修复（对应 hotfix 版本号） |
| `release` | 版本发布分支（仅版本号，不带功能） |

```bash
git fetch origin
git checkout -b feature/26.10.1.0/episode-qc-visibility origin/main
```

## Commit 信息格式

遵循 Conventional Commits 规范：

```text
<type>(<scope>): <描述>

[可选 body]

[可选 footer]
```

## Types

| Type | 用途 |
| --- | --- |
| `feat` | 新功能 |
| `fix` | Bug 修复 |
| `docs` | 仅文档变更 |
| `style` | 代码格式（不影响逻辑） |
| `refactor` | 既非修复也非新功能的代码重构 |
| `perf` | 性能优化 |
| `test` | 新增或修正测试 |
| `build` | 构建系统或外部依赖 |
| `ci` | CI 配置变更 |
| `chore` | 其他维护性工作 |
| `revert` | 回滚之前的提交 |

## 规则

1. 描述使用祈使语气（写「新增 xx 功能」而非「新增了 xx 功能」）
2. 描述结尾不加句号
3. 描述控制在 72 字符以内
4. subject 与 body 之间空一行
5. body 用来说明改动的动机、背景、注意事项，不要复述 diff
6. 提交信息正文使用中文

## Scope

可选。用于标明改动范围，优先使用仓库模块名，如 `api`、`service`、`biz`、`data`、`conf`、`wire`。

## 破坏性变更

在 type/scope 后加 `!`，或在 footer 中加入 `BREAKING CHANGE:`：

```text
feat(api)!: 移除废弃的 /v1/users 接口

BREAKING CHANGE: /v1/users 接口已被删除。
```

## 署名

提交信息中禁止出现任何 AI 署名、co-author 标签或 generated-by footer。

## 文档审计门（commit 之前，必过）

代码改完不等于交付完。提交前拿 `git diff` 逐项核对下面这张表，**每项只能填「已同步」或「不需要」+ 一句依据**；填不出依据就是「缺失」。

| 检查项 | 判定 |
|--------|------|
| `specs/<feature>/spec.md` | 接口/字段/行为与最终代码一致？决定表里没有「拍了但没做」的项？ |
| `specs/<feature>/plan.md` | 实际实现与方案的偏差已写进「偏差记录」？ |
| `specs/<feature>/impact.md` | 影响面清单里标 UNVERIFIED 的项，事后补证了还是仍然悬空？ |
| `specs/<feature>/interface-test.md` | 三级门裁决齐了？WAIVED 项写了为什么可以 waive？ |
| `specs/<feature>/change-note.md` | 本次改动摘要、欠的债（在哪、怎么还、何时还）、接受的文档风险都记了？ |
| `specs/<feature>/rollback.md` | 回滚步骤与真实 commit / 迁移号对得上？ |
| 仓库 `README.md` | 新增能力、新增命令、新增配置项要不要露出？ |
| 仓库 `CONTEXT.md` | 新增或改名的领域术语要不要登记？（改术语走全局技能 `domain-modeling`） |
| `api/**` 生成物 | 改了 proto 是否已重新生成（`make api` / `make api-one PROTO=...`），生成物与 proto 同一次提交？ |

不过门的处理，只有两条路，**没有第三条**：

1. **补齐**：把缺的文档写完，再提交。
2. **显式接受**：在 `specs/<feature>/change-note.md` 的「接受的文档风险」表里写清 *缺哪份、为什么现在不补、什么时候补、不补的后果*，并把这几行原文贴给用户确认。

沉默提交 = 没过门。文档债务不留痕，三个月后没人知道那份 spec 是过期的还是错的。

`change-note.md` 用 `$PLUGIN_ROOT/templates/change-note-template.md` 生成，落 `specs/<feature>/change-note.md`，**与代码同一次提交**（工件树见 `../../references/artifact-tree.md`）。

审计结论写进 commit body 或 MR 描述的「测试情况 / 关联设计方案」字段，不要只留在会话里。

## 提交 MR（glab CLI）

使用 `glab` 创建或编辑 MR，描述必须使用以下模板：

```bash
glab mr create --title "<type>(<scope>): <描述>" --description-file <描述文件>
glab mr update <mr-id> --description-file <描述文件>
```

生成 MR 说明时，先读 `$MR_TEMPLATE`（由 `paths.sh` 解析，见 `data-workflow`），按该文件的章节和字段名填写。`$MR_TEMPLATE` 为空或文件读不到时用下面这份兜底，字段名保持不变，并在 MR 说明里注明「未使用公司模板」。

```markdown
## 变更背景

## 改动内容

- 

## 关联需求/Issue

- 需求文档：
- Jira：

## 关联设计方案

- 

## 测试情况

- 自测文档：
- 自测结论：

## SQL 性能评估

- 预检截图：
- 只读节点试跑耗时：
- 查询数据量：
```

## 这次新知道的一条

提交前，如果这次运行证实或推翻了一条口径，按 `../oncall/references/capture.md` 写一张卡：谁、何时、为什么，加上 `object`、`applies_to` 和 `kind`。实现已经不符合旧卡时，旧卡标 `superseded`，新卡用 `supersedes` 指向它。只写这次用到的那一条。不写复盘，不补全整个域。没有新口径就跳过。

### MR 要求

1. 同一次回复先给 commit 信息，再给填好的 MR 正文。MR 标题与 commit subject 一致。
2. 输出去掉模板里的注释和代码围栏，只留填好的章节。
3. 「变更背景」「改动内容」写这次改动的原因和要点。
4. 没有文档、Jira、设计方案或自测文档时，该字段写「无」。有链接时写成 `[名称](URL)`。不编造链接。
5. 「自测结论」写「通过」或「不通过」，并在该节用一句话说明测了什么。
6. 无 SQL 变更时，「SQL 性能评估」下三项都写「无」。有 SQL 变更时填预检截图、只读节点试跑耗时、查询数据量。
7. 用户只要文案、尚未确认时，不执行 `git commit`、`glab mr create`。确认后再提交。
8. MR 描述更新后用 `glab mr update` 同步。
