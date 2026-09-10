#!/bin/bash
# Repro user's exact topology: SQLite + ONE mock + 3 providers with the SAME base_url
# but DIFFERENT api keys (3 provider records) + alias with 3 upstreams, round-robin.
# Observations per streamed request: chunk count, [DONE], which upstream key served it.
set -e
RUNDIR=/root/gateway-test
PORT="${1:-18091}"
source "$RUNDIR/env.sh"
bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-rr.pid" "$PORT" >/dev/null 2>&1 || true
pkill -f mockup[.]py 2>/dev/null || true
sleep 1
rm -f "$RUNDIR/data/gateway.db"   # fresh sqlite, default general settings
{ MOCK_PORT=18081 MOCK_STREAM_CHUNKS=30 MOCK_STREAM_DELAY=0.15 nohup python3 "$RUNDIR/mockup.py" >"$RUNDIR/mock-rr.log" 2>&1 & }
sleep 1
DB_TYPE=sqlite DB_DSN="$RUNDIR/data/gateway.db" bash "$RUNDIR/run_server.sh" "$RUNDIR" "$PORT" "$RUNDIR/server-rr.log" "$RUNDIR/server-rr.pid"
BASE=http://127.0.0.1:$PORT
PW='E2eAdmin#2026'
TOK=$(curl -s -X POST $BASE/api/admin/v1/auth/login -H 'Content-Type: application/json' -d "{\"username\":\"admin\",\"password\":\"$PW\"}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["token"])')
# 3 providers, SAME base_url, DIFFERENT keys
P1=$(curl -s -X POST $BASE/api/admin/v1/providers -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d '{"name":"same-1","protocol":"openai_chat","base_url":"http://127.0.0.1:18081/v1","api_key":"sk-AAAA-1111","enabled":true}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["id"])')
P2=$(curl -s -X POST $BASE/api/admin/v1/providers -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d '{"name":"same-2","protocol":"openai_chat","base_url":"http://127.0.0.1:18081/v1","api_key":"sk-BBBB-2222","enabled":true}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["id"])')
P3=$(curl -s -X POST $BASE/api/admin/v1/providers -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d '{"name":"same-3","protocol":"openai_chat","base_url":"http://127.0.0.1:18081/v1","api_key":"sk-CCCC-3333","enabled":true}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["id"])')
curl -s -X POST $BASE/api/admin/v1/models -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d "{\"alias\":\"rr\",\"enabled\":true,\"upstreams\":[{\"provider_id\":$P1,\"upstream_model\":\"mock-small\",\"weight\":1},{\"provider_id\":$P2,\"upstream_model\":\"mock-small\",\"weight\":1},{\"provider_id\":$P3,\"upstream_model\":\"mock-small\",\"weight\":1}]}" >/dev/null
KEY=$(curl -s -X POST $BASE/api/admin/v1/apikeys -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d '{"name":"k"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["key"])')
sleep 4
echo "== 5 streamed requests, default settings (timeout=120s, guard on) =="
for i in 1 2 3 4 5; do
  curl -sN -X POST $BASE/v1/chat/completions -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' -d "{\"model\":\"rr\",\"stream\":true,\"messages\":[{\"role\":\"user\",\"content\":\"round-$i\"}]}" > "$RUNDIR/rr-$i.raw" 2>/dev/null
  python3 - "$RUNDIR/rr-$i.raw" <<'PY'
import sys, json
data=[]; done=False; bad=0; err=None
for line in open(sys.argv[1], encoding="utf-8", errors="replace"):
    line=line.strip()
    if not line.startswith("data:"): continue
    p=line[5:].strip()
    if p=="[DONE]": done=True; continue
    try: o=json.loads(p)
    except Exception: bad+=1; continue
    if isinstance(o, dict) and o.get("error"): err=o["error"]
    ch=o.get("choices") or []
    if ch and ch[0].get("delta",{}).get("content"): data.append(ch[0]["delta"]["content"])
print("  chunks=%d done=%s bad=%d err=%s content=%r" % (len(data), done, bad, err, "".join(data)))
PY
done
echo "== mock log: which keys arrived (round-robin check) =="
grep -a CHAT "$RUNDIR/mock-rr.log" | tail -6
echo "== gateway log: stream/upstream lines =="
grep -a "stream" "$RUNDIR/server-rr.log" | tail -8 || true
grep -a "access.*v1/chat" "$RUNDIR/server-rr.log" | tail -6
bash "$RUNDIR/stop_server.sh" "$RUNDIR/server-rr.pid" "$PORT" >/dev/null 2>&1 || true
echo RR_REPRO_DONE
