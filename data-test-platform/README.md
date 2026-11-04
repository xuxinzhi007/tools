# Data Service 接口测试 & 日志 SQL 实时监控平台

专为 `data-service` 打造的本地与线上接口测试、实时日志解析滚动监听与慢 SQL 捕获平台。彻底替代繁琐沉重的 Swagger，提供 Postman 级别的直觉测试体验与 Datadog/Grafana 级别的实时日志/SQL 监控面板。

---

## 🌟 核心特性

### 1. 🚀 替代 Swagger 的全功能接口测试平台
- **自动同步 `make api` 产物**：直接解析 `data-service/openapi.yaml`（支持包含 proto 注释的 OpenAPI 3.0 规范），覆盖 140+ 个核心业务接口与 15 个微服务分组（`JobService`, `ApprovalService`, `EpisodeService`, `TaskService`, `QCService` 等）。
- **一键生成 Mock 数据**：根据 Protobuf/OpenAPI Schema 自动解析 `$ref` 嵌套模型与字段类型，一键生成结构完整、字段语义贴合的请求 JSON 示例。
- **环境一键切换**：内置 `Local (本地 127.0.0.1:8005)`、`Dev (https://dev-api.x2robot.com/core)`、`Test (https://test-api.x2robot.com/core)` 及 `Custom (自定义地址)`。
- **微服务路由前缀**：支持自定义或自动填充 `/core` 路由前缀，适应企业级 API 网关路由。
- **网关鉴权与租户透传**：支持自动注入 `tenant-id`、`x-xr-auth-type`、`x-xr-auth-uid`、`x-xr-auth-sub`、`x-xr-auth-tid`、`x-xr-auth-aut`，杜绝 401 UNAUTHORIZED。
- **交互式 JSON 树形美化**：多色彩高亮（Key/String/Number/Bool/Null）、支持单节点点击折叠/展开、全局一键折叠展开、行内实时搜索高亮。
- **针对 dev/test 统一返回结构优化**：自动检测 `{ code: 0, data: ... }` 包装，提供 `[📦 仅看 data 数据]` 与 `[🌐 完整响应]` 一键切换，并支持一键单独复制纯净 `data` 业务 JSON。
- **明暗双主题切换**：支持 🌙 暗色模式 与 ☀️ 亮色模式 一键平滑切换。
- **动态参数表单**：自动提取 Path 变量（如 `{id}`、`{job_id}`）与 Query 变量，输入即实时同步 URL。
- **内置后端代理 (Proxy)**：直接解决跨域（CORS）限制，准确统计往返耗时（毫秒级）与响应体大小。
- **cURL 一键导出**：根据当前请求参数、Header 与 Body 实时生成标准 `curl` 命令行，方便分享与脚本复用。
- **测试历史追溯**：本地记录所有测试过的接口请求与响应状态，支持随时一键回溯。

### 2. 📜 实时日志滚动监听 & 慢 SQL 智能标记
- **WebSocket 实时流式推送**：连接即可享受平滑的日志自动滚动流，支持一键锁定到底部或暂停滚动。
- **高精度日志解析**：
  - 原生解析 Kratos 结构化 JSON 日志（`JSONLogger`）与 Compact 紧凑日志。
  - 自动剥离 ANSI 终端彩色转义字符。
  - 提取 `ts` 时间戳、`level` 日志级别、`source/caller` 源码行号、`trace.id`、`tenant_id`、`uid`。
- **多维度筛选与检索**：
  - 级别过滤：`DEBUG` / `INFO` / `WARN` / `ERROR`
  - 快捷预设：`全部日志`、`仅看 SQL 🔍`、`慢 SQL 告警 🐢`、`错误日志 ❌`
  - 全文字符串/正则表达式即时检索。
- **慢 SQL 智能捕获与预警**：
  - 监听所有 GORM 数据库操作，精准计算查询耗时。
  - 超过阈值（默认 `200ms`，可在界面即时调整）自动标记为醒目的 **`[🐢 慢 SQL 告警]`**。
  - 突出显示 SQL 执行耗时、影响行数（Rows Affected）、调用来源代码文件行号（例如 `internal/data/job.go:134`）。
  - 内置 SQL 语法高亮与一键复制功能。
- **SQL 性能指标面板**：
  - 实时统计：总 SQL 次数、慢 SQL 累计次数、平均查询耗时、最大耗时峰值。
  - **Top 慢 SQL 排行榜**：随时调出历史最慢查询列表，精确定位性能瓶颈。

### 3. ⚡ 分屏联调模式 (Split View)
- 左半屏为 **接口测试**，右半屏为 **实时日志与慢 SQL 监控**。
- 发送接口测试请求后，自动注入与关联 `x-trace-id`，右侧实时显示该请求在服务端触发的所有日志和 SQL 执行语句，极致提升联调效率！

### 4. ☁️ 接入 Dev / Test 远端云日志流 (DevOps 可观测平台)
- **云端服务日志无缝直连**：对接 `https://devops.x2robot.com/api/v1/observability/logs/query` 接口，支持配置 `Authorization` 鉴权 Token。
- **环境随选切换**：提供 `🔄 自动跟随环境`、`💻 本地日志文件`、`☁️ Dev 云端日志`、`☁️ Test 云端日志` 模式。
- **智能滑动窗口与多副本去重**：后台定时轮询滑动窗口拉取云端日志流，通过时间戳、Pod 名称与内容指纹进行内存去重，保障日志流平滑连贯不重复。
- **K8s Pod 标识与一键过滤**：自动提取云端 `pod_name`、`namespace`、`container_name` 并渲染徽章（如 `☸️ data-service-dev2-78d76496db-mscw6`），点击 Pod 标签即可一键过滤该 Pod 的所有执行日志与 SQL！
- **可视化连接检测**：内置 **`⚡ 立即测试连接与拉取`** 工具，在配置 Token 和 Namespace/ProjectIDs 时可即时验证连通性与数据返回。

---

## 🛠️ 快速启动

### 方式一：直接运行 (开发模式)
```bash
cd data-test-platform
make run
```
控制台将输出：
```text
✅ Successfully loaded OpenAPI: 140 endpoints across 15 services

🚀 =================================================================
   Data Service API 接口测试 & 日志 SQL 实时监听平台
=================================================================
🌐 Web 访问界面:       http://localhost:9099
🎯 默认测试目标服务:   http://127.0.0.1:8005
📄 OpenAPI 规范文件:   ../data-service/openapi.yaml
📜 日志监控文件:       ../data-service/logs/app.log
🐢 慢 SQL 判定阈值:    200ms
=================================================================
```
打开浏览器访问：**`http://localhost:9099`** 即可。

### 方式二：编译为单二进制文件
```bash
make build
./bin/data-test-platform
```
> 注：所有 Web 前端静态资源已通过 Go 1.16+ `embed.FS` 打包到二进制文件中，无需安装 Node.js 或任何前端运行时，单文件即可随处运行！

---

## ⚙️ 命令行参数说明

| 参数 | 默认值 | 说明 |
| :--- | :--- | :--- |
| `-port` | `9099` | 平台 Web 服务监听端口 |
| `-openapi` | `../data-service/openapi.yaml` | `data-service` 生成的 OpenAPI 文件路径 |
| `-log` | `../data-service/logs/app.log` | 需要监听 tail 的日志文件路径 |
| `-target` | `http://127.0.0.1:8005` | 默认被测试的 `data-service` 实例地址 |
| `-slow-threshold` | `200ms` | 慢 SQL 判定时间阈值（如 `200ms`、`500ms`、`1s`） |

---

## 🔄 与 data-service 协同工作流

1. **生成最新接口定义**：
   在 `data-service` 目录下执行：
   ```bash
   make api
   ```
2. **刷新平台接口**：
   点击测试平台右上角的 **`🔄 刷新 API`** 按钮，无需重启即可立即加载最新接口和 Schema！
3. **启动 data-service 本地服务**：
   ```bash
   cd ../data-service
   make build
   ./bin/data-service -conf configs
   ```
   如果希望将终端日志写入文件供平台监听：
   ```bash
   ./bin/data-service -conf configs 2>&1 | tee logs/app.log
   ```
   也可以直接使用平台自带的模拟日志生成器（点击右上角 **`⚡ 模拟测试日志与慢SQL`**）进行功能验证。
4. **进行接口测试**：
   在平台上选定接口，点击 **`✨ Schema Mock 填充`**，点击 **`🚀 发送`**，即可直接查看返回数据与底层执行的 SQL 详情。

---

## 📂 项目结构

```
services/data-test-platform/
├── Makefile                # 编译与启动脚本
├── README.md               # 项目使用文档
├── go.mod / go.sum         # Go 依赖配置
├── main.go                 # 主程序入口与 CLI 参数解析
├── internal/
│   ├── config/             # 配置管理与多环境预设
│   ├── openapi/            # OpenAPI 3.0 解析器与 Schema Mock 生成引擎
│   ├── proxy/              # HTTP 接口测试代理 (解决跨域与耗时测量)
│   ├── logger/             # 日志解析引擎、文件 Tailer、WebSocket 广播 Hub、SQL 性能统计
│   └── server/             # REST API、WebSocket 路由与静态资源服务
└── web/
    └── dist/               # 前端单页面应用 (嵌入到 Go 二进制中)
        ├── index.html      # 主页面结构
        ├── app.js          # 前端交互与 WebSocket 处理
        └── style.css       # 响应式暗黑主题样式
```
