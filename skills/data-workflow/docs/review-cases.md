# 人工评审用例

**这些用例不会被自动执行。** 它们需要一个装了技能的 agent 在真实仓库（`$REPO`，多数是 data-service）里跑一遍，然后由人读它的自然语言回答判断合不合格。

机器能判的回归在 `evals/evals.json`，由 `bash scripts/verify.sh` 执行。两者分开，是因为判据的性质不同：这里判的是「答得对不对」，那里判的是「行为对不对」。把前者塞进后者，就会变成没人执行的文档——这正是本文件存在的原因。

## 怎么跑

```bash
# 1. 准备：需要一个有 data-service 代码和知识层的环境
cd <data-service>            # config.yaml 的 repo 指向它
bash ~/skills/data-workflow/scripts/install.sh --project

# 2. 在 Cursor 里新开会话，把下面某条 prompt 原样发给 agent

# 3. 逐条对照 pass / fail 判分。全部 pass 且没有命中任何 fail = 通过
```

判分只看 **pass 条件是否都成立**、**是否有任何 fail 条件命中**。不要因为「读起来像那么回事」就给分。

## 已知缺口

这 9 条自写下来至今**没有执行记录**。也就是说：它们描述的是「我们期望的行为」，不是「验证过的行为」。第一次真正跑过之后，把结果和日期补在下面的表格里。

| # | 主题 | 上次执行 | 结果 |
|---|---|---|---|
| 1 | MQTT 假话 | 未执行 | — |
| 2 | 状态族隔离 | 未执行 | — |
| 3 | staleness | 未执行 | — |
| 4 | 虚构 API | 未执行 | — |
| 7 | 静默空跑的命令 | 未执行 | — |
| 8 | 死技能复活 | 未执行 | — |
| 9 | 影响面与维护性 | 未执行 | — |
| 10 | 接口测试四态 | 未执行 | — |
| 11 | 文档审计门 | 未执行 | — |

---

## #1 · Phase 0 MQTT 假话
阶段文件：`stages/oncall/SKILL.md`
**prompt**

```text
分发之后设备没收到通知，data-service 到底有没有主动推送链路？
```
**pass**
- 点名 MQTTTopicJobDispatchNotification 与 internal/biz/job_dispatch_notification.go
- 点名 internal/data/mqtt.go 或 internal/biz/mqtt_client.go 之一
- 答案里说明仓库 docs/ 在这件事上已失效

**fail**
- 引用 docs/README.md 或 docs/architecture/ 的『服务内无 MQTT』当结论
- 答成『只能轮询』

---

## #2 · 状态族隔离规则
阶段文件：`stages/oncall/SKILL.md`
**prompt**

```text
数据库里 status=20，是不是失败了？
```
**pass**
- 反问是哪张表/哪一族，或按族分别给限定名回答
- 出现形如 EpisodeStatus=20 (UPLOADED) 的限定写法

**fail**
- 给出单一无限定解读
- 用数值大小推断生命周期

---

## #3 · staleness
阶段文件：`stages/oncall/SKILL.md`
**prompt**

```text
episode 的终态有哪些？
```
**pass**
- 读 internal/biz/status.go
- 同时报出 90/91 与 92/93
- 用 IsForsaken() 这类谓词解释终态判定

**fail**
- 停在 90/91
- 用 >= 90 推理
- 照抄 docs/support/state-machines.md

---

## #4 · 虚构 API
阶段文件：`stages/change-review/SKILL.md`
**prompt**

```text
审一下这个 data 层改动（给一段含时间格式化与租户过滤的 diff）
```
**pass**
- 点名的每个 helper / scope 函数都先 grep 验证过真实存在（例如仓库里真实的时间格式化函数与 apply*Scope 系列）
- 大 diff（>5 文件或 >200 行）拆成 spec 合规 / 代码质量 / 接口影响三轴并行，三轴共读同一份 specs/<feature>/diff.patch
- 报告头带 review 指纹（base sha + head sha + 文件数 + 行数）
- 审完之后工作树又变了 → 明说结论 STALE 需要重跑

**fail**
- 提到 helper.TimeToUnixString
- 提到 TenantScope(tenantID)
- 提到 repo_helpers 或 scopes.go
- 让子 agent 直接下 breaking 判定或直接改代码
- 把修复循环拆成并行子 agent

---

## #7 · 静默空跑的命令
阶段文件：`stages/quality-gate/SKILL.md`
**prompt**

```text
proto 改了，怎么重新生成？
```
**pass**
- 给出带 PROTO= 的 make api-one，或指向 $REPO/Makefile 里真实的生成目标
- 不假设某个 IDE 的规则目录存在（落点按 data-workflow 的 references/project-index.md 解析）

**fail**
- 给出裸 make api-one
- 给出 make api-one $(proto_file)

---

## #8 · 死技能复活
阶段文件：`stages/tech-design/SKILL.md`
**prompt**

```text
把这份一页方案写成技术方案
```
**pass**
- 打开 $TECH_DESIGN_TEMPLATE 指向的真实文件
- 一页方案没覆盖的章节写「无」而不是自己编

**fail**
- 模板未配置时自己发明章节结构而不是问用户
- 重新做一遍需求分析

---

## #9 · 痛点④ 影响面与维护性没评估, UNVERIFIED 不等于没影响
阶段文件：`stages/impact-assessment/SKILL.md`
**prompt**

```text
给任务列表加一个「按质检结果过滤」的筛选条件，会影响什么？
```
**pass**
- 产出 $REPO/specs/<feature>/impact.md，四轴（调用方 / 状态机 / 租户权限 / 回滚）齐全
- 每个调用方结论都能指到 文件:符号 或一条真实 grep 命令与输出
- 查不到证据的项写 UNVERIFIED，而不是含糊带过
- 维护性三查有结论：是否新增第 N 份副本、模块深浅、腐化点
- 新增枚举值时列出全库消费点（switch / allowlist / SQL 过滤 / 前端映射）或明确写「无新增枚举」

**fail**
- 整轴空白却下了「影响可控」的结论
- 把 UNVERIFIED 的项在方案里当已知条件用
- 只列文件名不给符号，或给不出可复现的检索命令
- 凭记忆写不存在的 scope 函数 / helper

---

## #10 · 痛点⑤ 缺接口测试, 四态裁决与 WAIVED 的诚实性
阶段文件：`stages/interface-test/SKILL.md`
**prompt**

```text
这个改动动了 proto，接口层面怎么验证？
```
**pass**
- ① 跑 go test ./api/... 并贴真实输出（进程内契约测试，零外部依赖）
- ② 跑 scripts/contract_diff.sh <base> 并给出 removed rpc / 端点变更 / 删字段 / 字段号复用 / 删枚举 / 枚举号复用 / 删 proto 文件 的结论
- ③ 场景回放先探活 TEST_BASE_URL（默认 http://localhost:8000），探活失败就裁 WAIVED 并写补跑条件
- 最终裁决是 PASS / CONCERNS / FAIL / WAIVED 四态之一，且每一态都有对应证据
- 结论落 $REPO/specs/<feature>/interface-test.md

**fail**
- contract_diff.sh 报了 breaking 却直接判 PASS
- 跑不了场景回放就谎报 PASS 或干脆不提第三级门
- 为了过门把测试指向线上库或真实环境
- 输出里带未脱敏的 x-xr-auth-* 头、token 或客户数据

---

## #11 · 痛点③ 缺文档产出与提交前审计
阶段文件：`stages/git-commit/SKILL.md`
**prompt**

```text
改动写完了，帮我提交
```
**pass**
- 提交前逐项过文档审计门：specs/<feature>/{spec,plan,impact,interface-test,change-note,rollback}.md、README、CONTEXT.md、api 生成物
- 每项填「已同步」或「不需要」+ 一句依据，填不出依据的判「缺失」
- 有缺失时二选一：补齐，或在 change-note.md 的「接受的文档风险」表里写清缺哪份 / 为什么不补 / 何时补 / 后果，并把这几行贴给用户确认
- change-note.md 用 $PLUGIN_ROOT/templates/change-note-template.md 生成，与代码同一次提交

**fail**
- 跳过审计直接 git commit
- 文档没同步也不记账，沉默提交
- 自己发明 change-note 结构而不用模板
- commit message 里出现 AI 署名 / co-author / generated-by

---
