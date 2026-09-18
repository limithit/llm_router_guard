// main.go 程序入口：数据库/热加载/配额任务/MFA/审计/静态资源/管理API/网关端点初始化。
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"llmrouter/internal/admin"
	"llmrouter/internal/audit"
	"llmrouter/internal/config"
	"llmrouter/internal/crypto"
	"llmrouter/internal/db"
	"llmrouter/internal/gateway"
	"llmrouter/internal/health"
	"llmrouter/internal/metrics"
	"llmrouter/internal/model"
	"llmrouter/internal/quota"
	"llmrouter/internal/runtime"
	"llmrouter/internal/settings"
	"llmrouter/internal/slb"
	"llmrouter/internal/tokens"
	"llmrouter/internal/util"
)

func main() {
	// 先加载 .env（若存在）再读取配置：真实环境变量始终优先于文件值。
	// 使裸进程 `./server` 与 docker compose 共享同一份 .env 语义（见 internal/config/dotenv.go）。
	config.LoadDotenv()
	cfg := config.Load()

	// SEC-01：占位/出厂密钥一律拒绝启动（除非 ALLOW_INSECURE_DEFAULTS=1 的显式本地开发豁免）。
	enforceSecretPolicy(cfg)

	// 预热 token 编码器（cl100k_base BPE）：后台下载/加载编码文件，
	// 避免首个请求承担延迟；离线场景（TIKTOKEN_CACHE_DIR 缺失且无网络）会降级到启发式估算。
	util.SafeGo("tokens.Init", func() { _ = tokens.Init() })

	// 数据库
	gormDB, err := db.Open(cfg.DBType, cfg.DBDSN)
	if err != nil {
		log.Fatalf("open database failed: %v", err)
	}

	// 加密器（主密钥）
	enc, err := crypto.NewCipher(cfg.MasterKey)
	if err != nil {
		log.Fatalf("init cipher failed: %v", err)
	}

	// 写入默认设置
	if err := settings.EnsureDefaults(gormDB, cfg.ListenPort); err != nil {
		log.Fatalf("ensure defaults failed: %v", err)
	}

	// 热加载运行时 + 其他组件
	mgr := runtime.NewManager(gormDB, enc)
	bl := slb.New()
	rl := rateLimiter(cfg, gormDB)
	qm := quota.NewQuotaManager(gormDB)
	mx := metrics.New()
	al := audit.NewLogger(gormDB, auditRetentionFn(gormDB))
	if _, distributed := rl.(*quota.RedisLimiter); distributed {
		bl.SetRedis(cfg.RedisAddr, cfg.RedisPassword) // 熔断打开状态多实例共享
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr.Start(ctx)
	al.Start(ctx)
	qm.StartBackground(ctx, rl)
	// 上游健康检查：周期探测启用供应商，失败经熔断计数累计（多实例时 Redis 广播）。
	// HEALTH_CHECK_SECONDS=0 可禁用。
	util.SafeGo("health.Run", func() { health.New(gormDB, enc, bl).Run(ctx, time.Duration(cfg.HealthCheckSeconds)*time.Second) })

	// 首次引导管理员
	bootstrapAdmin(gormDB, cfg)

	// Gin 路由
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	// 仅信任配置内的反向代理；默认空 → 不解析 X-Forwarded-For，c.ClientIP() 取 TCP 对端，
	// 防止客户端伪造 XFF 头绕过 API Key 的 IP 白名单。反代部署时设 TRUSTED_PROXIES=<代理 CIDR>。
	if err := engine.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		log.Printf("[security] set trusted proxies: %v", err)
	}
	engine.Use(gin.Recovery())
	// 安全响应头：抑制 MIME 嗅探 / 点击劫持 / 引用泄漏；M-15：CSP 收敛 XSS→localStorage 令牌失窃面
	// （style 'unsafe-inline' 为 antd 运行时样式所需；img data: 为 MFA 二维码 data-URI 所需）。
	engine.Use(func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; "+
				"object-src 'none'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		c.Next()
	})
	// 访问日志：方法/路径/状态/延迟/来源 IP，便于排查 404 等。
	// 跳过静态资源与 SPA 回退（NoRoute 200）以降噪。
	engine.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		// M-29：URL.Path 已百分号解码——路径里的 %0a/%0d 会还原成真实换行伪造日志行。
		// 记录保持编码形态的 EscapedPath（可逆、无控制字符注入面）。
		path := c.Request.URL.EscapedPath()
		if strings.HasPrefix(path, "/assets/") {
			return
		}
		if c.FullPath() == "" && c.Writer.Status() == http.StatusOK {
			return
		}
		log.Printf("[access] %3d  %-6s %s  %s  %s",
			c.Writer.Status(), c.Request.Method, path, time.Since(start), c.ClientIP())
	})

	adminServer := admin.New(gormDB, mgr, bl, mx, al, enc, cfg.JWTSecret, cfg.ListenPort, cfg.RedisAddr, cfg.RedisPassword)
	gw := gateway.New(gormDB, mgr, bl, rl, qm, al, mx)

	adminServer.Register(engine, gw)
	adminServer.MountStatic(engine, cfg.FrontendDist)

	// SEC-06：入站超时显式生效（cfg.ReadTimeout 此前从未挂到 server 上）。
	// ReadTimeout=90s 约束「读请求头+请求体」阶段（含匿名的 /auth/login body）；
	// WriteTimeout 保持 0（SSE 长流的响应写不设总超时，由逐请求 context 控制）；
	// ReadHeaderTimeout 单独收紧到 10s（slowloris 头部阶段防线）。
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.ListenPort),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	// HTTP 服务监听：出错不退出进程（网络抖动 / 端口瞬时占用 / 系统休眠唤醒后 bind 失败等
	// 可恢复场景），退避重试；仅在 ctx 取消（收到 SIGINT/SIGTERM）时优雅退出。
	util.SafeGo("http.ListenAndServe", func() {
		log.Printf("LLM Router Guard listening on :%d", cfg.ListenPort)
		backoff := time.Second
		for {
			err := srv.ListenAndServe()
			if err == nil || err == http.ErrServerClosed {
				return
			}
			if ctx.Err() != nil {
				return
			}
			log.Printf("[http] listen error: %v — retrying in %s", err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	})

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}

// bootstrapAdmin 首启引导管理员（SEC-02 加固）：
//   - 不再内置 admin123 出厂口令；未设 ADMIN_PASSWORD 时随机生成一次性口令（仅本次启动
//     打印一次），账户置 must_change_password，首次登录只发「仅可改密」受限会话；
//   - ADMIN_PASSWORD 已设但短于 8 位 → 拒绝启动；
//   - 已有用户时 ADMIN_PASSWORD 不再生效（打印提示，避免“改了环境变量没生效”的静默陷阱）。
func bootstrapAdmin(gdb *gorm.DB, cfg *config.Config) {
	var count int64
	gdb.Model(&model.AdminUser{}).Count(&count)
	if count > 0 {
		if cfg.AdminPassword != "" {
			log.Println("[bootstrap] NOTE: ADMIN_PASSWORD is only applied on first boot (users already exist). Manage passwords via the console.")
		}
		return
	}
	pass := cfg.AdminPassword
	generated := false
	if pass == "" {
		pass = crypto.RandomHex(12) // 一次性随机口令，打印后要求首登即改
		generated = true
	} else if len(pass) < 8 {
		log.Fatalf("[bootstrap] ADMIN_PASSWORD must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash admin password: %v", err)
	}
	u := model.AdminUser{Username: cfg.AdminUser, PasswordHash: string(hash), MustChangePassword: generated}
	if err := gdb.Create(&u).Error; err != nil {
		log.Fatalf("create admin user: %v", err)
	}
	log.Printf("[bootstrap] created admin user: %s", cfg.AdminUser)
	if generated {
		log.Printf("[bootstrap] ONE-TIME admin password for %q: %s", cfg.AdminUser, pass)
		log.Println("[bootstrap] This password is shown once. Sign in and change it immediately (forced on first login).")
	}
}

// placeholderSecret 判断密钥是否为占位值/出厂默认（SEC-01：compose 里的大写 CHANGE-ME-*
// 曾与告警比对的小写默认值不匹配，导致零告警裸奔——改为不区分大小写的前缀+已知值全查）。
func placeholderSecret(s string) bool {
	l := strings.ToLower(s)
	return l == "" || strings.HasPrefix(l, "change-me") || l == "changeme" ||
		l == "llm-router-guard-master-key" || l == "admin123"
}

// enforceSecretPolicy 占位密钥直接拒绝启动；ALLOW_INSECURE_DEFAULTS=1 仅限本地开发，
// 设置后降级为醒目告警。
func enforceSecretPolicy(cfg *config.Config) {
	allowInsecure := os.Getenv("ALLOW_INSECURE_DEFAULTS") == "1"
	check := func(name, val, why string) {
		if !placeholderSecret(val) {
			return
		}
		if allowInsecure {
			log.Printf("[security] WARNING: %s is a placeholder/default value (%s). ALLOW_INSECURE_DEFAULTS=1 is set for local development — NEVER use this in production.", name, why)
			return
		}
		log.Fatalf("[security] refusing to start: %s is a placeholder/default value (%s). Set a unique strong %s, e.g. with: openssl rand -hex 32. For local development only, set ALLOW_INSECURE_DEFAULTS=1.", name, why, name)
	}
	check("JWT_SECRET", cfg.JWTSecret, "admin tokens would be forgeable by anyone who reads the docs")
	check("MASTER_KEY", cfg.MasterKey, "stored provider API keys would be decryptable by anyone who reads the docs")
}

// rateLimiter 选择限流实现：Redis 配置且 DB 为 mysql/postgres（多节点部署）→
// 分布式固定窗口（Redis 原子计数）；其余 → 实例内存版（单节点语义）。
func rateLimiter(cfg *config.Config, gdb *gorm.DB) quota.Limiter {
	if cfg.RedisAddr != "" {
		switch strings.ToLower(cfg.DBType) {
		case "mysql", "postgres", "postgresql":
			return quota.NewRedisLimiter(cfg.RedisAddr, cfg.RedisPassword)
		default:
			log.Printf("[ratelimit] REDIS_ADDR set but DB_TYPE=%s is sqlite: per-instance limiter (sqlite is single-node)", cfg.DBType)
		}
	}
	return quota.NewRateLimiter(gdb)
}

func auditRetentionFn(gdb *gorm.DB) func() time.Duration {
	return func() time.Duration {
		var g settings.General
		_ = settings.LoadKV(gdb, model.SetKeyGeneral, &g, settings.DefaultGeneral())
		return time.Duration(g.AuditRetentionDays) * 24 * time.Hour
	}
}
