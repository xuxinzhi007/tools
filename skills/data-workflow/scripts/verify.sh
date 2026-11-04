#!/usr/bin/env bash
# 本仓库声明的测试入口。config.yaml 的 verify 指向这里，workflow_next 的 done 会执行它。
# 装进别的仓库时不需要这个文件——那个仓库用它自己的入口（package.json / Makefile / pytest / cargo）。
set -uo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# 不直接执行 check_runner.py：Windows 上 shebang 指向的 /usr/bin/python3 常常不存在。
# shellcheck source=../skills/data-workflow/scripts/find_python.sh
. "$repo/skills/data-workflow/scripts/find_python.sh"

if ! py="$(dw_python)"; then
  printf '找不到可用的 python（试过 py -3 / python3 / python）\n'
  exit 2
fi

# $py 可能是 "py -3"，不要加引号。
# shellcheck disable=SC2086
exec $py "$repo/skills/data-workflow/scripts/check_runner.py"
