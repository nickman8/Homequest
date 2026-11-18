#!/usr/bin/env bash
set -euo pipefail
: "${DATA_DIR:?Set DATA_DIR outside the Git checkout}"
case "$(realpath -m "$DATA_DIR")/" in "$PWD/"*) echo 'DATA_DIR must be outside checkout' >&2; exit 1;; esac
umask 077
mkdir -p "$DATA_DIR"
if [ ! -f "$DATA_DIR/jwt-secret" ]; then python3 -c 'import secrets; print(secrets.token_hex(32))' > "$DATA_DIR/jwt-secret"; fi
export DT_JWT_SECRET="$(cat "$DATA_DIR/jwt-secret")"
export DT_SQLITE_PATH="$DATA_DIR/donetick.db"
export DT_SERVER_SERVE_FRONTEND=false DT_BRIDGE_ENABLED=false DT_IS_DONE_TICK_DOT_COM=false DT_LOGGING_LEVEL=info
exec ./bin/donetick
