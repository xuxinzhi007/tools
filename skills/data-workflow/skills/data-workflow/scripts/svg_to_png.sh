#!/usr/bin/env bash
# svg_to_png.sh —— 把 diagram_generator.js 产出的 SVG 转成 PNG。
#
# 为什么要这个脚本：diagram_generator.js 依赖 rsvg-convert，本机没装，
# 它照样会打印 "✓ ... .png"，但文件根本不存在。这个脚本用 headless Chrome 补上那一步。
#
# 用法: svg_to_png.sh <in.svg> <out.png> [scale]     # scale 默认 2
set -euo pipefail

in_svg="${1:?用法: svg_to_png.sh <in.svg> <out.png> [scale]}"
out_png="${2:?缺少输出路径}"
scale="${3:-2}"

CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
if [ ! -x "$CHROME" ]; then
  printf 'MISSING 找不到 Chrome: %s\n' "$CHROME"
  printf '        SVG 仍然可用（GitHub 与 Obsidian 都能直接渲染），只是没有 PNG。\n'
  exit 0
fi

# 从 viewBox 读画布尺寸，再乘 scale 得到窗口尺寸
read -r w h < <(sed -n 's/.*viewBox="0 0 \([0-9]*\) \([0-9]*\)".*/\1 \2/p' "$in_svg" | head -1)
if [ -z "${w:-}" ] || [ -z "${h:-}" ]; then
  printf 'UNPARSEABLE 读不出 viewBox: %s\n' "$in_svg"
  exit 2
fi

"$CHROME" --headless=new --disable-gpu --hide-scrollbars --no-sandbox \
  --force-device-scale-factor="$scale" \
  --window-size="${w},${h}" \
  --default-background-color=FFFFFFFF \
  --screenshot="$out_png" \
  "file://$(cd "$(dirname "$in_svg")" && pwd)/$(basename "$in_svg")" >/dev/null 2>&1

if [ -s "$out_png" ]; then
  printf 'OK %s (%sx%s @%sx)\n' "$out_png" "$w" "$h" "$scale"
else
  printf 'FAILED 转换没产出文件: %s\n' "$out_png"
  exit 2
fi
