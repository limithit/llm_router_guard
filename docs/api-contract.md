# LLM Router Guard — API 契约（前后端对接唯一依据）

Base URL（管理 API）: `/api/admin/v1`
认证头: `Authorization: Bearer <JWT>`（登录接口与网关端点除外）
网关端点: `POST /v1/chat/completions` `POST /v1/responses` `POST /v1/messages`，认证头 `Authorization: Bearer sk-...` 或 `x-api-key: sk-...`
健康检查: `GET /healthz`（无需认证）

## 统一响应包裹

```json
{ "code": 0, "message": "ok", "data": ... }
```

- `code=0` 成功；非 0 失败（同时返回相应 HTTP 状态码：400/401/403/404/409/429/500）。
- 失败时 `data=null`，`message` 为中文错误描述，前端直接 toast。
- 分页数据：`data = { "items": [...], "total": 123, "page": 1, "page_size": 20 }`。
- 时间字段统一为 RFC3339 字符串（如 `2026-09-02T14:30:22+08:00`）。

## 通用枚举

| 枚举 | 取值 |
|------|------|
| Provider.protocol | `openai_chat` \| `openai_responses` \| `anthropic` |
| Keyword.category | `political` \| `porn` \| `violence` \| `illegal` \| `discrimination` \| `custom` |
| Rule.match_mode | `contains` \| `exact` \| `regex` |
| Rule.action | `block` \| `warn` \| `log` \| `mask`(仅 PII) |
| Quota.quota_type | `requests` \| `tokens` |
| Quota.period | `day` \| `week` \| `month` |
| Quota.over_action | `reject` \| `degrade` |
| CallLog.status | `ok` \| `error` \| `blocked` \| `rate_limited` \| `quota_exceeded` |
| OpLog.action | `create` \| `update` \| `delete` \| `enable` \| `disable` \| `reload` \| `rollback` \| `reset` \| `login` \| `mfa_bind` \| `mfa_unbind` \| `backup` \| `restore` |
| OpLog.module | `provider` \| `model_alias` \| `failover` \| `guard` \| `quota` \| `rate_limit` \| `apikey` \| `settings` \| `security` \| `backup` \| `auth` |

---

## 1. 认证 / MFA

### POST /auth/login
```json
{ "username": "admin", "password": "***", "totp_code": "123456" }
```
- 密码正确但未绑定 MFA → `data = { "token": "***", "user": {...}, "need_bind_mfa": true }`
- 密码正确且已绑定 MFA、未传/传错 totp_code → `data = { "mfa_required": true, "mfa_token": "..." }`（HTTP 200，`token` 为空）
- 携带有效 `mfa_token` + `totp_code`（或 `recovery_code`）再次调用完成验证 → 返回正式 token
- 连续 5 次验证码错误锁定账户（message 提示）。

`user` 对象: `{ "id":1, "username":"admin", "mfa_enabled":false, "last_login_at":"..." }`

### GET /auth/me → `{ id, username, mfa_enabled, created_at, last_login_at }`
### PUT /auth/password → body `{ "old_password":"", "new_password":"" }`
### POST /auth/logout → `data = null`

### 账户 MFA 自助绑定（需 token）
- `POST /account/mfa/setup` → `{ "secret":"JBSWY3DP...", "otpauth_url":"otpauth://...", "qr_png_base64":"<base64 png，前端用 data:image/png;base64,... 展示>" }`
- `POST /account/mfa/enable` body `{ "code":"123456" }` → `{ "recovery_codes":["xxxx-....", ...] }`（仅此一次展示）
- `POST /account/mfa/disable` body `{ "code":"123456 或 recovery code" }`
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

## 3. 供应商管理 (REQ-005)

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/providers?page=&page_size=&keyword=` | 分页列表 |
| POST | `/providers` | 创建 |
| PUT | `/providers/{id}` | 更新（`api_key` 传空字符串=不修改） |
| DELETE | `/providers/{id}` | 删除；被模型别名引用时 HTTP 409 |
| POST | `/providers/{id}/test` | 测试连接 |

Provider 对象：
```json
{ "id":1, "name":"openai-main", "protocol":"openai_chat", "base_url":"https://api.openai.com/v1",
  "api_key_masked":"sk-ab****yz", "enabled":true, "remark":"", "created_at":"...", "updated_at":"..." }
```
创建/更新 body：`{ "name","protocol","base_url","api_key","enabled","remark" }`
测试结果：`{ "ok":true, "latency_ms":230, "message":"连接成功" }`

---

## 4. 模型别名 (REQ-006)

| 方法 | 路径 |
|------|------|
| GET | `/models?page=&page_size=&keyword=` |
| POST | `/models` |
| PUT | `/models/{id}` |
| DELETE | `/models/{id}` |
| GET | `/models/{id}/stats` |

ModelAlias 对象：
```json
{ "id":1, "alias":"gpt-4", "enabled":true, "remark":"",
  "upstreams":[ { "provider_id":1, "provider_name":"openai-main", "upstream_model":"gpt-4-0613", "weight":70 } ],
  "created_at":"...", "updated_at":"..." }
```
创建/更新 body：`{ "alias","enabled","remark","upstreams":[{"provider_id","upstream_model","weight"}] }`
stats：`{ "calls_7d":123, "calls_today":20, "blocked_7d":3, "avg_latency_ms":900 }`

---

## 5. 故障转移 (REQ-007)

`GET /failover` / `PUT /failover`，对象：
```json
{ "enabled":true, "retry_count":2, "backoff":"fixed|exponential", "retry_interval_ms":200,
  "trigger_status_codes":[429,500,502,503], "circuit_failure_threshold":5, "circuit_reset_seconds":30 }
```

---

## 6. 护栏管理

### 6.1 敏感词 (REQ-008)
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/guard/keywords?page=&page_size=&keyword=&category=&enabled=&action=` | 列表 |
| POST | `/guard/keywords` | 单条创建 |
| PUT | `/guard/keywords/{id}` | 更新 |
| DELETE | `/guard/keywords/{id}` | 删除 |
| POST | `/guard/keywords/batch` | 批量导入 body `{ "items":[{word,category,match_mode,action,enabled}], "mode":"merge|replace" }` |
| GET | `/guard/keywords/export?format=csv` | 返回 CSV 文本（`text/csv`，不包 envelope） |
| POST | `/guard/keywords/import` | body `{ "csv":"word,category,match_mode,action\n..." }` → `{ "created":10, "skipped":2 }` |
| POST | `/guard/keywords/test` | body `{ "text":"..." }` → 见下 |

Keyword 对象：`{ "id":1,"word":"xxx","category":"political","match_mode":"contains","action":"block","enabled":true,"created_at":"..." }`

测试响应：`{ "blocked":true, "hits":[ { "type":"keyword","value":"xxx","category":"political","action":"block" } ] }`

### 6.2 PII 规则 (REQ-009)
| 方法 | 路径 |
|------|------|
| GET | `/guard/pii-rules?page=&page_size=&keyword=` |
| POST/PUT/DELETE | `/guard/pii-rules` `/guard/pii-rules/{id}` |
| GET | `/guard/pii-rules/templates` |
| POST | `/guard/pii-rules/test` body `{ "text":"..." }` |

PIIRule 对象：`{ "id":1,"name":"手机号","category":"phone","pattern":"1[3-9]\\d{9}","replacement":"[PHONE]","action":"mask","enabled":true,"created_at":"..." }`
templates 返回可批量创建数组（phone/id_card/email/bank_card/address）。
测试响应：`{ "blocked":false, "findings":[ { "rule":"手机号","category":"phone","sample":"138****8000" } ], "masked_text":"..." }`

### 6.3 注入规则 (REQ-010)
CRUD `/guard/injection-rules[/{id}]`；`GET /guard/injection-rules/templates`。
InjectionRule 对象：`{ "id":1,"name":"忽略系统指令","pattern":"ignore (all |the )?previous instructions","match_mode":"regex","action":"block","enabled":true,"created_at":"..." }`

### 6.4 输出过滤 (REQ-011)
`GET /guard/output` / `PUT /guard/output`：
```json
{ "enabled":true, "violation_strategy":"replace|block|log", "safe_message":"抱歉，该回答包含不当内容。", "stream_chunk_threshold":256 }
```

---

## 7. 配额与速率限制

### 7.1 配额 (REQ-012)
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/quotas?page=&page_size=` | 列表（含实时 used） |
| POST | `/quotas` | 创建 |
| PUT | `/quotas/{id}` | 更新 |
| DELETE | `/quotas/{id}` | 删除 |
| POST | `/quotas/{id}/reset` | 手动重置 used=0 |
| POST | `/quotas/batch` | body `{ "items":[QuotaInput...] }` 批量创建 |

Quota 对象：
```json
{ "id":1, "api_key_id":2, "api_key_label":"team-a", "model_alias":"gpt-4",
  "quota_type":"requests", "period":"day", "limit":1000, "used":120,
  "over_action":"reject", "degrade_alias":"gpt-4-mini", "enabled":true,
  "reset_at":"2026-09-03T00:00:00+08:00", "created_at":"..." }
```
创建/更新 body：`{ api_key_id, model_alias, quota_type, period, limit, over_action, degrade_alias, enabled }`
`model_alias` 可为 `"*"` 表示全部模型。

### 7.2 速率限制 (REQ-013)
CRUD `/rate-limits[/{id}]`；对象：
```json
{ "id":1, "api_key_id":0, "api_key_label":"(全局)", "model_alias":"*",
  "window_seconds":60, "max_requests":60, "enabled":true, "total_hits":12 }
```

### 7.3 配额预警 (REQ-014)
`GET /quota-alerts` / `PUT /quota-alerts`：
```json
{ "enabled":false, "thresholds":[80,95], "channel":"ui|webhook", "webhook_url":"", "receivers":"" }
```

---

## 8. 审计日志

### 8.1 调用日志 (REQ-015)
`GET /audit/calls?start=&end=&api_key_id=&model=&status=&blocked=&category=&page=&page_size=`
行对象：
```json
{ "request_id":"uuid", "created_at":"...", "api_key_label":"team-a", "protocol":"openai_chat",
  "model_alias":"gpt-4", "upstream":"openai-main/gpt-4-0613",
  "input_preview":"你好...（脱敏截断200字）", "output_preview":"...",
  "prompt_tokens":12, "completion_tokens":34, "latency_ms":850,
  "status":"ok", "blocked":false, "block_category":"", "block_reason":"" }
```
`GET /audit/calls/{request_id}` → 详情，额外含 `"input_text"`, `"output_text"`, `"error_msg"`, `"guard_findings":[...]`（JSON 字符串）。
`GET /audit/calls/export?format=csv&<同上筛选>` → CSV 文本（不包 envelope）。

### 8.2 操作审计 (REQ-003)
`GET /audit/operations?start=&end=&operator=&module=&action=&page=&page_size=`
行对象：
```json
{ "id":1, "created_at":"...", "operator":"admin", "action":"update", "module":"guard",
  "target":"keyword#12", "before_json":"{...}", "after_json":"{...}", "ip":"10.0.0.1", "effective":true }
```
`GET /audit/operations/export?format=csv&...` → CSV 文本。

---

## 9. 系统设置

### 9.1 通用设置 (REQ-001)
`GET /settings` / `PUT /settings`：
```json
{ "log_level":"info|debug|warn|error", "audit_retention_days":90, "default_timeout_seconds":120,
  "max_connections":1000, "guard_enabled":true, "hot_reload_seconds":3,
  "listen_port":8080, "listen_port_note":"仅启动参数生效，修改后需重启" }
```

### 9.2 API Key 管理 (REQ-002)
| 方法 | 路径 |
|------|------|
| GET | `/apikeys?page=&page_size=&keyword=` |
| POST | `/apikeys` body `{ "name","remark" }` → data 额外一次性返回完整 `"key":"sk-..."` |
| PUT | `/apikeys/{id}` body `{ "name","remark","enabled" }` |
| DELETE | `/apikeys/{id}` |

ApiKey 对象：`{ "id":1,"name":"team-a","key_masked":"sk-ab****yz","remark":"","enabled":true,"last_used_at":"...","created_at":"..." }`

### 9.3 安全设置 (REQ-020/023)
`GET /settings/security` / `PUT /settings/security`：
```json
{ "mfa_enabled":true, "mfa_required_for_all":false, "recovery_code_count":10, "grace_days":7 }
```
用户 MFA 状态：`GET /users?page=&page_size=` → 行 `{ "id":1,"username":"admin","mfa_enabled":true,"last_login_at":"...","locked":false }`
`POST /users/{id}/unbind-mfa`（管理员强制解绑，记审计）；`POST /users/{id}/unlock`。

### 9.4 配置状态 (REQ-004A)
`GET /config-status` →
```json
{
  "version": "v20260902.143022",
  "last_loaded_at": "2026-09-02T14:30:22+08:00",
  "status": "ok",
  "error_message": "",
  "modules": { "providers":12, "model_aliases":8, "keywords":156, "pii_rules":6, "injection_rules":4, "quotas":23, "rate_limits":5, "apikeys":10 },
  "logs": [ { "time":"...", "module":"providers", "status":"success|error", "message":"供应商配置加载完成 (12项)" } ],
  "history": [ { "version":"v20260902.143022", "created_at":"...", "status":"ok", "message":"..." } ]
}
```
`POST /config/reload` → 立即重载，`data=null`
`POST /config/rollback` → 回滚到上一版本，`data={ "rolled_back_to":"v20260902.142500" }`
`GET /config/export?format=json` → 全量配置 JSON 文本（不包 envelope）

---

## 10. 系统运维

### 10.1 状态监控 (REQ-018)
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

### 10.2 数据备份 (REQ-019)
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/backups` | 列表 `{ items:[{id,filename,created_at,size_bytes}] }` |
| POST | `/backups` | 手动备份 |
| GET | `/backups/{id}/download` | 下载 JSON（不包 envelope） |
| POST | `/backups/restore` | body `{ "id":1 }` 或 `{ "content":"<json文本>" }` |
| DELETE | `/backups/{id}` | 删除 |

---

## 11. 网关端点（业务侧，前端仅作文档展示）

- `POST /v1/chat/completions` — OpenAI Chat Completions（含 `stream:true` SSE）
- `POST /v1/responses` — OpenAI Responses API
- `POST /v1/messages` — Anthropic Messages（含 SSE）

请求中 `model` 字段填模型别名；网关按别名做 SLB/护栏/配额/审计，错误以对应协议风格返回
（OpenAI: `{"error":{"message","type","code"}}`；Anthropic: `{"type":"error","error":{"type","message"}}`；拦截时 `type="content_policy_violation"` / `http 451`→ 统一 `invalid_request_error`+code=`content_filtered`）。

---

## 12. 错误码约定

| code | HTTP | 含义 |
|------|------|------|
| 0 | 200 | 成功 |
| 40001 | 400 | 参数错误 |
| 40101 | 401 | 未登录/Token 失效 |
| 40102 | 401 | 用户名或密码错误 |
| 40103 | 401 | MFA 验证码错误 |
| 40301 | 403 | 无权限/账户锁定 |
| 40401 | 404 | 资源不存在 |
| 40901 | 409 | 资源冲突（名称重复/被引用） |
| 42901 | 429 | 触发速率限制 |
| 50001 | 500 | 服务器内部错误 |
