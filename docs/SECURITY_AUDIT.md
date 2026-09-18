# LLM Router Guard 安全审计报告

> ⚠️ 本报告为内部审计存档（中文）。

| 项 | 值 |
|---|---|
| 审计日期 | 2026-09-11 |
| 代码版本 | `master @ 835aacd` |
| 审计范围 | `backend/` 全部 Go 源码（Gin+GORM 网关 + 管理 API）、`frontend/src` 关键路径、`deploy/` + `docker-compose.yml` + `.env.template` |
| 方法 | 8 个攻击面域独立审计（认证会话 / 授权与危险操作 / 网关转发与 SSRF / 注入 / 密码学与密钥 / DoS 与资源 / 信息泄露 / 前端与传输），两轮执行；全部 critical/high 发现由**对抗性复核代理**独立读码 refute-or-confirm（默认倾向驳回，误报已剔除） |
| 证据存档 | 见 §七 修复状态矩阵（逐项处置摘要 + 批次提交引用） |
| 修复状态 | **已于 `security` 分支全量处置**（批1 `f951d76` / 批2 `997b8b7` / 批3 `70b8a23` / 批4 `b46eff7`）；逐项状态见 **§七 修复状态矩阵**（含残余风险说明） |

**裁定后统计**：1 严重（critical）、9 高危（high）、约 34 中危（medium）、约 12 低危（low）。

**严重度定义**：critical = 未授权即可 RCE/越权/拖库；high = 需低权限但后果重大、或账户接管类缺陷；medium = 有真实影响的纵深防御缺口；low = 卫生项。

---

## 一、严重（CRITICAL）

### SEC-01 默认安装的 JWT 签名密钥公开可知，且启动告警因大小写不匹配**永远不会触发**

- **位置**：`docker-compose.yml:15-16`、`backend/cmd/server/main.go`（`warnInsecureDefaults`）、`backend/internal/config/config.go:63-64`
- **证据**：compose 默认 `JWT_SECRET=${JWT_SECRET:-CHANGE-ME-JWT-SECRET}`（全大写），而告警函数只比对代码默认值 `"change-me-jwt-secret"`（全小写）→ 永不匹配、永不告警。MASTER_KEY 同理（`CHANGE-ME-MASTER-KEY` vs `llm-router-guard-master-key`）。
- **影响**：任何用仓库内 compose 开箱部署的实例，都运行在**仓库公开的 JWT 签名密钥**上且零告警。攻击者 clone 仓库即知密钥 → 本地伪造 `auth.Issue` 管理令牌 → 完整接管管理控制台（无需任何口令/MFA），进而签发网关 Key、读取全部调用审计原文、改配置、SSRF（见 SEC-03/11）。这是事实上的未认证管理面接管。
- **修复建议**：① 统一两处默认值字面量；② 密钥改为**前缀/哨兵检测**（任何以 `CHANGE-ME`/`change-me` 开头一律拒绝启动，或拒绝签发管理令牌直到配置真实密钥）；③ compose 对 `JWT_SECRET`/`MASTER_KEY` 用 `${JWT_SECRET:?must-set}` 强制语法，取消可用默认值。

---

## 二、高危（HIGH）

### SEC-02 世界知名的默认口令 admin/admin123 贯穿所有首启路径，且密码生命周期断裂

- **位置**：`backend/cmd/server/main.go:170-173`（`pass = "admin123"`）、`docker-compose.yml:18`、`.env.template:28`
- **证据链**：① 代码在 `ADMIN_PASSWORD` 为空时静默用 `admin123`（仅日志告警）；② compose 默认 `ADMIN_PASSWORD=${ADMIN_PASSWORD:-admin123}` 使其**非空** → 连那条告警都不打，容器默认 admin/admin123 静默上线；③ `bootstrapAdmin` 仅在 0 用户时执行 → **首启后再设 ADMIN_PASSWORD 被永久忽略**（改环境无济于事但无提示）；④ compose 未持久化 DB 文件时（`DB_DSN=gateway.db` 相对路径、卷只挂 `/app/data`），容器重建 → 库回空 → 密码**回退为 admin123**；⑤ 无强制改密机制（`must_change_password` 类字段不存在）。
- **影响**：互联网扫描器对公开端口做 `admin/admin123` 一次即接管（`POST /api/admin/v1/auth/login`）。
- **修复建议**：未设 `ADMIN_PASSWORD` 时**随机生成并打印一次性口令**（或直接拒绝启动）；compose 用 `:?` 强制；DB 落卷；首登强制改密 + 改密后置位。

### SEC-03 备份恢复/一键回滚**不可逆地销毁全部供应商 API Key**，却向操作者报告成功

- **位置**：`backend/internal/runtime/bundle.go:81-91`、`backend/internal/model/model.go:107`（`APIKeyEnc ... json:"-"`）
- **证据**：所有快照/备份序列化 provider 时因 `json:"-"` 丢失密文；`RestoreBundle` 先 `tx.Where("1=1").Delete(...)` 物理删除 providers 表再重建（密文为空），解密失败路径 `manager.go:217-218` 仅置 `p.APIKey=""` **不计入 loadErrs** → 状态显示 ok，UI 弹「恢复成功」。上游调用发空凭证全部失败，且**任何快照/备份文件里都没有密钥可恢复**（同一序列化路径丢失），运维只能逐个重录供应商 Key。
- **影响**：两个官方安全特性（REQ-019 备份恢复、REQ-004A 一键回滚）实为凭证湮灭开关；`configStatus` 帮助文案还引导用户在加载出错时点回滚 → 自伤型全站中断。
- **修复建议**：恢复时按 ID/Name 将现存 `APIKeyEnc` 合并回写（空密文不覆盖非空）；或 bundle 内用显式字段随库保存密文（仅排除出 `GET /config/export` 响应）；解密失败必须计入加载错误并在响应中返回受损 provider 数。

### SEC-04 客户端 X-Request-ID 原样写入 UNIQUE 审计列：钉住一个值即可**系统性摧毁全部租户的调用审计**

- **位置**：`backend/internal/gateway/gateway.go:84-85`（header 透传仅限 128 字节）、`backend/internal/model/model.go:248`（`uniqueIndex;size:40`）、`backend/internal/audit/audit.go:67-70`（批量插入失败仅记日志后整批丢弃）
- **证据**：任何合法网关 Key 的请求（含 400/404/429/被拦截——全部发生在限流入队之后的 defer）都带着客户端指定的 ID 入审计队列；同 ID 第二次落库触发 UNIQUE 冲突，GORM `CreateInBatches(batch,50)` 外层包事务 → **一次冲突回滚整个 flush（最多 64 行，含其他租户的日志）**，且无 OnConflict/重试/行拆分。
- **影响**：REQ-015 审计完整性可被最低权限租户以每秒几次请求远程摧毁；同时隐藏其自身行为。
- **修复建议**：DB 主键用服务端生成 ID，客户端 header 另存非唯一列；或批量插入 `ON CONFLICT DO NOTHING` + 失败拆行重试；顺带把 header 校验到 `size:40` 的字符集。

### SEC-05 metrics 按未解析的客户端 model 串无界增长：永久性内存 DoS（且发生在限流之前）

- **位置**：`backend/internal/gateway/gateway.go:314-319`（defer `s.mx.Observe(ap.alias, ...)` 先于模型解析注册）、`gateway.go:334`（`ap.alias = cr.Model` 原样客户端串）、`backend/internal/metrics/metrics.go:43,56-71`（`stats map[string]*ModelStat` 无上限、无淘汰、无 TTL；每条目 ~1KB）
- **证据**：未知模型 404 返回前 Observe 已用原始串登记；404 路径不经过限流/配额（都在解析成功后）→ 唯一成本是一次快速 404；model 字符串无长度校验（16MiB 体内可放巨型名），单请求可永久驻留 ~16MiB map key + 无限多个小 key。`/metrics` 还无鉴权，任何人可反复支付 O(N) 渲染成本并持有与网关请求路径同一把锁（dos-2）。
- **影响**：普通租户 Key 循环发随机模型名 → 堆单调增长至 OOM，网关+管理台+SPA 同进程一起死；仅重启可清。
- **修复建议**：Observe 只对**解析成功**的别名计数，失败统一记 `__unknown__`；解析时限制 model 长度（如 128）；给 stats 加 LRU/TTL 上限；/metrics 加鉴权或只绑内网（见 M-11）。

### SEC-06 配置的入站超时从未应用到 http.Server：零超时监听器（slowloris 友好）

- **位置**：`backend/cmd/server/main.go:125`（`srv := &http.Server{Addr: ..., Handler: engine}`）、`config.go:72`（`ReadTimeout: 90s` 加载后无人使用）
- **证据**：无 `ReadHeaderTimeout/ReadTimeout/IdleTimeout/MaxHeaderBytes`；`cfg.ReadTimeout` 是死配置。**未认证**的 `POST /auth/login`（routes.go:42）请求体读取无任何上限（无 MaxBytesReader、无读超时）→ 匿名攻击者可慢速吊住连接/灌巨型 body 耗尽内存与连接。
- **修复建议**：`srv` 显式设置 `ReadHeaderTimeout=10s`、`IdleTimeout`、`MaxHeaderBytes`，登录/管理路由外套 `http.MaxBytesReader`；流式路径保留 WriteTimeout=0 设计。

### SEC-07 配额 check→转发→consume：并发超额可达 MaxConnections×无界 token，且失败/中断流完全不计

- **位置**：`backend/internal/quota/quota.go:148`（Check）与 `gateway.go:768`（成功后 Consume）
- **证据**：检查与扣减隔着整个上游往返；N 个并发请求全部通过旧值检查，全部成功后再各自累加 → tokens 配额可被瞬时突破 N×；而失败/被拦截/客户端断开的流不产生 Consume → 上游 token 实际已消耗（供应商侧计费），网关侧配额却零记账（计量缺口反向可被利用于烧供应商账单不撞自己配额）。
- **修复建议**：预扣（check 时 `used_value += est`，完成后按真实 usage 冲正），或至少 requests 类改原子 `UPDATE ... WHERE used < limit`；对失败流按已收 usage 记账。

### SEC-08 failed_logins 非原子读改写 → 并发登录可击穿 5 次锁定，锁定策略对撞库失效

- **位置**：`backend/internal/admin/auth.go:114-121`（`u.FailedLogins++` 后整值写回）
- **证据**：并发请求各自基于快照值 +1 写回，计数丢失更新；配合登录接口无 per-IP 限速，攻击者可维持高并发尝试而账户几乎不锁。
- **修复建议**：`UpdateColumn("failed_logins", gorm.Expr("failed_logins + 1"))` 原子自增并回读判断；叠加 per-IP 登录限速（同属 SEC-06/M-03 修复面）。

### SEC-09 「全员必须 MFA」是纯前端摆设：服务端照常发全权令牌

- **位置**：`backend/internal/admin/auth.go:124-138`（`issueLogin` 先 `auth.Issue` 再算 `need_bind_mfa` 布尔）
- **证据**：`mfa_required_for_all=true` 时未绑定用户仍拿到 24h 全权 JWT，响应里只有建议位；服务端无任何路由以其为门槛 → 只防君子。
- **修复建议**：未绑定 MFA 的用户只签发受限/引导态票据（仅可访问 `/account/mfa/*`、`/auth/me`），AuthMiddleware 按用户状态+令牌 scope 拦截其余路由。

### SEC-10 输入护栏/PII 脱敏/调用审计对 tool 调用字段全盲：改放 `tool_calls.arguments` 即可无痕投递违禁内容

- **位置**：`backend/internal/gateway/gateway.go:381-385`（仅 `CheckInput(m.Content)`）、`adapter.go:429-441`（`InputPlainText()` 不含工具字段）、`adapter.go:522-529,534-541,638-641`（`ToolCalls[].Name/Arguments`、`tools[]` 定义原样序列化转上游）
- **证据**：违规文本放 `messages[].tool_calls[].function.arguments`、工具名、`tool_call_id` 或 `tools[].function.parameters` 即绕过 block/warn/mask 全套控制抵达上游；`call_logs.input_text` 同样不含 → 审计“看不到”实际发出的内容。缓冲路径组装的工具文本也未过 CheckOutput（流式路径已过）。
- **影响评注**：需合法 Key、且文本层本来就有大小写/Unicode 等绕过面（护栏是启发式）；但它是产品核心承诺（REQ-008~010）的**结构性**绕过+审计失明，复核裁定维持 high。
- **修复建议**：对「将转发内容」做序列化投影统一过检（Content + ToolCalls 各字段 + Tools 定义），命中处替换/阻断对应子字段；`InputPlainText()` 同步纳入；缓冲路径对工具文本补 CheckOutput。

---

## 三、中危（MEDIUM）

| # | 标题 | 位置 | 要点 / 修复方向 |
|---|---|---|---|
| M-01 | 上游出口零管控（SSRF + x-api-key 重定向外泄） | `gateway.go:58-61`、`adapter.go:679`、`health.go:37`、`providers.go:184,232` | base_url 不校验 scheme/内网段；无 CheckRedirect——Go 跨主机重定向只剥 `Authorization` **不剥 `x-api-key`** → anthropic 供应商 Key 可被恶意/被劫上游经 307 骗走；错误回显把 ≤200B 内网响应体带给租户。修：共享加固 client（禁重定向 + Dial 层拒私网/按 operator 白名单 + https-only + ResponseHeaderTimeout） |
| M-02 | JWT 无任何吊销：登出/改密/解绑 MFA 后 24h 令牌照用 | `auth.go:152-155`、`jwt.go:21` | 登出是 no-op；改密不作废旧会话。修：jti+拒绝表（内存/sqlite）或改密后置用户版本号入 claims 校验 |
| M-03 | 匿名锁定 DoS：5 次错密锁 admin 15 分钟且无 per-IP 限速 | `auth.go:38-40,114-121` | 攻击者可反复把唯一管理员锁在门外（锁到期时 failed_logins 亦不清零，锁连环）；登录接口本身不限速（还兼 bcrypt CPU 放大器）。修：per-IP 限速 + 锁定改验证码/只增时不硬锁 + 到期重置计数 |
| M-04 | 管理操作审计 IP 无条件信任 X-Forwarded-For | `server.go:108-109` | 绕过 TRUSTED_PROXIES=空 的既定姿势，任何管理写操作可伪造来源 IP 入 OperationLog。修：统一走 c.ClientIP() 且仅反代后启用 |
| M-05 | CSV 导出公式注入 | `admin/audit.go:102-110`、`tokenstats.go`、`guard.go` | 租户可控的 model/X-Request-ID 未转义写入 CSV，管理员 Excel 打开触发 `=cmd` 执行。修：`=+-@` 前缀加 `'` |
| M-06 | MASTER_KEY 派生仅单轮无盐 SHA-256 | `crypto.go:21-31` | 弱口令环境密钥 → 拿到 DB/导出即离线爆破解全包。修：HKDF 或 scrypt/argon2id 拉伸 + 启动强度检查 |
| M-07 | TOTP 密钥明文入库；恢复码非原子消费可双花；90s 窗口无重放抑制 | `model.go:17`、`mfa.go:35,61` | 备份/库读 = 静默 MFA 伪造。修：mfa_secret AES-GCM 加密；恢复码条件 UPDATE；记录已用 step |
| M-08 | 内存版 MFA 票据/待绑定密钥无过期（文档称 5 分钟） | `redis_mfa.go:21,37` | 预认证票据永存直至重启，可被捡回继续二步。修：带 TTL 的惰性过期 |
| M-09 | changePassword 忽略 bcrypt 错误；>72 字节密码致 hash 空串 | `auth.go:180-181` | 边界条件下把账户改成空口令哈希。修：处理 err + 限长 72 |
| M-10 | 用户名枚举预言机（锁定分支先于口令校验）+ 前端把 403 锁定渲染成「网络错误」 | `auth.go:38`、`Login.tsx` | 响应/时序差异可枚举管理账户；UI 又泄漏锁定态。修：口令校验先行、统一错误 |
| M-11 | /metrics 完全未认证 | `routes.go:21-24` | 匿名读取模型清单、供应商/熔断状态、流量与 token 体量（商业情报+侦察）。修：Bearer/内网绑定二选一 |
| M-12 | 模型名预言机：404（不在目录）vs 403（不在 Key 白名单）语义差 | `gateway.go:356-362` | 任意 Key 可枚举全平台别名+隐式上游目录。修：对外统一 404 文案 |
| M-13 | 拦截原因回显命中关键词与规则名 | `gateway.go:404` | 黑名单枚举器，辅助绕过。修：只回“被策略拦截” |
| M-14 | 审计 WS：JWT 走 URL query + 握手不校验 Origin | `auditws.go:31`、`useAuditStream.ts:65` | token 进反代访问日志/历史；CSWSH 面（缓解=仍须带合法 token）。修：Sec-WebSocket-Protocol 携票或首帧认证 + Origin 白名单 |
| M-15 | 无 CSP（仅 3 个安全响应头） | `main.go:96-102` | 任何 XSS 即 localStorage JWT 失守；React 转义是独苗防线。修：CSP + 收敛 script-src |
| M-16 | 全链路明文 HTTP，仓库无 TLS 终结示例 | `main.go:126`、deploy/ | 网关 sk-、管理 JWT 裸奔。修：文档/示例补反代 TLS+HSTS，默认拒绝非本机 admin？至少醒目提示 |
| M-17 | 16MiB 体读+JSON 解码+护栏全扫发生在限流**之前** | `gateway.go:322,381` | 高并发大包可让成本先于任何闸门落地。修：连接级准入提前 |
| M-18 | 审计队列满 → 请求路径同步写 SQLite | `audit.go:41-45` | 压力下正反馈延迟放大（且写失败被忽略=静默丢审计）。修：降级采样/异步兜底 |
| M-19 | 每请求配额 Consume 同步 DB 写 + 冗余 settings 查询 | `quota.go:183-205` | sqlite 单写者下的热点争用。修：合并批量落账 |
| M-20 | MaxConnections 全局计数 TOCTOU、无 per-key 并发上限 | `gateway.go:195` | 单 Key 可吃光全局并发拖死他人。修：原子化 + 每 Key 上限 |
| M-21 | slb 全局互斥锁横跨阻塞 Redis 往返 | `slb.go:246` | Redis 抖动 → 全请求路径串行等待。修：分段锁/异步广播 |
| M-22 | 每次 reload 全量再导出并写 ConfigVersion 大 JSON | `manager.go:365-391,384` | Bump 风暴（批量导入/频繁改动）放大为 DB 写风暴。修：去抖 + 增量/上限 |
| M-23 | 流式护栏每阈值全文重扫：O(L²)×规则数×每规则 ToLower | `gateway.go:699-708`、`engine.go:150` | 一条长流（或配合 M-25 巨型规则集）即可拉高 CPU。修：增量扫描（只扫新增+回溯窗口），规则侧缓存 lower 形 |
| M-24 | importProviderModels 无校验无上限吞外部模型清单 | `providers.go:327` | 被劫上游可向目录注入海量垃圾（喂 SEC-05/磁盘）。修：限数限长字符集 |
| M-25 | 敏感词批量导入无行/字节上限，batch 端点绕过正则校验 | `admin/guard.go:116` | AC 自动机/快照体积炸弹。修：导入上限 + 统一校验 |
| M-26 | 护栏热路径每关键词整文 ToLower | `engine.go:204` | 常数放大（16MiB 级文本×K 词）。修：单次 lower 复用 |
| M-27 | KV 设置数值零校验（如 recovery_code_count 巨大值） | `admin/settings.go:115` | 一次 MFA 绑定即可造内存/时长异常。修：按结构体键位定界 |
| M-28 | PII 掩码替换串直接当 regexp 扩展模板 | `engine.go:99` | 配置里的 `$` 展开成非预期文本。修：QuoteReplacement 或转义 |
| M-29 | 访问日志可被百分号编码换行伪造日志行 | `main.go:105-117` | path 未转义 %s 直出。修：strconv.Quote 或去换行 |
| M-30 | allowed_models 空/坏 JSON **fail-open** | `model.go:58`、`gateway.go` 检查点 | 存储侧无校验，坏记录=放行全部。修：fail-closed + 写入校验 |
| M-31 | 配额降级路由不复检 Key 白名单与降级别名限流 | `gateway.go:422` | 降级可把流量塞进本不允许的模型/绕过其限流。修：degrade 前同规则链复跑 |
| M-32 | restore 接受任意自造 bundle 一把改全配置（关护栏/MFA/审计、清空规则表），OperationLog 前后值皆空、target 记成 `backup#0` | `backup.go:59-86`、`bundle.go:136` | 复核降级 medium（单角色本就全权 + ConfigVersion 可回滚），但取证链失真+无确认是真实缺口。修：KV 键白名单、行级校验复用 CRUD、恢复前强制快照 + 带摘要的审计条目 |
| M-33 | deploy/ 跟踪脚本内置示例口令（DB root 等） | `secrets-6: deploy/env.sh:3` | 与 SEC-02 同族。修：全部改 `:?` 或随机 |

---

## 四、低危（LOW，卫生项）

| # | 标题 | 位置 |
|---|---|---|
| L-01 | /v1 超 16MiB 请求体被静默截断按 400 而非 413，语义误导 | `gateway.go:322` |
| L-02 | IP 白名单只收 CIDR，裸 IP 写进去永不匹配（fail-closed 锁死自己） | `model.go:86` 解析处 |
| L-03 | 单角色设计：任何管理员可解他人 MFA/解锁/枚举用户名，无二次确认 | `users.go:48` |
| L-04 | 高危操作 recordOp 普遍缺 before/after 快照（改密钥、删规则等） | `backup.go:85` 等 |
| L-05 | 列表接口 page/size 无上限、LIKE 通配符未转义（探测/慢查询） | `server.go:81` |
| L-06 | token 计数用 dlclark/regexp2（回溯引擎），是全仓唯一非 RE2 正则面（输入=请求文本） | `tokens/tokens.go:46` |
| L-07 | 上游错误体未净化的 vendor 认证失败细节回显给客户端/入审计 | `gateway.go:542` |
| L-08 | MaskKey 露前4后2（约 12bit），短 Key 场景碰撞 | `crypto.go:60-65` |
| L-09 | 客户端 X-Request-ID/模型名原样进 SSE 帧并转发上游（日志伪造面，JSON 已转义无注入） | `sse.go:249` |
| L-10 | 管理端多处 raw err.Error() 直出（SQL/驱动文本进 toast） | `admin/models.go:112` |
| L-11 | 限流/配额错误暴露内部规则 id、阈值、账户配额体量 | `gateway.go:412` |
| L-12 | 前端把 40301 锁定当网络错误展示（文案缺陷，与 M-10 同修） | `Login.tsx` |

---

## 五、修复优先级建议

1. **开源前必须（本周）**：SEC-01（告警失配+公开密钥默认值）、SEC-02（默认口令生命周期）、SEC-03（恢复毁密钥——功能性数据丢失，即使不算安全也是发布阻塞）、M-05（CSV 公式注入，小改动）。
2. **第一迭代**：SEC-04（审计钉 ID）、SEC-05（metrics 无界，和 dos-2/披露同修）、SEC-06（服务器零超时）、SEC-08（锁定原子化+登录限速）、SEC-09（MFA 服务端强制）、M-30/M-31（allowlist fail-open/降级复检）。
3. **第二迭代**：SEC-10（护栏覆盖工具字段）、M-01（共享加固 HTTP client：禁重定向+出口策略，一次修掉 health/testProvider/gateway 三处）、M-02/M-07/M-08（会话吊销 + MFA 密钥加密 + TTL）、SEC-07（配额预扣）。
4. **持续加固**：M-11~M-16（/metrics 鉴权、预言机文案、WS token 传递、CSP、TLS 部署文档）、DoS 组（M-17~M-23）、审计组（M-04/M-29、L-04）。

## 六、审计说明与限制

- 两轮独立审计（第一轮 87 条/14 复核，第二轮 81 条/19 落盘复核）；同一缺陷被不同域重复上报的条目已在注册表合并。冲突裁定以**落盘的对抗复核 reason** 为准（例：SEC-10 首轮有复核员主张 medium，终版按复现链完整性维持 high）。
- 未覆盖：第三方依赖 CVE 扫描（govulncheck/npm audit）、渗透式运行时测试、多节点 postgres/redis 路径实战、前端全量页面逐一审查。建议发布前补 `govulncheck ./...` 与 `npm audit` 存档。
- 本报告为静态读码结论；对上游/供应商生态的信任假设（“恶意/被劫供应商”类）均按产品文档的部署姿势评估。

---

## 七、修复状态矩阵（`security` 分支，2026-09）

图例：✅ 已修复 ｜ ◐ 部分修复（附边界） ｜ ○ 记录为残余风险（附理由）。
批1 `f951d76`（认证会话面）· 批2 `997b8b7`（网关/审计完整性面）· 批3 `70b8a23`（备份/写侧校验/MFA 加密）· 批4 `b46eff7`（M/L 清理 + 运维卫生 + TLS 文档）· 批5（矩阵评审中揪出的两处残余：锁定链复位、登录体上限 + 本矩阵定稿）。

| # | 状态 | 处置摘要（批次） |
|---|---|---|
| SEC-01 | ✅ | ①（批1）compose `${JWT_SECRET:?}`/`${MASTER_KEY:?}` 强制必填；`enforceSecretPolicy` 哨兵前缀检测（`change-me`/`changeme`/已知默认值→拒绝启动，`ALLOW_INSECURE_DEFAULTS=1` 显式豁免）；大小写不敏感比较，消除"告警永不触发" |
| SEC-02 | ✅ | ①（批1）`ADMIN_PASSWORD` 留空→首启 `crypto.RandomHex(12)` 一次性口令打印日志 + `must_change_password` 强制改密（改密前仅 `scope=pw` 受限会话）；已有用户时设置它给出明确"仅首启生效"日志；compose DB 落卷防口令回退；`.env.template` 同步 |
| SEC-03 | ✅ | ③（批3）`BundleProvider` 显式序列化 `api_key_enc`（密文形态）；恢复时缺失密文按名称保留库内现钥、两边皆无→拒绝恢复（不再静默建空钥供应商）；运行时解密失败→告警日志 + 载入错误状态页可见 + 该供应商从选路剔除 |
| SEC-04 | ✅ | ②（批2）主 ID 恒为服务端 32-hex；客户端 `X-Request-ID` 清洗（去控制符/截 128B）后存新列 `client_request_id`（非唯一）；审计批插失败→逐条重试仅丢坏行并限量留痕 |
| SEC-05 | ✅ | ②（批2）`Observe` 标签清洗+截断；基数上限 512、溢出归并 `*` 桶；QPS 采样空闲驱逐（>10min 零流量出表）；模型名解析长度界 |
| SEC-06 | ✅ | ①⑤（批1 `http.Server{ReadHeaderTimeout:10s, ReadTimeout:90s, IdleTimeout:120s, MaxHeaderBytes:1MiB}`，`cfg.ReadTimeout` 死配置接通；批5 登录路由 `MaxBytesReader` 1MiB——匿名面唯一无界 body 关死。SSE 不设 WriteTimeout 属设计内） |
| SEC-07 | ✅ | ②（批2）配额改 预扣(reserve)→结算(settle)/退还(release)：条件原子 `UPDATE ... SET used=used+? WHERE used+?<=limit`（周期惰性重置）；失败/中断/被拦截流按已产生量结算或退还；上游 4xx 保留请求计数、退还 token 预扣 |
| SEC-08 | ✅ | ①（批1）`failed_logins` 原子自增（`gorm.Expr`）+ 条件锁定；成功登录（全链认证后）才清零；per-IP 登录限速叠加（M-03） |
| SEC-09 | ✅ | ①（批1）`mfa_required_for_all` 时未绑定用户只发 `scope=mfa` 受限令牌（2h，白名单外路由 40303），绑定/登出/查我可用 |
| SEC-10 | ✅ | ②（批2）`guardCanonicalInput` 序列化投影：Content/工具调用 name+arguments（JSON 字符串级 walk 掩码）/工具定义 name+description+parameters/结果名全部过检，命中就地改写；`InputPlainText` 纳入工具文本（审计/计费可见）；缓冲与流式路径的工具文本均过 CheckOutput；`mergeToolText` JSON 压缩防边界标签伪造 |
| M-01 | ◐ | ②③（批2+3）共享 `httpclient` 工厂拒绝一切重定向（gateway/health/testProvider/webhook 四处）；base_url 仅 http/https。**残余**：不拦私网目标——单角色"管理员=可信运营商"信任模型使然，多租户请网络层收敛出网（已写入报告 §六 姿势与 quickstart §9） |
| M-02 | ✅ | ①（批1）`sessions_invalid_before`：登出/改密/解绑 MFA/解绑他人 → 该用户此前签发 JWT 全失效（对照 `iat`） |
| M-03 | ✅ | ①⑤（批1 per-IP 登录限速 10/min，429+Retry-After，bcrypt 前先查；批5 锁定到期后计数清零——否则"过期后一次输错即再锁 15 分钟"，攻击者每 16 分钟一个坏请求即可永久锁死管理员） |
| M-04 | ✅ | ①（批1）审计 IP 统一 `c.ClientIP()`（尊重 TRUSTED_PROXIES 姿势，默认不信任 XFF） |
| M-05 | ✅ | ③④（批3+4）`csvCell` 公式前缀防护应用于全部导出：敏感词、调用日志（含 request_id/model/reason）、操作日志、token 统计 |
| M-06 | ✅ | ④（批4）HKDF-SHA256 拉伸派生（`k2:` 密文格式，历史密文透明兼容）+ 批1 启动弱密钥拒绝。爆破可行性取决于口令熵——策略已挡占位/弱默认 |
| M-07 | ✅ | ①③（批1 step 单调防重放 + 恢复码 CAS 原子消费；批3 mfa_secret AES-GCM "v2:" 加密，明文旧行兼容读） |
| M-08 | ✅ | ①（批1）内存票据 5min TTL + 惰性清扫 |
| M-09 | ✅ | ①（批1）GenerateFromPassword 错误→500 不出坏哈希；8..72 字节界 |
| M-10 | ✅ | ①（批1）口令校验先于锁定判定、不存在用户走 dummy bcrypt 等时、统一"用户名或密码错误"；40301 仅认证后返回；前端 interceptor 正确展示（`a56f7e5`+批1） |
| M-11 | ✅ | ④（批4）`METRICS_TOKEN` Bearer（常量时间比较）；未配置时开放+启动醒目告警——内网姿势仍是主防线（文档化） |
| M-12 | ✅ | ②（批2）白名单外/不存在统一 404 `model_not_found`（同文案同状态，无预言机） |
| M-13 | ✅ | ②（批2）拦截对外只回通用"rejected by content policy"+Retry-After；命中详情仅入审计 |
| M-14 | ◐ | ④（批4）握手校验 `Origin` 与 Host 同源（浏览器不可伪造 Origin → CSWSH 关死）；非浏览器无 Origin 客户端兼容。**残余**：token 仍在 query（反代日志面）——浏览器 WS 无法带自定义头的折中，已在 TLS 文档提示反代勿记 query |
| M-15 | ✅ | ①（批1）CSP 全指令（script-src 'self' 无 unsafe-eval、frame-ancestors none、form-action 'self'…） |
| M-16 | ✅ | ④（批4）quickstart 中英 §9：Caddy/nginx TLS+HSTS+SSE 不缓冲+TRUSTED_PROXIES 姿势。进程原生 TLS 不做=设计（反代终结） |
| M-17 | ✅ | ④（批4）原子连接准入移到体读取之前（Handle+CountTokens 双路径）；解析/护栏在限流后是因果必需（需 alias），以连接级+每 Key 并发封顶前置成本 |
| M-18 | ✅ | ②③（批2+3）队列满同步兜底**失败不再静默**（日志留痕）；同步写为"审计不丢优先"的设计取舍，保留并文档化 |
| M-19 | ◐ | ②（批2）check→扣减合并为 reserve 条件原子写（每请求 2 写：预扣+结算，原为 3）；"合并批量落账"未做——sqlite 单写者下由 `SetMaxOpenConns(1)`+WAL 串行化兜底 |
| M-20 | ✅ | ④（批4）`TryAdmitConn` 检查+占坑原子化；每 Key 在途上限 64（分表计数，条目随归零回收） |
| M-21 | ✅ | ④（批4）SLB 候选快照在锁内（纯内存），Redis SWRR 往返（≤300ms）挪到锁外；本地游标兜底路径二次短锁 |
| M-22 | ✅ | ④（批4）快照内容+状态与上一条相同→跳过 ConfigVersion 大行与加载日志（只推进 meta）；存量剪枝（保留 50）原有 |
| M-23 | ◐ | ②（批2）单次 ToLower+输出命中即停。**残余**：流式全文增量扫描未实现（O(L²/阈值) 仍在）；缓解=每请求体积封顶 16MiB+阈值下限 64B，属性能项非安全项 |
| M-24 | ✅ | ④（批4）导入模型 ≤2000 条、单名 ≤128B、控制字符拒 |
| M-25 | ✅ | ③（批3）CSV 导入 2MiB/5000 行/单词 512B；batch 端点 5000 条上限 + word 长度界 |
| M-26 | ✅ | ②（批2）lower 每请求一次并传入全部匹配 |
| M-27 | ✅ | ③（批3）KV 五模块全量定界（保留天数/超时/采样/热加载/重试/熔断/阈值/策略枚举/恢复码数/宽限期）；③批3 provider/alias/quota 写侧同修 |
| M-28 | ✅ | ④（批4）`ReplaceAllLiteralString`（两处），`$1` 不再展开捕获组 |
| M-29 | ✅ | ②（批2）访问日志 `EscapedPath()`（%0a 不再还原为换行） |
| M-30 | ✅ | ①③（批1 读侧坏 JSON→fail-closed；批3 写侧 JSON 形态/长度/CIDR 合法性校验） |
| M-31 | ✅ | ②（批2）degrade 目标再过 Key 白名单（不在授权内→不换 Key 直接拒）；限流维度按目标别名复跑 |
| M-32 | ◐ | ③④（批3 密文完整性+拒绝半残恢复；批4 审计条目含来源文件名/体积/各表计数/导出时间）。**残余**：restore 仍接受自造 bundle 全量覆盖（单角色全权设计内）；KV 键白名单/行级校验复用未做 |
| M-33 | ✅ | ④（批4）deploy/（含 env.sh 本地 DSN 口令与 linux 二进制）从库中移除 + .gitignore；新克隆零口令 |
| L-01 | ✅ | ②（批2）`MaxBytesReader` → 413 显式拒绝（不再截断伪 400） |
| L-02 | ✅ | ①（批1）裸 IP 等价 /32、/128 参与匹配 |
| L-03 | ○ | 残余：单角色是产品既定设计（README 声明）；解绑他人 MFA 已有操作审计+会话吊销（批1 unbind→invalidate）缓解。RBAC 属功能演进 |
| L-04 | ◐ | ③④（批3+4）restore/批量导入等补齐前后摘要。**边界**：改 Key 类操作故意不落 before/after 明文（避免二次泄密），以失效时间戳/掩码替代 |
| L-05 | ✅ | ④（批4）page_size ≤500（原缺界→归 20 兜底已有，补上界说明）+ 全部 10 处 LIKE 通配符转义 `ESCAPE '\'` |
| L-06 | ○ | 残余：token 计数必须 regexp2（tiktoken 兼容），输入=租户文本但体积封顶 16MiB+长度预算；上游依赖面（护栏正则为 RE2）。建议发布前 `govulncheck` 补扫（§六 已列） |
| L-07 | ○ | 残余：上游错误体透传为产品特性（客户端需厂商原始错误）；网关自身错误已通用化（批2），透传内容不含网关内部信息 |
| L-08 | ○ | 残余：前4后2 展示为 Key 定位所需；sk-128bit 熵下 12bit 泄露无实际碰撞风险 |
| L-09 | ✅ | ②（批2）SSE 帧/上游头只带服务端 reqID；客户端值清洗后仅入审计列 |
| L-10 | ✅ | ④（批4）`failInternal`：驱动/IO 原文只进日志，客户端固定文案；400 文案全部为自撰校验语 |
| L-11 | ✅ | ②（批2）限流/配额对外通用文案 + Retry-After；规则 id/阈值/体量仅入审计 |
| L-12 | ✅ | ①（批1）403/429 交 interceptor 正确渲染（与 `a56f7e5` 通用文案合并生效） |

**汇总**：✅ 50 ｜ ◐ 6（M-01/M-14/M-19/M-23/M-32/L-04，残余均附理由与部署侧对策）｜ ○ 4（L-03/L-06/L-07/L-08，产品设计与熵预算内）。每批合入前：`go build ./...` + `go test ./...` 全绿 + 前端 `npm run build` 通过；批 3 另附 admin 测试 3 连跑消 flake（审计 WS"101 先于订阅"为彼时修真的既有竞态）。
