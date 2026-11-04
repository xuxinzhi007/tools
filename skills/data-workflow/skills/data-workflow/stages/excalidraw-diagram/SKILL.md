# Excalidraw Architecture & Diagram Generator

为项目快速生成高品质、高保真的系统架构图、数据流水线图与流程拓扑图。

自动同步生成三位一体的产物：
1. **`.excalidraw`**：官方原生 JSON 源码，可直接拖入 [excalidraw.com](https://excalidraw.com) 或 Obsidian Excalidraw 插件无损编辑；
2. **`.svg`**：面向 GitHub README 深度优化的高清矢量图（字大清晰、高对比度、自适应排版）；
3. **`.png`**：通过 `rsvg-convert` 高清导出的高分辨率位图，方便在即时通讯软件或文档中快速预览和粘贴。

> **PNG 依赖**：`rsvg-convert`（来自 `librsvg`）不存在时，PNG 步骤会被静默跳过，只产出 `.excalidraw` 与 `.svg`。需要位图时先 `brew install librsvg`，或自行用其他渲染器把 SVG 转成 PNG。

---

## 核心设计法则（防止字小模糊）

GitHub 文本区域通常限制在 **850px ~ 1012px** 宽度内。为了避免单行挤压导致文字缩放后模糊：

1. **双行回折布局（S 型流水线）**：
   - 超过 3 个处理环节时，**坚决避免单行排布 4~6 个方块**。
   - 采用第一行（输入与前置处理）→ 折返汇入第二行（核心处理与输出）的 2 行 3 栏网格。
2. **黄金字号标准**：
   - 画布总标题：`30px ~ 32px`
   - 模块卡片标题：`19px ~ 21px`（加粗）
   - 正文列表要点：`14.5px ~ 16px`（行距 1.3 ~ 1.4）
   - 卡片宽度：`300px ~ 340px`
   - 流动标签：`13px`（必须带圆角浅色背景胶囊，确保对比度）
3. **语义调色盘（Pastel 系列）**：
   - `green`（数据源/接入层）: 背景 `#ebfbee` / 边框 `#0ca678`
   - `blue`（信号集/转换层）: 背景 `#e7f5ff` / 边框 `#1971c2`
   - `purple`（规则引擎/计算层）: 背景 `#f3f0ff` / 边框 `#7048e8`
   - `orange`（调度时钟/输出层）: 背景 `#fff4e6` / 边框 `#e8590c`
   - `red`（配置中心/策略层）: 背景 `#fff5f5` / 边框 `#e03131`
   - `dark` / `gray`（硬件终端/外部依赖）: 背景 `#f1f3f5` / 边框 `#343a40`

排版与 Excalidraw JSON 数据模型的完整细节见 [`references/excalidraw-spec.md`](references/excalidraw-spec.md)，手写 spec 或调图元属性前先读它。

---

## 使用方式

**本机未安装 `rsvg-convert`，generator 的 PNG 步骤是假的**——它仍会打印 `test.png`，但文件不存在。要位图就跑插件根的 `scripts/svg_to_png.sh <in.svg> <out.png> [scale]`（headless Chrome，默认 2x），别照抄 generator 的打印，只报实际存在的文件。

脚本路径以本 skill 的加载目录为基准（Qoder 加载 skill 时会给出 `Base directory for this skill`）。下文用 `$SKILL_DIR` 指代该目录，即 `scripts/diagram_generator.js` 的父级的父级。

### 方式 1：快速编写 Node 脚本（推荐）

在当前工作区直接调用本 skill 提供的通用构建器 `diagram_generator.js`，例如：

```javascript
const { DiagramBuilder } = require('$SKILL_DIR/scripts/diagram_generator.js');

const spec = {
  title: "项目系统架构图",
  subtitle: "核心数据处理流水线与控制机制",
  width: 1180,
  height: 890,
  cards: [
    {
      title: "数据输入源",
      sub: "Inputs",
      theme: "gray",
      x: 50, y: 130, w: 320, h: 220,
      items: ["HTTP API 请求", "Kafka 消息流", "本地配置文件"]
    },
    {
      title: "① 接入网关",
      sub: "pkg/gateway",
      theme: "green",
      x: 430, y: 130, w: 320, h: 220,
      items: ["JWT 身份鉴权", "反向代理路由", "全局限流与熔断"]
    },
    {
      title: "② 处理核心",
      sub: "pkg/core",
      theme: "blue",
      x: 810, y: 130, w: 320, h: 220,
      items: ["业务规则求值", "状态机驱动", "缓存与数据同步"]
    }
  ],
  arrows: [
    { startX: 370, startY: 240, points: [[0, 0], [50, 0]], label: "请求流", labelPos: { x: 400, y: 226 } },
    { startX: 750, startY: 240, points: [[0, 0], [50, 0]], label: "内部消息", labelPos: { x: 780, y: 226 } }
  ],
  configBoxes: [
    {
      title: "⚙️ 配置与元数据中心",
      sub: "集中式管控与热更新支持",
      theme: "red",
      x: 50, y: 660, w: 1080, h: 180,
      items: [
        "**Apollo / Nacos**：远程拉取动态规则与限流阈值",
        "**Local Cache**：进程内缓存加速读取"
      ]
    }
  ]
};

const builder = new DiagramBuilder(spec);
builder.generate('./docs', 'architecture');
```

执行后将自动生成：
- `./docs/architecture.excalidraw`
- `./docs/architecture.svg`
- `./docs/architecture.png`（仅在 `rsvg-convert` 可用时）

---

### 方式 2：CLI 配置文件生成

准备一个 `diagram.json`，随后执行：

```bash
node "$SKILL_DIR/scripts/diagram_generator.js" diagram.json ./docs architecture
```

---

## README.md 嵌入规范

生成图片后，在项目的 `README.md` 中按如下标准嵌入展示：

```markdown
<p align="center">
  <img src="docs/architecture.svg" alt="系统架构设计与数据流" width="100%">
</p>
<p align="center">
  <sub>（注：架构图源文件见 <a href="docs/architecture.excalidraw">docs/architecture.excalidraw</a>，支持在 <a href="https://excalidraw.com">Excalidraw</a> 中导入编辑）</sub>
</p>
```
