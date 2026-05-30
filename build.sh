#!/usr/bin/env bash
# 在 Ubuntu 上编译 fzf-web 可执行文件
# 用法:
#   ./build.sh
#   OUTPUT_DIR=dist BINARY_NAME=fzf-web ./build.sh
#   GOARCH=arm64 ./build.sh          # ARM 机器

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

OUTPUT_DIR="${OUTPUT_DIR:-bin}"
BINARY_NAME="${BINARY_NAME:-fzf-web}"
GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"

if ! command -v go >/dev/null 2>&1; then
  echo "错误: 未找到 go，请先安装 Go (https://go.dev/dl/)" >&2
  exit 1
fi

mkdir -p "$OUTPUT_DIR"

echo "==> 下载依赖..."
go mod download

echo "==> 编译 ${BINARY_NAME} (${GOOS}/${GOARCH}) ..."
CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build \
  -trimpath \
  -ldflags="-s -w" \
  -o "${OUTPUT_DIR}/${BINARY_NAME}" \
  ./cmd

chmod +x "${OUTPUT_DIR}/${BINARY_NAME}"

echo "==> 完成: ${ROOT}/${OUTPUT_DIR}/${BINARY_NAME}"
echo "    运行示例: ${OUTPUT_DIR}/${BINARY_NAME} -d /path/to/search -p 8080"
