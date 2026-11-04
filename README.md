# tools

个人工具仓库。所有工具通过 **toolbox** 控制台统一启动，不用再翻目录找 `make run`。

```bash
make toolbox        # → http://127.0.0.1:7700
make toolbox-stop   # 停掉全部子工具
```

配置集中在 `~/.toolbox/`（凭证 + 工具清单），换机恢复见 [toolbox/README.md](toolbox/README.md)。

## 工具索引

### 本地 Web 工具（由 toolbox 启停）

| 工具 | 端口 | 用途 |
|---|---|---|
| [data-test-platform](data-test-platform/) | 9099 | OpenAPI 接口测试 + 实时日志 + 慢 SQL 聚合 + 分屏联调 |
| [system_tags_editor](system_tags_editor/) | 8765 | `system_tags.json` 可视化编辑 + 预训练重刷 / 脏数据修复 / List转Init |
| [sql-monitor](sql-monitor/) | 8767 | 慢 SQL 捕获、飞书告警、K8s 日志跟踪（CLI 采集后端） |
| [quality-gate](quality-gate/) | 8080 | AI 工作区文件管理 + MCP 工具集 |

> `data-test-platform` 原本是 `gitlab.x2robot.com/pengcong/data-test-platform`，
> 已脱离上游归入本仓库。原 git 历史归档在 `~/.toolbox/backup/data-test-platform-history.bundle`。

### MCP Server（挂到 Qoder / Cursor，不由此处启动）

| 工具 | 传输 | 用途 |
|---|---|---|
| [feishu-mcp](feishu-mcp/) | stdio | 读写飞书文档、发消息 |
| [apifoxmcp](apifoxmcp/) | stdio | Apifox 场景用例 / 接口搜索与创建 |
| [grill-mcp](grill-mcp/) | stdio | 交互式需求拆解 / Bug 分析 / 架构理解 |
| [qoder-mcp](qoder-mcp/) | stdio | 飞书驱动 qodercli 跑编码任务 |
| [comfyui-mcp](comfyui-mcp/) | stdio + HTTP | 本地 ComfyUI 便携包出图：txt2img / img2img / 跑界面里保存的工作流；HTTP 模式供网页版客户端（Grok）接入 |

### 技能与插件（由外部宿主加载，此处只做备份与索引）

| 目录 | 上游 | 说明 |
|---|---|---|
| [skills/data-workflow](skills/data-workflow/) | `~/skills/data-workflow`<br>[github](https://github.com/xuxinzhi007/data-workflow) | 开发流程技能包（需求分析 → 方案 → 编码 → 审核 → 提交）。Claude `.claude-plugin` / Qoder `.qoder-plugin` 双形态 |
| [skills/dsh-workflow-hud](skills/dsh-workflow-hud/) | `~/skills/dsh-workflow-hud` | data-workflow 运行状态浮层：各 stage 门禁、必填项、产物、证据。DeepSeek Harness 插件 |

这两个是**备份**，实际运行仍以 `~/skills/` 下的原目录为准 —— 宿主（Claude / Qoder / dsh）读的是那边的路径。
备份里不含 `config.yaml`（本机绝对路径，上游已 gitignore）、`.git`、`specs/`。

### 归档

| 目录 | 说明 |
|---|---|
| [api-scenario-tester](api-scenario-tester/) | 手动编排多步接口场景（`tag-service-scenarios.json`）。功能已被 [data-test-platform](#本地-web-工具由-toolbox-启停) 覆盖，保留备查 |
| [utd](utd/) | UDP 延迟 / 抖动 / 丢包探测 |

## 依赖

- Go 1.24+
- Python 3.12+ / [uv](https://docs.astral.sh/uv/)（仅 MCP 工具）

## 版本控制

全部子项目已合并到本仓库单一 `.git` 管理。
原先各子项目独立仓库的历史归档在 `~/.toolbox/backup/*.bundle`，需要时可恢复：

```bash
git clone ~/.toolbox/backup/sql-monitor.bundle sql-monitor-restored
```

仓库里不该出现的东西（已在 `.gitignore` 挡掉）：编译产物、`.venv`、`__pycache__`、
`.DS_Store`、含凭证的 `config.json` / `*.local.json`。凭证统一放 `~/.toolbox/config.yaml`。
