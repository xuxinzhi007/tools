# Cyber Web Crawler Demo

一个**单文件、可直接双击打开**的网页视觉 Demo：把 `data-workflow` 的门禁运行画成一张会呼吸的数据网。

- 主体：一只由参数化姿态驱动的肥鱼/蛛形生物，六条 Verlet 触手。
- 网线：从身体连向页面上真实的 DOM 锚点（阶段节点、门禁卡、缺失项、verify 失败行），颜色跟随该锚点的门禁语义。
- 事件：由 mock 事件总线驱动，完整走一遍「阶段 → 门禁拦截 → 补齐 → 放行 → 质量门 → RED / GREEN」。
- 物理：Stress / Urgency 两个标量驱动 `timeScale`、颤动幅度、触手拉力、重力和阻尼；RED 时身体瘫软。

## 直接打开

```bash
open demo/index.html          # macOS
# 或直接把 index.html 拖进浏览器
```

`index.html` 已内联 CSS、核心逻辑、场景逻辑和立绘（约 250 KB），不依赖网络、不依赖本地服务器，`file://` 下即可运行。

## 开发与构建

```bash
cd demo
node build.mjs          # 由 index.template.html + src/*.js + styles.css + assets 生成 index.html
node build.mjs --check  # 只校验，不写文件
node --test test/*.test.mjs
```

## 操作

| 控件 | 作用 |
|---|---|
| 暂停 / 继续（或 `Space`） | 暂停或继续 mock 时间线 |
| 重放（或 `R`） | 从 0 重放当前模式 |
| 切到 RED / GREEN | 切换成功与失败两种结局 |
| 速度 0.5× / 1× / 1.5× / 2× | 时间线倍速 |

## 确定性冻结（截图 / 调试）

URL 参数可以把 mock 时间线直接跳到任意时刻并冻结，不依赖真实等待：

```text
index.html?at=2500              # 2.5s：门禁拦截，触手锁定两个缺失项
index.html?at=10000             # 10s：质量门 mid-stream，verify 行逐条吸附
index.html?mode=red&at=11400    # RED：verify exit 3，模型瘫软
```

冻结由 `ScenarioPlayer.skipTo()` 与 `cyberWebDemo.seek()` 实现：跳到目标时刻、发完所有到期事件、姿态弹簧收敛、物理推进 90 步，然后暂停。
`screenshots/` 下的三张图就是用这组参数截出来的：

| 文件 | 参数 | 画面 |
|---|---|---|
| `gate-blocked.png` | `?at=2500` | 门禁拦截，触手锁定两个缺失项 |
| `quality-gate.png` | `?at=10000` | 质量门 mid-stream，verify 行逐条吸附 |
| `verify-red.png` | `?mode=red&at=11400` | verify exit 3，模型瘫软 |

重新截图：

```bash
node tools/cdp-probe.mjs "file://$PWD/index.html?at=2500" screenshots/gate-blocked.png
node tools/cdp-probe.mjs "file://$PWD/index.html?at=10000" screenshots/quality-gate.png
node tools/cdp-probe.mjs "file://$PWD/index.html?mode=red&at=11400" screenshots/verify-red.png
```

注意：无头截图若用 `--virtual-time-budget`，虚拟时钟会在图片解码前冻结，画布会是空的；`tools/cdp-probe.mjs`（原生 CDP，无第三方依赖）会真实等待页面解码和冻结完成后再截图。

## 架构

```
src/core.js   纯逻辑，无 DOM / 无定时器 / 无 canvas
  EventBus          通配事件总线，监听器异常被隔离
  Activity          事件流 → stress / urgency / timeScale / tremor
  PoseMixer         参数化姿态 + 临界阻尼弹簧（约 0.15–0.2 s 收敛，不跳帧）
  VerletChain       Verlet 积分 + 距离约束 + 软目标吸引
  Pointer           1D 弹簧，驱动本体在阶段间移动
  ScenarioPlayer    确定性时间线，可暂停 / 恢复 / 倍速
  buildScenario()   绿 / 红两条 mock 事件流

src/scene.js  场景层：DOM 锚点、canvas 绘制、物理循环、控制
styles.css    赛博网格、玻璃面板、门禁状态配色
```

### 规格逐条对应

1. **数据与事件流驱动** — 场景只订阅 `EventBus`，从不直接读文件。`Activity` 维护 `stress`（流量，指数衰减）、`urgency`（阻塞 / 错误密度）和当前阶段指针，并派生出 `timeScale` 与 `tremor`。
2. **骨骼混合** — `PoseMixer` 对 `sx/sy/tilt/spread/bob` 做二阶弹簧插值；切换 idle / running / blocked / verify / fail / done 时是连续过渡，不是换图。播放速率和颤动幅度与 `stress` / `urgency` 绑定。
3. **Verlet 触手** — 六条链，根部钉在身体中心；每个子步先积分重力 + 风，再迭代 7 次距离约束。门禁拦截时末端以更高的拉力锁定到缺失项 DOM 中心；目标滚出视口或消失时自动回到身体周围的挂机轨道。
4. **主链拓扑与物理反馈** — 顶部六阶段拓扑轴，本体作为进度指针在阶段间弹簧移动；质量门打开时逐条吸附 `verify.log` 气泡粒子，`exit 0` 撒绿色通关粒子，RED 时降低阻尼、加重重力，身体下沉。
5. **鲁棒性** — 所有绘制/物理都在 `requestAnimationFrame` 内，`try/catch` 计数，连续 6 次异常就停表并显示 fault，不影响页面其余部分；`visibilitychange` 暂停；`prefers-reduced-motion` 关闭粒子与颤动。

## 与真实 data-workflow 的对应

Demo 用 `buildScenario()` 模拟，接入真实运行时只需把事件映射成同一组 type：

| 真实来源 | 事件 |
|---|---|
| `run.json` 的 `stage` / `status` | `stage.enter` / `run.start` / `run.done` |
| 内容门 `GATE`、`缺：…` | `gate.blocked` + `missing[]`（每项带 DOM 锚点 id） |
| 补齐字段 / `next` 放行 | `anchor.fix` / `gate.passed` |
| `verify.log` 每行 | `verify.line` |
| `verify.log` 首行 `exit N` | `verify.exit` |
| 工具调用（可选） | `tool.call` / `tool.result` |

`data-workflow` 的 `run.json`、`evidence.log`、`verify.log` 已经是结构化文本，Host 侧读取后直接发这些事件即可；Demo 本身不写任何文件。

## 立绘署名

角色立绘来自 **YunYueSama / codex-deepseek-pet**，按其《大肥鱼项目署名许可 1.0》使用。完整署名、修改说明与许可文本见 [`assets/ATTRIBUTION.md`](assets/ATTRIBUTION.md)。

> 注意：本机 `dsh-deepseek-pet` 自带的 `ASSET_LICENSE.md` 把授权范围限定为「仅限本插件」。如果这个 Demo 要对外分发（而不是个人本地使用），请先向作者确认该范围；授权问题不能靠“二次生成”自动解决。

## 已知边界

- 画布是 **Canvas 2D**，不是 WebGL。这个效果是 2D 物理 + 发光曲线，Canvas 2D 足够，且少一整层 WebGL 样板；如果要 3D 景深或大量后处理再换 WebGL。
- 页面只读取**自己的** DOM 锚点，不读取宿主页面或其他插件的 DOM。
- 立绘只有一张（idle），姿势靠参数化形变；没有多张立绘切换，符合“禁止换图”的要求。
- Demo 尚未接入 HUD 的实时 SSE；下一步是把 `scene.js` 的锚点/绘制逻辑搬进 `client.js` 的 `shell.overlay`，用真实 HUD 状态替换 `buildScenario()`。
