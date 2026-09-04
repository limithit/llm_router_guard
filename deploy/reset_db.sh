#!/bin/bash
# Reset one test database. Usage: reset_db.sh mysql|postgres
set -e
source /root/gateway-test/env.sh
case "$1" in
  mysql)
    mysql -uroot -e "DROP DATABASE IF EXISTS gateway_mysql; CREATE DATABASE gateway_mysql CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
    echo MYSQL_DB_RESET
    ;;
  postgres)
    psql -h 127.0.0.1 -U postgres -c "DROP DATABASE IF EXISTS gateway_pg;" -c "CREATE DATABASE gateway_pg OWNER postgres;"
    echo PG_DB_RESET
    ;;
  *)
    echo "usage: reset_db.sh mysql|postgres"; exit 1
    ;;
esac
