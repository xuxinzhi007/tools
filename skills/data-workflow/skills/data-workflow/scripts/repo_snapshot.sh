#!/usr/bin/env bash
set -euo pipefail

# 快照的是「当前工作目录所在的仓库」，不是本脚本所在的插件仓库。
repo_dir="${DS_REPO:-$(git -C "$(pwd)" rev-parse --show-toplevel)}"

printf 'repository=%s\n' "$repo_dir"
printf 'branch=%s\n' "$(git -C "$repo_dir" branch --show-current)"
printf 'commit=%s\n' "$(git -C "$repo_dir" rev-parse HEAD)"
printf 'verified_at=%s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
printf 'status_begin\n'
git -C "$repo_dir" status --short --branch
printf 'status_end\n'

