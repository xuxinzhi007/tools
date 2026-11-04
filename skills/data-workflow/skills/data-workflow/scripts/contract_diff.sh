#!/usr/bin/env bash
# contract_diff.sh —— 契约破坏性变更检测（proto 事实源，不依赖 oasdiff / buf / apifox CLI）
#
# 用法:
#   contract_diff.sh [base]        # base 默认 main，实际基线取 merge-base(base, HEAD)
#   contract_diff.sh main api/     # 只比某个子目录的 proto
#
# 为什么比 proto 不比 openapi.yaml：openapi.yaml 在本仓是 gitignore 的生成物（.gitignore 里有它），
# 取不到历史基线；proto 是 tracked 的事实源，`git archive <base>` 就能拿基线。
#
# 基线 = merge-base 的 proto；对比面 = **工作区**（含未提交改动），所以改完 proto 不提交也能测。
# 提取器 scripts/proto_contract.py 把 proto 归一化成 RPC / REQ / RES / FIELD / ENUMVAL 事实行。
#
# 门禁永不阻塞：恒 exit 0。exit 2 仅表示无法比较（找不到仓库、基线没有 proto、缺 python3）。
# 判定权在技能：清单非空 = 需要人拍板，不是脚本说了算。

set -uo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
base="main"
subdir=""
for a in "$@"; do
  case "$a" in
    -*) printf 'UNKNOWN 未知参数 %s\n' "$a"; exit 2 ;;
    *.proto|api/*|api|third_party*) subdir="$a" ;;
    *) base="$a" ;;
  esac
done

command -v python3 >/dev/null 2>&1 || { printf 'UNPARSEABLE 缺 python3（提取器需要）\n'; exit 2; }

repo="$(bash "$script_dir/resolve.sh" repo 2>/dev/null)"
[ -n "$repo" ] || repo="$(git -C "$(pwd)" rev-parse --show-toplevel 2>/dev/null || true)"
if [ -z "$repo" ] || ! git -C "$repo" rev-parse --show-toplevel >/dev/null 2>&1; then
  printf 'UNPARSEABLE 找不到被测仓库（config.yaml: repo / 当前目录）\n'; exit 2
fi
cd "$repo" || exit 2

mb="$(git merge-base "$base" HEAD 2>/dev/null)"
if [ -z "$mb" ]; then
  printf 'UNPARSEABLE 基线 %s 与 HEAD 没有共同祖先（或 %s 不存在）\n' "$base" "$base"; exit 2
fi

paths="api third_party"
[ -n "$subdir" ] && paths="$subdir"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/base" "$tmp/head"

# 基线：merge-base 的 proto（路径不存在就跳过，全都没有才算失败）
got=0
for p in $paths; do
  if git cat-file -e "$mb:$p" 2>/dev/null; then
    git archive "$mb" "$p" 2>/dev/null | tar -x -C "$tmp/base" 2>/dev/null && got=1
  fi
done
if [ "$got" -eq 0 ]; then
  printf 'UNPARSEABLE 基线 %s 里取不到 proto（%s）\n' "${mb:0:8}" "$paths"; exit 2
fi
# 对比面：工作区（含未提交改动）
for p in $paths; do
  [ -d "$p" ] && cp -R "$p" "$tmp/head/" 2>/dev/null
done

python3 "$script_dir/proto_contract.py" "$tmp/base" > "$tmp/base.txt" 2>"$tmp/err" || {
  printf 'UNPARSEABLE 基线提取失败: %s\n' "$(head -3 "$tmp/err")"; exit 2; }
python3 "$script_dir/proto_contract.py" "$tmp/head" > "$tmp/head.txt" 2>>"$tmp/err" || {
  printf 'UNPARSEABLE 工作区提取失败: %s\n' "$(head -3 "$tmp/err")"; exit 2; }

printf 'CONTRACT base=%s(%s) head=工作区  事实行 %s→%s\n' \
  "$base" "${mb:0:8}" "$(wc -l < "$tmp/base.txt" | tr -d ' ')" "$(wc -l < "$tmp/head.txt" | tr -d ' ')"

grep -v '^FILE ' "$tmp/base.txt" > "$tmp/b.facts"; grep -v '^FILE ' "$tmp/head.txt" > "$tmp/h.facts"

# --- 端点与 rpc -------------------------------------------------------------
grep '^RPC ' "$tmp/b.facts" | sort > "$tmp/b.rpc"; grep '^RPC ' "$tmp/h.facts" | sort > "$tmp/h.rpc"
cut -d' ' -f2 "$tmp/b.rpc" | sort -u > "$tmp/b.rpcnames"; cut -d' ' -f2 "$tmp/h.rpc" | sort -u > "$tmp/h.rpcnames"

removed_rpc="$(comm -23 "$tmp/b.rpcnames" "$tmp/h.rpcnames")"
changed_rpc="$(comm -23 "$tmp/b.rpc" "$tmp/h.rpc" | while read -r _ name rest; do
  grep -qxF "$name" "$tmp/h.rpcnames" && printf '%s: %s → %s\n' "$name" "$rest" \
    "$(grep -F " $name " "$tmp/h.rpc" | cut -d' ' -f3- | paste -sd'; ' -)"
done)"
added_rpc="$(comm -13 "$tmp/b.rpcnames" "$tmp/h.rpcnames" | wc -l | tr -d ' ')"

# --- 字段 -------------------------------------------------------------------
grep '^FIELD ' "$tmp/b.facts" | sort > "$tmp/b.fld"; grep '^FIELD ' "$tmp/h.facts" | sort > "$tmp/h.fld"
# 以 消息+字段号 为键：键在、名字或类型变了 = 线上不兼容
awk '{print $2"."$3}' "$tmp/b.fld" | sort -u > "$tmp/b.keys"
join -j1 -o 1.1,1.2,2.2 \
  <(awk '{print $2"."$3, $4"/"$5}' "$tmp/b.fld" | sort -u) \
  <(awk '{print $2"."$3, $4"/"$5}' "$tmp/h.fld" | sort -u) \
  | awk '$2 != $3 {printf "%s: %s → %s\n", $1, $2, $3}' > "$tmp/renumbered"

removed_field="$(join -v1 -j1 \
  <(awk '{print $2"."$3, $0}' "$tmp/b.fld" | sort -k1,1) \
  <(awk '{print $2"."$3}' "$tmp/h.fld" | sort -u) | cut -d' ' -f2-)"
added_field="$(join -v1 -j1 \
  <(awk '{print $2"."$3}' "$tmp/h.fld" | sort -u) \
  <(awk '{print $2"."$3, $0}' "$tmp/b.fld" | sort -k1,1) | wc -l | tr -d ' ')"

# --- 枚举 -------------------------------------------------------------------
grep '^ENUMVAL ' "$tmp/b.facts" | sort > "$tmp/b.enum"; grep '^ENUMVAL ' "$tmp/h.facts" | sort > "$tmp/h.enum"
removed_enum="$(comm -23 "$tmp/b.enum" "$tmp/h.enum")"
reused_enum="$(join -j1 -o 1.1,1.2,2.2 \
  <(awk '{print $2"."$3, $4}' "$tmp/b.enum" | sort -u) \
  <(awk '{print $2"."$3, $4}' "$tmp/h.enum" | sort -u) \
  | awk '$2 != $3 {printf "%s: %s → %s\n", $1, $2, $3}')"

# --- proto 文件 -------------------------------------------------------------
removed_files="$(comm -23 <(grep '^FILE ' "$tmp/base.txt" | cut -d' ' -f2 | sort) \
                          <(grep '^FILE ' "$tmp/head.txt" | cut -d' ' -f2 | sort))"

show() { [ -n "$2" ] && { printf '\n%s\n' "$1"; printf '%s\n' "$2" | sed "s/^/  $3 /"; }; }
show 'BREAKING 删除的 rpc（调用方会直接 404 / Unimplemented）' "$removed_rpc" '-'
show 'BREAKING 端点变更（同一 rpc 的 method 或 path 变了）' "$changed_rpc" '~'
show 'BREAKING 删除的字段' "$removed_field" '-'
show 'BREAKING 字段号复用/改名/改类型（同一 message+number，线上不兼容）' "$(cat "$tmp/renumbered")" '~'
show 'BREAKING 删除的枚举值' "$removed_enum" '-'
show 'BREAKING 枚举号复用（同一 enum+number 换了名字）' "$reused_enum" '~'
show 'BREAKING 删除的 proto 文件' "$removed_files" '-'

printf '\nADDED（非 breaking，仅记录）: rpc +%s  字段 +%s\n' "$added_rpc" "$added_field"

if [ -z "$removed_rpc$changed_rpc$removed_field$removed_enum$reused_enum$removed_files" ] && [ ! -s "$tmp/renumbered" ]; then
  printf 'OK 未检测到删除 rpc / 端点变更 / 删字段 / 字段号复用 / 删枚举 / 枚举号复用\n'
else
  printf 'NEEDS-RULING 上面每一行都要在产物里写「谁需要跟着改」或明确判为可接受\n'
fi

exit 0
