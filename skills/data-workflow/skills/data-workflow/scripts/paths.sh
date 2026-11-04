#!/usr/bin/env bash
# paths.sh —— 一次性解析本框架用到的所有外部路径，输出可 eval 的赋值行。
#
# 用法（在技能里）:
#   eval "$(bash "$PLUGIN/scripts/paths.sh")"
#   → 得到 $KNOW $CARDS $DOMAIN_DOCS $TEMPLATES $REPO $DOMAINS $TECH_DESIGN_TEMPLATE $MR_TEMPLATE $PERSONAL_AGENTS_MD
#
# 未配置的键输出空串，并在 stderr 打印 MISSING，由调用方决定「问用户」还是「声明降级」。
# 本脚本绝不猜绝对路径。

set -uo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
resolve() { bash "$script_dir/resolve.sh" "$1" 2>/dev/null; }

emit() { # emit <VAR> <key>
  local val; val="$(resolve "$2")"
  [ -z "$val" ] && printf 'MISSING %s 未配置（config.yaml: %s）\n' "$1" "$2" >&2
  printf "%s='%s'\n" "$1" "$(printf '%s' "$val" | sed "s/'/'\\\\''/g")"
}

emit KNOW knowledge_root
emit TEMPLATES templates_root
emit REPO repo
emit DOMAINS domains
emit TECH_DESIGN_TEMPLATE tech_design_template
emit MR_TEMPLATE mr_template
emit PERSONAL_AGENTS_MD personal_agents_md

# cards_dir：显式配置优先；否则约定 <knowledge_root>/排查卡片（目录存在才用）。
cards="$(resolve cards_dir)"
if [ -z "$cards" ]; then
  know="$(resolve knowledge_root)"
  [ -n "$know" ] && [ -d "$know/排查卡片" ] && cards="$know/排查卡片"
fi
[ -z "$cards" ] && printf 'MISSING CARDS 未配置（config.yaml: cards_dir）\n' >&2
printf "CARDS='%s'\n" "$(printf '%s' "$cards" | sed "s/'/'\\\\''/g")"

# domain_docs：仓库内业务域文档目录。只有 <repo>/docs/domain 真存在才给值；不存在就空，不建目录。
# 优先级高于 $KNOW：仓内那份随代码提交、能 review、能判新鲜度。
repo_val="$(resolve repo)"
if [ -z "$repo_val" ]; then
  repo_val="$(git -C "$PWD" rev-parse --show-toplevel 2>/dev/null || true)"
fi
domain_docs=""
if [ -n "$repo_val" ] && [ -d "$repo_val/docs/domain" ]; then
  domain_docs="$(cd "$repo_val/docs/domain" && pwd -P)"
fi
printf "DOMAIN_DOCS='%s'\n" "$(printf '%s' "$domain_docs" | sed "s/'/'\\\\''/g")"
