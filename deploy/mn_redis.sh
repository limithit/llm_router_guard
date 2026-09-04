#!/bin/bash
# Multi-node live test WITH Redis distributed state:
# reset PG -> mock + two gateway instances (18080/18082) sharing PG + Redis
# -> run verification (config propagation / GLOBAL rate limit / window recovery).
set -e
RUNDIR=/root/gateway-test
source "$RUNDIR/env.sh"

bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-pg.pid" 18080
bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-pg2.pid" 18082
pkill -f '[m]ockup.py' 2>/dev/null || true
sleep 1
bash "$RUNDIR/reset_db.sh" postgres
redis-cli DEL gw:rl:v1:* gw:cb:v1:* gw:mfa:* >/dev/null || true

nohup python3 "$RUNDIR/mockup.py" >"$RUNDIR/mock.log" 2>&1 &
echo $! >"$RUNDIR/mock.pid"
sleep 1

DB_TYPE=postgres DB_DSN="$DB_PG_DSN" REDIS_ADDR=127.0.0.1:6379 REDIS_PASSWORD=comeback \
  bash "$RUNDIR/run_server.sh" "$RUNDIR" 18080 "$RUNDIR/server-pg.log" "$RUNDIR/server-pg.pid"
DB_TYPE=postgres DB_DSN="$DB_PG_DSN" REDIS_ADDR=127.0.0.1:6379 REDIS_PASSWORD=comeback \
  bash "$RUNDIR/run_server.sh" "$RUNDIR" 18082 "$RUNDIR/server-pg2.log" "$RUNDIR/server-pg2.pid"

echo "=== instance A startup: distributed-state log lines ==="
grep -aE "\[ratelimit\]|\[slb\]|\[mfa\]" "$RUNDIR/server-pg.log" | tail -4
echo "=== instance B startup ==="
grep -aE "\[ratelimit\]|\[slb\]|\[mfa\]" "$RUNDIR/server-pg2.log" | tail -4

python3 "$RUNDIR/multi_node_redis_test.py" http://127.0.0.1:18080 http://127.0.0.1:18082 "E2eAdmin#2026"
