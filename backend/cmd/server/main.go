// main.go 程序入口：数据库/热加载/配额任务/MFA/审计/静态资源/管理API/网关端点初始化。
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	"llmrouter/internal/metrics"
	"llmrouter/internal/model"
	"llmrouter/internal/quota"
	"llmrouter/internal/runtime"
	"llmrouter/internal/settings"
	"llmrouter/internal/slb"
	"llmrouter/internal/tokens"
)

func main() {
	cfg := config.Load()

	// 预热 token 编码器（cl100k_base BPE）：后台下载/加载编码文件，
	// 避免首个请求承担延迟；离线场景（TIKTOKEN_CACHE_DIR 缺失且无网络）会降级到启发式估算。
	go tokens.Init()

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
	rl := quota.NewRateLimiter(gormDB)
	qm := quota.NewQuotaManager(gormDB)
	mx := metrics.New()
	al := audit.NewLogger(gormDB, auditRetentionFn(gormDB))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr.Start(ctx)
	al.Start(ctx)
	qm.StartBackground(ctx, rl)

	// 首次引导管理员
	bootstrapAdmin(gormDB, cfg)

	// Gin 路由
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	engine.Use(gin.Recovery())

	adminServer := admin.New(gormDB, mgr, bl, mx, al, enc, cfg.JWTSecret, cfg.ListenPort)
	gw := gateway.New(gormDB, mgr, bl, rl, qm, al, mx)

	adminServer.Register(engine, gw)
	adminServer.MountStatic(engine, cfg.FrontendDist)

	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.ListenPort), Handler: engine}

	go func() {
		log.Printf("LLM Router Guard listening on :%d", cfg.ListenPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}

func bootstrapAdmin(gdb *gorm.DB, cfg *config.Config) {
	var count int64
	gdb.Model(&model.AdminUser{}).Count(&count)
	if count > 0 {
		return
	}
	pass := cfg.AdminPassword
	if pass == "" {
		pass = "admin123" // 仅在未设置环境变量时默认，首次启动后必须修改
		log.Println("[bootstrap] WARNING: using default admin password 'admin123'. Set ADMIN_PASSWORD to change.")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash admin password: %v", err)
	}
	u := model.AdminUser{Username: cfg.AdminUser, PasswordHash: string(hash)}
	if err := gdb.Create(&u).Error; err != nil {
		log.Fatalf("create admin user: %v", err)
	}
	log.Printf("[bootstrap] created admin user: %s", cfg.AdminUser)
}

func auditRetentionFn(gdb *gorm.DB) func() time.Duration {
	return func() time.Duration {
		var g settings.General
		_ = settings.LoadKV(gdb, model.SetKeyGeneral, &g, settings.DefaultGeneral())
		return time.Duration(g.AuditRetentionDays) * 24 * time.Hour
	}
}
