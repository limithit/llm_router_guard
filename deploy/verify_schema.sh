#!/bin/bash
# Round-trip verify the committed schema dumps:
#   fresh DB <- apply deploy/schema/*.sql -> server (AutoMigrate) -> full E2E
#   -> re-dump -> must be identical to the committed dump (mod comments/AUTO_INCREMENT).
set -e
RUNDIR=/root/gateway-test
source "$RUNDIR/env.sh"
bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-v.pid" 18080 >/dev/null 2>&1 || true
# mock 必须先起（E2E 的上游连接步骤依赖它）；脚本文件内执行，pgrep 不会误匹配
pkill -f '[m]ockup.py' 2>/dev/null || true
sleep 1
nohup python3 "$RUNDIR/mockup.py" >"$RUNDIR/mock.log" 2>&1 &
sleep 1

echo "===== MySQL ====="
mysql -uroot -pcomeback -e "DROP DATABASE IF EXISTS gateway_mysql_v; CREATE DATABASE gateway_mysql_v CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;" 2>/dev/null
mysql -uroot -pcomeback gateway_mysql_v < "$RUNDIR/schema.mysql.sql" 2>/dev/null
DB_TYPE=mysql DB_DSN="root:comeback@tcp(127.0.0.1:3306)/gateway_mysql_v?charset=utf8mb4&parseTime=True&loc=Local" \
  bash "$RUNDIR/run_server.sh" "$RUNDIR" 18080 "$RUNDIR/server-v.log" "$RUNDIR/server-v.pid"
python3 "$RUNDIR/e2e.py" "http://127.0.0.1:18080" "E2eAdmin#2026" | tail -2
bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-v.pid" 18080 >/dev/null
mysqldump -uroot -pcomeback --no-data --skip-dump-date --no-tablespaces gateway_mysql_v 2>/dev/null \
  | sed 's/ AUTO_INCREMENT=[0-9]*//' > "$RUNDIR/v.mysql.sql"
# 装饰差异过滤：每列 CHARACTER SET utf8mb4（与表默认一致，转储上下文决定是否显示）
norm_mysql() { grep -vE '^--|^/\*' "$1" | sed 's/ CHARACTER SET utf8mb4 / /g'; }
if diff <(norm_mysql "$RUNDIR/schema.mysql.sql") <(norm_mysql "$RUNDIR/v.mysql.sql") >/dev/null; then
  echo "MYSQL_SCHEMA_ROUNDTRIP_IDENTICAL"
else
  echo "MYSQL_SCHEMA_DIFF_ABOVE"; diff <(norm_mysql "$RUNDIR/schema.mysql.sql") <(norm_mysql "$RUNDIR/v.mysql.sql") | head -30
fi

echo "===== PostgreSQL ====="
PGPASSWORD=comeback psql -h 127.0.0.1 -U postgres -c "DROP DATABASE IF EXISTS gateway_pg_v;" -c "CREATE DATABASE gateway_pg_v;" >/dev/null
PGPASSWORD=comeback psql -h 127.0.0.1 -U postgres -d gateway_pg_v -f "$RUNDIR/schema.postgres.sql" >/dev/null 2>&1
DB_TYPE=postgres DB_DSN="host=127.0.0.1 port=5432 user=postgres password=comeback dbname=gateway_pg_v sslmode=disable TimeZone=Asia/Shanghai" \
  bash "$RUNDIR/run_server.sh" "$RUNDIR" 18080 "$RUNDIR/server-v.log" "$RUNDIR/server-v.pid"
python3 "$RUNDIR/e2e.py" "http://127.0.0.1:18080" "E2eAdmin#2026" | tail -2
bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-v.pid" 18080 >/dev/null
PGPASSWORD=comeback pg_dump -h 127.0.0.1 -U postgres --schema-only --no-owner --no-privileges gateway_pg_v > "$RUNDIR/v.pg.sql"
# 装饰差异过滤：psql 17 的 \restrict 保护令牌（每次转储随机）
norm_pg() { grep -vE '^--|^\\(un)?restrict ' "$1"; }
if diff <(norm_pg "$RUNDIR/schema.postgres.sql") <(norm_pg "$RUNDIR/v.pg.sql") >/dev/null; then
  echo "PG_SCHEMA_ROUNDTRIP_IDENTICAL"
else
  echo "PG_SCHEMA_DIFF_ABOVE"; diff <(norm_pg "$RUNDIR/schema.postgres.sql") <(norm_pg "$RUNDIR/v.pg.sql") | head -30
fi

echo VERIFY_SCHEMA_DONE
