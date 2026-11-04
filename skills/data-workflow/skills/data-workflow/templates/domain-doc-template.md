---
id: domain-<域>
title: <这个域在业务里叫什么>
object: <这个名字在业务里指什么，一句话。不写取值>
applies_to: <服务 / 仓库>
status: verified-static          # verified-static / verified-runtime / needs-runtime / stale / superseded
verified_branch: <核实时的分支>
verified_commit: <核实时的 HEAD 全 hash>
verified_at: <ISO8601 带时区>
verification: static             # static / runtime
source_paths:
  - <真值源文件路径>
watches:
  - <被盯的文件或目录；变了 knowledge_drift.sh 就判 STALE>
invalidates_when:
  - <什么变化后必须重新核验>
---

<!--
落点是 $REPO/docs/domain/<域>.md（团队可见，随代码提交）。
frontmatter 的字段名与调查卡片一致，所以同一个 knowledge_drift.sh 能判它新鲜度。
不写：状态数字、字段名清单、SQL 谓词、行号。写"取值看哪里"，不写取值。
维度说明见 references/business-model.md。
-->

# <域> 业务骨架

## 1 这一域负责什么

一句话。超出这句的都算别的域。

## 2 对象与身份

| 对象 | 身份/主键 | 谁创建 | 存在多久 |
|---|---|---|---|

## 3 状态族

只写族名与权威位置，不写取值。

| 族 | 落在哪（表.列 / 字段） | 权威定义在哪 |
|---|---|---|

- 终态判定走谓词函数：<函数名>，不用数值区间。

## 4 写入口与守卫

| 写入口 `文件:符号` | 允许的旧状态 | 守卫 / 事务边界 |
|---|---|---|

## 5 读侧可见性

值写进去之后，还要过哪些过滤才被看见：租户 / 数据范围 / 软删 / 关联 / 分页 / 派生筛选。逐项写 `文件:符号`。

## 6 角色与数据范围

| 角色 | 能做什么 | 边界在哪一层拦 |
|---|---|---|

## 7 外部通道、幂等与重试

主动推送 / 回调 / 定时任务的发送点与接收方；重放靠什么挡。

## 8 已知冲突与待确认

来源互相矛盾时写在这里：哪几份、冲突点、以哪份为准、已问过谁。

## 9 变更记录

| 日期 | commit | 改了什么 | 谁确认 |
|---|---|---|---|
