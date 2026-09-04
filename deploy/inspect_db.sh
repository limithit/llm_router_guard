#!/bin/bash
# Inspect server logs and call_logs for E2E debugging. Usage: inspect_db.sh mysql|postgres
source /root/gateway-test/env.sh
LOG="$2"
echo "=== server log: audit/error lines ==="
grep -aiE "audit|error|warn" "$LOG" | tail -25
echo "=== call_logs ==="
if [ "$1" = mysql ]; then
  mysql -uroot -N -e "SELECT COUNT(*) FROM gateway_mysql.call_logs"
  mysql -uroot -N -e "SELECT id,status,blocked,model_alias,prompt_tokens,completion_tokens,LEFT(created_at,19) FROM gateway_mysql.call_logs ORDER BY id DESC LIMIT 8"
else
  psql -h 127.0.0.1 -U postgres -d gateway_pg -tAc "SELECT COUNT(*) FROM call_logs"
  psql -h 127.0.0.1 -U postgres -d gateway_pg -tAc "SELECT id,status,blocked,model_alias,prompt_tokens,completion_tokens,LEFT(created_at::text,19) FROM call_logs ORDER BY id DESC LIMIT 8"
fi
