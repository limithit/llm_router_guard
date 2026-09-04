#!/bin/bash
# Stop a gateway instance and free a port. Usage: stop_server.sh PIDFILE PORT
PIDFILE="$1"; PORT="$2"
if [ -f "$PIDFILE" ]; then
  PID=$(cat "$PIDFILE")
  kill "$PID" 2>/dev/null || true
  for _ in $(seq 1 20); do
    kill -0 "$PID" 2>/dev/null || break
    sleep 0.5
  done
  kill -9 "$PID" 2>/dev/null || true
  rm -f "$PIDFILE"
fi
# free the port regardless of pidfile state (fuser is in psmisc; ignore if missing)
fuser -k "${PORT}/tcp" 2>/dev/null || true
for _ in $(seq 1 20); do
  ss -ltn "sport = :$PORT" 2>/dev/null | grep -q ":$PORT " || break
  sleep 0.5
done
exit 0
