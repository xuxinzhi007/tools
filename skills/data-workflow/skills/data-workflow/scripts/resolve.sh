#!/usr/bin/env bash
# resolve.sh <key> —— 解析配置键。stdout 只输出值（可能为空），来源说明走 stderr。
#
# 优先级: DATA_WORKFLOW_<KEY 大写> 环境变量
#         > <项目>/.cursor|qoder|agents|claude|codex/skills/data-workflow/config.yaml
#         > <当前 git 根>/config.yaml
#         > ~/.config/data-workflow/config.yaml > 空（不猜）。
# config.yaml 和技能装在同一个目录里，不读技能仓库里的那份。
# 为什么要有这个脚本：v1 把知识源硬编码成某台机器的 Obsidian 绝对路径（14 处），
# 换机器/换知识源就要改技能正文。配置化之后技能正文只写「resolve.sh <key>」。
#
# 用法: resolve.sh <key>
#   key: knowledge_root | templates_root | cards_dir | repo | tech_design_template | mr_template | personal_agents_md
set -uo pipefail

key="${1:?用法: resolve.sh <key>   # knowledge_root templates_root cards_dir repo tech_design_template mr_template personal_agents_md}"

env_key="DATA_WORKFLOW_$(printf '%s' "$key" | tr '[:lower:]' '[:upper:]')"
if [ -n "${!env_key:-}" ]; then
  printf '%s\n' "${!env_key}"
  printf 'resolve: %s 来自环境变量 %s\n' "$key" "$env_key" >&2
  exit 0
fi

yaml_get() { # yaml_get <file> <key> —— 扁平 key: value，去掉引号与行尾注释
  [ -f "$1" ] || return 0
  awk -v k="$2" '
    $0 ~ "^" k ":" {
      sub("^" k ":[[:space:]]*", "")
      gsub(/^"|"$/, "")
      gsub(/^\047|\047$/, "")
      sub(/[ \t]+#.*$/, "")
      sub(/[ \t]+$/, "")
      print
      exit
    }' "$1"
}

project_root="$PWD"
if git_root="$(git -C "$PWD" rev-parse --show-toplevel 2>/dev/null)"; then
  project_root="$git_root"
fi
configs=()
for tool in .cursor .qoder .agents .claude .codex; do
  configs+=("$project_root/$tool/skills/data-workflow/config.yaml")
done
configs+=("$project_root/config.yaml" "$HOME/.config/data-workflow/config.yaml")
for cfg in "${configs[@]}"; do
  val="$(yaml_get "$cfg" "$key")"
  if [ -n "$val" ]; then
    printf '%s\n' "$val"
    printf 'resolve: %s 来自 %s\n' "$key" "$cfg" >&2
    exit 0
  fi
done

printf 'resolve: %s 未配置（env / <项目>/<工具>/skills/data-workflow/config.yaml / <项目>/config.yaml / ~/.config/data-workflow/config.yaml 均无）\n' "$key" >&2
exit 0
