> 🌐 **English** | [中文](quickstart.md)

# Quickstart — Up and running in 5 minutes

> Goal: start the service locally → configure the first model → have the gateway successfully proxy one request.
> For architecture, the full set of environment variables, and multi-node topology, see [README.md](README.md); for the API contract, see [docs/api-contract.en.md](docs/api-contract.en.md).

## 0. Prerequisites

| Dependency | Version | Use |
|------------|---------|-----|
| Go | ≥ 1.24 | Build the backend (the only runtime process) |
| Node.js | ≥ 18 | Only for building the frontend (production can skip this and use the pre-built `web/dist`) |
| Docker | any | Optional, for one-shot compose |

The database is **zero-dependency**: SQLite by default (pure-Go driver, auto-creates tables); for MySQL/PostgreSQL see section 5.

## 1. Build & start (single process, one port does it all)

```bash
# Linux / macOS (key: run from backend/ so it auto-detects web/dist and gateway.db)
cd frontend && npm install && npm run build && cd ..
mkdir -p backend/web/dist && cp -r frontend/dist/* backend/web/dist/
cd backend && go build -ldflags="-s -w" -o bin/llm-router-guard ./cmd/server && ./bin/llm-router-guard
# Default :8080. One port serves admin API + /v1/* gateway + frontend SPA
# (if CWD is not backend/, use FRONTEND_DIST to point at the absolute path of the frontend output)
```

```powershell
# Windows: one script does it all (install deps + build frontend + copy output + compile)
.\build.ps1
cd bin; .\server.exe        # auto-locates the frontend by falling back to the repo-root frontend/dist
```

Open **http://localhost:8080** in a browser.

Smoke check (three unauthenticated endpoints):

```bash
curl -s http://localhost:8080/healthz    # {"status":"ok"} — used by LB health checks
curl -s http://localhost:8080/metrics | head   # Prometheus text format
curl -s http://localhost:8080/v1/models  # requires Authorization; returning 401 without a key is expected
```

## 2. First-boot login & the three security items

Default account `admin` / `admin123` (when `ADMIN_PASSWORD` is unset; startup logs print a WARNING).

After logging in, open the user menu in the top-right → **Account** (the account-security page):

1. **Change the password** (do it right after login)
2. **Bind TOTP MFA** (scan the QR code → enter a dynamic code to confirm → one-time display of backup recovery codes, save them securely; unbinding requires the code or a recovery code)
3. For production, replace the default env-var values: `JWT_SECRET`, `MASTER_KEY` (copy from `.env.template`: `cp .env.template .env`)

## 3. Run the first proxied request in 5 minutes

Follow the left sidebar in order:

1. **Models → Providers**: create a provider
   - Protocol **must** be one of `openai_chat` / `openai_responses` / `anthropic` (entering `openai` is rejected with 400)
   - Base URL includes `/v1` (e.g. `https://api.openai.com/v1`); after saving, click **Test connection** (strict criterion: `GET /v1/models` returns a model list to count as passing)
2. **Models → Model aliases**: create an alias (e.g. `gpt-4o-mini`), attach the provider from the previous step as an upstream (multiple upstreams with weights → automatic weighted round-robin + circuit breaker + failover; see **Models → Failover** to tune thresholds)
3. **Settings → API Key**: create a key; the plaintext is **shown only once**, copy it immediately (format `sk-` + 32 hex)
4. Call the gateway (any OpenAI-compatible SDK; just point `base_url` at the gateway):

```bash
curl -s http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-yourkey" -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}'
# Add "stream": true for SSE paseload; Anthropic-client protocol goes to /v1/messages, Responses to /v1/responses
```

> **About `max_tokens`**: the gateway does **passthrough, no clamping** — whatever the client sets is forwarded to the upstream as-is (you may set it to the actual per-model upper bound; if you need a very large value, `max_tokens: 1000000` works too — acceptance is up to the upstream vendor: out-of-range values are returned verbatim as the vendor's 4xx, e.g. `upstream returned 400: field MaxTokens invalid, should be in [1, 131072]`; bring the value into range to fix it).
> **Reasoning models (GLM/DeepSeek style) must set it large**: the chain-of-thought (`reasoning_content`) shares the max_tokens budget with the body; too small a value (e.g. 300) triggers `finish_reason=length` during the reasoning phase, and the client sees an empty or very short body. The gateway transparently forwards reasoning deltas (openai_chat `delta.reasoning_content` / anthropic `thinking_delta`); the SDK recognizes them to display the chain-of-thought.

5. Verify the loop: **Audit → Call logs** should show the request (token usage / latency / guard verdict); when you send the header `-H "X-Request-ID: my-trace-1"`, the response header echoes the same value, and you can locate the record precisely by that ID via the detail endpoint: `GET /api/admin/v1/audit/calls/my-trace-1` (requires an admin JWT).

Optional guards experience: **Guards → Keywords**, add a sensitive word (action: block) → a prompt that hits it returns 400 `content_filtered`.

## 4. Docker Compose (easiest on a single machine)

```bash
cp .env.template .env      # edit JWT_SECRET / MASTER_KEY / ADMIN_PASSWORD
docker compose up -d       # container :8080, data volume ./data, /healthz health check is built in
docker compose logs -f gateway
```

## 5. Switch to MySQL / PostgreSQL (a one-line change)

First boot runs `AutoMigrate` and creates all 16 tables automatically; no manual SQL:

```bash
DB_TYPE=postgres DB_DSN="postgres://user:pass@host:5432/gateway?sslmode=disable" ./bin/llm-router-guard
DB_TYPE=mysql    DB_DSN="user:pass@tcp(host:3306)/gateway" ./bin/llm-router-guard   # charset etc. auto-appended
```

For DBA pre-review / manual database creation / read-only-account deployments, use the repo's prebuilt baseline SQL (column-for-column consistent with the AutoMigrate output, round-trip verified):

```bash
# MySQL
mysql -uroot -p -e "CREATE DATABASE gateway CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
mysql -uroot -p gateway < deploy/schema/schema.mysql.sql
# PostgreSQL
psql -U postgres -c "CREATE DATABASE gateway;"
psql -U postgres -d gateway -f deploy/schema/schema.postgres.sql
```

> To regenerate after model changes: run `deploy/make_schema.sh` on a test machine; verify with `deploy/verify_schema.sh` (see `deploy/schema/README.en.md`).

## 6. Multi-node (horizontal scaling) at a glance

Run N instances of the same binary behind a front LB (health-check `/healthz`); all instances use the **same** PG/MySQL + the **same** Redis, with `JWT_SECRET` / `MASTER_KEY` completely identical. SQLite is single-node only.

```bash
DB_TYPE=postgres DB_DSN=... REDIS_ADDR=redis-host:6379 REDIS_PASSWORD=*** \
TRUSTED_PROXIES=10.0.0.0/8 ./bin/llm-router-guard    # every instance must set TRUSTED_PROXIES=LB subnet, otherwise the IP whitelist is useless
```

`REDIS_ADDR` scope (mysql/postgres only): distributed rate limiting (globally exact 429), circuit-breaker-open state broadcast (≤1s cross-instance sync), MFA two-step tickets (any instance behind the LB can complete step two), SWRR global round-robin cursor. On Redis failure it auto-degrades to per-instance semantics with a warning, and switches back on recovery.
For the topology diagram / dual-gateway Compose template / consistency quick-reference table, see the [multi-node section of the README](README.md#multi-node-deployment-postgresql--mysql--redis).

## 7. Development mode (frontend hot-reload)

```bash
cd backend && go run ./cmd/server          # terminal 1: backend on :8080
cd frontend && npm install && npm run dev  # terminal 2: Vite dev (port in frontend/vite.config.ts, default 5174); /api and /v1 auto-proxied to 8080
```

Save a frontend change to see it live (http://localhost:5174); for backend changes, restart `go run`.

## 8. Production startup per OS

A single binary plus environment variables is enough to host it. Common preface (identical across the three platforms):

```bash
# Environment variables (must change for production; defaults in .env.template)
PORT=8080 DB_TYPE=sqlite DB_DSN=/var/lib/llm-router-guard/gateway.db \
JWT_SECRET=change-me MASTER_KEY=change-me ADMIN_PASSWORD=change-me
```

### Linux — systemd (recommended)

```bash
# 1) Create a dedicated user and data directory
sudo useradd -r -s /usr/sbin/nologin llmrg
sudo mkdir -p /opt/llm-router-guard /var/lib/llm-router-guard
sudo cp bin/llm-router-guard /opt/llm-router-guard/   # binary (with web/dist in the same dir, or FRONTEND_DIST set)
sudo chown -R llmrg:llmrg /opt/llm-router-guard /var/lib/llm-router-guard

# 2) /etc/systemd/system/llm-router-guard.service
[Unit]
Description=LLM Router Guard gateway
After=network-online.target
Wants=network-online.target

[Service]
User=llmrg
Group=llmrg
WorkingDirectory=/opt/llm-router-guard
EnvironmentFile=/opt/llm-router-guard/llmrg.env     # KEY=VALUE, one per line
ExecStart=/opt/llm-router-guard/llm-router-guard
Restart=on-failure
RestartSec=3
# Hardening (optional): the service may only write to its own data directory
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/var/lib/llm-router-guard
ProtectHome=true

[Install]
WantedBy=multi-user.target

# 3) Start the service + enable on boot + watch logs
sudo systemctl daemon-reload
sudo systemctl enable --now llm-router-guard
systemctl status llm-router-guard          # status
sudo journalctl -u llm-router-guard -f     # live logs (incl. [stream]/[upstream] lines)
```

After editing `llmrg.env`, run `sudo systemctl restart llm-router-guard` to apply (in-app hot-changed config goes through the admin UI and needs no restart).

### Windows — Scheduled Task (auto-start on boot, no third-party deps)

```powershell
# Administrator PowerShell: auto-start on boot + auto-restart on crash (Task Scheduler as the backstop)
$action  = New-ScheduledTaskAction -Execute "F:\llm_router_guard\backend\bin\server.exe" `
           -WorkingDirectory "F:\llm_router_guard\backend"
$trigger = New-ScheduledTaskTrigger -AtStartup
$settings = New-ScheduledTaskSettingsSet -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) `
           -ExecutionTimeLimit ([TimeSpan]::Zero) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
Register-ScheduledTask -TaskName "llm-router-guard" -Action $action -Trigger $trigger `
  -Settings $settings -User "SYSTEM" -RunLevel Highest
Start-ScheduledTask -TaskName "llm-router-guard"   # start now
Get-ScheduledTask -TaskName "llm-router-guard"     # State should be Running
# Uninstall: Unregister-ScheduledTask -TaskName "llm-router-guard" -Confirm:$false
```

Two ways to set environment variables: system-level `$env:PORT="8080"; setx PORT 8080` (`setx` is permanent, takes effect in a new terminal), or use **NSSM** to register a real Windows service (`nssm install llm-router-guard F:\...\server.exe`, `nssm set llm-router-guard AppEnvironmentExtra PORT=8080 JWT_SECRET=...`) — NSSM is recommended for production.

### macOS — launchd

```bash
# ~/Library/LaunchAgents/cn.llmrg.gateway.plist (login auto-start; system-wide goes in /Library/LaunchDaemons)
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>cn.llmrg.gateway</string>
  <key>ProgramArguments</key><array>
    <string>/opt/llmrg/bin/llm-router-guard</string>
  </array>
  <key>WorkingDirectory</key><string>/opt/llmrg</string>
  <key>EnvironmentVariables</key><dict>
    <key>PORT</key><string>8080</string>
    <key>DB_TYPE</key><string>sqlite</string>
    <key>JWT_SECRET</key><string>change-me</string>
    <key>MASTER_KEY</key><string>change-me</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>/opt/llmrg/gateway.log</string>
  <key>StandardErrorPath</key><string>/opt/llmrg/gateway.err.log</string>
</dict></plist>

launchctl load ~/Library/LaunchAgents/cn.llmrg.gateway.plist   # load and start
launchctl list | grep llmrg                                    # confirm it's running
launchctl unload ~/Library/LaunchAgents/cn.llmrg.gateway.plist # stop
```

### Bare process (temporary / debugging)

```bash
nohup ./llm-router-guard > gateway.log 2>&1 &   # Linux/macOS; echo $! > gateway.pid
Start-Process .\server.exe -WindowStyle Hidden  # Windows (no auto-restart; use one of the above for production)
```

## 9. Troubleshooting quick-reference

| Symptom | Cause / Fix |
|---------------------------------------------|-------------|
| Startup log `[security] WARNING` default keys | Production did not change `JWT_SECRET`/`MASTER_KEY`/`ADMIN_PASSWORD` |
| `[ratelimit] redis ... NOAUTH ... DEGRADED` | Redis has `requirepass` but `REDIS_PASSWORD` is unset; during degradation, rate limiting is counted per-instance |
| `GET /` returns 404 | Deployment directory is missing `web/dist` (frontend output ships with the binary, or use `FRONTEND_DIST` to set an absolute path) |
| Test connection fails but chat works | Test connection is a strict criterion (requires a model-list JSON); upstream `/v1/models` being non-standard reports failure — expected |
| Upstream proxy 502 `unsupported upstream protocol` | The provider's protocol field has a value outside the enum (new versions reject this at create/update with 400) |
| Upstream 400 `field MaxTokens invalid, should be in [1, 131072]` | The client's `max_tokens` is outside **that vendor's** range (the gateway does not clamp, passes through); bring it within the vendor's upper bound |
| Reasoning model returns empty / very short with `finish_reason=length` | The chain-of-thought exhausted the max_tokens budget — increase `max_tokens` (reasoning is passed through via `reasoning_content`/`thinking_delta`, can be displayed) |
| Client IP is always the SLB address | `TRUSTED_PROXIES` is not set to the proxy subnet |
| Port in use, won't start | `PORT=18080 ./bin/llm-router-guard`; kill the old process `fuser -k 8080/tcp` (Linux) |
| Multi-instance config change takes effect on the other instance a few seconds later | Hot-reload polling cycle, default ≤3s (`hot_reload_seconds` is tunable) — this is by design |
