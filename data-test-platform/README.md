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
| `-openapi` | `openapi.yaml` | OpenAPI 3.0 文件路径 |
| `-log` | `app.log` | 本地日志文件路径 |
| `-target` | 空 | 覆盖默认目标服务地址（空 = 用 `--project` 里的环境） |
| `-slow-threshold` | `200ms` | 慢 SQL 判定阈值（如 `200ms`、`500ms`、`1s`） |
| `-project` | 空 | **项目配置**（环境、请求头、云端日志、本地启动命令）。不传则一切为中性默认 |
| `-k8s-config` | 空 | **K8s 日志源配置**。不传则永不调用 kubectl |

---

## 📜 日志源接入与分发

平台支持三种日志源，同一时刻只有一个是激活的。启动时自动选一个可用的，页面上也能随时切换。

### 三种源

| 源 | ID | 数据从哪来 | 前置条件 |
| :--- | :--- | :--- | :--- |
| 本地文件 | `local` | `tail -f` 指定的日志文件 | `-log` 指向的文件存在 |
| DevOps 云端 | `dev` / `test` / `prod` | 轮询可观测平台 HTTP API | `workflow_url` + `auth_token` + 该环境已配置 |
| K8s Pod | `k8s` | `kubectl logs -f` 跟随集群 pod | `--k8s-config` 且 `enabled=true` 且配了 `app` |

### 启动时怎么选（auto 的真实逻辑）

`log_source` 为空或 `auto` 时，启动按下面的优先级探测，选**第一个真正可用的**：

```
dev → test → prod → k8s → local
```

也就是：**云端优先于 K8s，K8s 优先于本地文件**。一个都没有就选 `none`，页面徽章变红提示。

启动日志会打印完整探测结果，不用猜：

```
-----------------------------------------------------------------
📜 日志源:             local
     dev    未配置该环境
     test   未配置该环境
     prod   未配置该环境
     k8s    未启用（需要 --k8s-config）
   → local  日志文件存在: /tmp/app.log
```

如果显式设了 `log_source`（比如你在页面上选了 `test`），**不会被自动探测覆盖**。

### 切换时发生什么

在页面「来源」下拉框切换时：

1. 目标源就绪 → 停掉当前源，启动目标源，清空日志流与 SQL 统计（避免两个环境的数据混在一起）
2. 从 K8s 切走 → 立即 `SIGTERM` kubectl
3. 目标源不就绪 → 拒绝切换，下拉框回滚，并提示缺什么

### 数据分发路径

三条源最终都汇到同一个 `Hub`，再分发给页面：

```
local   FileTailer ──┐
dev/test/prod RemoteLogPoller ──┼──> Hub ──> WebSocket /ws/logs ──> 日志流面板
k8s     K8sTailer ──┘                  └──> AggregateStore ──> 慢 SQL 排行
                                        └──> StatsCollector ──> 顶部统计
                                        └──> /api/logs/recent ──> 首屏历史
```

每条 `LogEntry` 带 `log_source` 字段标记来源，页面据此显示徽章（`LOCAL` / `DEV` / `K8S`…），K8s 源额外带 `pod_name`。

**源之间互斥**：`Hub.localGate` 会在切到云端时丢弃本地来源的行，避免本地慢 SQL 混进云端日志流。

### 凭证放哪

| 凭证 | 位置 | 是否进 git |
| :--- | :--- | :--- |
| 云端 `auth_token` | `project.json` | ❌ 文件已 gitignore |
| 飞书 `app_id`/`app_secret` | 环境变量 或 `~/.toolbox/config.yaml` | ❌ |
| kubeconfig | 复用本机 `~/.kube/config` 或 `--k8s-config` 指定 | ❌ |

> `project.json`、`k8s.json` 都在 `.gitignore` 里，仓库只提交 `*.example.json`。

---

## 🎯 项目配置（--project）

平台本身不绑定任何项目。域名、环境、网关鉴权头全部由 `--project` 指向的 JSON 提供：

```bash
cp project.example.json project.json    # 改成本项目的域名与环境
go run . --project project.json
```

字段说明：

| 字段 | 作用 |
| :--- | :--- |
| `name` / `title` | 项目名与页面标题 |
| `vars` | 变量表，供 `headers` 里的 `${var}` 展开 |
| `environments` | 环境下拉框的内容（`id` / `name` / `base_url` / `route_prefix`） |
| `headers` | 默认请求头，支持 `${var}`；`secret: true` 的项在 `/api/config` 里返回掩码 |
| `remote_log` | 云端日志查询地址、token、服务名、各环境映射 |
| `direct_hosts` | 绕过系统代理直连的域名后缀（如 `["example.com"]`） |
| `run` | 本地拉起被测试服务的默认命令（`command` / `args` / `dir`） |

不传 `--project` 时只有 `local` 和 `custom` 两个通用环境，没有任何网关头。

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
│   ├── logger/             # 日志解析引擎、文件/K8s Tailer、WebSocket Hub、SQL 指纹聚合
│   └── server/             # REST API、WebSocket 路由与静态资源服务
└── web/
    └── dist/               # 前端单页面应用 (嵌入到 Go 二进制中)
        ├── index.html      # 主页面结构
        ├── app.js          # 前端交互与 WebSocket 处理
        └── style.css       # 响应式暗黑主题样式
```

---

## K8s 日志源（可选加载）

默认不加载。不传 `--k8s-config` 时程序**完全不会调用 kubectl**，只跑原有的本地文件 + DevOps 云端日志。

启用后日志来源下拉框会多出 `K8s Pod` 选项，选中即开始 `kubectl logs -f` 跟随集群 pod 输出。

```bash
cp k8s.example.json k8s.json    # 填 kubeconfig / context / environments
go run . --k8s-config k8s.json
```

配置字段：

| 字段 | 说明 |
|---|---|
| `enabled` | `false` 或文件不存在 = 禁用，前端选项灰掉 |
| `kubectl` | 可执行文件路径，默认 `kubectl` |
| `kubeconfig` / `context` | 透传给 kubectl |
| `app` | label selector 的 `app=` 值，默认 `data-service` |
| `container` | 多容器 pod 时指定 |
| `tail` | 首连回填行数，默认 200；重连用 `tail=0` 避免重复 |
| `environments[]` | 可选环境列表，`lane` 留空 = 不过滤，看全部 pod |

行为约定：

- pod 滚动发布时 kubectl 会被终止，**自动重连**（1s 退避）
- 切走 K8s 源时立即 `SIGTERM` kubectl，不留孤儿进程
- 进程收到 `SIGINT`/`SIGTERM` 时会一并收掉 kubectl
- kubectl 的 stderr 只保留前 64 KiB，错误显示在来源徽章上
