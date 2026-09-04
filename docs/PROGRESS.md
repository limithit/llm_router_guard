# AI 网关与模型护栏系统 — 项目进度记录

最后更新：2026-09-03（第八轮：前端 i18n 中英文切换，纯前端改造）

##  本轮迭代变更（第八轮）

### 新增：前端 i18n（中文 / English 切换）
- **范围**：纯前端。后端 envelope 文案（Go `message` 字段）保持中文不动，用户数据/代码注释不翻译。
- **依赖**：`i18next@^26.4.1` + `react-i18next@^17.0.13`。
- **架构**（`frontend/src/i18n/`）：
  - `index.ts`：初始化 + `import.meta.glob('./locales/*/pages/*.ts', { eager: true })` 按语言目录整体挂载；`setLanguage()` 写 localStorage（`llm-router-lang`）→ `i18n.changeLanguage`；`languageChanged` 钩子同步 dayjs locale + `document.documentElement.lang` + `document.title`。
  - 初始语言：localStorage → `navigator.language` 前缀 `zh` → 默认 zh-CN。
  - 语言文件分三类：`common.ts`（通用动词/状态/单位/校验，956 个 key 的公共部分）、`menu.ts` + `dicts.ts`（菜单名 + 14 个字典命名空间的全部枚举值标签）、`pages/*.ts`（每页一个扁平 `{ 'prefix.key': 'value' }` 文件，zh/en 各一套，key 严格对齐）。
- **字典体系重构**（`constants/dicts.ts`）：
  - 旧 API（`XXX_MAP`/`XXX_OPTIONS`/`labelOf`/`colorOf` 静态中文）→ 新 API：`XXX_META`（value+color）+ `useDictOptions(ns, meta)`（返回本地化 Select options）+ `useDictLabel(ns)`（返回 `(v) => string`，无翻译时回退原值）+ `colorOf/dictColor(metas, value)`。
  - 14 个字典命名空间：protocol / keywordCategory / matchMode / action / quotaType / period / overAction / callStatus / logLevel / violationStrategy / alertChannel / backoff / piiCategory / opAction / opModule，key 模式 `dicts.<ns>.<value>`。
- **antd/dayjs 联动**：`App.tsx` 中 `ConfigProvider locale` 随 `i18n.language` 切换 zhCN/enUS；时间格式化（`utils/format.ts` 的 fmtTime/fmtDuration/fmtNumber）随语言切换；axios 拦截器兜底文案走 `i18n.t`。
- **语言切换入口**：`components/LanguageSwitch.tsx`（图标+当前语言下拉），放在 MainLayout 顶栏；登录页（MainLayout 外）固定在页面右上角。
- **页面覆盖**（22 个页面全部转换，组件内中文字面量清零，仅剩注释与示例数据）：
  - Login / NotFound / Dashboard / Help / TokenStats / Keywords / OutputFilter / PiiRules / InjectionRules / Providers / Models / Failover / Quotas / RateLimits / QuotaAlerts / Calls / Operations / General / ApiKeys / Security / ConfigStatus / RuntimeStatus / Backup / AccountSecurity
- **校验**：
  - `tsc --noEmit` 零错误；`vite build` 成功（dist 1,771 KB / gzip 551 KB，含 i18next 运行时 + 双语词表）。
  - 词表一致性脚本核对：zh/en 各 956 key，双向零缺失、零重复。
- **文档**：`docs/i18n-frontend.md` 使用规范（命名空间、旧→新字典 API 映射、新页面接入步骤）。

### 踩坑记录
- **`showTotal: (t) => ...` 参数名遮蔽**：antd Table 的 showTotal 回调参数与 `useTranslation()` 的 `t` 同名，多个页面（原代码）用 `(t)` 作参数；本次统一改为 `(total)`，否则翻译函数被覆盖。
- **i18next 插值 + JSX 片段**：帮助页等长文档段用 `t('key', { c1: <Text code>...</Text> })` 传 ReactNode 插值，避免把整段 HTML 塞进 JSON 词表。
- **子代理不可用**：本轮所有子代理（subagent）均陷入重复读文件的死循环、零文件修改，全部工作改为主线程直接执行。

##  本轮迭代变更（第七轮）

### 修复：Token 用量统计在 SQLite 下"成功日志存在但不计数"
- **现象**：调用审计里有成功日志（含 token 值），但 Token 用量看板统计不到刚产生的调用；后端默认时间范围（近 30 天）能看到数据，前端选任何时间范围（今天/近 7 天/自定义，均为 `dayjs().toISOString()` 的 UTC `Z` 串）就查不到今天的记录。
- **根因**（SQLite 文本时间比较 + 时区偏移）：
  - SQLite 的 `created_at`（`datetime` 声明 → NUMERIC 亲和）实际以**文本**存储，内容为**服务器本地墙钟**（如 `2026-09-03 14:39:47.791308+08:00`，空格分隔）；范围过滤按**字典序**比较。
  - 查询参数 `time.Time` 经 `driver.Valuer` 同样序列化为空格分隔文本，但**偏移量跟随该时间的 Location**：前端 UTC `Z` 参数绑定成 `+00:00` 文本，墙钟部分与库存值相差 8 小时。
  - 结果：`created_at <= end` 把**最近 8 小时**的记录全部排除（`created_at >= start` 同时会多纳入前 8 小时）——用户"刚刚"的成功调用恰好落在被排除窗口内，看起来就是"日志有记录但 token 没统计"。
  - 用真实 DB + 真实服务复现：同一 24h 窗口，后端本地参数 29 条全中，前端 UTC 参数只剩 4 条（今天的 25 条全被排除）。
- **修复**：
  - `admin/audit.go` `parseTimeQ`：解析结果统一归一化到服务器本地时区——RFC3339（带时区，前端 `Z`）→ `t.In(time.Local)`；不带时区的格式 → `time.ParseInLocation(..., time.Local)` 按本地墙钟解释。一次修复 4 个调用点（调用审计列表/操作审计列表/Token 统计/CSV 导出过滤）。
  - `admin/tokenstats.go` `dateBucketExpr`：SQLite 分桶加 `'localtime'` 修饰符，按本地日历日/小时分桶（此前 `strftime` 默认输出 UTC 桶，与 MySQL `DATE_FORMAT`/PG `to_char` 的会话本地时区行为不一致）。
  - MySQL/Postgres 为真实时间类型，驱动做正确的瞬间比较，不受此 bug 影响；归一化对两者无副作用。
- **测试**：
  - 新增 `TestTokenStats_FrontendUTCTimeRange`（模拟前端 UTC `Z` 参数：窗口内记录必计、窗口外/未来记录必排除）——已验证**旧实现下该测试失败**（只统计到 25h 前那条，复现用户症状）。
  - 新增 `TestParseTimeQ_LocalNormalization`（带时区瞬间不变/不带时区按本地/日期=本地午夜/空与非法输入）。
- **端到端验证**（真实服务 + mock 上游）：建 provider→模型别名→API Key，经网关发非流式（上游 usage 42/17）+ 流式（末块 usage 11/5）各一次，前端 UTC 参数下 token-stats 增量 **+2 calls / +75 tokens**，与预期完全一致；两条调用日志均 `status=ok` 且 token 值正确。
- **踩坑记录**：
  - **SQLite datetime 列是文本比较，时区偏移 = 窗口平移**：库存文本带本地偏移、参数文本带 UTC 偏移时，字典序比较让时间窗整体偏移一个时区差（+08:00 部署为 8 小时）。排查时先用 `hex(substr(created_at,1,20))` 看真实存储字节，再对同瞬间不同偏移的参数做 `<=` 探针矩阵，才能定位到"同偏移可比、跨偏移错乱"。
  - **GORM 读 datetime 列 Scan 到 string 会渲染成 RFC3339（T 格式）**，与真实存储字节（空格格式）不同——看 `typeof()` + `hex()` 才不被误导。
  - **复制 SQLite 库必须连 `-wal`/`-shm` 一起**，否则未 checkpoint 的数据全丢（本次 E2E 首次复现只拷了主文件，29 行只剩 15 行）。

### 残留限制（记录在案）
- SQLite 路径的时间比较依赖"库存文本与参数文本同偏移"。当前全部写入来自 `time.Now()`（服务器本地），修复后参数也归一化到本地，一致。若**部署机时区变更**（或 DST 时区），历史行与新参数偏移不一致会出现类似偏移，需要一次性把存量 `created_at` 文本重写到新时区（或升级为 UTC 规范存储 + 迁移）。MySQL/Postgres 无此问题。

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

### 新增：Token 估算精度提升（tiktoken-go）
- **新包**：`internal/tokens/tokens.go`
  - 使用 `tiktoken-go`（cl100k_base 编码器）精确分词，替代原来的 `runeCount/2+1` 启发式估算
  - 启动时后台预热（`main.go` 中 `go tokens.Init()`），避免首个请求延迟
  - 编码器不可用（离线 + 无缓存）时自动降级为 CJK/ASCII 混合启发式估算
  - 离线部署可设置 `TIKTOKEN_CACHE_DIR` 指向预下载的编码文件目录
- **影响**：`gateway/estimateUsage` 改用 `tokens.Count`，上游未返回 usage 的流式/非流式请求都能更精确估算 token
- **测试**：`tokens_test.go`（5 用例）+ `gateway_test.go`（2 用例），全绿

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
  | `internal/admin` | 15 | providers_test.go (111行) + tokenstats_test.go (426行) | 供应商测试连接 + 模型列表解析 + Token 用量统计（聚合/过滤/趋势/CSV + 前端 UTC 时间范围回归 + parseTimeQ 时区归一化） |
  | `internal/gateway` | 5 | gateway_test.go (180行) | 流式输出护栏：短输出兜底检测 / 阈值检测 / 正常输出不误拦截 + estimateUsage 回退与保留 |
  | `internal/tokens` | 5 | tokens_test.go (70行) | Count 精确编码 / Estimate 启发式回退 / 空串边界 |
  | **合计** | **109** | **7 文件** | — |

### Git 状态
- 当前分支：`dev`
- 最新提交：`8e9abef feat(frontend): add zh-CN/en-US i18n switching (frontend-only)`（第八轮）
- 工作区：干净

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

backend/internal/*/  *_test.go           # 7 测试文件 (2,329 行, 109 断言)

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
| 2 | P3 | `audit/audit.go:writerLoop()` | DB 慢时 buffer 满阻塞 handler | 已有 `default: db.Create()` 降级，合理 |
| 3 | Minor | `settings/general` | `listen_port_note` 字段前端未消费 | 前端读 API 返回值替代硬编码 |
| 4 | P2 | SQLite 时间过滤（第七轮残留） | `created_at` 文本比较依赖库存/参数同偏移；**部署机换时区或 DST** 时存量行需一次性重写 | 长期：UTC 规范存储 + 数据迁移；短期：部署文档注明 |

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
| 2 | ~~Token 估算精度提升 (tiktoken-go)~~ ✅ | — | **已完成**（2026-09-02） |
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
11. **安全加固（本轮）**:
    - **可信代理**: `engine.SetTrustedProxies(cfg.TrustedProxies)`，默认空 → `c.ClientIP()` 取 TCP 对端，防伪造 `X-Forwarded-For` 绕过 API Key 的 IP 白名单；反代部署用 `TRUSTED_PROXIES` 指定代理 CIDR。
    - **安全响应头**: `X-Content-Type-Options: nosniff` / `X-Frame-Options: DENY` / `Referrer-Policy`，抑制 MIME 嗅探与点击劫持。
    - **默认密钥告警**: 启动时若 `JWT_SECRET`/`MASTER_KEY` 仍为默认值打印 `[security] WARNING`（不阻断启动）。
    - **上游拓扑不外泄**: 面向客户端的错误信息不再包含上游供应商名（审计 `UpstreamProvider` 列 + 服务端 `[upstream]` 日志仍保留，供管理员排查）。
    - **JWT**: HS256 且 `Parse` 强制校验 `SigningMethodHMAC`（拒绝 `alg=none` / 算法混淆），`exp` 经 `tok.Valid` 校验。

---

**下一步行动**:
1. ⏳ P1 #1: X-Request-ID 转发（最小工作量，建议先做）
2. ⏳ P1 #3: 上游健康检查定时任务（提升 SLB 可用性）
3. ⏳ P1 #4: 前端路由懒加载（减小首屏体积）
4. ✅ P1 #2: Token 估算精度提升（tiktoken-go 集成）— 已完成（第六轮）
5. ⏳ 提交第七轮变更并（可选）为 Token 统计时区修复补一个 `X-Request-ID` 转发之外的回归冒烟
5. 后续按 P2/Minor/DevOps 推进
