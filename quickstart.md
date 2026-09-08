# Quickstart — 5 分钟从零跑通

> 目标：本机起服务 → 配好第一个模型 → 网关成功转发一次请求。
> 架构、全部环境变量、多节点拓扑请看 [README.md](README.md)；API 契约见 [docs/api-contract.md](docs/api-contract.md)。

## 0. 前置条件

| 依赖 | 版本 | 用途 |
|------|------|------|
| Go | ≥ 1.24 | 编译后端（唯一运行时进程） |
| Node.js | ≥ 18 | 仅构建前端产物（生产可跳过，直接用预构建 `web/dist`） |
| Docker | 任意 | 可选，compose 一键起 |

数据库**零依赖**：默认 SQLite（纯 Go 驱动，自动建表）；MySQL/PostgreSQL 见第 5 节。

## 1. 构建并启动（单进程，一个端口全包）

```bash
# Linux / macOS（关键：从 backend/ 目录运行，自动检测到 web/dist 与 gateway.db）
cd frontend && npm install && npm run build && cd ..
mkdir -p backend/web/dist && cp -r frontend/dist/* backend/web/dist/
cd backend && go build -ldflags="-s -w" -o bin/llm-router-guard ./cmd/server && ./bin/llm-router-guard
# 默认 :8080，同一端口提供 管理API + /v1/* 网关 + 前端SPA（CWD 非 backend/ 时用 FRONTEND_DIST 指产物绝对路径）
```

```powershell
# Windows：一条脚本搞定（装依赖 + 构建前端 + 拷产物 + 编译）
.\build.ps1
cd bin; .\server.exe        # 从仓库根的 frontend/dist 回退检测自动定位前端
```

浏览器打开 **http://localhost:8080** 。

冒烟检查（三个免认证端点）：

```bash
curl -s http://localhost:8080/healthz    # {"status":"ok"} —— LB 探活用这个
curl -s http://localhost:8080/metrics | head   # Prometheus 文本格式
curl -s http://localhost:8080/v1/models  # 需 Authorization，未带 key 返回 401 即正常
```

## 2. 首启登录与安全三件事

默认账号 `admin` / `admin123`（未设 `ADMIN_PASSWORD` 时；启动日志会打 WARNING）。

登录后进右上角用户菜单 → **账户**（账户安全页）：

1. **修改密码**（登录后立即改）
2. **绑定 TOTP MFA**（扫码 → 输动态码确认 → 一次性展示备用恢复码，妥善保存；解绑需验证码或恢复码）
3. 生产部署务必替换环境变量默认值：`JWT_SECRET`、`MASTER_KEY`（用 `.env.template` 抄：`cp .env.template .env`）

## 3. 5 分钟跑通第一次转发

按左侧菜单顺序操作：

1. **模型 → 供应商**：新建供应商
   - 协议**必须**选 `openai_chat` / `openai_responses` / `anthropic` 三者之一（填 `openai` 会被 400 拒绝）
   - Base URL 含 `/v1`（如 `https://api.openai.com/v1`）；保存后点 **测试连接**（严格判据：`GET /v1/models` 返回模型列表才算通）
2. **模型 → 模型别名**：新建别名（如 `gpt-4o-mini`），挂上一步供应商为上游（可多上游配权重 → 自动加权轮询 + 熔断 + 故障转移；见 **模型 → 故障转移** 调阈值）
3. **设置 → API Key**：新建 Key，**明文只显示这一次**，立即复制（格式 `sk-` + 32 hex）
4. 调网关（OpenAI 兼容任意 SDK，把 `base_url` 指到网关即可）：

```bash
curl -s http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-你的key" -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"你好"}]}'
# 流式加 "stream": true（SSE 透传）；Anthropic 客户端协议走 /v1/messages，Responses 走 /v1/responses
```

5. 验证闭环：**审计 → 调用日志** 应出现该次请求（Token 用量/延迟/护栏判定）；
   带请求头 `-H "X-Request-ID: my-trace-1"` 时，响应头原样回带同值，
   并可用详情接口按该 ID 精确定位记录：`GET /api/admin/v1/audit/calls/my-trace-1`（需管理端 JWT）。

可选体验护栏：**护栏 → 关键词** 加一条敏感词（动作 block）→ 提问命中即 400 `content_filtered`。

## 4. Docker Compose（单机最省事）

```bash
cp .env.template .env      # 改 JWT_SECRET / MASTER_KEY / ADMIN_PASSWORD
docker compose up -d       # 容器 :8080，数据卷 ./data，/healthz 健康检查已内置
docker compose logs -f gateway
```

## 5. 换 MySQL / PostgreSQL（一行配置的事）

应用首启 `AutoMigrate` 自动建全部 16 张表，无需手工 SQL：

```bash
DB_TYPE=postgres DB_DSN="postgres://user:pass@host:5432/gateway?sslmode=disable" ./bin/llm-router-guard
DB_TYPE=mysql    DB_DSN="user:pass@tcp(host:3306)/gateway" ./bin/llm-router-guard   # charset 等参数自动补齐
```

DBA 预审 / 手工建库 / 只读账号部署场景，用仓库预制的基线 SQL（与 AutoMigrate 产物逐列一致，已回灌往返验证）：

```bash
# MySQL
mysql -uroot -p -e "CREATE DATABASE gateway CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
mysql -uroot -p gateway < deploy/schema/schema.mysql.sql
# PostgreSQL
psql -U postgres -c "CREATE DATABASE gateway;"
psql -U postgres -d gateway -f deploy/schema/schema.postgres.sql
```

> 换模型后重新生成：测试机跑 `deploy/make_schema.sh`，验证跑 `deploy/verify_schema.sh`（详见 `deploy/schema/README.md`）。

## 6. 多节点（横向扩展）速览

同一二进制起 N 实例 + 前置 LB（探活 `/healthz`），全部实例：**同一个** PG/MySQL + **同一个** Redis，
且 `JWT_SECRET` / `MASTER_KEY` 完全一致。SQLite 仅限单节点。

```bash
DB_TYPE=postgres DB_DSN=... REDIS_ADDR=redis-host:6379 REDIS_PASSWORD=*** \
TRUSTED_PROXIES=10.0.0.0/8 ./bin/llm-router-guard    # 每个实例都要设 TRUSTED_PROXIES=LB 网段，否则 IP 白名单形同虚设
```

`REDIS_ADDR` 生效范围（仅 mysql/postgres）：分布式限流（全局精确 429）、熔断打开状态广播（≤1s 跨实例同步）、
MFA 二步票据（LB 后任意实例可完成二步登录）、SWRR 全局轮询游标。Redis 故障自动降级单实例语义并告警，恢复自动切回。
拓扑图 / Compose 双网关模板 / 一致性速查表见 [README 多节点章节](README.md#多节点部署postgresql--mysql--redis)。

## 7. 开发模式（前端热更新）

```bash
cd backend && go run ./cmd/server          # 终端 1：后端 :8080
cd frontend && npm install && npm run dev  # 终端 2：Vite :5173，/api 与 /v1 自动代理到 8080
```

改前端代码保存即生效（http://localhost:5173）；改后端 `go run` 重启即可。

## 8. 故障速查

| 症状 | 原因 / 处理 |
|------|-------------|
| 启动日志 `[security] WARNING` 默认密钥 | 生产未换 `JWT_SECRET`/`MASTER_KEY`/`ADMIN_PASSWORD` |
| `[ratelimit] redis ... NOAUTH ... DEGRADED` | Redis 有 `requirepass` 但没配 `REDIS_PASSWORD`；降级期间限流按单实例口径 |
| 访问 `/` 返回 404 | 部署目录缺 `web/dist`（前端产物随二进制部署，或用 `FRONTEND_DIST` 指定绝对路径） |
| 测试连接失败但聊天正常 | 测试连接是严格判据（要求模型列表 JSON）；上游 `/v1/models` 非标准时会报失败，属预期 |
| 上游转发 502 `unsupported upstream protocol` | 供应商协议字段填了枚举外的值（新版创建/更新时即 400 拦截） |
| 客户端 IP 全是 LB 地址 | `TRUSTED_PROXIES` 未设为代理网段 |
| 端口被占起不来 | `PORT=18080 ./bin/llm-router-guard`；老进程 `fuser -k 8080/tcp`（Linux） |
| 多实例改配置另一实例几秒后才生效 | 热加载轮询周期，默认 ≤3s（`hot_reload_seconds` 可调）——这是设计内行为 |
