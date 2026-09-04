#!/bin/bash
# Cleanup test environment: stop servers + mock, remove smoke db.
bash /root/gateway-test/stop_server.sh /root/gateway-test/server-mysql.pid 18080
bash /root/gateway-test/stop_server.sh /root/gateway-test/server-pg.pid 18080
bash /root/gateway-test/stop_server.sh /root/gateway-test/server-sqlite.pid 18079
pkill -f '[m]ockup.py' 2>/dev/null
rm -f /root/gateway-test/data/smoke.db /root/gateway-test/data/smoke.db-wal /root/gateway-test/data/smoke.db-shm
sleep 1
if ss -ltn | grep -qE ':(18079|18080|18081) '; then
  echo PORTS_STILL_BUSY
else
  echo ALL_PORTS_FREE
fi
ls /root/gateway-test/
