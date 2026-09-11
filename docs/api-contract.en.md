> 🌐 **English** | [中文](api-contract.md)

# LLM Router Guard — API Contract (sole reference for frontend-backend integration)

Base URL (Admin API): `/api/admin/v1`
Auth header: `Authorization: Bearer <JWT>` (except the login endpoint and gateway endpoints)
Gateway endpoints: `POST /v1/chat/completions` `POST /v1/responses` `POST /v1/messages` `GET /v1/models` (model catalog), auth header `Authorization: Bearer sk-...` or `x-api-key: sk-...`
Health check: `GET /healthz` (no auth required)

## Unified Response Envelope

```json
{ "code": 0, "message": "ok", "data": ... }
```

- `code=0` means success; non-zero means failure (the corresponding HTTP status code is also returned: 400/401/403/404/409/429/500).
- On failure, `data=null` and `message` is an error description that the frontend displays directly via toast.
- Paginated data: `data = { "items": [...], "total": 123, "page": 1, "page_size": 20 }`.
- Time fields are uniformly RFC3339 strings (e.g. `2026-09-02T14:30:22+08:00`).

## Common Enums

| Enum | Values |
|------|------|
| Provider.protocol | `openai_chat` \| `openai_responses` \| `anthropic` |
| Keyword.category | `political` \| `porn` \| `violence` \| `illegal` \| `discrimination` \| `custom` |
| Rule.match_mode | `contains` \| `exact` \| `regex` |
| Rule.action | `block` \| `warn` \| `log` \| `mask`(PII only) |
| Quota.quota_type | `requests` \| `tokens` |
| Quota.period | `day` \| `week` \| `month` |
| Quota.over_action | `reject` \| `degrade` |
| CallLog.status | `ok` \| `error` \| `blocked` \| `rate_limited` \| `quota_exceeded` |
| OpLog.action | `create` \| `update` \| `delete` \| `enable` \| `disable` \| `reload` \| `rollback` \| `reset` \| `login` \| `mfa_bind` \| `mfa_unbind` \| `backup` \| `restore` |
| OpLog.module | `provider` \| `model_alias` \| `failover` \| `guard` \| `quota` \| `rate_limit` \| `apikey` \| `settings` \| `security` \| `backup` \| `auth` |

---

## 1. Authentication / MFA

### POST /auth/login
```json
{ "username": "admin", "password": "***", "totp_code": "123456" }
```
- Password correct but MFA not bound → `data = { "token": "***", "user": {...}, "need_bind_mfa": true }`
- Password correct and MFA bound, but `totp_code` missing/incorrect → `data = { "mfa_required": true, "mfa_token": "..." }` (HTTP 200, `token` is empty)
- Call again with a valid `mfa_token` + `totp_code` (or `recovery_code`) to complete verification → returns the official token
- After 5 consecutive incorrect verification codes, the account is locked (indicated in `message`).

`user` object: `{ "id":1, "username":"admin", "mfa_enabled":false, "last_login_at":"..." }`

### GET /auth/me → `{ id, username, mfa_enabled, created_at, last_login_at }`
### PUT /auth/password → body `{ "old_password":"", "new_password":"" }`
### POST /auth/logout → `data = null`

### Account MFA self-service binding (requires token)
- `POST /account/mfa/setup` → `{ "secret":"JBSWY3DP...", "otpauth_url":"otpauth://...", "qr_png_base64":"<base64 png; the frontend renders it via data:image/png;base64,...>" }`
- `POST /account/mfa/enable` body `{ "code":"123456" }` → `{ "recovery_codes":["xxxx-....", ...] }` (shown only once)
- `POST /account/mfa/disable` body `{ "code":"123456 or recovery code" }`
- `GET /account/mfa/status` → `{ "enabled":true, "bound_at":"...", "recovery_codes_left":7 }`

---

## 2. Dashboard (REQ-016)

`GET /dashboard` →
```json
{
  "today": { "calls": 1200, "success": 1100, "blocked": 42, "errors": 8, "avg_latency_ms": 850 },
  "trend_7d": [ { "date": "2026-09-01", "calls": 900, "blocked": 12 } ],
  "top_models": [ { "model": "gpt-4", "calls": 500 } ],
  "block_categories": [ { "category": "keyword.political", "count": 20 } ],
  "upstream_health": [ { "provider": "openai-main", "protocol": "openai_chat", "healthy": true, "fail_count": 0 } ],
  "quota_usage": [ { "name": "team-a / gpt-4 / day-requests", "used": 120, "limit": 1000 } ],
  "system": { "version": "1.0.0", "uptime_seconds": 3600, "config_status": "ok", "go_version": "go1.24" }
}
```

---

## 3. Provider Management (REQ-005)

| Method | Path | Description |
|------|------|------|
| GET | `/providers?page=&page_size=&keyword=` | Paginated list |
| POST | `/providers` | Create |
| PUT | `/providers/{id}` | Update (pass an empty string for `api_key` = no change) |
| DELETE | `/providers/{id}` | Delete; returns HTTP 409 when referenced by a model alias |
| POST | `/providers/{id}/test` | Test connection |

Provider object:
```json
{ "id":1, "name":"openai-main", "protocol":"openai_chat", "base_url":"https://api.openai.com/v1",
  "api_key_masked":"sk-ab****yz", "enabled":true, "remark":"", "created_at":"...", "updated_at":"..." }
```
Create/Update body: `{ "name","protocol","base_url","api_key","enabled","remark" }`
Test result: `{ "ok":true, "latency_ms":230, "message":"Connection successful" }`

---

## 4. Model Aliases (REQ-006)

| Method | Path |
|------|------|
| GET | `/models?page=&page_size=&keyword=` |
| POST | `/models` |
| PUT | `/models/{id}` |
| DELETE | `/models/{id}` |
| GET | `/models/{id}/stats` |

ModelAlias object:
```json
{ "id":1, "alias":"gpt-4", "enabled":true, "remark":"",
  "upstreams":[ { "provider_id":1, "provider_name":"openai-main", "upstream_model":"gpt-4-0613", "weight":70 } ],
  "created_at":"...", "updated_at":"..." }
```
Create/Update body: `{ "alias","enabled","remark","upstreams":[{"provider_id","upstream_model","weight"}] }`
stats: `{ "calls_7d":123, "calls_today":20, "blocked_7d":3, "avg_latency_ms":900 }`

---

## 5. Failover (REQ-007)

`GET /failover` / `PUT /failover`, object:
```json
{ "enabled":true, "retry_count":2, "backoff":"fixed|exponential", "retry_interval_ms":200,
  "trigger_status_codes":[429,500,502,503], "circuit_failure_threshold":5, "circuit_reset_seconds":30 }
```

---

## 6. Guardrails Management

### 6.1 Sensitive Keywords (REQ-008)
| Method | Path | Description |
|------|------|------|
| GET | `/guard/keywords?page=&page_size=&keyword=&category=&enabled=&action=` | List |
| POST | `/guard/keywords` | Create single |
| PUT | `/guard/keywords/{id}` | Update |
| DELETE | `/guard/keywords/{id}` | Delete |
| POST | `/guard/keywords/batch` | Batch import; body `{ "items":[{word,category,match_mode,action,enabled}], "mode":"merge|replace" }` |
| GET | `/guard/keywords/export?format=csv` | Returns CSV text (`text/csv`, no envelope) |
| POST | `/guard/keywords/import` | body `{ "csv":"word,category,match_mode,action\n..." }` → `{ "created":10, "skipped":2 }` |
| POST | `/guard/keywords/test` | body `{ "text":"..." }` → see below |

Keyword object: `{ "id":1,"word":"xxx","category":"political","match_mode":"contains","action":"block","enabled":true,"created_at":"..." }`

Test response: `{ "blocked":true, "hits":[ { "type":"keyword","value":"xxx","category":"political","action":"block" } ] }`

### 6.2 PII Rules (REQ-009)
| Method | Path |
|------|------|
| GET | `/guard/pii-rules?page=&page_size=&keyword=` |
| POST/PUT/DELETE | `/guard/pii-rules` `/guard/pii-rules/{id}` |
| GET | `/guard/pii-rules/templates` |
| POST | `/guard/pii-rules/test` body `{ "text":"..." }` |

PIIRule object: `{ "id":1,"name":"Phone number","category":"phone","pattern":"1[3-9]\\d{9}","replacement":"[PHONE]","action":"mask","enabled":true,"created_at":"..." }`
templates returns an array that can be batch-created (phone/id_card/email/bank_card/address).
Test response: `{ "blocked":false, "findings":[ { "rule":"Phone number","category":"phone","sample":"138****8000" } ], "masked_text":"..." }`

### 6.3 Injection Rules (REQ-010)
CRUD `/guard/injection-rules[/{id}]`; `GET /guard/injection-rules/templates`.
InjectionRule object: `{ "id":1,"name":"Ignore system instructions","pattern":"ignore (all |the )?previous instructions","match_mode":"regex","action":"block","enabled":true,"created_at":"..." }`

### 6.4 Output Filtering (REQ-011)
`GET /guard/output` / `PUT /guard/output`:
```json
{ "enabled":true, "violation_strategy":"replace|block|log", "safe_message":"Sorry, this response contains inappropriate content.", "stream_chunk_threshold":256 }
```

---

## 7. Quotas and Rate Limits

### 7.1 Quotas (REQ-012)
| Method | Path | Description |
|------|------|------|
| GET | `/quotas?page=&page_size=` | List (includes real-time `used`) |
| POST | `/quotas` | Create |
| PUT | `/quotas/{id}` | Update |
| DELETE | `/quotas/{id}` | Delete |
| POST | `/quotas/{id}/reset` | Manually reset `used` to 0 |
| POST | `/quotas/batch` | body `{ "items":[QuotaInput...] }` batch create |

Quota object:
```json
{ "id":1, "api_key_id":2, "api_key_label":"team-a", "model_alias":"gpt-4",
  "quota_type":"requests", "period":"day", "limit":1000, "used":120,
  "over_action":"reject", "degrade_alias":"gpt-4-mini", "enabled":true,
  "reset_at":"2026-09-03T00:00:00+08:00", "created_at":"..." }
```
Create/Update body: `{ api_key_id, model_alias, quota_type, period, limit, over_action, degrade_alias, enabled }`
`model_alias` can be `"*"` to mean all models.

### 7.2 Rate Limits (REQ-013)
CRUD `/rate-limits[/{id}]`; object:
```json
{ "id":1, "api_key_id":0, "api_key_label":"(global)", "model_alias":"*",
  "window_seconds":60, "max_requests":60, "enabled":true, "total_hits":12 }
```

### 7.3 Quota Alerts (REQ-014)
`GET /quota-alerts` / `PUT /quota-alerts`:
```json
{ "enabled":false, "thresholds":[80,95], "channel":"ui|webhook", "webhook_url":"", "receivers":"" }
```

---

## 8. Audit Logs

### 8.1 Call Logs (REQ-015)
`GET /audit/calls?start=&end=&api_key_id=&model=&status=&blocked=&category=&page=&page_size=`
Row object:
```json
{ "request_id":"uuid", "created_at":"...", "api_key_label":"team-a", "protocol":"openai_chat",
  "model_alias":"gpt-4", "upstream":"openai-main/gpt-4-0613",
  "input_preview":"hello... (masked, truncated 200 chars)", "output_preview":"...",
  "prompt_tokens":12, "completion_tokens":34, "latency_ms":850,
  "status":"ok", "blocked":false, "block_category":"", "block_reason":"" }
```
`GET /audit/calls/{request_id}` → details; additionally includes `"input_text"`, `"output_text"`, `"error_msg"`, `"guard_findings":[...]` (JSON string).
`GET /audit/calls/export?format=csv&<same filters as above>` → CSV text (no envelope).

### 8.2 Operation Audit (REQ-003)
`GET /audit/operations?start=&end=&operator=&module=&action=&page=&page_size=`
Row object:
```json
{ "id":1, "created_at":"...", "operator":"admin", "action":"update", "module":"guard",
  "target":"keyword#12", "before_json":"{...}", "after_json":"{...}", "ip":"10.0.0.1", "effective":true }
```
`GET /audit/operations/export?format=csv&...` → CSV text.

### 8.3 Token Usage Statistics (per-API-Key dashboard)
`GET /token-stats?start=&end=&api_key_id=&model=&granularity=day|hour`
- Data source: aggregated from `call_logs`. The time range is inclusive on both ends (`created_at`); when `start` is omitted, the last 30 days are used by default.
- `granularity` defaults to `day`; `hour` buckets by the hour (suitable for short time windows).
- `by_key`: token consumption per API Key (includes zero-usage keys, sorted by total descending; deleted keys that still have historical logs also appear under their historical label).
- `by_model`: token consumption aggregated by model alias.
- `trend`: a usage time series bucketed by day/hour; supports historical queries.
Response `data`:
```json
{
  "range": { "start":"...", "end":"...", "granularity":"day" },
  "summary": { "calls":123, "prompt_tokens":1000, "completion_tokens":2000,
               "total_tokens":3000, "keys_with_usage":5, "total_keys":8 },
  "by_key": [ { "api_key_id":1, "api_key_label":"team-a", "calls":10,
                "prompt_tokens":100, "completion_tokens":200, "total_tokens":300 } ],
  "by_model": [ { "model":"gpt-4", "calls":5, "prompt_tokens":100,
                  "completion_tokens":200, "total_tokens":300 } ],
  "trend": [ { "bucket":"2026-09-01", "calls":50, "prompt_tokens":1000,
               "completion_tokens":2000, "total_tokens":3000 } ]
}
```
`GET /token-stats/export?format=csv&<same filters as above>` → CSV text (a per-key usage table, no envelope).

---

## 9. System Settings

### 9.1 General Settings (REQ-001)
`GET /settings` / `PUT /settings`:
```json
{ "log_level":"info|debug|warn|error", "audit_retention_days":90, "default_timeout_seconds":120,
  "max_connections":1000, "guard_enabled":true, "hot_reload_seconds":3,
  "listen_port":8080, "listen_port_note":"Only effective as a startup parameter; restart required after change" }
```

### 9.2 API Key Management (REQ-002)
| Method | Path |
|------|------|
| GET | `/apikeys?page=&page_size=&keyword=` |
| POST | `/apikeys` body `{ "name","remark" }` → `data` additionally returns the full `"key":"sk-..."` once |
| PUT | `/apikeys/{id}` body `{ "name","remark","enabled" }` |
| DELETE | `/apikeys/{id}` |

ApiKey object: `{ "id":1,"name":"team-a","key_masked":"sk-ab****yz","remark":"","enabled":true,"last_used_at":"...","created_at":"..." }`

### 9.3 Security Settings (REQ-020/023)
`GET /settings/security` / `PUT /settings/security`:
```json
{ "mfa_enabled":true, "mfa_required_for_all":false, "recovery_code_count":10, "grace_days":7 }
```
User MFA status: `GET /users?page=&page_size=` → row `{ "id":1,"username":"admin","mfa_enabled":true,"last_login_at":"...","locked":false }`
`POST /users/{id}/unbind-mfa` (admin force-unbind; recorded in audit); `POST /users/{id}/unlock`.

### 9.4 Configuration Status (REQ-004A)
`GET /config-status` →
```json
{
  "version": "v20260902.143022",
  "last_loaded_at": "2026-09-02T14:30:22+08:00",
  "status": "ok",
  "error_message": "",
  "modules": { "providers":12, "model_aliases":8, "keywords":156, "pii_rules":6, "injection_rules":4, "quotas":23, "rate_limits":5, "apikeys":10 },
  "logs": [ { "time":"...", "module":"providers", "status":"success|error", "message":"Providers config loaded (12 items)" } ],
  "history": [ { "version":"v20260902.143022", "created_at":"...", "status":"ok", "message":"..." } ]
}
```
`POST /config/reload` → reload immediately, `data=null`
`POST /config/rollback` → roll back to the previous version, `data={ "rolled_back_to":"v20260902.142500" }`
`GET /config/export?format=json` → full configuration as JSON text (no envelope)

---

## 10. System Operations

### 10.0 Prometheus Metrics Export (P2 #6)
`GET /metrics` (no JWT auth; restricting to the monitoring network segment is recommended) → `text/plain; version=0.0.4`.
Metric prefix `gw_`:

| Metric | Type | Description |
|------|------|------|
| `gw_uptime_seconds` | gauge | Process uptime in seconds |
| `gw_active_connections` | gauge | Current active connections |
| `gw_requests_per_second` | gauge | QPS across all models (60s window) |
| `gw_model_requests_per_second{model}` | gauge | Per-model QPS |
| `gw_model_errors_1m{model}` | gauge | Error count over the last 1 minute, per model |
| `gw_upstream_healthy{provider,protocol}` | gauge | Upstream reachable (1 = healthy, 0 = circuit breaker open / unreachable; includes shared Redis state) |
| `gw_upstream_consecutive_failures{provider,protocol}` | gauge | Circuit breaker consecutive failure count |
| `go_*` | — | Go runtime (heap/gc/goroutines/mutex, etc.) |

### 10.0a Real-time Call Audit Stream (P2 #5)
`GET /api/admin/v1/audit/ws?token=<JWT>` (WebSocket upgrade; browsers cannot set a custom Authorization header, so the token is passed via the query string; on auth failure, a 401 is returned before the upgrade).
The server pushes one JSON-text frame for every call audit it persists (fields = model.CallLog JSON: `request_id / created_at / api_key_label / protocol / model_alias / upstream_provider / upstream_model / prompt_tokens / completion_tokens / latency_ms / status / blocked / block_category / block_reason`). The server sends a 30s ping keepalive; once the slow-consumer drop count reaches the threshold (512), it proactively closes with 1013, and the client reconnects to resume.

### 10.1 Status Monitoring (REQ-018)
`GET /status` →
```json
{
  "running": true, "uptime_seconds": 7200, "version": "1.0.0",
  "memory_alloc_kb": 32000, "cpu_percent": 4.2, "goroutines": 35,
  "open_connections": 12,
  "upstreams": [ { "provider":"openai-main","healthy":true,"fail_count":0,"last_error":"" } ],
  "qps_by_model": [ { "model":"gpt-4","qps":1.2,"errors_1m":0 } ],
  "recent_errors": [ { "time":"...", "request_id":"...", "message":"..." } ],
  "config_status": { "version":"v...", "last_loaded_at":"...", "status":"ok" }
}
```

### 10.2 Data Backup (REQ-019)
| Method | Path | Description |
|------|------|------|
| GET | `/backups` | List `{ items:[{id,filename,created_at,size_bytes}] }` |
| POST | `/backups` | Manual backup |
| GET | `/backups/{id}/download` | Download JSON (no envelope) |
| POST | `/backups/restore` | body `{ "id":1 }` or `{ "content":"<json text>" }` |
| DELETE | `/backups/{id}` | Delete |

---

## 11. Gateway Endpoints (business side; the frontend only documents these for display)

- `POST /v1/chat/completions` — OpenAI Chat Completions (includes `stream:true` SSE)
- `POST /v1/responses` — OpenAI Responses API
- `POST /v1/messages` — Anthropic Messages (includes SSE)
- `GET /v1/models` — Model catalog: returns the union of OpenAI and Anthropic fields `{"object":"list","data":[{"id":<alias>,"object":"model","type":"model","display_name":<alias>,"owned_by":<provider>,"created":0,"created_at":"1970-01-01T00:00:00Z"}],"has_more":false,"first_id":...,"last_id":...}`, so that the OpenAI Chat, OpenAI Responses, and Anthropic SDKs can all parse it; filtered by the API Key's model scope

In requests, the `model` field holds a model alias; the gateway applies SLB / guardrails / quota / audit by alias, and returns errors in the corresponding protocol's style
(OpenAI: `{"error":{"message","type","code"}}`; Anthropic: `{"type":"error","error":{"type","message"}}`; on interception, `type="content_policy_violation"` / `http 451` → unified as `invalid_request_error` + code=`content_filtered`).

---

## 12. Error Code Conventions

| code | HTTP | Meaning |
|------|------|------|
| 0 | 200 | Success |
| 40001 | 400 | Bad request / parameter error |
| 40101 | 401 | Not logged in / token invalid |
| 40102 | 401 | Incorrect username or password |
| 40103 | 401 | Incorrect MFA verification code |
| 40301 | 403 | No permission / account locked |
| 40401 | 404 | Resource not found |
| 40901 | 409 | Resource conflict (duplicate name / referenced) |
| 42901 | 429 | Rate limit triggered |
| 50001 | 500 | Internal server error |
