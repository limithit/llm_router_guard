# AI 网关与模型护栏系统 — 项目进度记录

最后更新：2026-09-02（第三轮迭代：供应商测试连接修复 + 模型批量导入 + 内置帮助页）

## 📝 本轮迭代变更（第三轮，2026-09-02）

### 后端
- **供应商测试连接修复**（`providers.go`）：原任何 4xx(<500) 都误报"连接成功"，错误端点也提示成功。改为 `GET /v1/models` 真实验证（OpenAI + Anthropic 通用），仅 HTTP 200 且响应含 `data`/`models` 列表才算成功；401/403 报认证失败、404 报端点不存在、其余状态原样返回。成功时返回模型 ID 列表。
- **模型批量导入**（新 `POST /providers/:id/import-models`）：测试返回的模型列表让用户在供应商页勾选，批量创建为模型别名（别名=上游模型名，单上游指向本供应商，weight=1）；已存在别名自动跳过并在 `skipped_names` 返回。避免手输。
- **配额周重置 bug 修复**（`quota.go:NextReset`）：原按"周日"重置（Go `Weekday` 周日=0），改为以"周一"为周首（ISO/中国习惯），与测试预期一致。
- **静态资源路由 panic 修复**（`routes.go:MountStatic`）：`r.Static("/",...)` 的 catch-all `/*filepath` 与已注册的 `/api`、`/v1` 冲突导致启动 panic。改用 `NoRoute` 兜底 + SPA `index.html` 回退，且对 `/api`、`/v1` 路径返回 JSON 404。
- **测试补全**：新增 `slb_test.go`(22)、`quota_test.go`(32)、`admin/providers_test.go`(1)；全后端 `go test ./...` 绿。

### 前端
- **供应商页模型导入弹窗**（`Providers.tsx`）：测试连接成功且返回模型时弹出勾选表格（默认全选），确认后调导入接口并刷新模型别名列表；测试按钮 loading 改为按行独立（不再全行共享）。
- **内置帮助页**（新 `pages/Help.tsx` + 路由）：写入网关对外服务地址、三协议端点、鉴权方式（`Authorization: Bearer` / `x-api-key`）、`model` 字段填别名等使用说明，避免误以为是前后端各自启动的项目。
- `tsc --noEmit` + `vite build` 零错误。

### 文档
- **README**：修正"本地开发"与"构建与部署"章节——明确单进程/单端口模型；删除错误的 `go:embed` 声称（实际为运行时磁盘读取，`web/dist` 须随二进制部署）；补充前端热更新开发说明（Vite :5173 代理 :8080）。

## 📊 当前状态

### 编译状态
- ✅ **后端** `go build ./...` — 零错误
- ✅ **前端** `npm run build` — 零错误（dist/index.html + 1.59MB JS / gzip 500KB）
- ✅ **前端** `tsc --noEmit` — 零错误（类型安全验证通过）

### 测试状态
- ✅ **护栏引擎** `go test ./internal/guard -v` — 19 个测试全部通过（含基准测试）
  - 关键词匹配（contains/exact/regex 三种模式）
  - PII 检测与脱敏（手机号/邮箱/身份证号）
  - 注入检测（中英文模式）
  - 输出过滤策略（block/replace/log）
  - 护栏禁用、空白输入、Unicode 处理等边界场景
  - 全链路集成测试
- ✅ **SLB 负载均衡+熔断器** `go test ./internal/slb` — 22 个测试全部通过（加权选择 + 熔断器 + 故障转移）
- ✅ **配额+限流引擎** `go test ./internal/quota` — 32 个测试全部通过（NextReset 周/月计算 + 配额检查/消费/惰性重置 + 限流窗口；含"周一为周首"修复）
- ✅ **管理 API（供应商）** `go test ./internal/admin` — 测试连接 + 模型列表解析单测通过

### 完成度概览

| 模块 | 状态 | PRD REQ 覆盖 | 备注 |
|------|------|-------------|------|
| 数据库/多驱动 | ✅ 完成 | NFR-016 | SQLite(纯Go)/PostgreSQL/MySQL, GORM AutoMigrate |
| 配置热加载 | ✅ 完成 | REQ-004/004A | Atomic Pointer 快照切换、DB counter 增量触发 |
| 协议适配层 | ⚠️ 基础版 | REQ-017 | OpenAI Chat/Responses/Anthropic Messages; 工具调用未实现 |
| 护栏引擎 | ✅ 完成 | REQ-008~011 | 敏感词/PII/注入/输出过滤; AC自动机接口预留但未实现 |
| SLB+熔断 | ✅ 完成 | REQ-006/007 | 加权随机选择、熔断器、指数退避重试 |
| 配额+限流 | ✅ 完成 | REQ-012/013 | 固定窗口计数、惰性周期重置、Webhook预警 |
| 审计日志 | ✅ 完成 | REQ-003/015 | 调用审计异步批量写入、操作审计全记录、CSV导出 |
| JWT认证 | ✅ 完成 | - | 管理后台JWT签发校验、登录锁定策略 |
| MFA | ✅ 完成 | REQ-020~022 | TOTP绑定/解绑、恢复码、强制策略 |
| 管理API CRUD | ✅ 完成 | REQ-001~REQ-024(除计费外) | 70+ REST端点全部实现 |
| 前端页面 | ✅ 完成 | PRD 4.2 | 22个TSX页面，Ant Design 5.x + React Router v6 + TanStack Query |
| Docker部署 | ✅ 完成 | - | 多阶段构建、healthcheck、环境变量驱动 |

## 📁 项目文件清单

```
backend/                                # Go 后端
├── go.mod                              # Go module (go 1.24, toolchain local)
├── cmd/server/main.go                  # 入口程序
└── internal/
    ├── config/config.go                # 环境变量解析 (PORT/DB_TYPE/DB_DSN/JWT_SECRET...)
    ├── db/db.go                        # 多驱动打开 + AutoMigrate
    ├── model/model.go                  # 18 个 GORM 实体定义
    ├── crypto/crypto.go                # AES-GCM 加密 / SHA-256 摘要
    ├── settings/settings.go            # KV 配置结构体 (General/Failover/Output/etc.)
    ├── runtime/manager.go              # 热加载核心 (Snapshot atomic swap)
    ├── runtime/bundle.go               # 全量备份/回滚 JSON 序列化
    ├── guard/engine.go                 # 输入检测/输出过滤/脱敏
    ├── slb/slb.go                      # 加权负载 + 熔断器
    ├── quota/quota.go                  # 速率限制(固定窗口) + 配额(惰性重置)
    ├── adapter/adapter.go              # 三协议解析/构建/响应序列化
    ├── adapter/sse.go                  # SSE 流式增量解析/客户端重写
    ├── gateway/gateway.go              # 代理主循环 (认证→护栏→限流→配额→SLB→转发)
    ├── metrics/metrics.go              # QPS滑动窗口/连接数/错误环形缓冲
    ├── audit/audit.go                  # 调用审计异步写入/定期清理
    ├── auth/jwt.go                     # JWT Issue/Parse
    └── admin/                          # 管理 API handlers (16 文件)
        ├── server.go                   # Server 结构 + 响应工具 + 中间件
        ├── middleware.go               # JWT AuthMiddleware
        ├── auth.go                     # login/me/logout/password + MFA self-service
        ├── dashboard.go                # REQ-016 概览
        ├── providers.go                # REQ-005 CRUD + 测试连接
        ├── models.go                   # REQ-006 CRUD + 统计
        ├── settings.go                 # REQ-001/007/011/014/020 KV 设置 GET/PUT
        ├── apikeys.go                  # REQ-002 CRUD
        ├── users.go                    # REQ-023 用户 MFA 管理
        ├── guard.go                    # REQ-008/009/010 CRUD + 批量导入导出 + 模板 + 测试
        ├── quota.go                    # REQ-012/013 配额 + 限流 CRUD
        ├── audit.go                    # REQ-003/015 调用/操作审计查询 + CSV导出
        ├── backup.go                   # REQ-019 备份列表/创建/下载/恢复/删除
        ├── configstatus.go             # REQ-004A 配置状态 + reload/rollback/export
        └── status.go                   # REQ-018 运行状态监控
        └── routes.go                   # 路由注册 (网关端点 + 管理API + 静态资源)

frontend/src/
├── api/                                  # axios实例(envelope处理)+类型定义+端点函数
├── layouts/MainLayout.tsx               # 侧栏树形导航 + 面包屑 + 顶栏
├── store/auth.ts                         # Zustand 认证态 (token/user 持久化)
├── pages/                                # 22 个页面组件 (见下方明细)
│   ├── Login.tsx                         # 一级+二级(MFA)登录
│   ├── Dashboard.tsx                     # 概览卡片/趋势图/排行/分布/健康
│   ├── providers/Providers.tsx           # 供应商CRUD + 测试连接
│   ├── models/Models.tsx                 # 别名CRUD + upstream权重表单
│   ├── models/Failover.tsx               # 故障转移配置
│   ├── guard/Keywords.tsx                # 敏感词CRUD + 批量导入/测试
│   ├── guard/PiiRules.tsx                # PII规则CRUD + 预置模板
│   ├── guard/InjectionRules.tsx          # 注入规则CRUD + 预置模板
│   ├── guard/OutputFilter.tsx            # 输出过滤配置
│   ├── quota/Quotas.tsx                  # 配额CRUD + 进度条 + 重置
│   ├── quota/RateLimits.tsx              # 限流CRUD + 全局模式
│   ├── quota/QuotaAlerts.tsx             # 预警阈值配置
│   ├── audit/Calls.tsx                   # 调用日志筛选 + 详情Drawer + CSV导出
│   ├── audit/Operations.tsx              # 操作审计筛选 + before/after JSON diff
│   ├── settings/General.tsx              # 通用设置表单
│   ├── settings/ApiKeys.tsx              # API Key管理(一次性展示完整key)
│   ├── settings/Security.tsx             # MFA全局配置 + 用户MFA状态表
│   ├── settings/ConfigStatus.tsx         # 版本历史 + 加载日志Timeline + 回滚
│   ├── settings/RuntimeStatus.tsx        # 运行指标卡片 + QPS表 + 错误列表
│   ├── settings/Backup.tsx               # 备份列表 + JSON拖拽恢复
│   └── account/AccountSecurity.tsx       # MFA自助绑定流程(二维码+验证+恢复码)
└── components/                           # PageContainer/Charts/JsonView/StatusSwitch

根目录文档:
├── PRD.md                               # 产品需求文档 (全文档)
├── docs/api-contract.md                 # 前后端对接契约 (唯一依据)
├── docs/PROGRESS.md                     # 本文件 (进度记录)
├── README.md                            # 项目说明 + 快速开始 + 架构图
├── Dockerfile                           # 多阶段构建(Docker)
├── docker-compose.yml                   # 容器编排
└── build.ps1                            # PowerShell一键构建脚本
```

## 🔧 已知问题 / Bug 待修复

### 后端

#### 1. [P1] Gateway 网关——缺少上下文传递
- **位置**: `internal/gateway/gateway.go`
- **问题**: 上游 HTTP 请求的 `context.Context` 虽然被传递了超时控制 (`ctx = c.Request.Context()`)，但 request_id 未作为 header (如 `X-Request-ID`) 传递给上游。这导致上游无法追踪同一请求在分布式系统中的链路。
- **建议**: 在 `forward()` 中为每个请求生成 UUID 并设置 `req.Header.Set("X-Request-ID", reqID)`。

#### 2. [P2] Gateway 网关——非流式 Token 估算精度低
- **位置**: `internal/gateway/gateway.go:estimateUsage()`
- **问题**: 当上游未返回 token 用量时，使用 `rune_count / 2` 粗略估算。对中文字符串偏大，对英文偏小。
- **建议**: 可引入一个轻量 tokenizer (如 tiktoken-go)，或至少按字符集区分估算系数：CJK `/2`, ASCII `/4`, mixed `/3`。当前足够满足"仅在有上游数据时不估算"的兜底场景。

#### 3. ✅ 已修复 — Provider 测试连接假成功 + Anthropic base_url 拼接复杂
- **位置**: `internal/admin/providers.go:testProvider()`
- **原问题**: ① 任何 4xx(<500) 都被判为"连接成功"，错误端点也提示成功；② Anthropic URL 拼接冗余。
- **修复**: 统一改用 `GET /v1/models`（OpenAI + Anthropic 通用），严格判据仅 HTTP 200 且响应含 `data`/`models` 列表才算成功；401/403 报认证失败、404 报端点不存在、其余状态原样返回。成功时返回模型 ID 列表供前端勾选导入。base_url 归一化与 `adapter.BuildUpstreamRequest` 一致。

#### 4. [P3] Audit Logger——首次写入库可能阻塞启动
- **位置**: `internal/audit/audit.go:writerLoop()`
- **问题**: 如果数据库首次连接慢，buffer channel (cap 4096) 满时会直接同步 INSERT 阻塞 handler goroutine。
- **建议**: buffer 已满时使用 `select { case ch <- cl: ... default: s.db.Create(cl) }` 降级，并记录告警。当前代码已经做了 fallback (`default: l.db.Create(cl)`)，是合理的。

#### 5. [Minor] Settings General.ListenPortNote 未在 UI 层面消费
- **位置**: `settings/general` endpoint 返回 `listen_port_note` 字段
- **问题**: 前端 GeneralSettings.tsx 使用了硬编码的提示文本而非读取该字段。
- **建议**: 让前端使用 `data.listen_port_note || "默认提示"` 替代硬编码。

### 前端

#### 1. [P2] Chart 组件依赖缺失
- **位置**: `frontend/src/components/Charts.tsx`
- **问题**: 子代理尝试使用 `@ant-design/plots` 或 echarts，但最终回退到手动 SVG 绘制。图表较简单，可以工作但样式不完美。
- **建议**: 下一个会话可考虑添加 `echarts-for-react` 获得更好的可视化效果。

#### 2. [P2] RateLimit 列表接口分页格式不一致
- **位置**: `frontend/src/api/endpoints.ts` + `RateLimits.tsx`
- **问题**: `rateLimitApi.list()` 的 params 传给 `http.get()` 时 TypeScript 报错 `PageParams & { keyword?: string }` 不能赋值给 `Record<string, unknown>`。这是因为 `http.get` 的签名要求 `object` 而非索引签名。
- **修复方案**: client.ts 已改为 `params?: object`，但需确保所有 `.ts` 文件重新通过 tsc --noEmit 检查。**当前已知此 issue 已被 `api/client.ts` 修改 `get<T>(url, params?: object)` 解决。** 请确认是否已通过构建验证。

#### 3. [Minor] 部分页面缺少 loading 骨架屏
- **位置**: 多个 `pages/` 下的 Table 组件
- **问题**: 只在 `isLoading={true}` 时传给了 Ant Design Table 的 `loading` prop，但没有 skeleton 骨架屏体验。
- **建议**: 后续优化时可加 antd 5.x 的 `Skeleton` 组件。

## 📋 下一轮迭代建议

### 优先级排序

| 优先级 | 任务 | 预计工作量 | 描述 | 状态 |
|--------|------|-----------|------|------|
| **P0** | 完善 API Client 类型安全 | 小 | 确认 endpoints.ts 中 `pageParams` 类型兼容问题已彻底解决 | ✅ 已完成（tsc --noEmit 零错误） |
| **P0** | 护栏引擎单元测试 | 中等 | `guard/engine.go` 核心逻辑单测覆盖 | ✅ 已完成（19 测试全部通过） |
| **P0** | SLB 负载均衡单元测试 | 中等 | `slb/slb.go` 加权选择 + 熔断器逻辑单测 | ✅ 已完成（22 测试通过） |
| **P0** | 配额限流单元测试 | 中等 | `quota/quota.go` 速率限制 + 配额检查单测 | ✅ 已完成（32 测试通过；含 NextReset 周首修复） |
| **P1** | 增加 WebAssembly tokenizer | 中等 | 集成 `tiktoken-go` 用于准确的 Token 用量估算 (gateway.go → estimateUsage) | ⏳ 待办 |
| **P1** | 上游健康检查定时任务 | 小 | 每隔 N 分钟主动探测各 provider 连通性，更新 SLB health map | ⏳ 待办 |
| **P1** | 前端路由懒加载 | 小 | `React.lazy` + `Suspense` 按路由 chunk 拆分，减少首屏体积 | ⏳ 待办 |
| **P2** | 支持 WebSocket 实时审计推送 | 中等 | 替代定时轮询 ConfigStatus 和 Status 页面的 refetchInterval | ⏳ 待办 |
| **P2** | 增加 Prometheus Metrics 出口 | 中等 | 暴露 /metrics 端点供 Grafana 采集 | ⏳ 待办 |
| **P2** | 提供示例 .env 和 Docker Compose PostgreSQL | 小 | 提供开箱即用的多 DB 部署模板 | ⏳ 待办 |

### 可独立开展的工作块

```
区块 A (后端测试) — 进行中 ✅:
  ✅ unit tests for guard engine (keyword/pii/injection/output) — 19 tests PASS
  ⏳ unit tests for slb balancer (weighted pick, circuit breaker, failover)
  ⏳ unit tests for quota/rate-limit (fixed window, lazy reset, webhook alert)

区块 B (后端增强):
  - add X-Request-ID forwarding to upstream
  - improve token estimation (tiktoken-go Wasm)
  - periodic upstream health probe cron job
  - Prometheus metrics exporter (/metrics endpoint)

区块 C (前端优化):
  ✅ fix remaining ts errors — tsc --noEmit passes with 0 errors
  - lazy-load route chunks (React.lazy)
  - add Skeleton loaders to all data tables
  - consume listen_port_note field from API instead of hardcoded text

区块 D (DevOps):
  - GitHub Actions CI/CD pipeline
  - example docker-compose with postgres
  - example .env.template with all variables documented
```

## 🔑 关键技术决策记录

1. **配置存储**: 全部业务配置走数据库，不读磁盘 YAML/JSON 文件 (PRD 6.2 明确要求)。
2. **热加载机制**: Manager.AtomicPointer[Snapshot] 实现无锁读 + CAS 换快照；DB counter 作为多副本感知信号。
3. **护栏设计**: Guard.CheckInput 逐条消息检测并就地 PII mask；CheckOutput 对完整输出二次审核。stream 路径按 threshold 间隔检测。
4. **协议转换**: 不实现 full bidirectional message format conversion。采用 canonical intermediate representation，各 provider adapter 只做最小必要映射。tools/callbacks 等高级特性暂不实现。
5. **密码哈希**: bcrypt (golang.org/x/crypto), cost=10。恢复码存 sha256 哈希。
6. **API Key**: 明文存储于内存（Provider 解密后复用），数据库只存 encrypted version (AES-GCM)。
7. **前端状态管理**: Zustand（轻量级，不引入 Redux complexity）+ TanStack Query（服务端缓存）。
8. **测试策略**: 测试文件与源文件同目录 (`_test.go`)，使用标准 `testing` 包 + 表驱动测试 + 基准测试。Snapshot 手工构建，不依赖真实 DB。

---

**下一步行动**: 
1. ✅ 检查 `endpoints.ts` 中 PageParams 类型兼容性问题是否已被 client.ts 修复消除 — tsc --noEmit 零错误
2. ✅ 运行 `tsc --noEmit` 确认前端无 TS 错误 — 通过
3. ✅ 补充 `guard/engine.go` 单测 — 18 测试 + 基准全部通过
4. ✅ 补充 `slb/slb.go` 单测 — 22 测试全部通过
5. ✅ 补充 `quota/quota.go` 单测 — 32 测试全部通过（含 NextReset 周首修复）
6. ✅ 供应商测试连接假成功修复 + 模型批量导入 + 内置帮助页 + 静态路由 panic 修复
7. ⏳ 按上述区块 B-D 中的任务推进迭代（X-Request-ID 转发、token 估算、上游健康探测、Prometheus、路由懒加载等）
