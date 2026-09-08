#!/bin/bash
# ============================================
# ziway-ambs 一键编译运行脚本
# 知味生态全栈OS v4.9 - 原子能力层
# ============================================
set -e

echo "🔧 ziway-ambs 一键编译运行"
echo "================================"

# 检查Go
if ! command -v go &> /dev/null; then
    echo "❌ 未找到 Go 编译器"
    echo "   安装: https://go.dev/dl/"
    echo "   或:   brew install go (macOS)"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}')
echo "✅ Go $GO_VERSION"

# 进入项目目录
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# 设置中国镜像加速（可选，海外用户可注释掉）
export GOPROXY=https://goproxy.cn,direct

# 下载依赖
echo "📦 下载依赖..."
go mod tidy

# 编译
echo "🔨 编译..."
CGO_ENABLED=0 go build -o bin/ziway-ambs ./cmd/server/
echo "✅ 编译成功: bin/ziway-ambs"

# 生成RSA密钥（如果不存在）
if [ ! -f configs/keys/private.pem ]; then
    echo "🔑 生成RSA密钥对..."
    mkdir -p configs/keys
    openssl genrsa -out configs/keys/private.pem 2048 2>/dev/null
    openssl rsa -in configs/keys/private.pem -pubout -out configs/keys/public.pem 2>/dev/null
    echo "✅ RSA密钥已生成"
fi

# 用SQLite模式启动（无需PostgreSQL和Redis）
echo ""
echo "🚀 启动 ziway-ambs (SQLite模式)..."
echo "   HTTP: http://localhost:8080"
echo "   gRPC: localhost:50051"
echo "   健康检查: curl http://localhost:8080/health"
echo ""
echo "   按 Ctrl+C 停止"
echo "================================"

APP_ENV=sqlite ./bin/ziway-ambs
