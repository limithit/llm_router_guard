#!/bin/bash
# Debug: capture RAW streaming response body + access log status.
set -e
RUNDIR=/root/gateway-test
source "$RUNDIR/env.sh"
bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-dbg.pid" 18080 >/dev/null 2>&1 || true
pkill -f '[m]ockup.py' 2>/dev/null || true
sleep 1
bash "$RUNDIR/reset_db.sh" mysql >/dev/null 2>&1
MOCK_PORT=18081 MOCK_STREAM_CHUNKS=20 MOCK_STREAM_DELAY=0.1 nohup python3 "$RUNDIR/mockup.py" >"$RUNDIR/mock-18081.log" 2>&1 &
MOCK_PORT=18083 MOCK_STREAM_CHUNKS=20 MOCK_STREAM_DELAY=0.1 nohup python3 "$RUNDIR/mockup.py" >"$RUNDIR/mock-18083.log" 2>&1 &
MOCK_PORT=18085 MOCK_STREAM_CHUNKS=20 MOCK_STREAM_DELAY=0.1 nohup python3 "$RUNDIR/mockup.py" >"$RUNDIR/mock-18085.log" 2>&1 &
sleep 1
DB_TYPE=mysql DB_DSN="$DB_MYSQL_DSN" bash "$RUNDIR/run_server.sh" "$RUNDIR" 18080 "$RUNDIR/server-dbg.log" "$RUNDIR/server-dbg.pid"
sleep 1
BASE=http://127.0.0.1:18080
PW='E2eAdmin#2026'
TOK=$(curl -s -X POST $BASE/api/admin/v1/auth/login -H 'Content-Type: application/json' -d "{\"username\":\"admin\",\"password\":\"$PW\"}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["token"])')
P1=$(curl -s -X POST $BASE/api/admin/v1/providers -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d '{"name":"m1","protocol":"openai_chat","base_url":"http://127.0.0.1:18081/v1","api_key":"k","enabled":true}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["id"])')
P2=$(curl -s -X POST $BASE/api/admin/v1/providers -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d '{"name":"m2","protocol":"openai_chat","base_url":"http://127.0.0.1:18083/v1","api_key":"k","enabled":true}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["id"])')
P3=$(curl -s -X POST $BASE/api/admin/v1/providers -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d '{"name":"m3","protocol":"openai_chat","base_url":"http://127.0.0.1:18085/v1","api_key":"k","enabled":true}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["id"])')
curl -s -X POST $BASE/api/admin/v1/models -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d "{\"alias\":\"rr\",\"enabled\":true,\"upstreams\":[{\"provider_id\":$P1,\"upstream_model\":\"mock-small\",\"weight\":1},{\"provider_id\":$P2,\"upstream_model\":\"mock-small\",\"weight\":1},{\"provider_id\":$P3,\"upstream_model\":\"mock-small\",\"weight\":1}]}" >/dev/null
KEY=$(curl -s -X POST $BASE/api/admin/v1/apikeys -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d '{"name":"k"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["key"])')
sleep 4

echo "== RAW streaming response (curl -sS -N, dumped verbatim) =="
curl -sS -N -X POST $BASE/v1/chat/completions -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' -d '{"model":"rr","stream":true,"messages":[{"role":"user","content":"hi"}]}' > "$RUNDIR/stream.raw" 2>&1 || echo "curl exit=$?"
echo "--- raw bytes (head -40) ---"
head -40 "$RUNDIR/stream.raw"
echo "--- raw size ---"
wc -c "$RUNDIR/stream.raw"

echo "== single-upstream control (directly to mock, bypass gateway) =="
curl -sS -N -X POST http://127.0.0.1:18081/v1/chat/completions -H 'Content-Type: application/json' -d '{"model":"mock-small","stream":true,"messages":[{"role":"user","content":"hi"}]}' | head -5

echo "== gateway access log (v1 lines) =="
grep -a 'v1/chat' "$RUNDIR/server-dbg.log" | tail -5
echo "== gateway full log tail =="
tail -15 "$RUNDIR/server-dbg.log"

bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-dbg.pid" 18080 >/dev/null 2>&1 || true
pkill -f '[m]ockup.py' 2>/dev/null || true
echo DBG_DONE
