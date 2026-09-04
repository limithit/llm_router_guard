#!/bin/bash
# Multi-node live test: reset PG db -> start mock + two gateway instances (18080/18082)
# sharing the same db -> run 2-instance verification.
set -e
RUNDIR=/root/gateway-test
source "$RUNDIR/env.sh"

bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-pg.pid" 18080
bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-pg2.pid" 18082
pkill -f '[m]ockup.py' 2>/dev/null || true
sleep 1
bash "$RUNDIR/reset_db.sh" postgres

nohup python3 "$RUNDIR/mockup.py" >"$RUNDIR/mock.log" 2>&1 &
echo $! >"$RUNDIR/mock.pid"
sleep 1

DB_TYPE=postgres DB_DSN="$DB_PG_DSN" bash "$RUNDIR/run_server.sh" "$RUNDIR" 18080 "$RUNDIR/server-pg.log" "$RUNDIR/server-pg.pid"
DB_TYPE=postgres DB_DSN="$DB_PG_DSN" bash "$RUNDIR/run_server.sh" "$RUNDIR" 18082 "$RUNDIR/server-pg2.log" "$RUNDIR/server-pg2.pid"

python3 "$RUNDIR/multi_node_test.py" http://127.0.0.1:18080 http://127.0.0.1:18082 "E2eAdmin#2026"
