#!/bin/bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

PORT=5000

usage() {
  echo "Usage: $0 -p <port>"
}

while getopts "p:h" opt; do
  case "$opt" in
    p)
      PORT="$OPTARG"
      ;;
    h)
      usage
      exit 0
      ;;
    \?)
      echo "Invalid option: -$OPTARG"
      usage
      exit 1
      ;;
  esac
done

export APP_ENV=dev
export ZIWAY_SERVER_HTTP_PORT="$PORT"

# Use /tmp for SQLite data to ensure writability in deployment containers
DATA_DIR="${ZIWAY_DATA_DIR:-/tmp/ziway_data}"
mkdir -p "$DATA_DIR"
chmod 755 "$DATA_DIR"
export ZIWAY_DATABASE_SQLITE_PATH="$DATA_DIR/ziway_p0.db"

# Start OAS as the public-facing service
# OAS handles: POST /api/v1/os/{supply}/proxy/ams/auth/login (JWT issuance)
# OAS also provides RBAC policy management and audit logging
exec ./bin/oas
