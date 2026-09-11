> 🌐 [English](README.md) | **中文**

# AI 网关与模型护栏系统

轻量级、自托管、功能聚焦的 AI 网关平台，支持多协议统一接入（OpenAI Chat/Responses / Anthropic）、智能路由负载均衡、输入/输出双向护栏过滤、配额与速率限制、全链路审计日志、配置热加载及 TOTP MFA 管理后台。

**技术栈**：Go 1.24（Gin + GORM）+ React 18（Ant Design 5.x + Vite 5）

## 快速开始

> 想 5 分钟从零跑通（构建→首启→配供应商→第一次转发→排障速查）：见 **[quickstart.md](quickstart.md)**。本节及以下内容为完整部署参考。

### 环境要求

- Go >= 1.24
- Node.js >= 18  (仅构建前端使用；生产部署只需 Go 二进制)

### 本地开发

项目为**单进程模型**：Go 后端是唯一运行时进程，在同一个端口（默认 :8080）上同时提供管理 API、网关端点（`/v1/*`）和前端 SPA 静态资源——前端无需独立服务器，也不需要 Nginx。

```bash
# 1. 编译后端
cd backend && go mod tidy && go build -o ../bin/server ./cmd/server

# 2. 构建前端
cd frontend && npm install && npm run build

# 3. 运行服务端（运行时从 web/dist 或 ../frontend/dist 读取前端产物，按请求实时读盘）
cd ..
./bin/server          # 默认 :8080，SQLite 数据库 gateway.db
```

前端重新构建后刷新浏览器即可看到最新界面，无需重启服务。

> 前端热更新开发：另开终端 `cd frontend && npm run dev`，Vite 在 :5173 提供热更新，并通过 `vite.config.ts` 将 `/api`、`/v1` 代理到后端 :8080；此时访问 http://localhost:5173 即可。这是唯一会出现两个进程的场景，且仅用于开发。

环境变量控制关键行为：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PORT` | `8080` | 监听端口 |
| `DB_TYPE` | `sqlite` | 数据库类型：`sqlite` / `postgres` / `mysql`（切换见[数据库切换](#数据库切换sqlite--mysql--postgresql)） |
| `DB_DSN` | `gateway.db` | 连接串；sqlite 为文件路径，postgres / mysql 为标准 DSN（示例见下） |
| `JWT_SECRET` | `change-me-jwt-secret` | JWT 签名密钥 |
| `MASTER_KEY` | `llm-router-guard-master-key` | API Key 加密主密钥 |
| `ADMIN_USER` | `admin` | 首启管理员用户名 |
| `ADMIN_PASSWORD` | `admin123` | 首启密码（必须修改！）|
| `DATA_DIR` | `./data` | 备份文件存放目录 |
| `FRONTEND_DIST` | (auto-detect) | 前端构建产物目录 |
| `TRUSTED_PROXIES` | (empty) | 可信反代 CIDR（逗号分隔）。默认空 → `ClientIP` 取 TCP 对端、不解析 `X-Forwarded-For`，防伪造 IP 绕过 API Key 的 IP 白名单；反代部署时设为代理 CIDR（如 `127.0.0.1/32,10.0.0.0/8`） |
| `REDIS_ADDR` | (empty) | Redis 地址（`host:port`）。mysql/postgres 部署配置后启用**分布式限流 / 熔断状态广播 / MFA 二步票据**（多节点必备；sqlite 单节点忽略）。详见[多节点部署](#多节点部署postgresql--mysql--redis) |
| `REDIS_PASSWORD` | (empty) | Redis 密码；带 `requirepass` 的实例必须配置，否则组件 NOAUTH 自动降级为单实例语义 |
| `HEALTH_CHECK_SECONDS` | `30` | 上游健康检查周期秒数，`0`=禁用。周期探测各启用供应商 `GET <base>/v1/models`，任何 HTTP 响应=可达（清熔断），仅传输层错误累计失败；结果联动熔断并经 Redis 广播 |

首次启动会自动创建 admin 用户（用户名可在 `ADMIN_USER` 中自定义），并在控制台打印默认密码。**请立即通过 Web 界面修改密码**。

## 架构概览

```
┌──────────┐   ┌──────────────┐   ┌─────────────┐
│  React UI │   │  Gateway API │   │  Upstream   │
│  Ant Design│←→│  Gin + gorm │←→│ OpenAI/Antrop│
│  TanStack │   │  SLB/Failover│   │ Responses/  │
└──────────┘   │  Guard Engine │   │ Custom APIs │
                │  Quota/Limit  │   └─────────────┘
                │  Audit Log    │
                │  Config DB    │
                └──────┬────────┘
                       │ SQLite / Postgres / MySQL
```

### 核心能力

| 模块 | 描述 | 对应 PRD REQ |
|------|------|--------------|
| **三协议统一接入** | OpenAI Chat Completions / Responses / Anthropic Messages 统一入口 | REQ-017 |
| **SLB + 熔断 + 故障转移** | 加权轮询、指数退避重试、熔断器自动恢复 | REQ-006/007 |
| **输入/输出护栏** | 敏感词（AC 引擎接口预留）、PII 正则脱敏、提示词注入检测、输出内容审核 | REQ-008~011 |
| **配额 & 限流** | 按天/周/月请求次数或 Token 总量限额；滑动窗口速率限制；超限可降级到其他模型 | REQ-012/013 |
| **调用审计** | 全量记录每次请求/响应、Token 用量、延迟、拦截原因 | REQ-015 |
| **操作审计** | 所有配置变更的操作人、时间、前后对比、生效状态追溯 | REQ-003 |
| **配置热加载** | DB 驱动的增量重载，Manager 原子替换快照，≤3 秒全局生效 | REQ-004/004A |
| **MFA 管理后台** | TOTP 绑定、备用恢复码、账户锁定策略 | REQ-020~022 |
| **多数据库** | SQLite(纯 Go)、PostgreSQL、MySQL 一键切换 | NFR-016 |

## 目录结构

```
backend/                          # Go 后端
├── cmd/server/main.go            # 入口
├── internal/
│   ├── config/config.go          # 环境变量解析
│   ├── db/db.go                  # 多数据库驱动(GORM)
│   ├── model/model.go            # 实体定义(GORM 模型)
│   ├── crypto/crypto.go          # AES-GCM 加密
│   ├── settings/settings.go      # KV 配置结构体
│   ├── runtime/manager.go        # 热加载管理器(Snapshot)
│   ├── guard/engine.go           # 护栏引擎
│   ├── slb/slb.go                # 负载/熔断
│   ├── quota/quota.go            # 配额/限流
│   ├── adapter/                  # 协议适配层
│   ├── gateway/gateway.go        # 网关代理(含 SSE 流式)
│   ├── metrics/metrics.go        # QPS/连接数采集
│   ├── audit/audit.go            # 审计日志写入器
│   ├── auth/jwt.go               # JWT 签发校验
│   └── admin/                    # RESTful 管理 API(全部 handler)
frontend/                         # React 前端
├── src/
│   ├── api/                      # axios + 类型 + endpoints
│   ├── components/               # PageContainer/Charts/JsonView
│   ├── constants/dicts.ts        # 枚举展示字典
│   ├── layouts/MainLayout.tsx    # 侧栏导航 + 面包屑
│   ├── pages/                    # 各业务页面(~20 个)
│   ├── store/auth.ts             # Zustand 认证态
│   └── App.tsx                   # 路由表
```

## API 文档

完整的前后端对接契约见 [docs/api-contract.md](docs/api-contract.md)。  
管理 API Base URL: `/api/admin/v1`，网关端点: `POST /v1/chat/completions`, `POST /v1/responses`, `POST /v1/messages`, `GET /v1/models`（模型目录，OpenAI 兼容）。

## 构建与部署

CI：GitHub Actions（`.github/workflows/ci.yml`）——push/PR 到 `main`/`cluster` 自动执行
后端 `gofmt/vet/test -race` → 前端 `tsc + vite build` → Docker 镜像构建（不推送）；
托管在阿里云 Codeup 时 Flow 流水线按同序复用这三段命令即可。

部署形态为**单进程**：一个 Go 二进制即整个运行时，在 :8080 同时对外提供管理 API、网关端点和前端 SPA，前端不需要 Nginx 或独立 Node 服务。

### 单二进制部署（推荐）

```bash
# 1. 构建前端，产物拷入后端 web/dist
cd frontend && npm run build && cp -r dist ../backend/web/dist

# 2. 编译后端（前端资源为运行时磁盘读取、非内嵌；web/dist 须随二进制一同部署）
cd ../backend && go build -ldflags="-s -w" -o llm-router-guard ./cmd/server

# 3. 运行（工作目录需含二进制与 web/dist；或用 FRONTEND_DIST 指向前端产物绝对路径）
PORT=8080 DB_TYPE=postgresql DB_DSN="postgres://user:pass@host/gateway" \
MASTER_KEY="your-strong-secret" ADMIN_PASSWORD="change-me" ./llm-router-guard
```

> 部署目录结构：`llm-router-guard`（二进制）+ `web/dist/`（前端产物）。后端**未通过 `go:embed` 内嵌前端**，运行时从磁盘读取，因此 `web/dist` 不可缺失，否则访问 `/` 返回 404。

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
      - JWT_SECRET=${JWT_SECRET:-replace-with-random-secret}
      - MASTER_KEY=${MASTER_KEY:-replace-with-master-key}
      - ADMIN_PASSWORD=${ADMIN_PASSWORD:-admin123}
    volumes:
      - gateway-data:/app/data
volumes:
  gateway-data:
```

详细 Dockerfile 请参考项目根目录下的 `Dockerfile` 和 `docker-compose.yml`。

### 数据库切换（SQLite / MySQL / PostgreSQL）

通过两个环境变量切换；首次启动 `AutoMigrate` 自动建表，无需手动执行 SQL。

| 变量 | 说明 |
|------|------|
| `DB_TYPE` | `sqlite`（默认）/ `postgres` / `mysql` |
| `DB_DSN` | 连接串。sqlite 为文件路径；postgres / mysql 为标准 DSN |

**SQLite（默认，零配置）**
```bash
DB_TYPE=sqlite DB_DSN=gateway.db ./llm-router-guard
# 也可用绝对路径放到数据卷：DB_DSN=/app/data/gateway.db
```
WAL 模式运行，写连接数为 1（读仍并发）；适合单机 / 小规模。

**PostgreSQL**
```bash
DB_TYPE=postgres \
DB_DSN="postgres://user:pass@host:5432/gateway?sslmode=disable" \
./llm-router-guard
# 或 key=value 形式：
DB_DSN="host=pg-host user=gateway password=secret dbname=gateway port=5432 sslmode=disable"
```

**MySQL**
```bash
DB_TYPE=mysql \
DB_DSN="gateway:secret@tcp(mysql-host:3306)/gateway" \
./llm-router-guard
```
> 网关会自动补 `charset=utf8mb4&parseTime=True&loc=Local`（若 DSN 未含），保证时间字段与中文正确；MySQL 库建议 `utf8mb4`。

**Docker Compose + PostgreSQL 示例**
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
> 切库只改这两个变量，业务表（供应商 / 模型 / 配额 / 审计等）随 `AutoMigrate` 自动建到新库。**库之间不做数据迁移**——切到新 `DB_DSN` 即一个空库，历史数据需自行导出/导入。

### 多节点部署（PostgreSQL / MySQL + Redis）

横向扩展形态：N 个网关实例（同一二进制 + `web/dist`）+ 前置负载均衡，全部实例指向同一套共享存储。**单节点无需 Redis**，以下仅多实例部署需要。

```
                  ┌────────────────────────────────┐
                  │   LB（Nginx / HAProxy / 云 LB） │
                  │   探活端点: GET /healthz        │
                  └───────────────┬────────────────┘
           ┌──────────────────────┼──────────────────────┐
           ▼                      ▼                      ▼
     ┌────────────┐        ┌────────────┐         ┌────────────┐
     │ 网关实例 A  │        │ 网关实例 B  │   ...   │ 网关实例 N  │
     └──────┬─────┘        └──────┬─────┘         └──────┬─────┘
            │      所有实例指向同一套共享存储                 │
            └──────────────┬─────────────────────────────┘
                           ▼
    ┌─────────────────────────┐     ┌──────────────────────────┐
    │  PostgreSQL / MySQL      │     │  Redis（REDIS_ADDR）      │
    │  配置/配额/审计/API Key  │     │  限流计数/熔断广播/MFA票据 │
    └─────────────────────────┘     └──────────────────────────┘
```

**多节点环境变量**

| 变量 | 说明 |
|------|------|
| `DB_TYPE` + `DB_DSN` | 所有实例指向**同一个** PostgreSQL / MySQL 库；sqlite 仅限单节点（文件锁 + 单写连接） |
| `REDIS_ADDR` | Redis 地址 `host:port`，启用分布式限流（原子 INCR 固定窗口）、熔断打开状态广播（SETEX + TTL 自动半开）、MFA 二步票据（GET+DEL 原子核销）、**全局 SWRR 轮询游标**（Lua 原子推进，多实例分流互不重叠；掉线自动降级实例本地游标）。仅 mysql/postgres 生效 |
| `REDIS_PASSWORD` | Redis 密码；带 `requirepass` 的 Redis 必须配置，否则三组件启动即 `NOAUTH` 降级（行为如实告警） |
| `HEALTH_CHECK_SECONDS` | 上游健康检查周期秒（默认 30，`0`=禁用）：任何 HTTP 响应（含 401/404）=端点可达=清熔断；仅传输层错误（超时/DNS/连接拒绝）累计，达熔断阈值自动打开。多实例部署探测结果**经 Redis 广播**——任一实例打开熔断，其余实例 ≤1s 同步跳过该供应商 |
| `TRUSTED_PROXIES` | **LB 后必须设为代理 CIDR**（如 `10.0.0.0/8`）。默认空时 `ClientIP` 取 TCP 对端——不设则 API Key 的 IP 白名单会把所有请求匹配到 LB 地址，白名单形同虚设 |
| `JWT_SECRET` / `MASTER_KEY` | **所有实例必须完全一致**：前者保证任意实例可校验管理端 JWT，后者保证 API Key 密文（AES-GCM）可解密 |

**各状态域跨实例一致性**

- **全局一致（DB 承载）**：业务配置、配额（`used_value` 原子累加）、调用/操作审计、API Key/供应商/模型别名/限流规则、管理端 JWT（无状态，任一实例可校验）。
- **全局一致（Redis 承载，需 `REDIS_ADDR`）**：速率限制（"100/min" 全局精确 429）、SLB 熔断打开状态、MFA 二步登录票据（LB 后任意实例可完成二步验证）、**SWRR 全局轮询游标**（多实例加权分流互补重叠，干净请求路径 Lua 原子推进；故障转移重试路径走实例本地游标）。
- **实例本地**：SWRR 轮询游标——每实例独立轮询，单实例内分发仍正确，仅全局分布略有偏差（可接受，无需会话亲和）。

**配置热加载**：任一实例在管理后台变更配置 → DB `config_meta.counter` 递增 → 其余实例 ≤`hot_reload_seconds`（默认 3 秒）内自动重载快照；外部工具直改数据库同样生效（轮询兜底）。

**降级语义**：Redis 不可达（启动探活失败或运行中故障）时自动降级为实例本地限流/熔断语义并打印告警日志，热路径不受阻塞；每 5 秒探活，恢复后自动切回分布式计数。降级期间限流按单实例口径计数（口径放宽），恢复后重新全局精确。

**Docker Compose 多节点示例**（双网关 + PG + Redis；`REDIS_PASSWORD` 与实例变量需一致）：

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

> 前置 LB（Nginx/HAProxy/云 LB）轮询 `gateway-a:8080` / `gateway-b:8080`，探活 `GET /healthz`；LB 与网关间设置 `TRUSTED_PROXIES` 后客户端真实 IP 才会进入 API Key 的 IP 白名单判定。

**多节点实测**：双实例（同机 18080/18082，PG 共库 + Redis）已验证——分布式限流全局 429、熔断跨实例 ≤1s 同步打开 + TTL 自动半开恢复、配置热载跨实例 ≤4s 传播、配额全局精确。可复跑脚本见 `deploy/`（`mn_redis.sh`、`mn_circuit.sh`、`multi_node_redis_test.py`、`multi_node_circuit_test.py`）。

## 开发日志

`docs/PROGRESS.md` 是按轮次记录的开发日志（仅中文，不翻译），记录迭代历史。

## License

本项目基于 [Apache License 2.0](LICENSE) 开源。详见根目录 `LICENSE` 文件。
