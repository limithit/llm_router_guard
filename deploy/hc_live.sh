#!/bin/bash
# Live checks after E2E: X-Request-ID passthrough, protocol rejection, health-checker dead-provider trip.
set -e
BASE=http://127.0.0.1:18080
PW='E2eAdmin#2026'
TOK=$(curl -s -X POST $BASE/api/admin/v1/auth/login -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$PW\"}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["token"])')

PID=$(curl -s -X POST $BASE/api/admin/v1/providers -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"name":"hc-good","protocol":"openai_chat","base_url":"http://127.0.0.1:18081/v1","api_key":"mk","enabled":true}' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["id"])')
curl -s -X POST $BASE/api/admin/v1/models -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d "{\"alias\":\"hc-model\",\"enabled\":true,\"upstreams\":[{\"provider_id\":$PID,\"upstream_model\":\"mock-small\",\"weight\":1}]}" >/dev/null
KEY=$(curl -s -X POST $BASE/api/admin/v1/apikeys -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"name":"hc-key"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["key"])')
sleep 1

echo "== 1. X-Request-ID passthrough (inbound reused in response header) =="
RID=$(curl -s -D - -o /dev/null -X POST $BASE/v1/chat/completions \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' -H 'X-Request-ID: trace-live-42' \
  -d '{"model":"hc-model","messages":[{"role":"user","content":"hi"}]}' | grep -i '^x-request-id' | tr -d '\r' | awk '{print $2}')
if [ "$RID" = "trace-live-42" ]; then echo "[PASS] requestid.echo_inbound (X-Request-ID: $RID)"; else echo "[FAIL] requestid.echo got '$RID'"; fi
GEN=$(curl -s -D - -o /dev/null -X POST $BASE/v1/chat/completions \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"model":"hc-model","messages":[{"role":"user","content":"hi"}]}' | grep -i '^x-request-id' | tr -d '\r' | awk '{print $2}')
if [ -n "$GEN" ] && [ "$GEN" != "trace-live-42" ]; then echo "[PASS] requestid.generated_when_absent ($GEN)"; else echo "[FAIL] requestid.generated got '$GEN'"; fi

echo "== 2. invalid protocol rejected at create =="
CODE=$(curl -s -o /tmp/badproto.json -w '%{http_code}' -X POST $BASE/api/admin/v1/providers \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"name":"bad-proto","protocol":"openai","base_url":"http://127.0.0.1:9"}')
cat /tmp/badproto.json; echo
if [ "$CODE" = "400" ]; then echo "[PASS] protocol.reject_invalid (HTTP $CODE)"; else echo "[FAIL] protocol.reject got HTTP $CODE"; fi

echo "== 3. health checker trips dead provider (threshold 5, interval 5s) =="
curl -s -X POST $BASE/api/admin/v1/providers -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"name":"hc-dead","protocol":"openai_chat","base_url":"http://127.0.0.1:9/v1","api_key":"x","enabled":true}' >/dev/null
sleep 33
curl -s $BASE/api/admin/v1/status -H "Authorization: Bearer $TOK" | python3 -c '
import sys, json
ups = json.load(sys.stdin)["data"]["upstreams"]
for u in ups:
    print("  ", u["provider"], "healthy=", u["healthy"], "fails=", u["fail_count"], "err=", (u.get("last_error") or "")[:70])
dead = [u for u in ups if u["provider"] == "hc-dead"]
good = [u for u in ups if u["provider"] == "hc-good"]
assert dead and dead[0]["healthy"] is False and dead[0]["fail_count"] >= 5, "dead provider should be circuit-open by health checker"
print("[PASS] healthcheck.dead_provider_tripped")
assert good and good[0]["healthy"] is True, "good provider must stay healthy"
print("[PASS] healthcheck.good_provider_stays_healthy")
'
echo "ALL_LIVE_CHECKS_DONE"
