#!/bin/bash
# Full E2E cycle: stop server -> reset db -> start server -> run e2e.
# Usage: do_all.sh mysql|postgres PORT LOGFILE PIDFILE
set -e
KIND="$1"; PORT="$2"; LOG="$3"; PID="$4"
RUNDIR=/root/gateway-test
source "$RUNDIR/env.sh"
if [ "$KIND" = mysql ]; then
  DB_TYPE=mysql DB_DSN="$DB_MYSQL_DSN"
else
  DB_TYPE=postgres DB_DSN="$DB_PG_DSN"
fi
# stop both flavors first: PG DROP DATABASE refuses while any connection is open
bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-mysql.pid" "$PORT"
bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-pg.pid" "$PORT"
bash "$RUNDIR/reset_db.sh" "$KIND"
DB_TYPE="$DB_TYPE" DB_DSN="$DB_DSN" bash "$RUNDIR/run_server.sh" "$RUNDIR" "$PORT" "$LOG" "$PID"
python3 "$RUNDIR/e2e.py" "http://127.0.0.1:$PORT" "E2eAdmin#2026"
