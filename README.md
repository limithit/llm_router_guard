# AI 网关与模型护栏系统

轻量级、自托管、功能聚焦的 AI 网关平台，支持多协议统一接入（OpenAI Chat/Responses / Anthropic）、智能路由负载均衡、输入/输出双向护栏过滤、配额与速率限制、全链路审计日志、配置热加载及 TOTP MFA 管理后台。

**技术栈**：Go 1.24（Gin + GORM）+ React 18（Ant Design 5.x + Vite 5）

## 快速开始

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
| `DB_TYPE` | `sqlite` | 数据库类型：`sqlite` / `postgres` / `mysql` |
| `DB_DSN` | `gateway.db` | 连接串；sqlite 为文件路径，其余遵循标准 DSN |
| `JWT_SECRET` | `change-me-jwt-secret` | JWT 签名密钥 |
| `MASTER_KEY` | `llm-router-guard-master-key` | API Key 加密主密钥 |
| `ADMIN_USER` | `admin` | 首启管理员用户名 |
| `ADMIN_PASSWORD` | `admin123` | 首启密码（必须修改！）|
| `DATA_DIR` | `./data` | 备份文件存放目录 |
| `FRONTEND_DIST` | (auto-detect) | 前端构建产物目录 |

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

## License

内部开源 — 仅供企业自托管部署使用。
