#!/usr/bin/env bash
# 编译 data-test-platform (Linux) 并更新到统一部署目录 ../deploy/local/bin/
# 用法: ./deploy/local.sh [GOARCH]   例: ./deploy/local.sh amd64
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "$script_dir/.." && pwd)"
deploy_dir="$(cd "$project_dir/../deploy/local" && pwd)"
bin_dir="$deploy_dir/bin"

case "${1:-${GOARCH:-$(go env GOARCH)}}" in
  x86_64 | amd64) target_arch=amd64 ;;
  arm64 | aarch64) target_arch=arm64 ;;
  *)
    echo "不支持的架构: ${1:-${GOARCH:-$(go env GOARCH)}}" >&2
    exit 1
    ;;
esac

version="$(git -C "$project_dir" describe --tags --exact-match 2>/dev/null ||
  printf 'v%s-%s\n' "$(date +%Y%m%d)" "$(git -C "$project_dir" rev-parse --short=7 HEAD)")"

echo "编译 data-test-platform ($target_arch)"
(
  cd "$project_dir"
  CGO_ENABLED=0 GOOS=linux GOARCH="$target_arch" \
    go build -ldflags "-s -w -X main.Version=$version" -o "$bin_dir/data-test-platform.next" .
)
mv -f "$bin_dir/data-test-platform.next" "$bin_dir/data-test-platform"

echo "部署产物已更新: $bin_dir/data-test-platform"
echo "重启生效: cd $deploy_dir && podman compose restart data-test-platform"
