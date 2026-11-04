# 记忆回写

用户要求沉淀、**或调查中发现降级区文档失效**时加载。普通答题请求保持只读。

## 触发规则：一次廉价提议 + 一种强制

**提议（默认不写）**：任何需要追踪 3 个以上文件的回答之后，只输出一行——

> 这条结论可以沉淀成 `$CARDS/YYYY-MM-DD-<主题>.md`，要写吗？

不加解释、不重复提议。用户说不写就不写。这样既保持只读纪律，又让回写这件事**可被发现**（历史上零张卡的原因就是触发条件只有「用户主动要求」，而用户从不主动要求）。

**强制（不提议，直接写）**：当调查发现降级区文档过期或错误时。这是全系统价值最高的一类卡——它记录的是**失效本身**，而不是又一份会过期的事实。按此规则，MQTT 那条假话在第一次被问到分发时就会产出卡片，而不是躺到半年后被偶然发现。

强制卡至少包含：哪份文档哪一行说了什么、真值源在哪（文件+函数/const 块）、当前 HEAD、失效判定条件。

## 写在哪：四个落点的裁定

| 落点 | 类别 | 归属 |
|---|---|---|
| `$REPO/docs/domain/<域>.md` | F2 持久、随代码提交 | 团队共识。按 `templates/domain-doc-template.md` 写，frontmatter 带 `verified_commit` + `watches`。**用户确认后才写**，且不写取值 |
| `$CARDS/` | F2 持久、按症状索引 | 个人调查线索与一次事件的现象。**本技能默认只写这里** |
| `$KNOW/**` | F2 持久、按业务域组织 | 补充源。卡片中被反复引用的结论，后续可人工提升进对应域文件（不由本技能自动写） |
| `.cursor/plans/` | 决策前工作记忆 | 保持原样，临时/个人，不写调查结论 |
| `.qoder/repowiki/knowledge/` | 机器生成缓存 | 永不写入、永不引用为权威 |

`docs/support/cards/` 是旧的团队位置。现在团队共识写 `docs/domain/`（进 git、可 review、可判新鲜度），不再往那里复制卡片。

## 卡片格式

frontmatter 关键字段（本文件即模板；仓库内 `docs/support/knowledge-card-template.md` 把 `repo:` 硬编码成了别人机器的路径，不要沿用那一行）：

```yaml
---
id: support-YYYYMMDD-short-name
title: 简短、可检索的问题标题
kind: fact                     # fact 现状 / decision 当时为什么这样选 / problem 现存问题
status: verified-static        # verified-static / verified-runtime / needs-runtime / stale / superseded
domains: [job]                 # 业务域，与知识层域目录同名
object: ""                     # 这条关于哪个业务对象
state: ""                      # 状态转换；没有就空
relation: ""                   # 对象之间的关系；没有就空
valid_from: ""                 # 从何时起当前有效
supersedes: ""                 # 取代了哪张旧卡的 id；旧卡改成 superseded，不删除
applies_to: ""                 # 哪个服务 / 模块 / 仓库。别的服务的同名规则不拿来用
symptoms: [用户看到的关键词或错误码]   # ← 这是 grep 检索面，必须写用户会说的话
verified_branch: <当前分支>
verified_commit: <核实时的 HEAD 全 hash>
verified_at: <ISO8601 带时区>
verification: static           # static / runtime
source_paths:
  - <真值源文件路径>
watches:                       # ← knowledge_drift.sh 盯这些文件；变了就判 STALE
  - internal/biz/status.go
invalidates_when:
  - <什么变化后必须重新核验>
---
```

`symptoms` 与 `watches` 不是装饰：前者决定这张卡将来能不能被 grep 到，后者决定它能不能被门禁判新鲜。缺任一个，卡片等于没写。

**正文锚定函数名 / const 块名，不锚行号**（行号随 commit 漂移；函数名漂移得慢，且漂移时会响亮地 grep 不到，而不是安静地骗人）。

**写完必须更新 `$CARDS/README.md` 索引表**（每卡约 1 行）。索引就是检索层。

优先更新已有卡片，不要造重复卡。

## 安全硬门禁

不保存：token、密码、AccessKey、Cookie、完整用户隐私、原始客户数据。

- 示例请求与日志**必须脱敏**；ID 只保留排障所需形式。
- 不把仓库默认配置当作线上有效配置写进卡。
- 不把一次线上数据现象写成永久业务规则；单次观测要么标 `verified-runtime` 并写明只针对那次事件，要么回到代码找写入口再写规则。

## 检索侧对应

写入是为了被读到。检索入口在 [routing.md](./routing.md) step 0：打开任何文档之前先 `grep -rl` 卡片目录，并对命中的卡跑 `knowledge_drift.sh`。
