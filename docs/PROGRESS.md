# AI 网关与模型护栏系统 — 项目进度记录

最后更新：2026-09-02（第六轮：API Key Token 用量统计看板 + 流式输出护栏短输出兜底修复）

##  本轮迭代变更（第六轮）

### 新增：API Key Token 用量统计看板
- **后端**（`internal/admin/tokenstats.go`，新文件）：
  - `GET /api/admin/v1/token-stats` — 汇总 + 按 Key 排行 + 按模型分布 + 时间趋势分桶
  - `GET /api/admin/v1/token-stats/export` — 按 Key 用量 CSV 导出（含 BOM）
  - 时间范围左闭右闭，缺省近 30 天；支持 `api_key_id`/`model` 过滤与 `day|hour` 粒度
  - 跨数据库时间分桶：sqlite `strftime` / mysql `DATE_FORMAT` / postgres `to_char`
  - by_key 合并 APIKey 表（含零用量 Key + 已删除 Key 的历史记录，按 total 降序）
- **前端**（新 `pages/usage/TokenStats.tsx`）：
  - 快捷时间预设（今天/近7天/近30天/本月/自定义）+ 自定义 RangePicker + Key/模型过滤 + 天/小时粒度
  - 汇总卡片（总 Token / Prompt / Completion / 调用次数 / 有用量 Key 数）
  - 历史趋势折线（Prompt vs Completion）+ 模型分布环形图 + Key 排行表（可排序 + 占比进度条）+ CSV 导出
  - 路由 `/usage/token`，侧栏一级菜单「Token 用量」
- **契约**：docs/api-contract.md 新增 8.3 Token 用量统计章节
- **测试**：`tokenstats_test.go` 8 个用例全绿

### 修复：流式输出护栏短输出漏检
- **问题**：创建关键词 "Google" 后，模型输出未被拦截。
- **根因**：`internal/gateway/forwardStream` 仅在累积输出长度 `>= threshold`（默认 256）时检测一次；短输出（< 256 字节）在流结束前永远到不了阈值，且循环结束后没有兜底检测，导致被完全放行。
- **修复**：
  - 将检测/策略处理逻辑抽取为 `runStreamGuardCheck`（block/replace/log 三策略统一处理）。
  - 循环结束后对完整累积文本执行一次**兜底输出护栏检测**；短输出也能被正常 block/replace/log。
  - 保持阈值化检测作为“尽早介入”优化，避免漏检。
- **测试**：新增 `internal/gateway/gateway_test.go`，覆盖：
  - 短输出（< 256）命中敏感词被 replace 拦截
  - 长输出超过阈值被 block 拦截
  - 正常短输出不误拦截

### 踩坑记录
- **GORM Scan 不展开嵌入结构体**：聚合结果 Scan 进 `struct{ APIKeyID uint; tokenAgg }`（tokenAgg 为匿名嵌入）时数值列全部为零。GORM 的 `Scan` 对匿名嵌入字段不做字段提升，必须展平所有列字段。

## 📊 当前状态总览

### 编译状态
- ✅ **后端** `go test ./...` — 全绿（guard/slb/quota/admin/gateway）
- ✅ **前端** `vite build` — 零错误（dist/assets/index-D1hCx5BN.js 1,600 KB / gzip 503 KB）

### 测试状态
- ✅ **全后端** `go test ./...` — 全绿
  | 包 | PASS 断言数 | 测试文件 | 覆盖范围 |
  |----|-----------|---------|---------|
  | `internal/guard` | 32 | engine_test.go (450行) | 关键词 contains/exact/regex、PII 检测+脱敏、注入检测、输出过滤 block/replace/log、护栏禁用、Unicode、全链路集成、MaskForLog、FindingsJSON |
  | `internal/slb` | 22 | slb_test.go (502行) | 加权选择(SWRR)、权重分布统计、熔断器开/半开/恢复、故障转移全循环、PickIgnoringCircuit、HealthList、并发安全 |
  | `internal/quota` | 32 | quota_test.go (590行) | NextReset 日/周(周一)/月/跨年、matchQuotas 通配、限流窗口+重置、配额 Check/Consume/惰性重置、FlushHits、多规则并发 |
  | `internal/admin` | 13 | providers_test.go (111行) + tokenstats_test.go (340行) | 供应商测试连接 + 模型列表解析 + Token 用量统计（聚合/过滤/趋势/CSV） |
  | `internal/gateway` | 3 | gateway_test.go (160行) | 流式输出护栏：短输出兜底检测 / 阈值检测 / 正常输出不误拦截 |
  | **合计** | **102** | **6 文件** | — |

### Git 状态
- 当前分支：`dev`
- 最新提交：待提交（流式输出护栏短输出兜底修复）
- 工作区：有变更（未提交）

## 📝 已完成迭代历史

### 第一~二轮：初始实现
- 全部后端 13 个 internal 包 + main 入口
- 全部前端 22 个 TSX 页面 + api/layouts/store/components
- README / Dockerfile / docker-compose.yml / build.ps1 / .gitignore

### 第三轮：修复 + 测试 + 增强
- 供应商测试连接修复（`GET /v1/models` 严格判据）
- 模型批量导入（`POST /providers/:id/import-models`）
- 配额周重置 bug 修复（周一为周首）
- 静态资源路由 panic 修复（`NoRoute` + SPA 回退）
- 单元测试补全：guard(19) + slb(22) + quota(32) + admin(6)

### 第四轮：用户指定需求（全部 ✅）
1. **SLB 加权轮询去随机化** — 改为平滑加权轮询(SWRR)，确定性分发，保留熔断跳过+故障转移排除
2. **API Key 模型限定 + IP 白名单** — `AllowedModelsJSON` + `IPAllowlistJSON` + `IPAllowlistEnabled`，默认全放行
3. **审计日志写入开关** — 总开关 + 采样率 + 仅错误模式，操作审计不可关

## 📁 项目文件清单

```
backend/                                # Go 后端 (33 源文件 + 4 测试文件)
├── go.mod                              # Go module (go 1.24)
├── cmd/server/main.go                  # 入口程序
└── internal/
    ├── config/config.go                # 环境变量解析
    ├── db/db.go                        # 多驱动打开 + AutoMigrate
    ├── model/model.go                  # GORM 实体定义 (含 APIKey 模型限定/IP白名单字段)
    ├── crypto/crypto.go                # AES-GCM 加密 / SHA-256 摘要
    ├── settings/settings.go            # KV 配置结构体 (含 CallAudit 开关/采样)
    ├── runtime/manager.go              # 热加载核心 (Snapshot atomic swap)
    ├── runtime/bundle.go               # 全量备份/回滚 JSON 序列化
    ├── guard/engine.go                 # 输入检测/输出过滤/脱敏
    ├── slb/slb.go                      # 平滑加权轮询(SWRR) + 熔断器
    ├── quota/quota.go                  # 速率限制(固定窗口) + 配额(惰性重置)
    ├── adapter/adapter.go              # 三协议解析/构建/响应序列化
    ├── adapter/sse.go                  # SSE 流式增量解析/客户端重写
    ├── gateway/gateway.go              # 代理主循环 (认证→护栏→限流→配额→SLB→转发)
    ├── metrics/metrics.go              # QPS滑动窗口/连接数/错误环形缓冲
    ├── audit/audit.go                  # 调用审计异步写入(可关停+采样)/定期清理
    ├── auth/jwt.go                     # JWT Issue/Parse
    └── admin/                          # 管理 API handlers (16 文件)
        ├── server.go / middleware.go / auth.go / mfa.go
        ├── dashboard.go / providers.go / models.go
        ├── settings.go / apikeys.go / users.go
        ├── guard.go / quota.go / audit.go
        ├── backup.go / configstatus.go / status.go / routes.go

backend/internal/*/  *_test.go           # 4 测试文件 (1,653 行, 92 断言)

frontend/src/                           # React 前端 (37 个 .ts/.tsx 文件)
├── api/ (client.ts, types.ts, endpoints.ts)
├── layouts/MainLayout.tsx
├── store/auth.ts
├── pages/ (22 个页面 + Help.tsx)
└── components/ (PageContainer, Charts, JsonView, StatusSwitch)

根目录: PRD.md, docs/api-contract.md, docs/PROGRESS.md, README.md,
        Dockerfile, docker-compose.yml, build.ps1, .gitignore
```

## 🔧 已知问题 / Bug 待修复

### 后端

| # | 优先级 | 位置 | 问题 | 建议 |
|---|--------|------|------|------|
| 1 | P1 | `gateway/gateway.go` | request_id 未作为 `X-Request-ID` header 传给上游 | `forward()` 中生成 UUID 并设置 header |
| 2 | P2 | `gateway/gateway.go:estimateUsage()` | 上游无 token 用量时 `rune_count/2` 粗估 | 集成 tiktoken-go 或按字符集区分系数 |
| 3 | P3 | `audit/audit.go:writerLoop()` | DB 慢时 buffer 满阻塞 handler | 已有 `default: db.Create()` 降级，合理 |
| 4 | Minor | `settings/general` | `listen_port_note` 字段前端未消费 | 前端读 API 返回值替代硬编码 |

### 前端

| # | 优先级 | 位置 | 问题 | 建议 |
|---|--------|------|------|------|
| 1 | P2 | `components/Charts.tsx` | 手动 SVG 绘制，样式简单 | 添加 `echarts-for-react` |
| 2 | Minor | 多个 Table 组件 | 仅有 `loading` prop，缺骨架屏 | 加 antd `Skeleton` |
| 3 | Minor | 首屏体积 1.6MB | Vite chunk 超 1500KB 警告 | `React.lazy` 路由懒加载 |

## 📋 待办项（按优先级排序）

### P1 — 下一轮优先

| # | 任务 | 工作量 | 涉及文件 |
|---|------|--------|---------|
| 1 | X-Request-ID 转发给上游 | 小 | `gateway/gateway.go` |
| 2 | Token 估算精度提升 (tiktoken-go) | 中等 | `gateway/gateway.go` |
| 3 | 上游健康检查定时任务 | 小 | 新 `runtime/health.go` 或 `slb/slb.go` |
| 4 | 前端路由懒加载 | 小 | `frontend/src/App.tsx` + 各 page |

### P2 — 后续迭代

| # | 任务 | 工作量 | 描述 |
|---|------|--------|------|
| 5 | WebSocket 实时审计推送 | 中等 | 替代 ConfigStatus/Status 页面轮询 |
| 6 | Prometheus Metrics 出口 | 中等 | 暴露 `/metrics` 端点供 Grafana |
| 7 | 示例 .env + Docker Compose PG 模板 | 小 | 开箱即用多 DB 部署 |
| 8 | 前端 Chart 升级 echarts-for-react | 中等 | 替代手动 SVG |

### Minor — 体验优化

| # | 任务 | 描述 |
|---|------|------|
| 9 | 前端 Skeleton 骨架屏 | Table 组件加 `Skeleton` |
| 10 | `ListenPortNote` 前端消费 | 读 API 返回值替代硬编码 |

### DevOps

| # | 任务 | 描述 |
|---|------|------|
| 11 | GitHub Actions CI/CD | 自动构建+测试+推送镜像 |
| 12 | docker-compose with postgres | PG 部署模板 |
| 13 | .env.template | 环境变量文档化 |

## 🔑 关键技术决策记录

1. **配置存储**: 全部业务配置走数据库，不读磁盘 YAML/JSON (PRD 6.2)
2. **热加载机制**: Manager.AtomicPointer[Snapshot] 无锁读 + CAS 换快照；DB counter 多实例感知
3. **护栏设计**: CheckInput 逐消息检测+就地 PII mask；CheckOutput 二次审核；stream 按 threshold 间隔检测
4. **协议转换**: canonical intermediate representation，各 adapter 最小映射；tools/callbacks 暂不实现
5. **密码哈希**: bcrypt cost=10；恢复码 sha256
6. **API Key**: 明文内存复用，DB 存 AES-GCM 密文；支持模型限定+IP 白名单
7. **SLB 策略**: 平滑加权轮询(SWRR)，确定性分发，保留熔断跳过+故障转移排除
8. **审计开关**: 总开关+采样率+仅错误模式；操作审计不可关
9. **前端状态**: Zustand(轻量) + TanStack Query(服务端缓存)
10. **测试策略**: 同目录 `_test.go`，标准 `testing` + 表驱动 + 基准；Snapshot 手工构建不依赖 DB（quota 例外用内存 SQLite）

---

**下一步行动**:
1. ⏳ P1 #1: X-Request-ID 转发（最小工作量，建议先做）
2. ⏳ P1 #3: 上游健康检查定时任务（提升 SLB 可用性）
3. ⏳ P1 #4: 前端路由懒加载（减小首屏体积）
4. ⏳ P1 #2: Token 估算精度提升（tiktoken-go 集成）
5. 后续按 P2/Minor/DevOps 推进
