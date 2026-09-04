#!/bin/bash
# Start llm-router-guard in background and wait until healthy.
# Usage: run_server.sh RUNDIR PORT LOGFILE PIDFILE
# DB_TYPE/DB_DSN come from environment.
set -e
RUNDIR="$1"; PORT="$2"; LOGFILE="$3"; PIDFILE="$4"
RUNDIR="$(cd "$RUNDIR" && pwd)"

bash "$RUNDIR/stop_server.sh" "$PIDFILE" "$PORT"

cd "$RUNDIR"
nohup env PORT="$PORT" DB_TYPE="$DB_TYPE" DB_DSN="$DB_DSN" \
  JWT_SECRET="e2e-jwt-secret-2026" MASTER_KEY="e2e-master-key-2026" \
  ADMIN_PASSWORD="E2eAdmin#2026" GIN_MODE=release \
  ./llm-router-guard >>"$LOGFILE" 2>&1 &
echo $! >"$PIDFILE"

for _ in $(seq 1 60); do
  if curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1; then
    echo "READY port=$PORT pid=$(cat "$PIDFILE")"
    exit 0
  fi
  sleep 1
done
echo "FAILED to start on port $PORT"
tail -n 40 "$LOGFILE"
exit 1
