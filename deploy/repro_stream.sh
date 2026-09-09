#!/bin/bash
# Reproduce: 3 upstreams round-robin, streaming truncation?
# 3 mocks (20 chunks x 0.1s each = ~2s stream) + alias with 3 upstreams (weight 1 each)
# + curl --no-buffer: count content chunks client receives; expect 20 per request.
set -e
RUNDIR=/root/gateway-test
source "$RUNDIR/env.sh"

bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-repro.pid" 18080 >/dev/null 2>&1 || true
pkill -f '[m]ockup.py' 2>/dev/null || true
sleep 1
bash "$RUNDIR/reset_db.sh" mysql >/dev/null 2>&1

# 3 mocks on 18081/18083/18085, each 20 chunks x 0.1s
for p in 18081 18083 18085; do
  MOCK_PORT=$p MOCK_STREAM_CHUNKS=20 MOCK_STREAM_DELAY=0.1 \
    nohup python3 "$RUNDIR/mockup.py" >"$RUNDIR/mock-$p.log" 2>&1 &
done
sleep 1

DB_TYPE=mysql DB_DSN="$DB_MYSQL_DSN" bash "$RUNDIR/run_server.sh" "$RUNDIR" 18080 "$RUNDIR/server-repro.log" "$RUNDIR/server-repro.pid"
sleep 1

BASE=http://127.0.0.1:18080
PW='E2eAdmin#2026'
TOK=$(curl -s -X POST $BASE/api/admin/v1/auth/login -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$PW\"}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["token"])')

# 3 providers (one per mock), 1 alias with 3 upstreams weight 1
PIDS=""
for p in 18081 18083 18085; do
  PID=$(curl -s -X POST $BASE/api/admin/v1/providers -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
    -d "{\"name\":\"mck-$p\",\"protocol\":\"openai_chat\",\"base_url\":\"http://127.0.0.1:$p/v1\",\"api_key\":\"k\",\"enabled\":true}" \
    | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["id"])')
  PIDS="$PIDS $PID"
done
UPS=$(echo $PIDS | python3 -c 'import sys; ps=sys.stdin.read().split(); print(",".join("{\"provider_id\":%s,\"upstream_model\":\"mock-small\",\"weight\":1}"%p for p in ps))')
curl -s -X POST $BASE/api/admin/v1/models -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d "{\"alias\":\"rr-model\",\"enabled\":true,\"upstreams\":[$UPS]}" >/dev/null
KEY=$(curl -s -X POST $BASE/api/admin/v1/apikeys -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"name":"rr-key"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["key"])')
sleep 4  # hot reload

echo "== streaming 3 requests (round-robin should hit different upstreams) =="
for i in 1 2 3; do
  echo "--- request $i ---"
  OUT=$(curl -sN -X POST $BASE/v1/chat/completions \
    -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
    -d "{\"model\":\"rr-model\",\"stream\":true,\"messages\":[{\"role\":\"user\",\"content\":\"hello-$i\"}]}")
  CONTENT=$(echo "$OUT" | python3 -c '
import sys,json
deltas=[]
done=False
for line in sys.stdin:
    line=line.strip()
    if not line.startswith("data:"): continue
    payload=line[5:].strip()
    if payload=="[DONE]": done=True; continue
    try: o=json.loads(payload)
    except: continue
    ch=o.get("choices") or []
    if ch and ch[0].get("delta",{}).get("content"):
        deltas.append(ch[0]["delta"]["content"])
print("chunks=%d done=%s content=%r" % (len(deltas), done, "".join(deltas)))
')
  echo "  $CONTENT"
done

echo "== server log: any stream/upstream/error lines =="
grep -aE 'upstream|interrupt|stream|502|error|panic|EOF|context' "$RUNDIR/server-repro.log" | tail -20 || echo "no matches"

bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-repro.pid" 18080 >/dev/null 2>&1 || true
pkill -f '[m]ockup.py' 2>/dev/null || true
echo REPRO_DONE
