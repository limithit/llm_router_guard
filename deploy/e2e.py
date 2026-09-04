#!/usr/bin/env python3
"""E2E test against a running llm-router-guard instance.

Usage: e2e.py BASE_URL ADMIN_PASSWORD
Covers: login, provider CRUD + test-connection, model alias, guard keyword
(input/output), API key, gateway non-stream + stream calls with usage,
call audit, token-stats, quota reject, rate limit, op audit, dashboard.
"""
import json
import sys
import time
import urllib.error
import urllib.request

BASE = sys.argv[1]
ADMIN_PW = sys.argv[2]

passed, failed = [], []


def req(method, path, body=None, token=None, key=None, timeout=30):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(BASE + path, data=data, method=method)
    if data:
        r.add_header("Content-Type", "application/json")
    if token:
        r.add_header("Authorization", "Bearer " + token)
    elif key:
        r.add_header("Authorization", "Bearer " + key)
    try:
        with urllib.request.urlopen(r, timeout=timeout) as resp:
            payload = resp.read()
            return resp.status, (json.loads(payload) if payload else {})
    except urllib.error.HTTPError as e:
        payload = e.read()
        try:
            return e.code, json.loads(payload)
        except Exception:
            return e.code, {"_raw": payload.decode("utf-8", "replace")}


def step(name, cond, detail=""):
    tag = "PASS" if cond else "FAIL"
    (passed if cond else failed).append(name)
    print("[%s] %s%s" % (tag, name, (" | " + str(detail)[:220]) if detail else ""))
    return cond


def sse_call(key, body):
    """Gateway stream call: returns (status, joined_content, usage, done)."""
    r = urllib.request.Request(BASE + "/v1/chat/completions",
                               data=json.dumps(body).encode(), method="POST")
    r.add_header("Content-Type", "application/json")
    r.add_header("Authorization", "Bearer " + key)
    chunks, usage, done = [], None, False
    try:
        with urllib.request.urlopen(r, timeout=30) as resp:
            status = resp.status
            for raw in resp:
                line = raw.decode("utf-8", "replace").strip()
                if line.startswith("data: "):
                    payload = line[6:]
                    if payload == "[DONE]":
                        done = True
                        break
                    obj = json.loads(payload)
                    if obj.get("usage"):
                        usage = obj["usage"]
                    for ch in obj.get("choices", []):
                        d = ch.get("delta", {})
                        if d.get("content"):
                            chunks.append(d["content"])
    except urllib.error.HTTPError as e:
        try:
            return e.code, "", usage, done
        except Exception:
            return e.code, "", usage, done
    return status, "".join(chunks), usage, done


def summary():
    print("-- passed=%d failed=%d --" % (len(passed), len(failed)))
    if failed:
        print("FAILED steps:", ", ".join(failed))
    return 1 if failed else 0


def main():
    print("== E2E against %s ==" % BASE)
    # 1. login
    st, body = req("POST", "/api/admin/v1/auth/login",
                   {"username": "admin", "password": ADMIN_PW})
    if not step("login", st == 200 and body.get("code") == 0
                and (body.get("data") or {}).get("token"), "http=%s" % st):
        return summary()
    tok = body["data"]["token"]

    # 2. provider create + test connection
    st, body = req("POST", "/api/admin/v1/providers", {
        "name": "e2e-mock-provider", "protocol": "openai_chat",
        "base_url": "http://127.0.0.1:18081/v1",
        "api_key": "mock-upstream-key", "enabled": True, "remark": "e2e"}, token=tok)
    if not step("provider.create", st == 200 and body.get("code") == 0
                and (body.get("data") or {}).get("id"), body.get("message")):
        return summary()
    pid = body["data"]["id"]
    st, body = req("POST", "/api/admin/v1/providers/%d/test" % pid, {}, token=tok)
    step("provider.test_connection",
         st == 200 and body.get("code") == 0 and (body.get("data") or {}).get("ok") is True,
         (body.get("data") or {}).get("message") or body.get("message"))

    # 3. model alias
    st, body = req("POST", "/api/admin/v1/models", {
        "alias": "e2e-model", "enabled": True, "remark": "e2e",
        "upstreams": [{"provider_id": pid, "upstream_model": "mock-small",
                       "weight": 1}]}, token=tok)
    step("model.create", st == 200 and body.get("code") == 0
         and (body.get("data") or {}).get("id"), body.get("message"))

    # 4. guard keyword (block) — used by input & output guard tests
    st, body = req("POST", "/api/admin/v1/guard/keywords", {
        "word": "SECRETWORD", "category": "test", "match_mode": "contains",
        "action": "block", "enabled": True}, token=tok)
    step("guard.keyword.create", st == 200 and body.get("code") == 0,
         body.get("message"))

    # 5. API key
    st, body = req("POST", "/api/admin/v1/apikeys",
                   {"name": "e2e-key", "remark": "e2e"}, token=tok)
    if not step("apikey.create", st == 200 and body.get("code") == 0
                and (body.get("data") or {}).get("key", "").startswith("sk-"),
                body.get("message")):
        return summary()
    kid = body["data"]["id"]
    key = body["data"]["key"]

    time.sleep(4)  # wait hot-reload (HotReloadSeconds=3)

    # 6. gateway non-stream call
    st, body = req("POST", "/v1/chat/completions", {
        "model": "e2e-model",
        "messages": [{"role": "user", "content": "hello e2e"}]}, key=key)
    content = ""
    usage = {}
    if st == 200:
        ch = (body.get("choices") or [{}])[0]
        content = (ch.get("message") or {}).get("content", "")
        usage = body.get("usage") or {}
    step("gateway.non_stream", st == 200 and content == "echo(hello e2e)"
         and usage.get("prompt_tokens") == 42 and usage.get("completion_tokens") == 17,
         "http=%s content=%r usage=%s" % (st, content, json.dumps(usage)))

    # 7. gateway stream call
    st, content, usage, done = sse_call(key, {
        "model": "e2e-model", "stream": True,
        "messages": [{"role": "user", "content": "stream e2e"}]})
    step("gateway.stream", st == 200 and done
         and content == "echo(stream e2e)"
         and usage and usage.get("prompt_tokens") == 11
         and usage.get("completion_tokens") == 5,
         "http=%s content=%r usage=%s done=%s" % (st, content, json.dumps(usage), done))

    # 8. input guard: block on keyword in user message
    st, body = req("POST", "/v1/chat/completions", {
        "model": "e2e-model",
        "messages": [{"role": "user", "content": "please explain SECRETWORD"}]}, key=key)
    step("guard.input_block", st in (400, 403, 422),
         "http=%s body=%s" % (st, json.dumps(body, ensure_ascii=False)[:160]))

    # 9. output guard: mock replies with SECRETWORD -> default strategy=replace,
    #    so expect 200 with the safe message and NO leak of SECRETWORD
    st, body = req("POST", "/v1/chat/completions", {
        "model": "e2e-model",
        "messages": [{"role": "user", "content": "__TRIGGER_OUTPUT__"}]}, key=key)
    out = ""
    if st == 200:
        ch = (body.get("choices") or [{}])[0]
        out = (ch.get("message") or {}).get("content", "")
    step("guard.output_replace_short",
         st == 200 and "SECRETWORD" not in out and "安全策略" in out,
         "http=%s content=%r" % (st, out[:120]))

    # 10. call audit: async flush every 200ms -> poll until visible
    ok_logs, blocked_cnt, total = [], 0, 0
    for _ in range(10):
        st, body = req("GET", "/api/admin/v1/audit/calls?page=1&page_size=50", token=tok)
        items = ((body.get("data") or {}).get("items")) if st == 200 else []
        ok_logs = [i for i in (items or []) if i.get("status") == "ok"
                   and (i.get("prompt_tokens") or 0) > 0]
        blocked_cnt = len([i for i in (items or []) if i.get("status") == "blocked"])
        total = (body.get("data") or {}).get("total") or 0
        if len(ok_logs) >= 2 and blocked_cnt >= 1:
            break
        time.sleep(1)
    step("audit.calls", st == 200 and len(ok_logs) >= 2 and blocked_cnt >= 1,
         "total=%s ok_with_tokens=%s blocked=%s" % (total, len(ok_logs), blocked_cnt))

    # 11. token-stats: 2 calls, >=75 tokens (poll past async flush)
    summ = {}
    for _ in range(10):
        st, body = req("GET", "/api/admin/v1/token-stats", token=tok)
        d = body.get("data") or {}
        summ = d.get("summary") or d
        if (summ.get("calls") or 0) >= 2 and (summ.get("total_tokens") or 0) >= 75:
            break
        time.sleep(1)
    step("token.stats", st == 200 and (summ.get("calls") or 0) >= 2
         and (summ.get("total_tokens") or 0) >= 75,
         "summary=%s" % json.dumps(summ, ensure_ascii=False)[:200])

    # 12. quota: limit 1 request/day -> second call rejected
    st, body = req("POST", "/api/admin/v1/quotas", {
        "api_key_id": kid, "model_alias": "*", "quota_type": "requests",
        "period": "day", "limit": 1, "over_action": "reject",
        "enabled": True}, token=tok)
    if not step("quota.create", st == 200 and body.get("code") == 0, body.get("message")):
        return summary()
    qid = body["data"]["id"]
    time.sleep(4)
    st1, _ = req("POST", "/v1/chat/completions", {
        "model": "e2e-model",
        "messages": [{"role": "user", "content": "quota first"}]}, key=key)
    st2, body2 = req("POST", "/v1/chat/completions", {
        "model": "e2e-model",
        "messages": [{"role": "user", "content": "quota second"}]}, key=key)
    step("quota.reject", st1 == 200 and st2 in (400, 403, 429),
         "first=%s second=%s body=%s" % (st1, st2, json.dumps(body2, ensure_ascii=False)[:160]))
    st, _ = req("DELETE", "/api/admin/v1/quotas/%d" % qid, token=tok)
    step("quota.delete", st == 200)
    time.sleep(4)

    # 13. rate limit: 2 req / 10s -> 3rd rejected
    st, body = req("POST", "/api/admin/v1/rate-limits", {
        "api_key_id": kid, "model_alias": "*", "window_seconds": 10,
        "max_requests": 2, "enabled": True}, token=tok)
    if not step("ratelimit.create", st == 200 and body.get("code") == 0, body.get("message")):
        return summary()
    rlid = body["data"]["id"]
    time.sleep(4)
    codes = []
    for i in range(3):
        st, _ = req("POST", "/v1/chat/completions", {
            "model": "e2e-model",
            "messages": [{"role": "user", "content": "rl %d" % i}]}, key=key)
        codes.append(st)
    step("ratelimit.enforce", codes[0] == 200 and codes[1] == 200 and codes[2] in (400, 403, 429),
         "codes=%s" % codes)
    st, _ = req("DELETE", "/api/admin/v1/rate-limits/%d" % rlid, token=tok)
    step("ratelimit.delete", st == 200)
    time.sleep(4)

    # 14. op audit recorded
    st, body = req("GET", "/api/admin/v1/audit/operations?page=1&page_size=5", token=tok)
    step("audit.operations", st == 200 and ((body.get("data") or {}).get("total") or 0) >= 3,
         "total=%s" % (body.get("data") or {}).get("total"))

    # 15. dashboard
    st, body = req("GET", "/api/admin/v1/dashboard", token=tok)
    step("dashboard", st == 200 and body.get("code") == 0, "")

    return summary()


if __name__ == "__main__":
    sys.exit(main())
