#!/usr/bin/env bash
# find_python.sh —— 找一台机器上真正能跑的 python。找到就打印命令，找不到返回非 0。
#
# 为什么要这个文件：v0.2 的 hook 与脚本用 `#!/usr/bin/python3` 直接执行，
# Windows 上 `python3` 常解析到 Microsoft Store 的占位程序（跑什么都不输出，
# exit 49），`py` 也可能指向一个已删除的解释器（exit 101）。hook 因此没有
# stdout，而 Cursor 的 failClosed 把「没有输出」当成「拒绝」，于是所有写入被拦。
# 所以这里不信任 `command -v`：每个候选都**实跑一次** `-c "import sys"`。
#
# 顺序：$DATA_WORKFLOW_PYTHON > py -3 > python3 > python > PATH 目录里的 python.exe
# 最后一步是为了那种「解释器目录在 PATH 上，但被占位程序在名字上抢先」的机器。
#
# 用法:
#   . "$PLUGIN_ROOT/scripts/find_python.sh"
#   py="$(dw_python)" || { echo "没有可用的 python"; exit 2; }
#   $py <script.py>            # 不要加引号：结果可能是 "py -3"，也可能是绝对路径

dw_python() {
  local candidate dir
  local old_ifs="$IFS"

  if [ -n "${DATA_WORKFLOW_PYTHON:-}" ] &&
    $DATA_WORKFLOW_PYTHON -c 'import sys; sys.exit(0)' >/dev/null 2>&1; then
    printf '%s\n' "$DATA_WORKFLOW_PYTHON"
    return 0
  fi

  for candidate in "py -3" python3 python; do
    if command -v "${candidate%% *}" >/dev/null 2>&1 &&
      $candidate -c 'import sys; sys.exit(0)' >/dev/null 2>&1; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done

  IFS=:
  for dir in $PATH; do
    [ -n "$dir" ] || continue
    for candidate in "$dir/python.exe" "$dir/python3" "$dir/python"; do
      if [ -f "$candidate" ] && "$candidate" -c 'import sys; sys.exit(0)' >/dev/null 2>&1; then
        IFS="$old_ifs"
        printf '%s\n' "$candidate"
        return 0
      fi
    done
  done
  IFS="$old_ifs"
  return 1
}
