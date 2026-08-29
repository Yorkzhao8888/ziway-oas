#!/bin/bash
# ============================================
# ziway-ambs API 测试脚本
# 启动服务后运行此脚本验证核心功能
# ============================================

BASE_URL="http://localhost:8080/api/v1"
echo "🧪 ziway-ambs API 测试"
echo "================================"

# 1. 健康检查
echo -e "\n1️⃣  健康检查..."
curl -s "$BASE_URL/../health" | python3 -m json.tool 2>/dev/null || curl -s "http://localhost:8080/health"

# 2. 注册测试用户
echo -e "\n\n2️⃣  注册用户..."
REG_RESULT=$(curl -s -X POST "$BASE_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "13800138000",
    "email": "test@ziway.net",
    "password": "ziway123",
    "nickname": "测试消费者"
  }')
echo "$REG_RESULT" | python3 -m json.tool 2>/dev/null || echo "$REG_RESULT"

# 3. 登录
echo -e "\n3️⃣  登录..."
LOGIN_RESULT=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d '{
    "account": "13800138000",
    "password": "ziway123",
    "domain": "mall"
  }')
echo "$LOGIN_RESULT" | python3 -m json.tool 2>/dev/null || echo "$LOGIN_RESULT"

# 提取access_token
TOKEN=$(echo "$LOGIN_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])" 2>/dev/null)

if [ -n "$TOKEN" ]; then
    echo -e "\n✅ Token获取成功"

    # 4. 获取当前用户信息
    echo -e "\n4️⃣  当前用户信息..."
    curl -s "$BASE_URL/auth/me" \
      -H "Authorization: Bearer $TOKEN" | python3 -m json.tool 2>/dev/null

    # 5. 切换帽子（切换到shop事业场）
    echo -e "\n5️⃣  切换帽子（shop）..."
    curl -s -X POST "$BASE_URL/auth/switch-hat" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      -d '{"domain":"mall","role_code":"CU"}' | python3 -m json.tool 2>/dev/null

    # 6. 内部接口 - 签发NHI Token（Agent身份）
    echo -e "\n6️⃣  签发NHI Token（Agent身份）..."
    USER_ID=$(echo "$LOGIN_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['user']['user_id'])" 2>/dev/null)
    curl -s -X POST "$BASE_URL/internal/agent-token" \
      -H "X-Service-Token: dev-service-token-change-in-prod" \
      -H "Content-Type: application/json" \
      -d "{
        \"agent_service\": \"ziway-Agent\",
        \"delegated_by\": \"$USER_ID\"
      }" | python3 -m json.tool 2>/dev/null

    # 7. OAS策略快照
    echo -e "\n7️⃣  OAS策略快照..."
    curl -s "$BASE_URL/internal/policy-snapshot" \
      -H "X-Service-Token: dev-service-token-change-in-prod" | python3 -m json.tool 2>/dev/null

    # 8. 登出
    echo -e "\n8️⃣  登出..."
    curl -s -X POST "$BASE_URL/auth/logout" \
      -H "Authorization: Bearer $TOKEN" | python3 -m json.tool 2>/dev/null

    echo -e "\n✅ 所有API测试完成"
else
    echo -e "\n❌ Token获取失败，跳过后续测试"
fi

echo -e "\n================================"
echo "📊 测试完成"
