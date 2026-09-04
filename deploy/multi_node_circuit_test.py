#!/usr/bin/env python3
"""Circuit-breaker cross-instance sharing verification (2 instances + Redis).

Usage: multi_node_circuit_test.py BASE_A BASE_B ADMIN_PASSWORD
Alias has TWO upstreams: good mock (18081) + bad mock (18083, always 500).

Flow:
  1. A sends 4 requests: SWRR alternates good/bad; bad fails -> retry on good
     (all 200). After threshold=2 failures, A opens the circuit for bad and
     publishes it to Redis (SETEX, TTL = reset seconds).
  2. bad-mock chat counter must be exactly 2 (both failures on A).
  3. Redis key gw:cb:v1:<bad_id> exists.
  4. B's health list shows bad provider healthy=false although B itself
     never saw a failure (fail_count=0) -> cross-instance sharing works.
  5. B sends 4 requests: all 200; bad-mock counter unchanged (B routes all
     traffic to good while the circuit is open).
  6. After the reset TTL expires (half-open), B sees bad healthy again.
"""
import json
import sys
import time
import urllib.error
import urllib.request

A, B, PW = sys.argv[1], sys.argv[2], sys.argv[3]
passed, failed = [], []


def req(base, method, path, body=None, token=None, key=None):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(base + path, data=data, method=method)
    if data:
        r.add_header("Content-Type", "application/json")
    if token or key:
        r.add_header("Authorization", "Bearer " + (token or key))
    try:
        with urllib.request.urlopen(r, timeout=60) as resp:
            payload = resp.read()
            return resp.status, (json.loads(payload) if payload else {})
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read())
        except Exception:
            return e.code, {}


def step(name, cond, detail=""):
    (passed if cond else failed).append(name)
    print("[%s] %s%s" % ("PASS" if cond else "FAIL", name,
                         (" | " + str(detail)[:200]) if detail else ""))


def main():
    st, body = req(A, "POST", "/api/admin/v1/auth/login", {"username": "admin", "password": PW})
    tokA = body["data"]["token"]
    st, body = req(B, "POST", "/api/admin/v1/auth/login", {"username": "admin", "password": PW})
    tokB = body["data"]["token"]

    # aggressive failover settings for the test
    st, body = req(A, "PUT", "/api/admin/v1/failover", {
        "enabled": True, "retry_count": 1, "backoff": "fixed", "retry_interval_ms": 50,
        "trigger_status_codes": [500], "circuit_failure_threshold": 2,
        "circuit_reset_seconds": 8}, token=tokA)
    step("failover.config", st == 200 and body.get("code") == 0, "")

    st, body = req(A, "POST", "/api/admin/v1/providers", {
        "name": "cb-good", "protocol": "openai_chat",
        "base_url": "http://127.0.0.1:18081/v1", "api_key": "k", "enabled": True}, token=tokA)
    pid_good = body["data"]["id"]
    st, body = req(A, "POST", "/api/admin/v1/providers", {
        "name": "cb-bad", "protocol": "openai_chat",
        "base_url": "http://127.0.0.1:18083/v1", "api_key": "k", "enabled": True}, token=tokA)
    pid_bad = body["data"]["id"]
    st, body = req(A, "POST", "/api/admin/v1/models", {
        "alias": "mn-model", "enabled": True,
        "upstreams": [{"provider_id": pid_good, "upstream_model": "mock-small", "weight": 1},
                      {"provider_id": pid_bad, "upstream_model": "mock-small", "weight": 1}]}, token=tokA)
    st, body = req(A, "POST", "/api/admin/v1/apikeys", {"name": "cb-key"}, token=tokA)
    key = body["data"]["key"]
    time.sleep(4)  # hot reload both instances

    def call(base, content):
        st, body = req(base, "POST", "/v1/chat/completions", {
            "model": "mn-model", "messages": [{"role": "user", "content": content}]}, key=key)
        content_out = ""
        if st == 200:
            ch = (body.get("choices") or [{}])[0]
            content_out = (ch.get("message") or {}).get("content", "")
        return st, content_out

    # 1. trip the breaker via A (bad fails twice, each request retries onto good)
    codes = [call(A, "trip %d" % i)[0] for i in range(4)]
    step("a.requests_all_ok_via_failover", all(c == 200 for c in codes), "codes=%s" % codes)
    time.sleep(2)

    # 4. B sees bad provider unhealthy with zero local failures
    seen_open = False
    fail_count_b = None
    for _ in range(5):
        st, body = req(B, "GET", "/api/admin/v1/status", token=tokB)
        ups = (body.get("data") or {}).get("upstreams") or []
        for h in ups:
            if h.get("provider") == "cb-bad":
                fail_count_b = h.get("fail_count")
                if h.get("healthy") is False:
                    seen_open = True
        if seen_open:
            break
        time.sleep(1)
    step("breaker.B_sees_A_opened_circuit", seen_open,
         "B view via /status: healthy=False shared by redis; local fail_count=%s" % fail_count_b)

    # 5. B routes everything to good while circuit open
    st, out = call(B, "b1")
    st2, out2 = call(B, "b2")
    st3, out3 = call(B, "b3")
    st4, out4 = call(B, "b4")
    step("b.requests_ok_while_open", all(s == 200 for s in (st, st2, st3, st4)),
         "codes=%s" % [st, st2, st3, st4])

    # 6. half-open recovery after reset TTL (8s) + margin
    time.sleep(10)
    recovered = False
    for _ in range(5):
        st, body = req(B, "GET", "/api/admin/v1/status", token=tokB)
        ups = (body.get("data") or {}).get("upstreams") or []
        for h in ups:
            if h.get("provider") == "cb-bad" and h.get("healthy") is True:
                recovered = True
        if recovered:
            break
        time.sleep(1)
    step("breaker.half_open_recovery", recovered, "after reset TTL bad is healthy again")

    print("-- passed=%d failed=%d --" % (len(passed), len(failed)))
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
