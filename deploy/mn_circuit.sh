#!/bin/bash
# Circuit-breaker sharing test: PG + Redis, good mock (18081) + bad mock (18083).
set -e
RUNDIR=/root/gateway-test
source "$RUNDIR/env.sh"

bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-pg.pid" 18080
bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-pg2.pid" 18082
pkill -f '[m]ockup.py' 2>/dev/null || true
sleep 1
bash "$RUNDIR/reset_db.sh" postgres
redis-cli -a comeback DEL gw:cb:v1:* gw:rl:v1:* >/dev/null 2>&1 || true

MOCK_PORT=18081 MOCK_MODE=good nohup python3 "$RUNDIR/mockup.py" >"$RUNDIR/mock.log" 2>&1 &
echo $! >"$RUNDIR/mock.pid"
MOCK_PORT=18083 MOCK_MODE=bad nohup python3 "$RUNDIR/mockup.py" >"$RUNDIR/mock-bad.log" 2>&1 &
echo $! >"$RUNDIR/mock-bad.pid"
sleep 1

DB_TYPE=postgres DB_DSN="$DB_PG_DSN" REDIS_ADDR=127.0.0.1:6379 REDIS_PASSWORD=comeback \
  bash "$RUNDIR/run_server.sh" "$RUNDIR" 18080 "$RUNDIR/server-pg.log" "$RUNDIR/server-pg.pid"
DB_TYPE=postgres DB_DSN="$DB_PG_DSN" REDIS_ADDR=127.0.0.1:6379 REDIS_PASSWORD=comeback \
  bash "$RUNDIR/run_server.sh" "$RUNDIR" 18082 "$RUNDIR/server-pg2.log" "$RUNDIR/server-pg2.pid"

python3 "$RUNDIR/multi_node_circuit_test.py" http://127.0.0.1:18080 http://127.0.0.1:18082 "E2eAdmin#2026"
