#!/bin/bash
# Generate baseline schema dumps from what GORM AutoMigrate actually creates:
#   deploy/schema/schema.mysql.sql     (mysqldump --no-data, AUTO_INCREMENT=N stripped)
#   deploy/schema/schema.postgres.sql  (pg_dump --schema-only)
set -e
RUNDIR=/root/gateway-test
source "$RUNDIR/env.sh"

bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-mysql.pid" 18080 >/dev/null || true
bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-pg.pid" 18080 >/dev/null || true

# ---- MySQL ----
bash "$RUNDIR/reset_db.sh" mysql
DB_TYPE=mysql DB_DSN="$DB_MYSQL_DSN" bash "$RUNDIR/run_server.sh" "$RUNDIR" 18080 "$RUNDIR/server-mysql.log" "$RUNDIR/server-mysql.pid"
sleep 2
mysqldump -uroot -pcomeback --no-data --skip-dump-date --no-tablespaces gateway_mysql > "$RUNDIR/schema.mysql.sql" 2>/dev/null
sed -i 's/ AUTO_INCREMENT=[0-9]*//' "$RUNDIR/schema.mysql.sql"
bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-mysql.pid" 18080 >/dev/null

# ---- PostgreSQL ----
bash "$RUNDIR/reset_db.sh" postgres
DB_TYPE=postgres DB_DSN="$DB_PG_DSN" bash "$RUNDIR/run_server.sh" "$RUNDIR" 18080 "$RUNDIR/server-pg.log" "$RUNDIR/server-pg.pid"
sleep 2
PGPASSWORD=comeback pg_dump -h 127.0.0.1 -U postgres --schema-only --no-owner --no-privileges gateway_pg > "$RUNDIR/schema.postgres.sql"
bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-pg.pid" 18080 >/dev/null

echo "== table counts =="
grep -c 'CREATE TABLE' "$RUNDIR/schema.mysql.sql" || true
grep -c 'CREATE TABLE' "$RUNDIR/schema.postgres.sql" || true
echo SCHEMA_DUMP_DONE
