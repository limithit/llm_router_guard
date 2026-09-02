# AI 网关与模型护栏系统 — 项目进度记录

最后更新：2026-09-02

## 📊 当前状态

### 编译状态
- ✅ **后端** `go build ./...` — 零错误
- ✅ **前端** `npm run build` — 零错误（dist/index.html + 1.59MB JS / gzip 500KB）

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

#### 3. [P3] Provider 测试连接——Anthropic base_url 拼接逻辑复杂
- **位置**: `internal/admin/providers.go:testProvider()`, line 148-156
- **问题**: Anthropic 协议的 URL 处理有冗余的 `TrimSuffix/HasSuffix` 嵌套判断。
- **建议**: 统一用 `strings.TrimSuffix(baseURL, "/v1") + "/v1/messages"` 简化；或直接让用户在 BaseURL 字段末尾加上 `/v1`（与 OpenAI 约定一致）。

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

| 优先级 | 任务 | 预计工作量 | 描述 |
|--------|------|-----------|------|
| **P0** | 完善 API Client 类型安全 | 小 | 确认 endpoints.ts 中 `pageParams` 类型兼容问题已彻底解决 |
| **P0** | 单元测试补全 | 中等 | `guard/engine.go`, `slb/slb.go`, `quota/quota.go` 的核心逻辑需要单测覆盖 |
| **P1** | 增加 WebAssembly tokenizer | 中等 | 集成 `tiktoken-go` 用于准确的 Token 用量估算 (gateway.go → estimateUsage) |
| **P1** | 上游健康检查定时任务 | 小 | 每隔 N 分钟主动探测各 provider 连通性，更新 SLB health map |
| **P1** | 前端路由懒加载 | 小 | `React.lazy` + `Suspense` 按路由 chunk 拆分，减少首屏体积 |
| **P2** | 支持 WebSocket 实时审计推送 | 中等 | 替代定时轮询 ConfigStatus 和 Status 页面的 refetchInterval |
| **P2** | 增加 Prometheus Metrics 出口 | 中等 | 暴露 /metrics 端点供 Grafana 采集 |
| **P2** | 提供示例 .env 和 Docker Compose PostgreSQL | 小 | 提供开箱即用的多 DB 部署模板 |

### 可独立开展的工作块

```
区块 A (后端):
  - unit tests for guard engine (keyword/pii/injection/output)
  - add X-Request-ID forwarding to upstream
  - improve token estimation (tiktoken-go Wasm)

区块 B (后端):
  - periodic upstream health probe cron job
  - Prometheus metrics exporter (/metrics endpoint)

区块 C (前端):
  - fix remaining ts errors if any
  - lazy-load route chunks (React.lazy)
  - add Skeleton loaders to all data tables

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

---

**下一步行动**: 
1. 检查 `endpoints.ts` 中 PageParams 类型兼容性问题是否已被 client.ts 修复消除
2. 运行 `tsc --noEmit` 确认前端无 TS 错误
3. 补充 `guard/engine.go` 和 `slb/slb.go` 的单测
4. 按上述区块 A-D 中的任务推进迭代
