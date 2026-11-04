# 链路追踪

分类为 Contract / Behavior / State / Visibility / Incident 时加载。只读调查；诊断请求不授权改代码。

## 七步追踪

沿着**任何会改变答案的完整路径**走，缺一步都可能漏链路：

1. **Proto**：RPC、HTTP binding、request/response 字段、声明的错误码。
2. **注册与中间件**：路由可达性、鉴权、错误映射相关时，确认 `internal/server` 的注册。
3. **Service**：默认值、校验、身份/租户提取、响应转换。
4. **Biz**：解析出的 ID、守卫、状态迁移、事务、副作用、队列、scheduler、外部调用。
5. **Data**：持久化列、SQL 谓词、join、scope、锁/CAS、排序、分页、错误传播。
6. **运行时依赖**：有效配置、Redis、Kafka、OSS、MQTT、上游 HTTP/gRPC、挂载配置、特性开关。
7. **读侧**：解释状态、计数、可见性结果、展示字段时，**读侧和写侧都要追**。

停止条件：每一个实质性 request 字段、状态变更、过滤条件、副作用、响应字段、授权边界都有源码支撑的解释。

## 常见漏链路模式

| 症状 | 容易漏掉的那一段 |
|---|---|
| 「没有主动推送吧？」 | 第 6 步。只查了 HTTP handler，没 grep `internal/data/mqtt.go`、`internal/biz/*notification*.go`、`internal/server/scheduler/`。仓库文档在这点上已证实说谎，见 [knowledge-sources.md](./knowledge-sources.md) 降级区 |
| 「值明明写进去了，列表里看不到」 | 第 7 步 + 第 5 步的 scope。tenant/group/candidate scope、软删、关联表 join、分页前后过滤、派生筛选 |
| 「状态卡住不动」 | 第 4/6 步。写入口在 scheduler 或外部回调里，不在同步请求路径上；或 CAS 未命中旧状态守卫 |
| 「进度数字不对」 | 第 7 步。聚合口径排除了哪些状态（如中止态不计入、剔除态计入采集但不计入质检） |
| 「偶发」 | 第 4 步的事务边界与并发。读改写是否有并发窗口，事务里是否包了慢 IO |
| 「本地好的、线上不行」 | 第 6 步。仓库默认配置 ≠ 线上有效配置，静态审查证不了部署 commit 与挂载配置 |

## 只读纪律

确认、解释、事件类问题保持仓库只读。用户明确要求实现（分类 = Change）才写代码，且此时应交给 `../../implementation/SKILL.md`，不在本技能里改。
