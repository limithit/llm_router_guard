#!/bin/bash
# Proof: streaming is exempt from the per-request total timeout.
# default_timeout_seconds=2s, mock stream = 10 chunks x 0.5s (~5s total).
# OLD code: stream truncated at 2s (~3-4 chunks). FIXED code: all 10 chunks + [DONE].
set -e
RUNDIR=/root/gateway-test
PORT="${1:-18091}"   # 18080 被同机 dlp-backend 占用，避开
source "$RUNDIR/env.sh"
bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-proof.pid" "$PORT" >/dev/null 2>&1 || true
pkill -f '[m]ockup.py' 2>/dev/null || true
sleep 1
bash "$RUNDIR/reset_db.sh" mysql >/dev/null 2>&1
MOCK_PORT=18081 MOCK_STREAM_CHUNKS=10 MOCK_STREAM_DELAY=0.5 nohup python3 "$RUNDIR/mockup.py" >"$RUNDIR/mock-proof.log" 2>&1 &
sleep 1
DB_TYPE=mysql DB_DSN="$DB_MYSQL_DSN" bash "$RUNDIR/run_server.sh" "$RUNDIR" "$PORT" "$RUNDIR/server-proof.log" "$RUNDIR/server-proof.pid"
sleep 1
BASE=http://127.0.0.1:$PORT
PW='E2eAdmin#2026'
TOK=$(curl -s -X POST $BASE/api/admin/v1/auth/login -H 'Content-Type: application/json' -d "{\"username\":\"admin\",\"password\":\"$PW\"}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["token"])')
PID=$(curl -s -X POST $BASE/api/admin/v1/providers -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d '{"name":"p","protocol":"openai_chat","base_url":"http://127.0.0.1:18081/v1","api_key":"k","enabled":true}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["id"])')
curl -s -X POST $BASE/api/admin/v1/models -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d "{\"alias\":\"sm\",\"enabled\":true,\"upstreams\":[{\"provider_id\":$PID,\"upstream_model\":\"mock-small\",\"weight\":1}]}" >/dev/null
KEY=$(curl -s -X POST $BASE/api/admin/v1/apikeys -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d '{"name":"k"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["key"])')
# patch general: default_timeout_seconds=2（完整体，避免 GET+合并）
curl -s -X PUT $BASE/api/admin/v1/settings -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"log_level":"info","audit_retention_days":90,"default_timeout_seconds":2,"max_connections":1000,"guard_enabled":false,"hot_reload_seconds":3,"listen_port":8091,"call_audit_enabled":true,"call_audit_only_errors":false,"call_audit_sampling":1}' >/dev/null
sleep 4  # hot reload

echo "== STREAM request, total timeout=2s, stream lasts ~5s =="
curl -sN -X POST $BASE/v1/chat/completions -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' -d '{"model":"sm","stream":true,"messages":[{"role":"user","content":"proof"}]}' > "$RUNDIR/proof.raw" 2>"$RUNDIR/proof.err" || echo "curl exit=$?"
python3 - "$RUNDIR/proof.raw" <<'PY'
import sys, json
data=[]
done=False; bad=0
for line in open(sys.argv[1], encoding="utf-8"):
    line=line.strip()
    if not line.startswith("data:"): continue
    p=line[5:].strip()
    if p=="[DONE]": done=True; continue
    try:
        o=json.loads(p)
    except Exception as e:
        bad+=1; continue
    ch=o.get("choices") or []
    if ch and ch[0].get("delta",{}).get("content"):
        data.append(ch[0]["delta"]["content"])
print("chunks=%d  done=%s  unparseable=%d  content=%r" % (len(data), done, bad, "".join(data)))
PY
echo "-- stderr --"; cat "$RUNDIR/proof.err" || true

echo "== non-stream control (same 2s timeout, mock replies fast) =="
curl -s -X POST $BASE/v1/chat/completions -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' -d '{"model":"sm","messages":[{"role":"user","content":"proof"}]}' | python3 -c 'import sys,json;print("nonstream content=",json.load(sys.stdin)["choices"][0]["message"]["content"])'

echo "== gateway log tail =="
tail -8 "$RUNDIR/server-proof.log"

bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-proof.pid" "$PORT" >/dev/null 2>&1 || true
pkill -f '[m]ockup.py' 2>/dev/null || true
echo PROOF_DONE
