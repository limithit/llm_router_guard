#!/usr/bin/env python3
"""Multi-node (2-instance) live verification.

Usage: multi_node_test.py BASE_A BASE_B ADMIN_PASSWORD
Both instances share one PostgreSQL/MySQL database and one mock upstream.

Verifies:
  1. config hot-reload propagation: provider created via A appears via B (<= ~4s poll)
  2. global quota counting: limit=2, 1 call via A + 1 call via B ok, 3rd via B -> 429
  3. rate limit is PER-INSTANCE (documented limitation): 2 calls via A pass,
     3rd sent via B still passes (would 429 if rate limiting were global)
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
    st, body = req(base, "POST", "/v1/chat/completions", {
        "model": "mn-model",
        "messages": [{"role": "user", "content": content}]}, key=key)
    return st


def main():
    st, body = req(A, "POST", "/api/admin/v1/auth/login",
                   {"username": "admin", "password": PW})
    assert st == 200 and body.get("code") == 0, "login A failed: %s" % body
    tokA = body["data"]["token"]
    st, body = req(B, "POST", "/api/admin/v1/auth/login",
                   {"username": "admin", "password": PW})
    assert st == 200 and body.get("code") == 0, "login B failed: %s" % body
    tokB = body["data"]["token"]
    print("== both instances logged in ==")

    # 1. config propagation: create provider+alias+key via A, verify via B
    st, body = req(A, "POST", "/api/admin/v1/providers", {
        "name": "mn-provider", "protocol": "openai_chat",
        "base_url": "http://127.0.0.1:18081/v1",
        "api_key": "mock-key", "enabled": True}, token=tokA)
    assert st == 200 and body.get("code") == 0, "provider create failed: %s" % body
    pid = body["data"]["id"]
    st, body = req(A, "POST", "/api/admin/v1/models", {
        "alias": "mn-model", "enabled": True,
        "upstreams": [{"provider_id": pid, "upstream_model": "mock-small",
                       "weight": 1}]}, token=tokA)
    assert st == 200, "alias create failed: %s" % body
    st, body = req(A, "POST", "/api/admin/v1/apikeys",
                   {"name": "mn-key", "remark": "mn"}, token=tokA)
    assert st == 200, "key create failed: %s" % body
    kid, key = body["data"]["id"], body["data"]["key"]

    seen = False
    for _ in range(10):
        st, body = req(B, "GET", "/api/admin/v1/models?page=1&page_size=50", token=tokB)
        items = (body.get("data") or {}).get("items") or []
        if any(i.get("alias") == "mn-model" for i in items):
            seen = True
            break
        time.sleep(1)
    step("config.propagation A->B", seen,
         "alias created on A became visible on B (hot-reload poll)")

    time.sleep(1)

    # 2. global quota: limit 2 requests, consume from BOTH instances
    st, body = req(A, "POST", "/api/admin/v1/quotas", {
        "api_key_id": kid, "model_alias": "*", "quota_type": "requests",
        "period": "day", "limit": 2, "over_action": "reject",
        "enabled": True}, token=tokA)
    assert st == 200, "quota create failed: %s" % body
    time.sleep(4)  # let B reload quotas
    st1 = call(key, "mn first (via A)", A)
    st2 = call(key, "mn second (via B)", B)
    st3, body3 = req(B, "POST", "/v1/chat/completions", {
        "model": "mn-model",
        "messages": [{"role": "user", "content": "mn third (via B)"}]}, key=key)
    step("quota.global_across_instances",
         st1 == 200 and st2 == 200 and st3 == 429,
         "via A=%s, via B=%s, 3rd via B=%s (expect 429)" % (st1, st2, st3))
    # wait for used_value to settle before removing the quota
    time.sleep(2)
    req(A, "DELETE", "/api/admin/v1/quotas/%d" % body["data"]["id"], token=tokA)
    time.sleep(4)

    # 3. rate limit: 2/10s — 3rd call from the OTHER instance still passes
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
    step("ratelimit.per_instance_only",
         r1 == 200 and r2 == 200 and r3 == 200,
         "2 via A ok, 3rd via B=%s (200 confirms NOT global; documented #14)" % r3)
    req(A, "DELETE", "/api/admin/v1/rate-limits/%d" % body["data"]["id"], token=tokA)

    print("-- passed=%d failed=%d --" % (len(passed), len(failed)))
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
