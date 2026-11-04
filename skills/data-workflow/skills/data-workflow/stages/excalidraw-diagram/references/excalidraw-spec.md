# Excalidraw JSON 数据规范与排版最佳实践

## 1. Excalidraw 核心数据模型

Excalidraw 文件（`.excalidraw`）本质是一个 JSON 对象，包含画布状态、图元列表及关联文件。

```json
{
  "type": "excalidraw",
  "version": 2,
  "source": "https://excalidraw.com",
  "elements": [],
  "appState": {
    "gridSize": null,
    "viewBackgroundColor": "#ffffff"
  },
  "files": {}
}
```

### 常用图元类型 (Element Types)

1. **`rectangle`（矩形卡片）**
   - `x`, `y`, `width`, `height`: 坐标与尺寸
   - `strokeColor`: 边框色（建议使用调色盘）
   - `backgroundColor`: 填充色（建议柔和色系）
   - `fillStyle`: `"solid"` | `"hachure"` | `"cross-hatch"`
   - `roundness`: `{ "type": 3 }`（圆角卡片）
   - `roughness`: `1`（手绘感）或 `0`（平滑矢量）

2. **`text`（文本）**
   - `text`, `originalText`: 文本内容
   - `fontSize`: 标题 20~30，正文 14.5~16
   - `fontFamily`: `1` (Virgil 手绘风) | `2` (Helvetica 无衬线) | `3` (Cascadia 等宽代码)
   - `textAlign`: `"left"` | `"center"` | `"right"`

3. **`arrow`（箭头连线）**
   - `points`: 相对起点的坐标序列，如 `[[0, 0], [dx, dy]]`
   - `endArrowhead`: `"arrow"`
   - `strokeStyle`: `"solid"`（实线流程）| `"dashed"`（虚线配置/注入）

---

## 2. GitHub README 友好排版黄金法则

### 痛点根源
GitHub 网页正文阅读容器宽度通常仅有 **850px ~ 1012px**。如果在一行硬塞 5~6 个模块，整张图缩放后正文字号会急剧缩减到 6~8px，导致在屏幕上模糊难以阅读。

### 黄金规则

1. **模块单行限制**：
   - 横向单行最多排布 **3 个核心模块**（每个卡片宽度建议 300px ~ 340px）。
   - 超过 3 个模块时，优先采用 **双行闭环流水线（S 型回环）** 或 **分层树状布局**。
2. **字号阶梯保障**：
   - 画布总标题：`28px ~ 32px` 加粗
   - 卡片核心标题：`19px ~ 21px` 粗体
   - 卡片正文要点：**不得低于 `14.5px ~ 16px`**，行距保持 `1.3 ~ 1.4`
   - 流动标签与小 Tag：`12px ~ 13px` 加粗并带有独立浅色底框胶囊
3. **色彩语义清晰（Pastel 调色盘）**：
   - 🌿 **输入/采集**：薄荷绿系（`#ebfbee` / `#0ca678`）
   - 💧 **处理/转换**：天水蓝系（`#e7f5ff` / `#1971c2`）
   - 🔮 **规则/裁决**：紫罗兰系（`#f3f0ff` / `#7048e8`）
   - 🍊 **输出/时钟**：暖阳橙系（`#fff4e6` / `#e8590c`）
   - 🚨 **配置/策略**：珊瑚红系（`#fff5f5` / `#e03131`）
   - ⚙️ **硬件/底座**：沉稳灰系（`#f1f3f5` / `#343a40`）
