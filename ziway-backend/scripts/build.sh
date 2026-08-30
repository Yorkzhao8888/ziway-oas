#!/bin/bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

mkdir -p bin

# Check if pre-compiled binaries exist in dist/
if [ -f "dist/oas" ] && [ -f "dist/ms" ] && [ -f "dist/os" ]; then
    echo "Using pre-compiled binaries from dist/"
    cp dist/oas bin/oas
    cp dist/ms bin/ms
    cp dist/os bin/os
    chmod +x bin/oas bin/ms bin/os
    echo "Build complete (pre-compiled): bin/oas bin/ms bin/os"
    exit 0
fi

# Fallback: compile from source if Go is available
if command -v go &> /dev/null; then
    export GOPROXY=https://goproxy.cn,direct
    echo "Go found, compiling from source..."
    echo "Go version: $(go version)"
    
    echo "Building ziway-oas..."
    go build -o bin/oas ./cmd/oas/
    
    echo "Building ziway-ms..."
    go build -o bin/ms ./cmd/ms/
    
    echo "Building ziway-os..."
    go build -o bin/os ./cmd/os/
    
    echo "Build complete: bin/oas bin/ms bin/os"
else
    echo "ERROR: No pre-compiled binaries in dist/ and Go not found"
    echo "Please either:"
    echo "  1. Commit pre-compiled binaries to dist/"
    echo "  2. Install Go compiler"
    exit 1
fi
