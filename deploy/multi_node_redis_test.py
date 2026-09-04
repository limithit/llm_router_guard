#!/usr/bin/env python3
"""Multi-node verification with Redis distributed state.

Usage: multi_node_redis_test.py BASE_A BASE_B ADMIN_PASSWORD
Two instances share PostgreSQL AND Redis (REDIS_ADDR set on both).

Differences vs. the no-Redis run:
  - rate limit is now GLOBAL: 2 via A, 3rd via B -> 429 (previously 200)
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
        with urllib.request.urlopen(r, timeout=30) as resp:
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
                         (" | " + str(detail)[:180]) if detail else ""))


def call(key, content, base):
    st, _ = req(base, "POST", "/v1/chat/completions", {
        "model": "mn-model",
        "messages": [{"role": "user", "content": content}]}, key=key)
    return st


def main():
    st, body = req(A, "POST", "/api/admin/v1/auth/login",
                   {"username": "admin", "password": PW})
    tokA = body["data"]["token"]
    st, body = req(B, "POST", "/api/admin/v1/auth/login",
                   {"username": "admin", "password": PW})
    tokB = body["data"]["token"]
    print("== both instances logged in ==")

    st, body = req(A, "POST", "/api/admin/v1/providers", {
        "name": "mn-provider", "protocol": "openai_chat",
        "base_url": "http://127.0.0.1:18081/v1",
        "api_key": "mock-key", "enabled": True}, token=tokA)
    pid = body["data"]["id"]
    st, body = req(A, "POST", "/api/admin/v1/models", {
        "alias": "mn-model", "enabled": True,
        "upstreams": [{"provider_id": pid, "upstream_model": "mock-small",
                       "weight": 1}]}, token=tokA)
    st, body = req(A, "POST", "/api/admin/v1/apikeys",
                   {"name": "mn-key", "remark": "mn"}, token=tokA)
    kid, key = body["data"]["id"], body["data"]["key"]

    seen = False
    for _ in range(10):
        st, body = req(B, "GET", "/api/admin/v1/models?page=1&page_size=50", token=tokB)
        items = (body.get("data") or {}).get("items") or []
        if any(i.get("alias") == "mn-model" for i in items):
            seen = True
            break
        time.sleep(1)
    step("config.propagation A->B", seen, "hot-reload poll")

    time.sleep(1)

    # rate limit GLOBAL via Redis: 2/10s — 3rd call from the OTHER instance blocked
    st, body = req(A, "POST", "/api/admin/v1/rate-limits", {
        "api_key_id": kid, "model_alias": "*", "window_seconds": 10,
        "max_requests": 2, "enabled": True}, token=tokA)
    assert st == 200, "rl create failed: %s" % body
    time.sleep(4)
    r1 = call(key, "rl1 (via A)", A)
    r2 = call(key, "rl2 (via A)", A)
    r3, _ = req(B, "POST", "/v1/chat/completions", {
        "model": "mn-model",
        "messages": [{"role": "user", "content": "rl3 (via B)"}]}, key=key)
    r4, _ = req(A, "POST", "/v1/chat/completions", {
        "model": "mn-model",
        "messages": [{"role": "user", "content": "rl4 (via A)"}]}, key=key)
    step("ratelimit.GLOBAL_via_redis",
         r1 == 200 and r2 == 200 and r3 == 429 and r4 == 429,
         "via A=%s,%s; via B=%s; again A=%s (expect 200,200,429,429)" % (r1, r2, r3, r4))
    req(A, "DELETE", "/api/admin/v1/rate-limits/%d" % body["data"]["id"], token=tokA)
    time.sleep(4)

    # window expiry: after TTL the same key allows again
    r5 = call(key, "rl5 after window", A)
    step("ratelimit.window_recovered", r5 == 200, "after 10s window: via A=%s" % r5)

    print("-- passed=%d failed=%d --" % (len(passed), len(failed)))
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
