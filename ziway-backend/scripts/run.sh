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

# APP_ENV controls database mode:
# - dev/sqlite: local SQLite (single instance only, data split in multi-replica)
# - prod: PostgreSQL (shared database, supports multi-replica)
# For multi-replica deployment, MUST use APP_ENV=prod with PostgreSQL
export APP_ENV="${APP_ENV:-dev}"
export ZIWAY_SERVER_HTTP_PORT="$PORT"

# Database configuration
if [ "$APP_ENV" = "prod" ]; then
    # Production: PostgreSQL (shared across replicas)
    echo "Production mode: using PostgreSQL"
    # ZIWAY_DATABASE_DSN must be set via environment variables
    # Example: host=pg-host user=ziway password=xxx dbname=ziway port=5432
else
    # Development: SQLite (local file, single instance only!)
    echo "Development mode: using SQLite (WARNING: not suitable for multi-replica)"
    DATA_DIR="${ZIWAY_DATA_DIR:-/tmp/ziway_data}"
    mkdir -p "$DATA_DIR"
    chmod 755 "$DATA_DIR"
    export ZIWAY_DATABASE_SQLITE_PATH="$DATA_DIR/ziway_p0.db"
fi

# Start OAS as the public-facing service
# OAS handles: POST /api/v1/os/{supply}/proxy/ams/auth/login (JWT issuance)
# OAS also provides RBAC policy management and audit logging
exec ./bin/oas
