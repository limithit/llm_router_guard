> 🌐 **English** | [中文](README.zh-CN.md)

# AI Gateway & Model Guard System

A lightweight, self-hosted, feature-focused AI gateway platform. It offers unified multi-protocol ingress (OpenAI Chat / Responses / Anthropic), intelligent routing with weighted load balancing, bidirectional input/output guard filtering, quotas and rate limits, full-chain audit logging, hot-reloadable configuration, and a TOTP-MFA admin console.

**Stack**: Go 1.25 (Gin + GORM) + React 18 (Ant Design 5.x + Vite 5)

## Quick Start

> For a 5-minute end-to-end run (build → first boot → configure a provider → first proxied request → troubleshooting), see **[quickstart.en.md](quickstart.en.md)**. The sections below are the complete deployment reference.

### Requirements

- Go >= 1.25
- Node.js >= 18  (only for building the frontend; production deployments need only the Go binary)

### Local Development

The project is a **single-process model**: the Go backend is the only runtime process. It serves the admin API, the gateway endpoints (`/v1/*`), and the frontend SPA static assets all on a single port (default `:8080`) — the frontend needs no separate server and no Nginx.

```bash
# 1. Build the backend
cd backend && go mod tidy && go build -o ../bin/server ./cmd/server

# 2. Build the frontend
cd frontend && npm install && npm run build

# 3. Run the server (reads frontend assets from web/dist or ../frontend/dist at request time)
cd ..
./bin/server          # default :8080, SQLite database gateway.db
```

After rebuilding the frontend, refresh the browser to see the latest UI — no server restart needed.

> Frontend hot-reload development: in another terminal run `cd frontend && npm run dev`. Vite serves on :5173 and, via `vite.config.ts`, proxies `/api` and `/v1` to the backend on :8080; visit http://localhost:5173. This is the only scenario in which two processes run, and it is for development only.

Environment variables control key behavior:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Listen port |
| `DB_TYPE` | `sqlite` | Database type: `sqlite` / `postgres` / `mysql` (see [Database switching](#database-switching-sqlite--mysql--postgresql)) |
| `DB_DSN` | `gateway.db` | Connection string; a file path for sqlite, a standard DSN for postgres / mysql (examples below) |
| `JWT_SECRET` | `change-me-jwt-secret` | JWT signing secret |
| `MASTER_KEY` | `llm-router-guard-master-key` | Master key for encrypting API keys |
| `ADMIN_USER` | `admin` | First-boot admin username |
| `ADMIN_PASSWORD` | `admin123` | First-boot password (must be changed!) |
| `DATA_DIR` | `./data` | Backup file directory |
| `FRONTEND_DIST` | (auto-detect) | Frontend build output directory |
| `TRUSTED_PROXIES` | (empty) | Trusted reverse-proxy CIDRs (comma-separated). Empty by default → `ClientIP` uses the TCP peer and does not parse `X-Forwarded-For`, preventing forged-IP bypass of API-key IP whitelists. When behind a reverse proxy, set this to the proxy CIDRs (e.g. `127.0.0.1/32,10.0.0.0/8`) |
| `REDIS_ADDR` | (empty) | Redis address (`host:port`). For mysql/postgres deployments this enables **distributed rate limiting / circuit-breaker state broadcast / MFA two-step tickets** (required for multi-node; single-node sqlite ignores it). See [Multi-node deployment](#multi-node-deployment-postgresql--mysql--redis) |
| `REDIS_PASSWORD` | (empty) | Redis password; required for instances with `requirepass`, otherwise components auto-degrade to single-instance semantics on `NOAUTH` |
| `HEALTH_CHECK_SECONDS` | `30` | Upstream health-check interval in seconds; `0` = disabled. Periodically probes each enabled provider's `GET <base>/v1/models`; any HTTP response = reachable (clears the breaker); only transport-layer errors count toward failure. Results drive the breaker and are broadcast over Redis |

First boot auto-creates the admin user (the username is configurable via `ADMIN_USER`) and prints the default password to the console. **Change it immediately via the web UI.**

## Architecture Overview

```
┌──────────┐   ┌──────────────┐   ┌─────────────┐
│  React UI │   │  Gateway API │   │  Upstream   │
│  Ant Design│←→│  Gin + gorm │←→│ OpenAI/Anthrp│
│  TanStack │   │  SLB/Failover│   │ Responses/  │
└──────────┘   │  Guard Engine │   │ Custom APIs │
                │  Quota/Limit  │   └─────────────┘
                │  Audit Log    │
                │  Config DB    │
                └──────┬────────┘
                       │ SQLite / Postgres / MySQL
```

### Core Capabilities

| Module | Description | PRD REQ |
|--------|-------------|---------|
| **Unified tri-protocol ingress** | OpenAI Chat Completions / Responses / Anthropic Messages behind one entry | REQ-017 |
| **SLB + circuit breaker + failover** | Weighted round-robin, exponential-backoff retries, automatic breaker recovery | REQ-006/007 |
| **Input/output guards** | Sensitive-word (AC-engine interface reserved), PII regex masking, prompt-injection detection, output content review | REQ-008~011 |
| **Quota & rate limiting** | Per day/week/month request-count or token-total quotas; sliding-window rate limiting; over-quota can degrade to another model | REQ-012/013 |
| **Call audit** | Full recording of every request/response, token usage, latency, and block reason | REQ-015 |
| **Operation audit** | Every config change tracked by operator, time, before/after diff, and effective-state traceability | REQ-003 |
| **Hot-reloadable config** | DB-driven incremental reload; the Manager atomically swaps the snapshot; globally effective within ≤3s | REQ-004/004A |
| **MFA admin console** | TOTP binding, backup recovery codes, account-lockout policies | REQ-020~022 |
| **Multi-database** | SQLite (pure Go), PostgreSQL, MySQL — switch with one variable | NFR-016 |

## Directory Layout

```
backend/                          # Go backend
├── cmd/server/main.go            # entry point
├── internal/
│   ├── config/config.go          # env-var parsing
│   ├── db/db.go                  # multi-database driver (GORM)
│   ├── model/model.go            # entity definitions (GORM models)
│   ├── crypto/crypto.go          # AES-GCM encryption
│   ├── settings/settings.go      # KV config structs
│   ├── runtime/manager.go        # hot-reload manager (Snapshot)
│   ├── guard/engine.go           # guard engine
│   ├── slb/slb.go                # load balancing / circuit breaker
│   ├── quota/quota.go            # quota / rate limit
│   ├── adapter/                  # protocol adapter layer
│   ├── gateway/gateway.go        # gateway proxy (incl. SSE streaming)
│   ├── metrics/metrics.go        # QPS / connection metrics
│   ├── audit/audit.go            # audit-log writer
│   ├── auth/jwt.go               # JWT issue / verify
│   └── admin/                    # RESTful admin API (all handlers)
frontend/                         # React frontend
├── src/
│   ├── api/                      # axios + types + endpoints
│   ├── components/               # PageContainer/Charts/JsonView
│   ├── constants/dicts.ts        # enum display dictionaries
│   ├── layouts/MainLayout.tsx    # sidebar nav + breadcrumb
│   ├── pages/                    # business pages (~20)
│   ├── store/auth.ts             # Zustand auth state
│   └── App.tsx                   # route table
```

## API Documentation

The full front-end ↔ back-end contract is in [docs/api-contract.en.md](docs/api-contract.en.md).  
Admin API base URL: `/api/admin/v1`. Gateway endpoints: `POST /v1/chat/completions`, `POST /v1/responses`, `POST /v1/messages`, `GET /v1/models` (the model catalog, OpenAI-compatible).

## Build & Deployment

CI: GitHub Actions (`.github/workflows/ci.yml`) — on push/PR to `master` it runs backend `gofmt/vet/test -race` → frontend `tsc + vite build` → Docker image build (no push).

The deployment form is **single-process**: one Go binary is the entire runtime, exposing the admin API, gateway endpoints, and frontend SPA on :8080 simultaneously — the frontend needs no Nginx or standalone Node service.

### Single-binary deployment (recommended)

```bash
# 1. Build the frontend and copy output into backend/web/dist
cd frontend && npm run build && cp -r dist ../backend/web/dist

# 2. Build the backend (frontend assets are read from disk at runtime, not embedded; web/dist must ship with the binary)
cd ../backend && go build -ldflags="-s -w" -o llm-router-guard ./cmd/server

# 3. Run (working directory must contain the binary and web/dist; or use FRONTEND_DIST to point at the absolute path of the frontend output)
PORT=8080 DB_TYPE=postgresql DB_DSN="postgres://user:pass@host/gateway" \
MASTER_KEY="your-strong-secret" ./llm-router-guard
```

> Deployment directory layout: `llm-router-guard` (binary) + `web/dist/` (frontend output). The backend does **not** `go:embed` the frontend; it reads from disk at runtime, so `web/dist` must not be missing, otherwise `GET /` returns 404.

### Docker Compose

```yaml
version: "3.9"
services:
  gateway:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "8080:8080"
    environment:
      - PORT=8080
      - DB_TYPE=sqlite
      - JWT_SECRET=${JWT_SECRET:?generate with: openssl rand -hex 32}
      - MASTER_KEY=${MASTER_KEY:?generate with: openssl rand -hex 32}
      - ADMIN_PASSWORD=${ADMIN_PASSWORD:-}
    volumes:
      - gateway-data:/app/data
volumes:
  gateway-data:
```

See the project-root `Dockerfile` and `docker-compose.yml` for details.

### Database switching (SQLite / MySQL / PostgreSQL)

Switch via two environment variables; first boot runs `AutoMigrate` to create tables, no manual SQL needed.

| Variable | Description |
|----------|-------------|
| `DB_TYPE` | `sqlite` (default) / `postgres` / `mysql` |
| `DB_DSN` | Connection string. A file path for sqlite; a standard DSN for postgres / mysql |

**SQLite (default, zero-config)**
```bash
DB_TYPE=sqlite DB_DSN=gateway.db ./llm-router-guard
# absolute path on a data volume: DB_DSN=/app/data/gateway.db
```
Runs in WAL mode with a single write connection (reads still concurrent); suitable for single-machine / small scale.

**PostgreSQL**
```bash
DB_TYPE=postgres \
DB_DSN="postgres://user:pass@host:5432/gateway?sslmode=disable" \
./llm-router-guard
# or key=value form:
DB_DSN="host=pg-host user=gateway password=secret dbname=gateway port=5432 sslmode=disable"
```

**MySQL**
```bash
DB_TYPE=mysql \
DB_DSN="gateway:secret@tcp(mysql-host:3306)/gateway" \
./llm-router-guard
```
> The gateway auto-appends `charset=utf8mb4&parseTime=True&loc=Local` (if the DSN lacks them) to ensure correct time fields and Chinese text; the MySQL database should use `utf8mb4`.

**Docker Compose + PostgreSQL example**
```yaml
services:
  gateway:
    build: { context: ., dockerfile: Dockerfile }
    ports: ["8080:8080"]
    environment:
      - DB_TYPE=postgres
      - DB_DSN=postgres://gateway:secret@db:5432/gateway?sslmode=disable
      - JWT_SECRET=${JWT_SECRET:-replace-with-random-secret}
      - MASTER_KEY=${MASTER_KEY:-replace-with-master-key}
      - ADMIN_PASSWORD=${ADMIN_PASSWORD:-admin123}
    depends_on: [db]
  db:
    image: postgres:16
    environment:
      - POSTGRES_USER=gateway
      - POSTGRES_PASSWORD=secret
      - POSTGRES_DB=gateway
    volumes: ["pg-data:/var/lib/postgresql/data"]
volumes:
  pg-data:
```
> Switching databases only changes these two variables. Business tables (providers / models / quotas / audit, etc.) are created in the new database via `AutoMigrate`. **No cross-database data migration is performed** — switching to a new `DB_DSN` points at an empty database; export/import historical data yourself.

### Multi-node deployment (PostgreSQL / MySQL + Redis)

Horizontal-scaling form: N gateway instances (same binary + `web/dist`) behind a load balancer, all pointing at the same shared storage. **A single node needs no Redis**; the following applies only to multi-instance deployments.

```
                  ┌────────────────────────────────┐
                  │  LB (Nginx / HAProxy / cloud LB)│
                  │  health-check: GET /healthz     │
                  └───────────────┬────────────────┘
           ┌──────────────────────┼──────────────────────┐
           ▼                      ▼                      ▼
     ┌────────────┐        ┌────────────┐         ┌────────────┐
     │ Gateway A  │        │ Gateway B  │   ...   │ Gateway N  │
     └──────┬─────┘        └──────┬─────┘         └──────┬─────┘
            │   all instances point at the same shared storage  │
            └──────────────┬─────────────────────────────┘
                           ▼
    ┌─────────────────────────┐     ┌──────────────────────────┐
    │  PostgreSQL / MySQL     │     │  Redis (REDIS_ADDR)       │
    │  config/quota/audit/key │     │  rate-limit/breaker/MFA    │
    └─────────────────────────┘     └──────────────────────────┘
```

**Multi-node environment variables**

| Variable | Description |
|----------|-------------|
| `DB_TYPE` + `DB_DSN` | All instances point at the **same** PostgreSQL / MySQL database; sqlite is single-node only (file lock + single write connection) |
| `REDIS_ADDR` | Redis address `host:port`. Enables distributed rate limiting (atomic INCR fixed window), circuit-breaker-open state broadcast (SETEX + TTL auto-half-open), MFA two-step tickets (atomic GET+DEL redemption), and the **global SWRR round-robin cursor** (Lua atomic advance; multi-instance sharding never overlaps; auto-fallback to per-instance local cursor on disconnect). Effective for mysql/postgres only |
| `REDIS_PASSWORD` | Redis password; required for instances with `requirepass`, otherwise the three components auto-degrade to single-instance semantics on startup `NOAUTH` (reported honestly) |
| `HEALTH_CHECK_SECONDS` | Upstream health-check interval in seconds (default 30, `0` = disabled): any HTTP response (incl. 401/404) = endpoint reachable = clears the breaker; only transport-layer errors (timeout / DNS / connection refused) accumulate, and the breaker opens once the threshold is reached. Multi-instance probe results are **broadcast over Redis** — once any instance opens the breaker, all others skip that provider within ≤1s |
| `TRUSTED_PROXIES` | **Must be set to the proxy CIDRs behind an LB** (e.g. `10.0.0.0/8`). When empty, `ClientIP` uses the TCP peer — if unset, the API-key IP whitelist matches every request against the LB address, making the whitelist useless |
| `JWT_SECRET` / `MASTER_KEY` | **Must be identical across all instances**: the former lets any instance verify admin JWTs, the latter lets API-key ciphertext (AES-GCM) be decrypted on any instance |

**Cross-instance consistency by state domain**

- **Globally consistent (DB-backed)**: business config, quotas (`used_value` atomic increment), call/operation audit, API keys / providers / model aliases / rate-limit rules, admin JWT (stateless; any instance can verify).
- **Globally consistent (Redis-backed, requires `REDIS_ADDR`)**: rate limiting ("100/min" globally exact 429), SLB breaker-open state, MFA two-step login tickets (any instance behind the LB can complete step two), and the **SWRR global round-robin cursor** (multi-instance weighted sharding never overlaps; clean request paths advance atomically via Lua; failover-retry paths use per-instance local cursors).
- **Per-instance local**: the SWRR round-robin cursor — each instance rotates independently; distribution within a single instance is still correct, only the global distribution is slightly skewed (acceptable; no session affinity required).

**Hot-reloadable config**: changing config in the admin console on any instance → increments `config_meta.counter` in the DB → the other instances auto-reload the snapshot within ≤`hot_reload_seconds` (default 3 seconds); external tools editing the DB directly also take effect (polling fallback).

**Degradation semantics**: if Redis is unreachable (probe failure at startup or a runtime fault), the system auto-degrades to per-instance rate-limit / breaker semantics and logs a warning; the hot path is never blocked. It probes every 5 seconds and switches back to distributed counting on recovery. During degradation, rate limiting is counted per-instance (a looser bound); after recovery it returns to global precision.

**Docker Compose multi-node example** (dual gateway + PG + Redis; `REDIS_PASSWORD` must match across instances):

```yaml
services:
  gateway-a: &gateway
    build: { context: ., dockerfile: Dockerfile }
    ports: ["8080:8080"]
    environment: &gateway-env
      - DB_TYPE=postgres
      - DB_DSN=postgres://gateway:secret@db:5432/gateway?sslmode=disable
      - REDIS_ADDR=redis:6379
      - REDIS_PASSWORD=${REDIS_PASSWORD:?set-redis-password}
      - JWT_SECRET=${JWT_SECRET:?set-jwt-secret}
      - MASTER_KEY=${MASTER_KEY:?set-master-key}
      - ADMIN_PASSWORD=${ADMIN_PASSWORD:?set-admin-password}
    depends_on: [db, redis]
  gateway-b:
    <<: *gateway
    ports: ["8081:8080"]
  db:
    image: postgres:16
    environment:
      - POSTGRES_USER=gateway
      - POSTGRES_PASSWORD=secret
      - POSTGRES_DB=gateway
    volumes: ["pg-data:/var/lib/postgresql/data"]
  redis:
    image: redis:7
    command: ["redis-server", "--requirepass", "${REDIS_PASSWORD:?set-redis-password}"]
volumes:
  pg-data:
```

> Put a front LB (Nginx / HAProxy / cloud LB) round-robining `gateway-a:8080` / `gateway-b:8080`, health-checking `GET /healthz`. After setting `TRUSTED_PROXIES` between the LB and the gateways, the client's real IP enters the API-key IP-whitelist check.

**Multi-node, measured**: a dual-instance setup (same host, 18080/18082, shared PG + Redis) has been verified — distributed rate limiting 429s globally, breaker opens across instances within ≤1s with TTL-driven auto-half-open recovery, config hot-reload propagates across instances within ≤4s, quotas are globally precise.

## License

This project is open-sourced under the [Apache License 2.0](LICENSE). See the `LICENSE` file in the repository root.
