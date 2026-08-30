#!/bin/bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

export GOPROXY=https://goproxy.cn,direct

# Check if Go is installed, install if not
if ! command -v go &> /dev/null; then
    echo "Go not found, installing..."
    if command -v apt-get &> /dev/null; then
        apt-get update -qq && apt-get install -y -qq golang-go
    elif command -v apk &> /dev/null; then
        apk add --no-cache go
    else
        echo "ERROR: Cannot install Go - package manager not found"
        exit 1
    fi
fi

echo "Go version: $(go version)"

mkdir -p bin

echo "Building ziway-oas..."
go build -o bin/oas ./cmd/oas/

echo "Building ziway-ms..."
go build -o bin/ms ./cmd/ms/

echo "Building ziway-os..."
go build -o bin/os ./cmd/os/

echo "Build complete: bin/oas bin/ms bin/os"
