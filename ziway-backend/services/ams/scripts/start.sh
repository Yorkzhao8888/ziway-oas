#!/bin/bash
# ziway-AMBS 一键启动脚本
set -e

echo "=========================================="
echo "  ziway-AMBS 知味生态鉴权服务"
echo "=========================================="

# 1. 检查Go环境
if ! command -v go &> /dev/null; then
    echo "❌ Go 未安装，请先安装 Go 1.22+: https://go.dev/dl/"
    exit 1
fi
echo "✅ Go $(go version | grep -oP 'go\S+')"

# 2. 检查Docker
if ! command -v docker &> /dev/null; then
    echo "⚠️  Docker 未安装，需要手动启动 PostgreSQL 和 Redis"
else
    echo "✅ Docker 已安装"
    # 启动基础设施
    echo ""
    echo "📦 启动 PostgreSQL + Redis..."
    docker compose up -d postgres redis 2>/dev/null || docker-compose up -d postgres redis
    echo "⏳ 等待数据库就绪..."
    sleep 5
fi

# 3. 生成RSA密钥
if [ ! -f configs/keys/private.pem ]; then
    echo ""
    echo "🔑 生成 RS256 密钥对..."
    mkdir -p configs/keys
    openssl genrsa -out configs/keys/private.pem 2048 2>/dev/null
    openssl rsa -in configs/keys/private.pem -pubout -out configs/keys/public.pem 2>/dev/null
    echo "✅ 密钥已生成到 configs/keys/"
else
    echo "✅ RSA 密钥已存在"
fi

# 4. 安装依赖
echo ""
echo "📥 下载 Go 依赖..."
go mod tidy

# 5. 启动服务
echo ""
echo "=========================================="
echo "  启动 AMBS 服务..."
echo "  HTTP:  http://localhost:8080"
echo "  gRPC:  localhost:50051"
echo "  Health: http://localhost:8080/health"
echo "=========================================="
echo ""

go run cmd/server/main.go
