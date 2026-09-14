// Package admin 实现管理 API（RESTful，供前端 UI 调用，覆盖 REQ-001~024）。
// 所有变更写入数据库后经 runtime.Manager.Bump() 触发热加载（≤3 秒生效，REQ-004），
// 同时调用 audit.LogOp 记录操作审计（REQ-003）。
package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"llmrouter/internal/audit"
	"llmrouter/internal/crypto"
	"llmrouter/internal/metrics"
	"llmrouter/internal/model"
	"llmrouter/internal/runtime"
	"llmrouter/internal/settings"
	"llmrouter/internal/slb"
)

type Server struct {
	db               *gorm.DB
	mgr              *runtime.Manager
	bl               *slb.Balancer
	mx               *metrics.Metrics
	al               *audit.Logger
	enc              *crypto.Cipher
	secret           string
	port             int
	mfaStateInstance mfaStore    // 内存（单节点）或 Redis（多节点），见 redis_mfa.go
	loginLimiter     loginLimit  // /auth/login per-IP 限速（SEC-08/M-03）
}

// loginLimit 简单接口，便于测试替换。
type loginLimit interface {
	allow(ip string, now time.Time) bool
	retryAfter(ip string, now time.Time) int
}

func New(gdb *gorm.DB, mgr *runtime.Manager, bl *slb.Balancer, mx *metrics.Metrics,
	al *audit.Logger, enc *crypto.Cipher, jwtSecret string, listenPort int, redisAddr, redisPassword string) *Server {
	return &Server{db: gdb, mgr: mgr, bl: bl, mx: mx, al: al, enc: enc,
		secret: jwtSecret, port: listenPort, mfaStateInstance: newMFAStore(redisAddr, redisPassword),
		loginLimiter: newLoginLimiter()}
}

// userIDOf 从 JWT 中间件写入的 context 取当前管理员 ID。
func userIDOf(c *gin.Context) uint {
	if v, ok := c.Get("userID"); ok {
		if u, ok2 := v.(uint); ok2 {
			return u
		}
	}
	return 0
}

// securitySettings 读取 MFA 全局配置（REQ-020）。
func (s *Server) securitySettings() settings.Security {
	var sec settings.Security
	_ = settings.LoadKV(s.db, model.SetKeySecurity, &sec, settings.DefaultSecurity())
	return sec
}

// ---- 统一响应包裹（契约 §统一响应包裹）----

func (s *Server) ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": data})
}

func (s *Server) fail(c *gin.Context, httpStatus, code int, msg string) {
	c.JSON(httpStatus, gin.H{"code": code, "message": msg, "data": nil})
}

func okPaged[T any](c *gin.Context, items []T, total, page, pageSize int) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"items": items, "total": total, "page": page, "page_size": pageSize}})
}

func paramInt(c *gin.Context, key string, def int) int {
	if v := c.Query(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func parsePage(c *gin.Context) (int, int) {
	page := paramInt(c, "page", 1)
	if page < 1 {
		page = 1
	}
	size := paramInt(c, "page_size", 20)
	if size < 1 || size > 500 {
		size = 20
	}
	return page, size
}

// operator / ip：JWT 中间件写入 context；操作审计统一落库（REQ-003）。
func (s *Server) recordOp(c *gin.Context, action, module, target string, before, after any) {
	audit.LogOp(s.db, operatorOf(c), action, module, target, before, after, clientIP(c),
		s.mgr.Status == "ok")
}

func operatorOf(c *gin.Context) string {
	if v, ok := c.Get("operator"); ok {
		if u, _ := v.(string); u != "" {
			return u
		}
	}
	return "anonymous"
}

// clientIP M-04：操作/登录审计的来源 IP 一律取 gin 的 ClientIP()——
// 仅当 TRUSTED_PROXIES 显式配置才解析 X-Forwarded-For；此前手工读 header 使审计 IP 可被任意伪造。
func clientIP(c *gin.Context) string {
	return c.ClientIP()
}

// invalidateSessions SEC-13：把该用户全部已签发 JWT 作废（登出/改密/被解绑 MFA 时调用）。
func (s *Server) invalidateSessions(uid uint) {
	now := time.Now()
	s.db.Model(&model.AdminUser{}).Where("id = ?", uid).
		UpdateColumn("sessions_invalid_before", &now)
}
