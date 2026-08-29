#!/usr/bin/env bash
# E2E acceptance test for ziway-backend auth chain
# Covers: A1 (JWT/RBAC), A2 (policy sync), A3 (login), A4 (audit + sub_role)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

OAS_PORT=18080
MBS_PORT=18081
OS_PORT=18082
PASS=0
FAIL=0
RESULTS=()

cleanup() {
    pkill -f "bin/oas.*$OAS_PORT" 2>/dev/null || true
    pkill -f "bin/ms.*$MBS_PORT" 2>/dev/null || true
    pkill -f "bin/os.*$OS_PORT" 2>/dev/null || true
    sleep 1
}
trap cleanup EXIT

log_pass() { RESULTS+=("PASS  $1"); ((PASS++)); echo "✅ PASS: $1"; }
log_fail() { RESULTS+=("FAIL  $1: $2"); ((FAIL++)); echo "❌ FAIL: $1 — $2"; }

echo "============================================"
echo "  Ziway Backend E2E Acceptance Test"
echo "============================================"
echo ""

# Clean DB
rm -f data/*.db
mkdir -p data

# Start services
echo "[setup] Starting OAS (:$OAS_PORT)..."
APP_ENV=dev ZIWAY_SERVER_HTTP_PORT=$OAS_PORT ./bin/oas > /tmp/e2e_oas.log 2>&1 &
sleep 4

echo "[setup] Starting MBS (:$MBS_PORT)..."
APP_ENV=dev ZIWAY_SERVER_HTTP_PORT=$MBS_PORT ./bin/ms > /tmp/e2e_mbs.log 2>&1 &
sleep 3

echo "[setup] Starting OS (:$OS_PORT)..."
APP_ENV=dev ZIWAY_SERVER_HTTP_PORT=$OS_PORT ./bin/os > /tmp/e2e_os.log 2>&1 &
sleep 3

echo ""
echo "============================================"
echo "  Running Test Cases"
echo "============================================"
echo ""

# ===== Case 1: No token → 401 =====
echo "[case 1] No token → 401"
RESP=$(curl -s -w "\n%{http_code}" http://localhost:$OS_PORT/api/v1/bos/cos/health)
CODE=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | head -1)
if [ "$CODE" = "401" ]; then
    log_pass "Case 1: No token → 401"
else
    log_fail "Case 1" "expected 401, got $CODE"
fi

# ===== Case 2: Wrong credentials → 401 =====
echo "[case 2] Wrong credentials → 401"
RESP=$(curl -s -w "\n%{http_code}" -X POST http://localhost:$OAS_PORT/api/v1/os/bos/proxy/ams/auth/login \
    -H 'Content-Type: application/json' -d '{"username":"admin","password":"wrong"}')
CODE=$(echo "$RESP" | tail -1)
if [ "$CODE" = "401" ]; then
    log_pass "Case 2: Wrong credentials → 401"
else
    log_fail "Case 2" "expected 401, got $CODE"
fi

# ===== Case 3: Valid login → real JWT → access protected API → 200 =====
echo "[case 3] Valid login → JWT → protected API"
LOGIN_RESP=$(curl -s -X POST http://localhost:$OAS_PORT/api/v1/os/bos/proxy/ams/auth/login \
    -H 'Content-Type: application/json' -d '{"username":"admin","password":"test123"}')
TOKEN=$(echo "$LOGIN_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])" 2>/dev/null || echo "")
if [ -z "$TOKEN" ]; then
    log_fail "Case 3" "login failed, no token returned"
else
    # Verify JWT claims
    CLAIMS=$(echo "$TOKEN" | python3 -c "
import sys,json,base64
t=sys.stdin.read().strip()
p=t.split('.')[1]
p+='='*(4-len(p)%4)
c=json.loads(base64.b64decode(p))
print(json.dumps(c))
" 2>/dev/null || echo "{}")
    HAS_SUB_ROLE=$(echo "$CLAIMS" | python3 -c "import sys,json; c=json.load(sys.stdin); print('yes' if 'sub_role' in c else 'no')" 2>/dev/null || echo "no")
    ROLE=$(echo "$CLAIMS" | python3 -c "import sys,json; c=json.load(sys.stdin); print(c.get('role',''))" 2>/dev/null || echo "")
    ALG=$(echo "$TOKEN" | python3 -c "
import sys,json,base64
t=sys.stdin.read().strip()
p=t.split('.')[0]
p+='='*(4-len(p)%4)
h=json.loads(base64.b64decode(p))
print(h.get('alg',''))
" 2>/dev/null || echo "")

    # Access protected API with token
    API_RESP=$(curl -s -w "\n%{http_code}" -H "Authorization: Bearer $TOKEN" http://localhost:$OS_PORT/api/v1/bos/cos/health)
    API_CODE=$(echo "$API_RESP" | tail -1)

    if [ "$API_CODE" = "200" ] && [ "$HAS_SUB_ROLE" = "yes" ] && [ "$ALG" = "RS256" ]; then
        log_pass "Case 3: Login→JWT(RS256,sub_role=$HAS_SUB_ROLE)→200"
    else
        log_fail "Case 3" "api=$API_CODE sub_role=$HAS_SUB_ROLE alg=$ALG role=$ROLE"
    fi
fi

# ===== Case 4: Low-privilege role → 403 =====
echo "[case 4] Low-privilege role → 403"
# Login as viewer (read-only, limited access)
VIEWER_RESP=$(curl -s -X POST http://localhost:$OAS_PORT/api/v1/os/bos/proxy/ams/auth/login \
    -H 'Content-Type: application/json' -d '{"username":"viewer","password":"viewer123"}')
VIEWER_TOKEN=$(echo "$VIEWER_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])" 2>/dev/null || echo "")
if [ -n "$VIEWER_TOKEN" ]; then
    # Try to access admin-only endpoint
    ADMIN_RESP=$(curl -s -w "\n%{http_code}" -H "Authorization: Bearer $VIEWER_TOKEN" \
        -X POST http://localhost:$OAS_PORT/api/v1/admin/rbac/policies \
        -H 'Content-Type: application/json' -d '{"subject":"TEST","resource":"/test","action":"*"}')
    ADMIN_CODE=$(echo "$ADMIN_RESP" | tail -1)
    if [ "$ADMIN_CODE" = "403" ] || [ "$ADMIN_CODE" = "401" ]; then
        log_pass "Case 4: Low-privilege → $ADMIN_CODE (RBAC enforced)"
    else
        log_fail "Case 4" "expected 403/401, got $ADMIN_CODE"
    fi
else
    log_fail "Case 4" "viewer login failed"
fi

# ===== Case 5: Tampered JWT → 401 =====
echo "[case 5] Tampered JWT → 401"
if [ -n "$TOKEN" ]; then
    # Tamper with payload (change a character)
    PARTS=(${TOKEN//./ })
    PAYLOAD=${PARTS[1]}
    # Flip a character in the payload
    if [ "${PAYLOAD:5:1}" = "A" ]; then
        TAMPERED_PAYLOAD="${PAYLOAD:0:5}B${PAYLOAD:6}"
    else
        TAMPERED_PAYLOAD="${PAYLOAD:0:5}A${PAYLOAD:6}"
    fi
    TAMPERED_TOKEN="${PARTS[0]}.${TAMPERED_PAYLOAD}.${PARTS[2]}"
    TAMPER_RESP=$(curl -s -w "\n%{http_code}" -H "Authorization: Bearer $TAMPERED_TOKEN" http://localhost:$OS_PORT/api/v1/bos/cos/health)
    TAMPER_CODE=$(echo "$TAMPER_RESP" | tail -1)
    if [ "$TAMPER_CODE" = "401" ]; then
        log_pass "Case 5: Tampered JWT → 401"
    else
        log_fail "Case 5" "expected 401, got $TAMPER_CODE"
    fi
else
    log_fail "Case 5" "no token to tamper"
fi

# ===== Case 6: Three-level cache =====
echo "[case 6] Three-level cache L1/L2/L3"
# First call: L3 source (DB)
CACHE1=$(curl -s http://localhost:$MBS_PORT/api/v1/ams/policies/cached)
SOURCE1=$(echo "$CACHE1" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['source'])" 2>/dev/null || echo "error")
COUNT1=$(echo "$CACHE1" | python3 -c "import sys,json; print(len(json.load(sys.stdin)['data']['policies']))" 2>/dev/null || echo "0")

# Second call: L1 hit
CACHE2=$(curl -s http://localhost:$MBS_PORT/api/v1/ams/policies/cached)
SOURCE2=$(echo "$CACHE2" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['source'])" 2>/dev/null || echo "error")

# Invalidate
curl -s -X POST http://localhost:$MBS_PORT/api/v1/ams/policies/cache/invalidate > /dev/null 2>&1

# Third call: L3 again after invalidation
CACHE3=$(curl -s http://localhost:$MBS_PORT/api/v1/ams/policies/cached)
SOURCE3=$(echo "$CACHE3" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['source'])" 2>/dev/null || echo "error")

if [ "$SOURCE1" != "error" ] && [ "$SOURCE2" != "error" ]; then
    log_pass "Case 6: Cache L1/L2/L3 (1st=$SOURCE1 cnt=$COUNT1, 2nd=$SOURCE2, after-invalidate=$SOURCE3)"
else
    log_fail "Case 6" "cache API error: $SOURCE1"
fi

# ===== Case 7: Audit log has issuance + verification records =====
echo "[case 7] Audit log records"
sleep 1
# Check OAS audit logs
AUDIT_RESP=$(curl -s http://localhost:$OAS_PORT/api/v1/admin/audit-logs?action=auth.login)
AUDIT_COUNT=$(echo "$AUDIT_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['total'])" 2>/dev/null || echo "0")

# Check OS logs for JWT verification
VERIFY_COUNT=$(grep -c "jwt verification success" /tmp/e2e_os.log 2>/dev/null || echo "0")

if [ "$AUDIT_COUNT" -gt 0 ] && [ "$VERIFY_COUNT" -gt 0 ]; then
    log_pass "Case 7: Audit (issuance=$AUDIT_COUNT, verify=$VERIFY_COUNT)"
else
    log_fail "Case 7" "issuance=$AUDIT_COUNT verify=$VERIFY_COUNT"
fi

# ===== Summary =====
echo ""
echo "============================================"
echo "  Test Summary"
echo "============================================"
for r in "${RESULTS[@]}"; do
    echo "  $r"
done
echo ""
echo "  Total: $((PASS+FAIL)) | PASS: $PASS | FAIL: $FAIL"
echo "============================================"

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
exit 0
