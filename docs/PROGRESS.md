# AI 网关与模型护栏系统 — 项目进度记录

最后更新：2026-09-10（第二十轮：openai_chat 工具调用全链路透传——agent 场景"截断"的真正根因）

## 本轮迭代变更（第二十轮）

### 网关：openai_chat tools 全透传（请求定义 + 流式/非流式工具调用响应）
- **根因（真实库 call_logs id=77 铁证）**：用户 agent（DeepSeek Harness 走网关）请求携带 `tools`，
  网关转换层将其丢弃 → 上游模型不知道工具存在，把工具调用当**纯文本**输出
  `<tool_call>glob(pattern: "**/quickstart.md")</think>` 后 16 token 即停（finish=stop）。
  agent 收不到结构化 tool_calls → turn 直接中断，表象仍是"截断"。此前思维链透传（第十七轮）与
  别名默认 max_tokens（第十九轮）解决的是另两层，这是第三层。
- **请求侧**：`CanonicalRequest` 增加 `Tools`/`ToolChoice`（json.RawMessage 原样透传）；
  `Message` 增加 `tool_call_id`/`name`/`tool_calls`（工具结果消息与 assistant 历史工具调用，
  agent 多轮循环必需）；buildOpenAIRequest 在 openai_chat 上游注入以上字段（护栏输入检测/PII
  脱敏仍作用于消息文本，不受影响）。
- **响应侧**：流式 `delta.tool_calls` 分片逐块原样转发（SSEWriter.ToolCalls，仅 openai_chat 客户端，
  anthropic/responses 客户端暂静默跳过）；`finish_reason=tool_calls` 透传；非流式
  `message.tool_calls` 解析并输出。流式侧网关按 index 合并 id/name/arguments 分片，结束后以
  `<tool_call>{"name":...,"arguments":...}</tool_call>` 并入审计文本（护栏与 output_text 可见，
  usage 估算包含工具参数）。
- **实测（本地真实库 + sensenova glm-5.2）**：带 glob 工具的请求 → 7 个 tool_calls 分片正确转发、
  finish=tool_calls；带工具结果的第二跳（tool_call_id 关联）上游正常接受，模型基于结果作答
  finish=stop——agent 完整循环打通。

## 本轮迭代变更（第十九轮）

### 网关：模型别名支持默认 max_tokens（请求未带时自动注入）
- **背景（真实库调用日志定位）**：用户 agent 工作负载输出停在 `</think>` 后（completion_tokens=85），
  大量请求被厂商 429（tpm/rpm）。实测发现 sensenova glm 客户端**不带 max_tokens 时厂商默认补全上限
  仅 ~1000 token**（强制数 1→500 时 completion_tokens=999 截停且谎报 finish_reason=stop），思维链 +
  agent 正文根本走不完 → "截断"。
- **实现**：`model_aliases` 新增 `default_max_tokens`（0=不注入，AutoMigrate 自动加列）；
  `Snapshot.AliasSettings` 别名级覆盖；网关在别名解析后、构造上游请求前：`cr.MaxTokens==0` 且别名配置
  >0 则注入。上游请求体显式携带注入值，行为等同客户端自己带了该参数。
- **管理面**：别名创建/更新 API 与列表响应带 `default_max_tokens`；前端模型别名表单新增
  "默认 max_tokens" 输入（tooltip 说明推理模型建议 4096~131072）。
- **实测（本地真实库 + sensenova glm-5.2）**：别名设 131072 后，不带 max_tokens 的"数到 1500"请求
  completion_tokens=3500、finish=stop、1→1500 全部到达（未注入时必在 ~999 截停）。
- 观测铺垫（第十八轮补丁，随本轮一并入库）：`call_logs.finish_reason` 列 + `[stream] ... done:
  finish_reason=... content_bytes=... deltas_logged=...` 日志行。

## 本轮迭代变更（第十八轮）

### 文档：quickstart.md 生产启动方式（三平台）与 max_tokens 语义
- **新增「各 OS 生产启动方式」节**：Linux systemd（unit 文件全文 + 硬化项 ProtectSystem/ReadWritePaths +
  journalctl 看日志）、Windows 计划任务（Register-ScheduledTask 开机自启 + 崩溃重启；生产建议 NSSM 注册
  真服务）、macOS launchd（plist 全文 + load/unload）。环境变量统一用 EnvironmentFile / AppEnvironmentExtra /
  plist dict 三种给法；另附裸进程 nohup/Start-Process 调试法。
- **max_tokens 语义澄清**：网关**不校验不钳制、客户端设多大透传多大**（唯一例外 anthropic 上游
  `maxInt(max_tokens, 4096)` 兜底）；超厂商区间会原样回其 4xx（实测 sensenova glm-5.2 报
  `should be in [1, 131072]`，即 1M 这类值由厂商拒绝、调到区间内即可）。故障速查表补两行：
  厂商 MaxTokens 区间 400、推理模型空回答+finish=length（思维链烧预算）。
- 文档同步 vite dev 端口 5173→5174（与已入库的 vite.config.ts 一致）。

## 本轮迭代变更（第十七轮）

### 根因确诊与修复：推理模型思维链（reasoning_content）被网关整段丢弃
- **用户实测环境**（本地 SQLite、无 mock）：3 个 provider 同厂商 `token.sensenova.cn/v1`、
  不同 API key，别名 `glm5.2` 挂 3 上游轮询，模型 `glm-5.2`。
- **复现与隔离**（全部在用户真实库 + 真实厂商上完成）：
  1. 网关实测：`max_tokens=300` 流式请求，多数响应只有 Open 事件 + `finish_reason:"stop"` +
     `completion_tokens:0`（看似"一小段就断"），偶发完整回答——轮询放大了间歇感。
  2. 用一次性 Go 工具（AES-GCM 解密 provider key，**不打印明文**）**绕过网关直连厂商**：
     3 把 key 全部 `http=200`、17-20 个分块**全是 `delta.reasoning_content`**、
     `delta.content` 恒空、`finish_reason:"length"`——铁证：厂商行为如此，与 key/轮询无关。
- **根因**：`glm-5.2` 是推理模型，思维链走 `delta.reasoning_content`（GLM/DeepSeek 风格，
  非 OpenAI 标准）；`ParseUpstreamData` 只取 `delta.content` → 思维链整段丢弃；
  max_tokens 被思维链烧完时（`finish_reason=length`）正文一个字都没生成 → 客户端看到
  "只出一小段就结束"。仅当模型思维链较短、正文真的生成出来时才"正常"——解释了间歇性。
- **修复**（`adapter/sse.go`、`adapter/adapter.go`、`gateway/gateway.go`）：
  - `UpstreamChunk` 增 `ReasoningDelta`、`FinishReason`；解析 `delta.reasoning_content`。
  - `SSEWriter.Reasoning()`：流式思维链增量按客户端协议下发
    （openai_chat → `delta.reasoning_content`；anthropic → `thinking_delta` 事件；responses 暂跳过）。
  - `Finalize(usage, finish)`：转发上游真实 `finish_reason`（anthropic 映射 stop→end_turn、
    length→max_tokens）——此前**硬编码 stop**，length 截断被谎报为正常结束。
  - 非流式：`CanonicalResponse.Reasoning` ← `message.reasoning_content`，
    `BuildCompletionJSON` 透传（openai_chat 加 `reasoning_content` 字段；anthropic 前置 thinking 块）。
  - 流结束观测日志：`[stream] upstream closed before finish signal` / `read error=...` /
    `upstream error event`——三类截断从此在日志可区分（此前上游提前断开被静默当成功）。
- **实测验证**：修复后 `max_tokens=300` → 48 个思维链增量到达客户端；
  `max_tokens=1000` → 149 思维链 + 2 正文增量、`finish=stop` 正常收尾。
  全量单测通过（gateway/adapter 等全绿）。
- **语义边界**：思维链**不进**输出护栏、不计入审计正文（`acc`/`ap.output` 仍只含 content），
  usage 按上游报数透传。
- **运维注**：本机 8080 无服务、`bin/` 不存在——诊断用 `go build -o backend\bin\server.exe`
  从 `backend/` 起服（sqlite 相对路径 `gateway.db` 即真实库）；
  一次性解密工具已删不入库；临时 API key `diag-temp`/别名 `diag-p1..3` 留在用户库里待清理。

## 本轮迭代变更（第十六轮）

### 修复：流式请求不再受 `default_timeout_seconds` 总超时约束（用户报"流式只出一小段就中断"）
- **排查结论**：先按"三上游轮询截断"复现——3 个 mock（各 20 分块×0.1s）+ 单别名 3 上游轮询，
  抓原始响应体：**10/20 分块全到位 + `[DONE]`**，网关按上游到达顺序原样转发，无丢失。
  结论：流式转发核心（SSE 逐行解析、`finish_reason:null` 判定、每块即 Flush、
  `choices:[]` usage 块的 `len>0` 守卫）本身正确，E2E `gateway.stream` 亦通过。
  → 截断不是转发核心 bug。
- **定位到的真实缺陷**：`forward()` 对**流式与非流式共用**一个
  `context.WithTimeout(c.Request.Context(), default_timeout_seconds)`，该 ctx 挂在上游请求上、
  同样约束 `resp.Body` 的读取。长流一旦超过该值 → `scanner.Err()` 非 nil →
  `"upstream stream interrupted"` → `frFatal` → 中途截断。
  配置里 `WriteTimeout=0` 明写"SSE 长连接不设总写超时"，但**上游读取侧却仍吃这个总超时**，前后矛盾。
- **修复** `internal/gateway/gateway.go forward()`：`cr.Stream` 为真时直接用
  `c.Request.Context()`（仅受客户端断开约束），非流式仍保留 `default_timeout_seconds`
  兜底慢上游。
- **验证（反证式设计）**：把 `default_timeout_seconds` 设为 **2s**，mock 造 **~5s** 慢流
  （10 块×0.5s）。修复前会在 2s 截断（约 3-4 块），修复后实测
  `chunks=10 done=True unparseable=0`，access 日志 `POST /v1/chat/completions 5.02s`；
  同一 2s 超时下非流式仍正常（mock 快回）。全量 20 步 E2E **20/20 通过**。
- **给用户的补充判据**（若截断依然存在，按此排查）：① 输出护栏中途 stop——
  `runStreamGuardCheck` 在策略为 `block` 时发 error 事件、`replace`（默认）时追加安全文案后收尾，
  都是"前一小段 + 突然结束"的典型形态，且 `guard_enabled` 默认 true；② 上游自身提前关连接
  （EOF 非错误，流会以已有内容正常收尾，日志无 error 行）；③ 客户端主动断开（ctx 取消，同上）。

### 测试机（192.168.20.132）运维踩坑归档——本轮反复踩，务必记住
- **18080 已被同机 `dlp-backend` 占用**：我们的服务起来后 `address already in use` 即退出，
  但 18080 上的 `/healthz` 仍返回 `{"status":"ok"}`（是 dlp-backend 的），极易误判。
  → 本轮改用 **18091**；`cleanup.sh` 不检查该端口，**不可**去杀 dlp-backend。
- **`/tmp` 不可写**：`curl -o /tmp/x.json` 与 shell `> /tmp/x` 都不产出文件。
  → 临时文件一律落在 `/root/gateway-test/`（RUNDIR）。
- **`pkill -f mockup[.]py` 会杀掉自己的 ssh 会话**：同一命令行后段只要出现字面量
  `mockup.py`（含 `mockup.py.new`、`mockup.py.new`），正则就匹配上本会话 bash -c 的
  cmdline → 会话被杀、无任何输出、exit 1。→ **pkill 必须单独一个 ssh 调用**。
- **scp 覆盖既有文件报 `Permission denied`**（`lsattr` 无 immutable、目录可写、touch 新文件正常，
  原因未查清）：workaround = scp 到 `<name>.new` 再 `cp <name>.new <name>`。
- **ssh 层剥掉内层双引号**：内层 `grep -E "a|b"` 会变成两个命令；含 `(` `)` 的 `echo` 需先去掉引号。
  → 复杂逻辑一律写成文件再 `bash file.sh`，不要内联。

### 文档：新增 `quickstart.md`（5 分钟从零跑通）
- 8 节：前置条件 / 构建启动（含 CWD 语义：必须从 `backend/` 起，或显式 `FRONTEND_DIST`）/
  首启安全三件事 / 首次转发全流程（供应商→别名→Key→curl→审计闭环）/ Compose /
  换 MySQL·PG（AutoMigrate 与 `deploy/schema/` 基线 SQL 两条路）/ 多节点速览 / 开发模式 /
  故障速查表。
- 写前逐项对照真实代码校验：`/healthz`·`/metrics` 免认证而 `/v1/*` 带 Key、
  API Key 为 `sk-`+32hex、账户安全页在右上角用户菜单（非左下角）、
  调用日志**列表**不支持 request_id 过滤（只能走详情接口 `audit/calls/<id>`）。
- README「快速开始」标题下加了指向 quickstart 的链接。

##  本轮迭代变更（第十五轮）

### 修复：SQLite 时区规范存储（技术债清偿，P2 最后一项）
- **问题实证**：glebarez/go-sqlite 驱动无时区 DSN 开关，`time.Time` 按值自身时区写库
  （如 `2026-09-05T12:00:00+08:00`）；同表混存不同偏移后 TEXT 词法比较
  （BETWEEN/ORDER BY created_at）错序，库文件随实例时区漂移。
- **写侧修复** `internal/db/utcnormalizer.go`：包装 gorm.ConnPool（含事务 ConnPool——
  gorm Begin 整体替换 tx.Statement.ConnPool，两层必须都拦），在 database/sql 边界把
  全部绑定参数的 `time.Time` 归一化为**固定 3 位毫秒宽 UTC 文本**
  （`2006-01-02T15:04:05.000Z07:00`，如 `2026-09-05T04:00:00.000Z`）。
  定宽保证词法序 == 时间序（RFC3339Nano 变宽小数会破坏 BETWEEN）；
  主池/Raw/事务全路径单格式，杜绝驱动 T 形 vs 空格形双格式化分叉。
- **历史数据迁移** `internal/db/sqlite_utc.go`：启动时枚举 sqlite_master + PRAGMA
  table_info（不依赖模型注册表，覆盖历史遗留表），把 TEXT 日期列中"非 Z 结尾"的
  ISO 值经 `strftime('%Y-%m-%dT%H:%M:%f', x) || 'Z'` 改写为 UTC（与新写入同形）；
  julianday 存在性 + 日期前缀防误伤数字/文本列；幂等（仅非 Z 值）。
- **踩坑记录**：gorm `Commit/Rollback` 对 TxCommitter 做 `reflect.Value.IsNil`——
  包装体必须**指针形态**（结构体值会 panic）；gorm `DB()` 优先走 GetDBConnector，
  包装体实现 `GetDBConn()` 避免硬断言。
- **读路径兼容性发现**：驱动会把"长得像时间"的 TEXT 自动转 `time.Time`、gorm 扫进
  string 字段时按 RFC3339Nano 重格式化（`.000` 被裁剪）——文本级断言须走服务端
  LIKE/julianday，不能信读回的字符串形态。
- MySQL/PG 不受影响（各自驱动/DSN 已保证）；`tokenstats.go` 分桶注释同步更新
  （UTC 输入 + 'localtime' 修饰符 → 本地桶，语义不变）。

### 修复：热加载可见性（快照吞掉并发 Bump）——本轮 E2E 排障的真 bug
- **现象**：SQLite E2E 建完 API Key 后 4 秒起网关持续 401，直到下一次配置变更
  （quota 步骤）才恢复；MySQL 偶发。config_versions 表复盘：apikey 的 Bump 后
  **无任何 event/polling 重载**。
- **根因**：`Manager.Reload` 结束时**重新读取** counter 写入 `dbCount`。一次重载在途
  期间又有新的 Bump（counter 推进）→ 在途重载的快照里没有新 Key，但结束时把
  `dbCount` 直接顶到最新值 → 待处理的 trigger 判定"无变化"而跳过补载。
- **修复**：重载**开始前捕获** counter，结束时以起始值回写 dbCount——未覆盖的
  Bump 依然大于 dbCount，pending trigger 必然触发补载。
- **E2E 加固**（deploy/e2e.py）：建 Key 后的盲等 `sleep(4)` 改为轮询
  `/status` 的 `config_status.version` 直到变化（新快照已加载即 Key 必然在内）；
  不用网关探活（探活调用会被审计，污染 audit.calls/token.stats 计数）。

### 新增：SWRR 全局游标（可选 #16，多节点）
- `internal/slb/swrr_remote.go`：Redis Lua 原子执行 SWRR 一步推进
  （`gw:swrr:v1:<alias>` HASH：providerID → currentWeight），多实例轮询序列
  互不重叠、严格按权重比例分流。
- 仅"干净请求"（无 tried 排除）走全局游标且用**健康过滤后的候选集**
  （候选动态的故障转移重试路径走实例本地游标）；Redis 故障自动降级本地游标
  并后台探活恢复（与熔断共享状态同一降级语义）。
- 测试 4 项（Redis 门控，`GW_TEST_REDIS_ADDR` 提供实例或本机自动起
  redis-server，无 Redis 环境 skip）：跨实例严格交替、2:1 权重配比、
  掉线降级、tried 旁路。
- CI：backend job 增加 redis 服务容器（`GW_TEST_REDIS_ADDR` 指向 6379），
  全局游标测试在 CI 也真实执行。

### 实测与回归
- 后端 `go test ./...` **10 包全绿**（新增 db 包 UTC 套件 4 测试 + slb 全局游标 4 测试）；
  gofmt/vet 干净。
- **SQLite E2E 20/20**（全新库 + 修复后二进制）；**MySQL E2E 20/20**（回归确认）。

##  本轮迭代变更（第十四轮）

### 新增：GitHub Actions CI（待办 DevOps #11 ✅）
- **`.github/workflows/ci.yml`** 三段流水线（push/PR 到 main/cluster，并发去重）：
  1. **backend**：`gofmt -l` 检查 → `go vet` → `go test -race -count=1 ./...`（Go 1.25，go.sum 缓存）
  2. **frontend**：`npm ci`（lockfile 缓存）→ `tsc --noEmit + vite build` → 上传 dist artifact
  3. **docker**：依赖前两段通过后 `docker/build-push-action` 构建镜像（不推送，GHA 层缓存）
- **Dockerfile 修复**：`golang:1.24` → `1.25`（低版本镜像遇 go.mod `go 1.25` 会触发运行时工具链下载）；
  过时的 `go:embed` 注释改为运行时磁盘读取描述。
- **本地预演全绿**：`gofmt -l` 干净（修复 prom.go 格式）、`go vet ./...` 通过——推上去即绿。
- 仓库托管在**阿里云 Codeup**：GitHub Actions 文件随镜像/迁移仓库直接生效；Codeup Flow 按
  ci.yml 同序复用三段命令（README 已注明）。

### 新增：表格骨架屏（待办 Minor #9 ✅）
- **新组件** `components/TableSkeleton.tsx`：`<TableSkeleton rows={5}/>` 骨架行指示器 +
  `tableLoading(initial, busy, hasData)` 一行式 helper（antd Table loading prop 三态：
  首查无数据→骨架屏；翻页/刷新已有数据→转圈；操作忙碌→转圈）。
- **接入 12 张表格**：Providers / Keywords / PiiRules / InjectionRules / Quotas / RateLimits /
  Models / ApiKeys / Calls / Operations / Backup / TokenStats(by_key)——mutation 加载态
  保留原转圈语义不回归。
- 前端 `tsc + vite build` 零错误。

### 闭项：ListenPortNote 前端消费（待办 Minor #10，陈旧条目）
- 核实：`General.tsx` 自**首版骨架（0d6cdfc，2026-09-02）**即 `tooltip={data?.listen_port_note || t(...)}`，
  后端第三轮起返回该字段——功能一直完整，待办条目属陈旧登记，无代码改动。

### 修复：CallLog 大文本列方言截断（技术债，对应已知问题 #6）
- `model.CallLog.InputText/OutputText` 去掉 `gorm:"type:text"` 方言标签：
  **mysql→longtext（64KB→4GB）**、pg→text、sqlite→text——GORM 各方言默认全量存储，
  超长 prompt 不再被 MySQL TEXT 64KB 截断（第九轮事故根因的彻底解法）。
- 实测：MySQL 建表 `SHOW COLUMNS` 确认 `input_text/output_text = longtext`；
  全新库 E2E 20/20（含审计全文/详情链路）。
- SQLite 存 UTC 文本问题**不在本轮**（涉及历史数据迁移，保持 P2）。

### 实测与回归
- 后端 `go test ./...` 9 包全绿；前端构建零错误；**MySQL E2E 20/20**（全新库）。
- E2E 备忘：本机 18081 被会话外服务占用（Go 风格 404、无法从会话内终止）——
  `.e2e/e2e.py` 副本支持 `MOCK_PORT` 覆盖（`deploy/e2e.py` 原件未动），mock 改走 18089。
  **库重置（DROP/CREATE）后必须重启网关进程**（admin 引导只在启动时进行）。

##  本轮迭代变更（第十三轮）

### 新增：Prometheus 指标出口（待办 P2 #6 ✅）
- **新文件** `internal/metrics/prom.go`：手写 Prometheus 文本格式 v0.0.4（零第三方依赖，
  Prometheus/VictoriaMetrics 直接可抓）；`GET /metrics`（无 JWT，建议监控网段限制）。
- **指标**：`gw_uptime_seconds` / `gw_active_connections` / `gw_requests_per_second` /
  `gw_model_requests_per_second{model}` / `gw_model_errors_1m{model}` /
  `gw_upstream_healthy{provider,protocol}`（复用 slb.HealthList 语义，含 Redis 共享打开状态）/
  `gw_upstream_consecutive_failures{...}` + `go_*` runtime 指标（runtime/metrics 一次快照）。
- **admin.Server 实现 `PromCollector`** 接口抓取瞬间组装上游健康行；抓取互斥防并发放大。
- **踩坑**：label 值转义不能对已 `promEsc` 的串再用 `%q`（二次转义 `\"`→`\\"`）——改为手工拼引号。
- **单测** `prom_test.go`：转义/格式化/渲染含收集器与无收集器两条路径。

### 新增：WebSocket 实时审计推送（待办 P2 #5 ✅）
- **后端**（零新依赖，手写 RFC6455 服务端推送子集）：
  - `internal/audit/hub.go`：广播中心——每订阅者 64 缓冲、慢消费者丢弃计数（不阻塞写库路径），
    `Write()` 落库同链路 `broadcastLive`（无订阅者零开销：先查订阅数再 JSON 编码）。
  - `internal/admin/wschan.go`：最小 WS 帧读写（服务端不掩码写 text/ping/close；客户端帧解掩码
    读出即弃、只处理控制帧；>1MiB 帧拒绝）。
  - `internal/admin/auditws.go`：`GET /api/admin/v1/audit/ws?token=<JWT>`——浏览器 WS 无法带
    Authorization 头，token 走查询串，**鉴权失败在升级前 401**；30s ping 保活；积压丢弃达 512
    主动 1013 关闭（前端重连即恢复）；每帧 = model.CallLog JSON（与 /audit/calls 字段对齐）。
  - **大坑**：`http.Hijacker` 返回的 `*bufio.ReadWriter` 自带缓冲——再包一层 `bufio.NewWriter`
    后 Flush 只写进 rw 内部缓冲，**101 握手永远到不了客户端**（测试卡死 1 分钟定位）。
    修法：直接写 rw 并 Flush 它。
- **前端**：`hooks/useAuditStream.ts`（断线 3s 自动重连、保留 30 条、reset）+
  `components/LiveAuditStream.tsx`（实时卡片：状态 Tag、暂停=冻结快照+落后角标、清空），
  嵌入 Calls 页顶部；i18n 双语 9 键（zh/en 词表 784/784 对齐校验通过）。
- **单测**：hub 扇出/丢弃/并发安全（-race）+ WS 全链路 4 测试（原生 TCP 握手→推帧→close 回环→ping/pong）。
- **真机探针** `.e2e/ws_probe.py`（Python 原生 socket 手写握手）：登录→建别名/Key→订阅→
  发起网关调用→**断言收到该调用的实时帧**→close 回环，9/9 PASS。

### 新增：前端 Chart 升级 echarts-for-react（待办 P2 #8 ✅）
- `components/Charts.tsx` 重写：`LineChart`/`DonutChart` props 完全兼容（页面零改动），
  `echarts/core` 按需注册（折线/饼图/网格/提示/图例/标题 + CanvasRenderer）。
- **体积**：echarts 落独立 `Charts` chunk **535KB(gzip 181KB)**，仅随 Dashboard/TokenStats
  懒加载页按需拉取；应用入口保持 178KB(gzip 63KB) 不变。
- 增强项：tooltip 轴触发、饼图图例带百分比、滚动图例、空态沿用 antd Empty。

### 新增：.env.template + Compose 升级（待办 #7/#12/#13 ✅）
- 根目录 `.env.template`：全部环境变量文档化（含 REDIS_ADDR/REDIS_PASSWORD/HEALTH_CHECK_SECONDS/
  TRUSTED_PROXIES/TIKTOKEN_CACHE_DIR，逐项默认值与语义注释）；`cp .env.template .env` 即用。
- `docker-compose.yml`：env 直通（REDIS/健康检查/TRUSTED_PROXIES）+ 注释形态的 redis 服务；
  `.gitignore` 增加 `.env`。多节点 PG+Redis 拓扑见 README 第十二轮章节。

### 回归与实测
- 后端 `go test ./...` 9 包全绿（admin 新增 6 测试：prom 2 + hub 4 + WS 4 拆分见 git）。
- 前端 `tsc --noEmit` + `vite build` 零错误；词表 784/784 对齐。
- **MySQL 8.0 本机 E2E 20/20**（重置库后全新链路）；`/metrics` 实抓含上游健康行；
  WS 探针 9/9。

##  本轮迭代变更（第十二轮）

### 新增：前端路由级代码分割（待办 P1 #4 ✅）
- **App.tsx**：22 个业务页面全部 `React.lazy` 懒加载（Login/NotFound 轻量页保持静态引入），
  路由级自动拆 chunk；`App.tsx` 顶层 `Suspense` 全局兜底。
- **MainLayout.tsx**：`<Outlet />` 外再包一层 `Suspense`（Spin 兜底）——懒加载页加载期间
  **仅内容区显示 Spin，侧栏/顶栏保持挂载不闪**（若只有顶层边界，整页布局会被 fallback 替换）。
- **vite.config.ts**：`manualChunks` vendor 分层（react 全家桶 / antd+icons+dayjs / i18next）——
  业务页频繁迭代，vendor 稳定不变，独立 chunk 后发版只使业务 chunk 失效，命中浏览器长缓存。
- **效果**（vite build 实测）：单包 1,771KB(gzip 551KB) → **入口 index 177KB(gzip 63KB)** +
  antd 1,362KB(gzip 424KB，首载后长缓存) + react 65KB + i18n 70KB + 22 个按页 chunk；
  首次访问串行瀑布减半，后续访问 vendor 零流量。
- `tsc --noEmit` 零错误，构建通过；chunk 警告消除（单 chunk 不再超 1500KB 限制）。

### 新增：多节点部署文档（待办 #17 ✅）
- README 新增「**多节点部署（PostgreSQL / MySQL + Redis）**」章节：拓扑图（LB → N 实例 → 共享 PG/MySQL + Redis）、
  LB 探活端点 `GET /healthz`（无鉴权，实测存在）。
- 环境变量表补充 `REDIS_ADDR` / `REDIS_PASSWORD` / `HEALTH_CHECK_SECONDS` 三个变量的精确语义
  （与 `config.go` 默认值逐一核对：健康检查默认 30s、0=禁用；任何 HTTP 响应=可达=清熔断）。
- 跨实例一致性速查：DB 承载（配置/配额/审计/JWT）+ Redis 承载（限流/熔断广播/MFA 票据）+ 实例本地（SWRR 游标）；
  **LB 后必须设 `TRUSTED_PROXIES`**，否则 API Key IP 白名单全匹配到 LB 地址形同虚设；
  多实例 `JWT_SECRET`/`MASTER_KEY` 必须一致（JWT 互认 + API Key 密文可解）。
- 双网关 + PG + Redis 的 Docker Compose 多节点模板（YAML anchor 复用，`:?` 强制注入密钥）。

### 修复：TokenStats 两个时区脆弱测试（新环境实测暴露）
- `TestTokenStats_Aggregation`：`base = now-2d` 直用当前时刻，**21:00 后跑测试时 base+3h 跨过本地午夜**，
  日分桶裂成 2 个 → `trend len=2, want 1` 失败（新机当晚 22:03 复现）。修复：base 固定到
  「2 天前的本地 09:00」，+3h→12:00 永不跨日。
- `TestTokenStats_HistoricalTrend`：`Truncate(24h)` 按 **UTC** 对齐（东八区 UTC+8：UTC 16:00 起=本地次日 0 点起），
  极端时区下两个相邻 UTC 日可能塌进同一本地日 → 3 桶变 2。修复：用本地年月日重建正午 12:00。
- **教训**：跨午夜/跨日分桶测试的相对时间种子必须固定在远离午夜的安全时刻（如 09:00/12:00），
  且禁用 `Truncate(24h)` 做本地日对齐（它按 UTC 截断）。

### 新环境连库实测（Ubuntu 24.04 + MySQL 8.0.46 本机）
- **MySQL E2E 20/20 ✅**（`deploy/e2e.py`，root 账号 auth_socket 插件不可 TCP 密码登录——
  Ubuntu 默认——建专用 `gateway` 账号 `mysql_native_password` 后实测）。
- 非流式 usage 42/17 透传、流式末块 11/5 汇聚、输入护栏 block、输出护栏 replace、
  配额/限流 429、审计落库、Token 统计 134 tokens 精确吻合——全部通过。
- 踩坑：Go 1.24+ 的 `-buildvcs` 在 root 跑 `limit` 用户的 git 仓库时 `dubious ownership` 报错，
  本地编译加 `-buildvcs=false`；VPN（香港节点）下 Go 模块代理用 `proxy.golang.org`，
  `goproxy.cn` 跨境巨慢（golang.org/x/* 每个 10-20s）。

##  本轮迭代变更（第十一轮）

### 新增：X-Request-ID 全链路透传（待办 P1 #1 ✅）
- **入站复用**：客户端带合法 `X-Request-ID`（非空、≤128 字节）则透传复用（跨服务同一 ID 贯穿），
  缺失/超长/空白则生成 32 位 hex（`gateway.resolveRequestID`）。
- **三处落点**：响应头回带同值（客户端凭 ID 回查审计 `call_logs.request_id` 列）；
  转发上游时 `req.Header.Set("X-Request-ID", ...)`（上游日志可关联网关审计）；
  审计落库沿用既有 request_id 字段。
- **实测**：入站 `trace-live-42` → 响应头原样回带 ✅；无头 → 生成新 ID ✅；
  mock 上游收到同值 header ✅（单测 `TestForward_UpstreamReceivesRequestID` + `TestResolveRequestID`）。

### 新增：供应商 protocol 枚举校验（第九轮实测遗留 ✅）
- `createProvider` / `updateProvider` 校验 `protocol ∈ {openai_chat, openai_responses, anthropic}`
  （`adapter` 常量，单一事实源），非法值 400「protocol 必须是 …」。
- 此前错误值（如 `openai`）一路存库，直到转发时才 502 `unsupported upstream protocol`；
  现在配置期即拦截。实测：`{"protocol":"openai"}` → HTTP 400 ✅，更新同理。

### 新增：上游健康检查定时任务（待办 P1 #2/#3 ✅）
- **新包** `internal/health`：`Checker.Run(ctx, interval)` 周期探测全部启用供应商
  （`GET <base>/v1/models`，10s 超时，单轮有界并发 ≤8，启动即先探测一轮）。
- **判据刻意宽松**：任何 HTTP 响应（含 401/403/404）= 端点可达 = `RecordSuccess`（清计数/清共享打开态）；
  仅传输层错误（超时/DNS/连接拒绝）`RecordFailure` 累计——复用既有熔断阈值/重置逻辑，
  **多实例部署自动经 Redis 广播**（第十轮机制，零额外代码）。
- **开关**：`HEALTH_CHECK_SECONDS`（默认 30，0=禁用）。探测不进审计、不占配额。
- **实测**（interval=5s）：死上游 5 轮后 `healthy=False fails=7`（阈值 5 触发）✅；
  好上游全程 `healthy=True` ✅。
- **单测**：可达即清熔断（401 也算健康）、传输失败累计到阈值开断、禁用供应商跳过、interval=0 直接返回。

### 回归
- 单测全绿（新增 8 个断言组）；MySQL + Redis + 健康检查器同开，20 步 E2E **20/20**。

##  本轮迭代变更（第十轮）

### 新增：Redis 分布式限流（待办 #14 ✅）
- **新文件** `internal/quota/redis_limiter.go`：固定窗口键 `gw:rl:v1:<ruleID>:<apiKeyID>:<alias>`，
  Lua 脚本原子 `INCR + 首次 PEXPIRE`（TTL=窗口秒数，只在计数=1 时设置，窗口不漂移），N 实例共享精确计数。
- **接口化**：`quota.Limiter`（Allow/FlushHits）——内存版 `*RateLimiter`（单节点/sqlite）与
  分布式版 `*RedisLimiter`（mysql/postgres + `REDIS_ADDR`）实现同一签名；gateway 通过接口注入，单测零改动。
- **降级**：启动探活失败或运行中 Redis 故障 → 自动降级本地内存限流（单实例语义）+ 告警日志，
  每 5s 探活恢复后自动切回分布式；热路径超时 ≤400ms，Redis 故障不拖死网关。
- **单测**（miniredis）：双实例共享计数交替打满双向 429、窗口过期恢复、Redis 不可用降级本地仍有效。

### 新增：Redis 共享熔断打开状态（待办 #15 ✅）
- **方案**（`internal/slb/slb.go`）：失败计数留实例本地；熔断**打开事件**广播 Redis
  `SETEX gw:cb:v1:<providerID> = openUntil(ms)`，TTL=熔断重置秒数 → 到期键消失即自动半开；
  成功恢复 `DEL` 共享键。判定侧节流读共享状态（≤1 次/秒/供应商，热路径零额外压力）——
  任一实例打开熔断，其余实例 ≤1s 同步跳过该供应商。
- **修复**：`HealthList` 对"本实例无本地 breaker"的供应商直接读共享状态，
  运行状态页能如实反映他实例打开的熔断（此前只查本地 map，跨实例打开状态在状态页不可见）。
- **降级**：同限流，Redis 故障自动退化单实例语义，恢复自动切回。

### 新增：MFA 二步登录票据 / 绑定密钥 Redis 化（矩阵补充项 ✅）
- **新文件** `internal/admin/redis_mfa.go`：`mfaStore` 接口（内存 memMFA / Redis redisMFA）。
  票据键 `gw:mfa:login:v1:<token>`、绑定键 `gw:mfa:setup:v1:<uid>`，TTL 5 分钟，
  Lua `GET+DEL` 原子核销——LB 后任意实例写入、任意实例可完成二步验证。
- `admin.New` 增加 `redisAddr/redisPassword` 参数；Redis 不可达自动回退内存（不阻断启动）。

### 部署开关
- **环境变量**：`REDIS_ADDR`（host:port）+ `REDIS_PASSWORD`（可选）。
- **规则**：sqlite 模式忽略 Redis（单节点零依赖，保持不变）；mysql/postgres + REDIS_ADDR
  → 限流/熔断/MFA 票据全部分布式。`/status` 无新字段，启动日志打印各组件启用状态
  （`[ratelimit]|[slb]|[mfa] redis ... connected: ... ENABLED/cluster-wide`）。
- SWRR 游标：第十五轮 #16 起为**全局游标**（Redis Lua 原子推进 `gw:swrr:v1:<alias>`，多实例分流互不重叠）；掉线自动降级实例本地游标。

### 多节点实测（真实环境，PG + Redis 共享，双实例 18080/18082）
- **分布式限流 ✅**：A 打满 2/10s → 经 B 429、再经 A 仍 429（全局计数）；TTL 过期后恢复 200。
- **熔断跨实例共享 ✅**：A 对坏上游（500×2）打开熔断（失败全部故障转移到好上游，客户端全 200）；
  B 本地 fail_count=0 却在 ≤1s 内看到该上游 unhealthy 并把流量全部路由到好上游；
  8s 重置窗口过后自动半开恢复。
- **MFA 票据跨实例**：实现层完成（Redis GET+DEL 任意实例核销）；未单独 E2E（需 TOTP 真实绑定流程）。
- **单实例回归 ✅**：MySQL + Redis 模式完整 20 步 E2E 20/20。
- **配置热加载跨实例 ✅**（第十轮复测，同第九轮）。
- 测试脚本：`deploy/mn_redis.sh`（限流）、`deploy/mn_circuit.sh`（熔断，good+bad 双 mock）、
  `deploy/multi_node_redis_test.py`、`deploy/multi_node_circuit_test.py`。

### 多节点能力矩阵（第十轮后）

| 状态域 | 是否跨实例一致 | 机制 | 多节点表现 |
|--------|--------------|------|------------|
| 配置热加载 | ✅ 是 | `Bump()` 递增 `config_meta.counter` + 每 `HotReloadSeconds` 轮询 | 一实例改配置，其余 ≤3s 内重载 |
| 配额（quota） | ✅ 是 | `Consume` 走 DB `UPDATE used_value + delta` | 全局精确 |
| 调用审计 | ✅ 是 | 异步批量写 DB | 共享 |
| API Key/供应商/别名/限流规则 | ✅ 是 | 全 DB 存储 + 快照重载 | 共享 |
| 管理后台 JWT | ✅ 是 | 无状态 HS256 | 任一实例可校验 |
| **速率限制** | ✅ 是（第十轮） | Redis 原子 INCR+PEXPIRE（`REDIS_ADDR`）| "100/min" 全局精确；故障降级单实例 |
| **SLB 熔断器** | ✅ 是（第十轮） | 打开事件 SETEX 广播 + TTL 自动半开 | 任一实例打开，全体 ≤1s 跳过 |
| **MFA 二步票据** | ✅ 是（第十轮） | Redis SET/GET+DEL + TTL 5min | LB 任意实例完成二步验证 |
| **SWRR 游标** | ✅ 是（第十五轮） | Redis Lua 原子推进 `gw:swrr:v1:<alias>`，干净请求全局推进；重试路径本地 | 多实例加权分流互不重叠；掉线自动降级实例本地 |

**部署结论（更新）**：
- 单节点：sqlite / pg / mysql 均可，无需 Redis。
- 多节点：pg/mysql + `REDIS_ADDR`（+ `REDIS_PASSWORD`）→ 限流/熔断/MFA/SWRR 游标全部跨实例一致。
- ~~仅剩 SWRR 游标实例本地（轮询分布差异，可选优化 #16）。~~ → 第十五轮 #16 已完成。

### 踩坑记录（第十轮）
- **测试机 Redis 带 `requirepass`**：三组件启动即 `NOAUTH` 降级（降级逻辑按设计工作并如实告警）——
  部署带密码的 Redis 必须配 `REDIS_PASSWORD`。
- **二进制被运行中进程占用（ETXTBSY）**：Linux 上 scp 覆盖运行中的二进制会失败，
  先停进程再上传；`run_server.sh` 的 fuser 释放端口逻辑顺带解决了进程残留。
- **SWRR+故障转移的双上游熔断观测**：坏上游 500 → 网关静默重试好上游（客户端只见 200），
  失败计数照常累计到阈值；观测跨实例共享必须用"读运行状态 API"而非客户端状态码。
- **HealthList 只查本地 map**：跨实例共享状态要在该接口显式回退读 Redis，否则状态页对
  "从未路由过该供应商"的实例永远显示 healthy。

##  本轮迭代变更（第九轮）

### 连库实测（真实 Linux 环境 + 真实 MySQL/PG）
- **环境**：debian13 (192.168.20.132)，MySQL 8.4.11 (3306) / PostgreSQL 17.10 (5432)；
  Windows 交叉编译 `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`，二进制 + mock 上游 + E2E 脚本 scp 部署，全程真实服务（非容器）。
- **E2E 覆盖 20 项**（`deploy/e2e.py`，对运行中服务实测）：
  管理端登录 → 供应商创建+测试连接（真实 HTTP 探测 `/v1/models`）→ 模型别名 → 敏感词护栏 → API Key 签发 →
  网关非流式调用（上游 usage 42/17 透传）→ 流式调用（末块 usage 11/5 汇聚）→ 输入护栏 block（400 content_filtered）→
  输出护栏 replace（短输出兜底检测生效，泄漏词被安全文案替换）→ 调用审计（ok/blocked 落库）→
  Token 用量统计（calls=4, 95+39=134 tokens 与 mock usage 精确吻合）→ 配额超限 429 → 配额删除 →
  限流 2/10s 第三发 429 → 限流删除 → 操作审计 → Dashboard。
- **结果**：MySQL 20/20 ✅、PostgreSQL 20/20 ✅、MySQL 修复后回归 20/20 ✅、SQLite 启动冒烟 ✅。

### 修复：MySQL 保留字 `key` 导致启动失败（P0，阻断 MySQL 部署）
- **现象**：`settings.EnsureDefaults` 报 `Error 1064 (42000): ... near 'key = ?'`，进程启动即退出。
- **根因**：`system_settings` 主键列名 `key` 是 MySQL 保留字；`Where("key = ?")` 是裸 SQL 片段，
  GORM 不加反引号（结构体字段才会按方言转义），SQLite/PG 恰好能容忍所以单库开发发现不了。
- **修复**（`internal/settings/settings.go`）：两处裸片段改为结构体条件
  `Where(model.SystemSetting{Key: ...})`，由 GORM 按方言加引号，三库通吃。
- **教训**：列名撞保留字时，裸 `Where("col = ?")` 在 GORM 里不会转义；要么用结构体条件，要么写 `"\\`key\\` = ?"`。

### 修复：`gorm:"type:longtext"` 导致 PostgreSQL 迁移失败（P0，阻断 PG 部署）
- **现象**：PG 下 AutoMigrate 报 `ERROR: type "longtext" does not exist (SQLSTATE 42704)`，启动即退出。
- **根因**：`model.go` 两处硬编码 MySQL 专属类型（`ConfigVersion.SnapshotJSON` / `BackupRecord.Content`）；
  SQLite 无类型系统所以从没暴露。
- **修复**（`internal/model/model.go`）：删掉 `type:longtext` 标签。无 size 的 string 由 GORM 按方言默认映射：
  mysql→longtext、pg→text、sqlite→text，行为不变且跨库正确。
- **教训**：跨库项目禁用方言专属 gorm type 标签；需要大文本就用无标签 string 或 `type:text`。

### 附带发现（未修，记录在案）
- **供应商 protocol 无后端校验**：创建供应商时 protocol 传任意值（如 `openai`）都接受，
  直到网关转发才报 `unsupported upstream protocol "openai"`（502）。建议在 `createProvider/updateProvider`
  校验枚举 `openai_chat|openai_responses|anthropic`，把错误提前到管理面。

### 多节点双实例实测（真实环境，PG 共库）
- **环境**：同机双实例（18080/18082）共享 gateway_pg + 共享 mock 上游（`deploy/mn_do.sh` + `multi_node_test.py`）。
- **结果 3/3**：
  - **配置热加载跨实例传播 ✅**：实例 A 创建供应商/别名，实例 B 在 ≤4s 的 `config_meta.counter` 轮询内可见；
  - **配额全局精确 ✅**：limit=2，A 消费 1 次 + B 消费 1 次均 200，第 3 次（经 B）429——`used_value` 走 DB 原子累加；
  - **限流确认每实例独立 ✅（文档预期）**：A 打满 2/10s 后，第 3 次经 B 仍 200——证实待办 #14（Redis 全局限流）的必要性；
  - 管理端 JWT 无状态，两实例各自登录可用。
- **结论**：多实例横向部署（pg/mysql + 前置 LB）开箱可用；全局精确限流/熔断仍需 #14/#15。

### 实测工具链沉淀（`deploy/`，可在测试机 `/root/gateway-test/` 复跑）
- `mockup.py`：OpenAI 兼容 mock 上游（/v1/models + 非流式/流式 chat，usage 固定值，可控触发输出违规）。
- `e2e.py`：20 步 E2E（用法 `python3 e2e.py BASE_URL ADMIN_PASSWORD`，审计断言带异步落库轮询）。
- `env.sh` / `reset_db.sh` / `stop_server.sh` / `run_server.sh` / `do_all.sh` / `cleanup.sh`：
  DSN 集中管理、建库、停服（pidfile+fuser 双保险释放端口）、起服+健康等待、一键「停→重置→起→测」、现场清理。
- **Windows 侧踩坑**：`ssh root@host '命令含引号'` 时引号会被 PowerShell→ssh 两层剥掉，
  含 `$`/引号/括号的命令务必写成 `.sh` 文件 scp 上去再 `bash xxx.sh`，不要内联。

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
- ✅ **后端** `go test ./...` — 全绿 9 包；`gofmt -l` 干净；`go vet ./...` 通过（CI 三项本地预演）
- ✅ **前端** `vite build` — 零错误（入口 178KB/gzip 63KB + Charts 535KB + antd 1,363KB + 22 个按页 chunk）

### 测试状态
- ✅ **全后端** `go test ./...` — 全绿
  | 包 | PASS 断言数 | 测试文件 | 覆盖范围 |
  |----|-----------|---------|---------|
  | `internal/guard` | 32 | engine_test.go (450行) | 关键词 contains/exact/regex、PII 检测+脱敏、注入检测、输出过滤 block/replace/log、护栏禁用、Unicode、全链路集成、MaskForLog、FindingsJSON |
  | `internal/slb` | 22 | slb_test.go (502行) | 加权选择(SWRR)、权重分布统计、熔断器开/半开/恢复、故障转移全循环、PickIgnoringCircuit、HealthList、并发安全 |
  | `internal/quota` | 35 | quota_test.go (590行) + redis_limiter_test.go | NextReset 日/周(周一)/月/跨年、matchQuotas 通配、限流窗口+重置、配额 Check/Consume/惰性重置、FlushHits、多规则并发 + **Redis 分布式限流（miniredis）：双实例共享计数双向 429、窗口过期恢复、Redis 故障降级本地** |
  | `internal/admin` | 15 | providers_test.go (111行) + tokenstats_test.go (426行) | 供应商测试连接 + 模型列表解析 + Token 用量统计（聚合/过滤/趋势/CSV + 前端 UTC 时间范围回归 + parseTimeQ 时区归一化） |
  | `internal/gateway` | 5 | gateway_test.go (180行) | 流式输出护栏：短输出兜底检测 / 阈值检测 / 正常输出不误拦截 + estimateUsage 回退与保留 |
  | `internal/tokens` | 5 | tokens_test.go (70行) | Count 精确编码 / Estimate 启发式回退 / 空串边界 |
  | **合计** | **~112** | **8 文件** | — |

### 连库/多节点实测状态（第九~十二轮）
- ✅ MySQL 8.4.11 / PostgreSQL 17.10：20 步 E2E 各 **20/20**（第九轮 `deploy/e2e.py`，debian13 真实环境）
- ✅ MySQL 8.0.46（Ubuntu 24.04 本机）：20 步 E2E **20/20**（第十二轮复测，新环境从零搭建）
- ✅ 双实例 + Redis：分布式限流全局 429、熔断跨实例 ≤1s 同步 + 半开恢复（第十轮 `deploy/mn_redis.sh` / `mn_circuit.sh`）
- ✅ MySQL + Redis 单实例回归 20/20（第十轮）
- ✅ **基线 schema.sql 往返验证**（第十五轮补遗，提交 `a5cbcb5`）：
  `deploy/schema/{schema.mysql.sql,schema.postgres.sql}` 从当前 HEAD 的 AutoMigrate 产物导出
  （mysqldump --no-data / pg_dump --schema-only，16 表含 CallLog longtext 修正、PG 版剥离 psql17
  `\restrict` 保护行）；`deploy/verify_schema.sh` 回灌全新库 → 服务启动（AutoMigrate 幂等）
  → 20 步 E2E **双库 20/20** → 重新 dump 与提交文件**逐行一致**
  （`MYSQL_SCHEMA_ROUNDTRIP_IDENTICAL` / `PG_SCHEMA_ROUNDTRIP_IDENTICAL`）。
  用途：DBA 预审 / 手工建库 / 只读账号部署；自动建表仍是默认路径。用法见 `deploy/schema/README.md`。

### Git 状态
- 当前分支：`master`（origin/master 已同步至 `647dc63`；历史分支 `cluster`/`dev`/`secure` 保留）
- 最新功能提交：`a5cbcb5` feat(db): baseline schema.sql for MySQL and PostgreSQL, round-trip verified
- 第十五轮 4 笔：`0ecf45c` fix(db) UTC、`0725403` fix(runtime) 热加载、`b343ddd` feat(slb) 全局游标、`dfa6a60` docs
- 最新文档补遗：`a5cbcb5` schema 基线（MySQL+PG，往返一致已验证）
- 第十五轮涉及：`backend/internal/db/{db.go,utcnormalizer.go,sqlite_utc.go,sqlite_time_test.go}`、
  `backend/internal/runtime/manager.go`、`backend/internal/slb/{slb.go,swrr_remote.go,swrr_remote_test.go}`、
  `backend/internal/admin/tokenstats.go`（注释）、`deploy/e2e.py`、`.github/workflows/ci.yml`、
  `README.md`、`docs/PROGRESS.md`

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
| 1 | ~~P1~~ ✅ | `gateway/gateway.go` | ~~request_id 未作为 `X-Request-ID` header 传给上游~~ | **已修**（2026-09-04 第十一轮：入站复用 + 响应回带 + 转发上游） |
| 2 | P3 | `audit/audit.go:writerLoop()` | DB 慢时 buffer 满阻塞 handler | 已有 `default: db.Create()` 降级，合理 |
| 3 | ~~Minor~~ ✅ | `settings/general` | ~~`listen_port_note` 字段前端未消费~~ | **闭项**（2026-09-05 第十四轮核实：General.tsx 首版即消费 `data?.listen_port_note`，条目陈旧登记） |
| 4 | ~~P2~~ ✅ | SQLite 时间过滤（第七轮残留） | ~~`created_at` 文本比较依赖库存/参数同偏移；换时区/DST 时存量行错序~~ | **已修**（2026-09-05 第十五轮：UTC 规范存储 + 启动时历史数据一次性迁移，见第十五轮记录） |
| 5 | ~~P2~~ ✅ | `admin/providers.go` | ~~供应商 `protocol` 创建/更新时无枚举校验~~ | **已修**（2026-09-04 第十一轮：create/update 校验 adapter 枚举常量，配置期 400） |
| 6 | ~~P3~~ ✅ | `model/model.go` | ~~`CallLog.InputText/OutputText` 用 `type:text`，MySQL 下上限 64KB~~ | **已修**（2026-09-05 第十四轮：去方言标签 → mysql=longtext / pg=text，实测 `SHOW COLUMNS` 确认） |

### 前端

| # | 优先级 | 位置 | 问题 | 建议 |
|---|--------|------|------|------|
| 1 | ~~P2~~ ✅ | `components/Charts.tsx` | ~~手动 SVG 绘制，样式简单~~ | **已完成**（2026-09-05 第十三轮：echarts-for-react，props 兼容） |
| 2 | ~~Minor~~ ✅ | 多个 Table 组件 | ~~仅有 `loading` prop，缺骨架屏~~ | **已完成**（2026-09-05 第十四轮：TableSkeleton + tableLoading 接入 12 表） |
| 3 | ~~Minor~~ ✅ | 首屏体积 1.6MB | ~~Vite chunk 超 1500KB 警告~~ | **已完成**（2026-09-04 第十二轮：React.lazy 22 页 + vendor 分包，入口 gzip 63KB） |

## 📋 待办项（按优先级排序）

### 当前待办速览（第十五轮后）

- **P1/P2/Minor/DevOps/多节点**：**全部完成**（#4~#13 + #11 CI + #16 全局游标）
- **技术债**：全部清偿（CallLog longtext ✅、SQLite UTC ✅）；审计 writerLoop（P3，已有降级，可接受，仅余优化空间）

### P1 — 下一轮优先

| # | 任务 | 工作量 | 涉及文件 |
|---|------|--------|---------|
| 1 | ~~X-Request-ID 转发给上游~~ ✅ | — | **已完成**（2026-09-04 第十一轮：入站复用 + 响应回带 + 转发上游） |
| 2 | ~~Token 估算精度提升 (tiktoken-go)~~ ✅ | — | **已完成**（2026-09-02） |
| 3 | ~~上游健康检查定时任务~~ ✅ | — | **已完成**（2026-09-04 第十一轮：`internal/health` + `HEALTH_CHECK_SECONDS`，联动熔断/Redis 广播） |
| 4 | ~~前端路由懒加载~~ ✅ | — | **已完成**（2026-09-04 第十二轮：React.lazy + 双层 Suspense + vendor manualChunks，入口 177KB/gzip 63KB） |

### P2 — 后续迭代（全部完成）

| # | 任务 | 状态 |
|---|------|------|
| 5 | WebSocket 实时审计推送 | ✅ 2026-09-05 第十三轮：零依赖 RFC6455 + hub 广播 + 前端实时卡片 |
| 6 | Prometheus Metrics 出口 | ✅ 2026-09-05 第十三轮：`/metrics` 文本 v0.0.4，零依赖 |
| 7 | 示例 .env + Docker Compose PG 模板 | ✅ .env.template + compose env 直通（PG/Redis 拓扑见 README 多节点章节） |
| 8 | 前端 Chart 升级 echarts-for-react | ✅ 2026-09-05 第十三轮：props 兼容重写，echarts/core 按需注册 |

### Minor — 体验优化

| # | 任务 | 描述 |
|---|------|------|
| 9 | ~~前端 Skeleton 骨架屏~~ ✅ | **已完成**（2026-09-05 第十四轮：TableSkeleton + tableLoading 接入 12 张表格） |
| 10 | ~~ListenPortNote 前端消费~~ ✅ | **闭项**（2026-09-05 第十四轮核实：General.tsx 首版即消费 API 值 `data?.listen_port_note || t(...)`，条目陈旧） |

### DevOps

| # | 任务 | 描述 |
|---|------|------|
| 11 | ~~GitHub Actions CI/CD~~ ✅ | **已完成**（2026-09-05 第十四轮：`.github/workflows/ci.yml` gofmt/vet/test-race → tsc+vite build → docker build；Codeup Flow 同序复用） |
| 12 | ~~docker-compose with postgres (+Redis)~~ ✅ | PG+Redis 拓扑：README 多节点章节双网关模板（第十二轮）+ compose env 直通（第十三轮） |
| 13 | ~~.env.template~~ ✅ | **已完成**（2026-09-05 第十三轮：全部变量文档化，cp 即用） |

### 多节点（多实例横向扩展）

| # | 任务 | 描述 |
|---|------|------|
| 14 | ~~Redis 全局速率限制~~ ✅ | **已完成**（2026-09-04 第十轮：固定窗口 Lua 原子计数 + 故障降级，双实例实测全局 429） |
| 15 | ~~Redis 全局熔断状态~~ ✅ | **已完成**（2026-09-04 第十轮：打开事件 SETEX 广播 + TTL 半开，双实例实测 ≤1s 同步） |
| 16 | ~~SWRR 全局游标（可选）~~ ✅ | **已完成**（2026-09-05 第十五轮：Redis Lua 原子推进 `gw:swrr:v1:<alias>`，多实例分流互不重叠；干净请求走全局、重试路径走本地；掉线自动降级；CI 加 redis 服务容器） |
| 17 | ~~多节点部署文档~~ ✅ | **已完成**（2026-09-04 第十二轮：README 多节点章节——拓扑图 /healthz 探活、REDIS_ADDR/REDIS_PASSWORD/HEALTH_CHECK_SECONDS 语义、TRUSTED_PROXIES 警示、一致性速查、Compose 双网关模板） |

## 🌐 多节点能力矩阵（第十轮后）

> SQLite = 单节点（文件锁、`MaxOpenConns=1`）；以下针对 **Postgres/MySQL** 多实例。
> 最新矩阵见本文档顶部「本轮迭代变更（第十轮）」，此处为历史记录存档。

| 状态域 | 是否跨实例一致 | 机制 | 多节点表现 |
|--------|--------------|------|------------|
| 配置热加载 | ✅ 是 | `Bump()` 递增 `config_meta.counter` + 每 `HotReloadSeconds` 轮询 | 一实例改配置，其余 ≤3s 内重载 |
| 配额（quota） | ✅ 是 | `Consume` 走 DB `UPDATE used_value + delta` | 全局精确 |
| 调用审计 | ✅ 是 | 异步批量写 DB | 共享 |
| API Key/供应商/别名/限流规则 | ✅ 是 | 全 DB 存储 + 快照重载 | 共享 |
| 管理后台 JWT | ✅ 是 | 无状态 HS256 | 任一实例可校验 |
| **速率限制** | ✅ 是（第十轮） | Redis 原子 INCR+PEXPIRE（`REDIS_ADDR`，故障自动降级本地） | "100/min" 全局精确（双实例实测） |
| **SLB 熔断器** | ✅ 是（第十轮） | 打开事件 Redis SETEX 广播 + TTL 自动半开（故障降级） | 任一实例打开，全体 ≤1s 跳过（双实例实测） |
| **MFA 二步登录票据** | ✅ 是（第十轮） | Redis SET/GET+DEL + TTL 5min（不可达回退内存） | LB 任意实例完成二步验证 |
| **SWRR 游标** | ✅ 是（第十五轮） | Redis Lua 原子推进 `gw:swrr:v1:<alias>`（干净请求全局推进；重试路径本地；故障降级） | 多实例加权分流互不重叠 |

**部署结论**：
- 单节点：sqlite / pg / mysql 均可，无需 Redis。
- 多节点：pg/mysql 多实例 + 前置 LB + `REDIS_ADDR`（+`REDIS_PASSWORD`）→ 限流/熔断/MFA/SWRR 游标全部跨实例一致
  （第十轮双实例实测：分布式限流 429 全局生效、熔断跨实例 ≤1s 同步、半开自动恢复；第十五轮起 SWRR 游标全局化）。

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
    - **Help 页安全说明**: 新增「6. 安全说明」卡片（中英双语）。修复 i18n 用 `t()` 插值 React 元素导致 `[object Object]` 的 bug（deploymentBody/addrBody/aliasBody/deployBody 等 5 处改用 `<Trans components={{...}}>` + 翻译串 `<cN>...</cN>` 标签）。

---

**下一步行动**:
1. ✅ 第十三轮：P2 全清——/metrics、WS 实时审计（零依赖 RFC6455）、echarts、.env 模板
2. ✅ 第十四轮：CI（GitHub Actions）+ 表格骨架屏 ×12 + CallLog longtext（已知问题 #6 闭项）+ #10 陈旧条目闭项
3. ✅ 第十五轮：SQLite UTC 规范存储（技术债最后一项清偿）+ 热加载可见性修复（E2E 排障发现的真 bug）+ #16 SWRR 全局游标
4. 功能性待办已全部完成——后续按需求/issue 驱动
5. 维护性关注点：审计 writerLoop 优化（P3，可选）、CI 镜像发布（可选 GHCR/Codeup 制品库）
