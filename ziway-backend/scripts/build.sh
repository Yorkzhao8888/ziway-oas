#!/bin/bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

export GOPROXY=https://goproxy.cn,direct

mkdir -p bin

echo "Building ziway-oas..."
go build -o bin/oas ./cmd/oas/

echo "Building ziway-ms..."
go build -o bin/ms ./cmd/ms/

echo "Building ziway-os..."
go build -o bin/os ./cmd/os/

echo "Build complete: bin/oas bin/ms bin/os"
