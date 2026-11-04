# 图

`assets/architecture.svg` 和 `assets/flow.svg` 是手写 SVG，PNG 由它转出。

```bash
bash skills/data-workflow/scripts/svg_to_png.sh assets/architecture.svg assets/architecture.png 2
bash skills/data-workflow/scripts/svg_to_png.sh assets/flow.svg assets/flow.png 2
```

## 两种改法

**当前两张图都是手写 SVG，不是生成器产的。** 需要改文字或排版就直接编辑 `.svg`，再跑上面两条转 PNG。SVG 是纯文本，git diff 可读，评审能看出改了哪句。

`.staging/*.spec.json` 是早期用生成器起稿留下的，**已与当前 SVG 不同源**。只在要从零重画时用它——生成器只排等宽卡片网格，适合起稿，做带层级的版式需在它产出的 SVG 上手改，或直接新写。

`.excalidraw` 可拖进 excalidraw.com 手改。PNG 进 git，不跑脚本也能看图。

## 注意

- 生成器依赖 `rsvg-convert`，本机可能没装。它照样打印 `✓ ...png`，但文件不存在。`svg_to_png.sh` 用 headless Chrome 补这一步，缺 Chrome 时打印 MISSING 并以 0 退出（SVG 仍可用）。
- 图里出现的文件名、脚本名、退出码语义都要能在仓库里 grep 到。写图前先验证，别凭记忆填。

当前版本 v0.3：补入四类门、`approve` 放行、`change` 三步、`evidence.log`，并修正第一屏「五行是硬门、七行是模板要求」的区分。
