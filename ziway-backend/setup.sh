#!/bin/bash
# ============================================
# 知味生态全栈OS v4.9 - 一键部署
# 在你的 Mac 终端里执行: bash setup.sh
# ============================================
set -e

echo "🚀 知味生态全栈OS v4.9 - 后端部署"
echo "===================================="

# 1. 检查Go
if ! command -v go &> /dev/null; then
    echo "❌ 请先安装Go: brew install go"
    exit 1
fi
echo "✅ Go $(go version | awk '{print $3}')"

# 2. 设置镜像
export GOPROXY=https://goproxy.cn,direct
echo "✅ GOPROXY=$GOPROXY"

# 3. 进入项目目录
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# 4. 下载主模块依赖
echo ""
echo "📥 下载Go依赖（主模块）..."
go mod tidy

# 5. 生成RSA密钥（AMBS P1 独立服务需要）
echo ""
echo "🔑 生成JWT密钥..."
mkdir -p services/ambs/configs/keys
if [ ! -f services/ambs/configs/keys/private.pem ]; then
    openssl genrsa -out services/ambs/configs/keys/private.pem 2048 2>/dev/null
    openssl rsa -in services/ambs/configs/keys/private.pem -pubout -out services/ambs/configs/keys/public.pem 2>/dev/null
    echo "✅ RSA密钥已生成"
else
    echo "✅ RSA密钥已存在"
fi

# 6. 创建目录
mkdir -p bin logs

# 7. 编译 P0 单体服务（cmd/bos + cmd/mbs + cmd/oas）
echo ""
echo "🔨 编译 P0 服务..."
for svc in bos mbs oas; do
    printf "  → ziway-%-10s " "$svc"
    if go build -o "bin/ziway-$svc" "./cmd/$svc/" 2>/dev/null; then
        echo "✅"
    else
        echo "⚠️ (需要修复)"
    fi
done

# 8. 编译 AMBS P1 独立服务
echo ""
echo "🔨 编译 AMBS（P1 独立部署）..."
cd services/ambs
go mod tidy 2>/dev/null
printf "  → ambs           "
if go build -o "$SCRIPT_DIR/bin/ambs" ./cmd/server/ 2>/dev/null; then
    echo "✅"
else
    echo "⚠️ (需要修复)"
fi
cd "$SCRIPT_DIR"

echo ""
echo "===================================="
echo "✅ 部署完成！"
echo ""
echo "P0 启动（开发模式）:"
echo "  APP_ENV=sqlite ./bin/ziway-mbs    # 12 MBS 统一服务 :8081"
echo "  APP_ENV=sqlite ./bin/ziway-bos    # 12 BOS 编排/直通 :8080"
echo "  APP_ENV=sqlite ./bin/ziway-oas    # OAS 治理底座 :8082"
echo ""
echo "P1 独立服务:"
echo "  ./bin/ambs                        # AMBS 鉴权（独立Go模块）"
echo ""
echo "健康检查:"
echo "  curl http://localhost:8081/health"
echo "  curl http://localhost:8080/health"
echo ""
echo "完整开发环境(可选):"
echo "  docker-compose up -d   # 启动 PG + Redis + Kafka"
echo "===================================="
