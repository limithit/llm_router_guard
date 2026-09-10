// routes.go 注册网关端点 + 管理 API + 前端静态资源。
package admin

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/adapter"
	"llmrouter/internal/gateway"
)

// Register 在 gin 实例上注册所有路由。
func (s *Server) Register(r *gin.Engine, gws *gateway.Server) {
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	// Prometheus 抓取端点（P2 #6）：文本 v0.0.4，零依赖；建议仅内网/监控网段可达
	// （LB/网关层限制），故不做 JWT 鉴权——鉴权会让抓取端无法简单配置。
	r.GET("/metrics", func(c *gin.Context) {
		c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		s.mx.WritePromRender(c.Writer, s)
	})

	// 网关端点（REQ-017）
	gw := r.Group("/v1")
	gw.Use(gws.AuthMiddleware())
	gw.POST("/chat/completions", gws.Handle(adapter.ProtoOpenAIChat))
	gw.POST("/responses", gws.Handle(adapter.ProtoOpenAIResponses))
	gw.POST("/messages", gws.Handle(adapter.ProtoAnthropic))
	// anthropic 客户端的请求前预算探测（Claude Code），本地估算不调上游
	gw.POST("/messages/count_tokens", gws.CountTokens)
	// 模型目录（OpenAI/Anthropic 字段并集）。兼容不同工具把 /models 拼到端点后的发现路径。
	gw.GET("/models", gws.ListModels)
	gw.GET("/responses/models", gws.ListModels)
	gw.GET("/messages/models", gws.ListModels)
	gw.GET("/chat/completions/models", gws.ListModels)

	// 管理 API
	api := r.Group("/api/admin/v1")
	api.POST("/auth/login", s.login)
	api.POST("/auth/logout", s.AuthMiddleware(), s.logout)
	api.GET("/auth/me", s.AuthMiddleware(), s.me)
	api.PUT("/auth/password", s.AuthMiddleware(), s.changePassword)

	// 账户 MFA
	api.POST("/account/mfa/setup", s.AuthMiddleware(), s.mfaSetup)
	api.POST("/account/mfa/enable", s.AuthMiddleware(), s.mfaEnable)
	api.POST("/account/mfa/disable", s.AuthMiddleware(), s.mfaDisable)
	api.GET("/account/mfa/status", s.AuthMiddleware(), s.mfaStatus)

	api.GET("/dashboard", s.AuthMiddleware(), s.dashboard)

	api.GET("/providers", s.AuthMiddleware(), s.listProviders)
	api.POST("/providers", s.AuthMiddleware(), s.createProvider)
	api.PUT("/providers/:id", s.AuthMiddleware(), s.updateProvider)
	api.DELETE("/providers/:id", s.AuthMiddleware(), s.deleteProvider)
	api.POST("/providers/:id/test", s.AuthMiddleware(), s.testProvider)
	api.POST("/providers/:id/import-models", s.AuthMiddleware(), s.importProviderModels)

	api.GET("/models", s.AuthMiddleware(), s.listModels)
	api.POST("/models", s.AuthMiddleware(), s.createModel)
	api.PUT("/models/:id", s.AuthMiddleware(), s.updateModel)
	api.DELETE("/models/:id", s.AuthMiddleware(), s.deleteModel)
	api.GET("/models/:id/stats", s.AuthMiddleware(), s.modelStats)

	api.GET("/failover", s.AuthMiddleware(), s.getFailover)
	api.PUT("/failover", s.AuthMiddleware(), s.saveFailover)

	api.GET("/guard/keywords", s.AuthMiddleware(), s.listKeywords)
	api.POST("/guard/keywords", s.AuthMiddleware(), s.createKeyword)
	api.PUT("/guard/keywords/:id", s.AuthMiddleware(), s.updateKeyword)
	api.DELETE("/guard/keywords/:id", s.AuthMiddleware(), s.deleteKeyword)
	api.POST("/guard/keywords/batch", s.AuthMiddleware(), s.batchKeywords)
	api.POST("/guard/keywords/import", s.AuthMiddleware(), s.importKeywords)
	api.GET("/guard/keywords/export", s.AuthMiddleware(), s.exportKeywords)
	api.POST("/guard/keywords/test", s.AuthMiddleware(), s.testKeywords)

	api.GET("/guard/pii-rules", s.AuthMiddleware(), s.listPiiRules)
	api.POST("/guard/pii-rules", s.AuthMiddleware(), s.createPiiRule)
	api.PUT("/guard/pii-rules/:id", s.AuthMiddleware(), s.updatePiiRule)
	api.DELETE("/guard/pii-rules/:id", s.AuthMiddleware(), s.deletePiiRule)
	api.GET("/guard/pii-rules/templates", s.AuthMiddleware(), s.piiTemplates)
	api.POST("/guard/pii-rules/test", s.AuthMiddleware(), s.testPii)

	api.GET("/guard/injection-rules", s.AuthMiddleware(), s.listInjectionRules)
	api.POST("/guard/injection-rules", s.AuthMiddleware(), s.createInjectionRule)
	api.PUT("/guard/injection-rules/:id", s.AuthMiddleware(), s.updateInjectionRule)
	api.DELETE("/guard/injection-rules/:id", s.AuthMiddleware(), s.deleteInjectionRule)
	api.GET("/guard/injection-rules/templates", s.AuthMiddleware(), s.injectionTemplates)

	api.GET("/guard/output", s.AuthMiddleware(), s.getOutput)
	api.PUT("/guard/output", s.AuthMiddleware(), s.saveOutput)

	api.GET("/quotas", s.AuthMiddleware(), s.listQuotas)
	api.POST("/quotas", s.AuthMiddleware(), s.createQuota)
	api.PUT("/quotas/:id", s.AuthMiddleware(), s.updateQuota)
	api.DELETE("/quotas/:id", s.AuthMiddleware(), s.deleteQuota)
	api.POST("/quotas/:id/reset", s.AuthMiddleware(), s.resetQuota)
	api.POST("/quotas/batch", s.AuthMiddleware(), s.batchQuotas)

	api.GET("/rate-limits", s.AuthMiddleware(), s.listRateLimits)
	api.POST("/rate-limits", s.AuthMiddleware(), s.createRateLimit)
	api.PUT("/rate-limits/:id", s.AuthMiddleware(), s.updateRateLimit)
	api.DELETE("/rate-limits/:id", s.AuthMiddleware(), s.deleteRateLimit)

	api.GET("/quota-alerts", s.AuthMiddleware(), s.getQuotaAlerts)
	api.PUT("/quota-alerts", s.AuthMiddleware(), s.saveQuotaAlerts)

	api.GET("/audit/calls", s.AuthMiddleware(), s.listCallLogs)
	api.GET("/audit/calls/:request_id", s.AuthMiddleware(), s.callLogDetail)
	api.GET("/audit/calls/export", s.AuthMiddleware(), s.exportCallLogs)
	api.GET("/audit/operations", s.AuthMiddleware(), s.listOpLogs)
	api.GET("/audit/operations/export", s.AuthMiddleware(), s.exportOpLogs)
	// 实时调用审计流（P2 #5）：浏览器 WS 无法自定义 Authorization 头，token 走查询串，
	// 鉴权在升级前完成（401 不产生半开 WebSocket）。
	api.GET("/audit/ws", s.auditWS)

	// Token 用量统计看板（API Key 维度）
	api.GET("/token-stats", s.AuthMiddleware(), s.tokenStats)
	api.GET("/token-stats/export", s.AuthMiddleware(), s.exportTokenStats)

	api.GET("/settings", s.AuthMiddleware(), s.getGeneral)
	api.PUT("/settings", s.AuthMiddleware(), s.saveGeneral)
	api.GET("/settings/security", s.AuthMiddleware(), s.getSecurity)
	api.PUT("/settings/security", s.AuthMiddleware(), s.saveSecurity)
	api.GET("/apikeys", s.AuthMiddleware(), s.listApiKeys)
	api.POST("/apikeys", s.AuthMiddleware(), s.createApiKey)
	api.PUT("/apikeys/:id", s.AuthMiddleware(), s.updateApiKey)
	api.DELETE("/apikeys/:id", s.AuthMiddleware(), s.deleteApiKey)

	api.GET("/users", s.AuthMiddleware(), s.listUsers)
	api.POST("/users/:id/unbind-mfa", s.AuthMiddleware(), s.unbindUserMfa)
	api.POST("/users/:id/unlock", s.AuthMiddleware(), s.unlockUser)

	api.GET("/config-status", s.AuthMiddleware(), s.configStatus)
	api.POST("/config/reload", s.AuthMiddleware(), s.reloadConfig)
	api.POST("/config/rollback", s.AuthMiddleware(), s.rollbackConfig)
	api.GET("/config/export", s.AuthMiddleware(), s.exportConfig)

	api.GET("/status", s.AuthMiddleware(), s.status)

	api.GET("/backups", s.AuthMiddleware(), s.listBackups)
	api.POST("/backups", s.AuthMiddleware(), s.createBackup)
	api.GET("/backups/:id/download", s.AuthMiddleware(), s.downloadBackup)
	api.POST("/backups/restore", s.AuthMiddleware(), s.restoreBackup)
	api.DELETE("/backups/:id", s.AuthMiddleware(), s.deleteBackup)

	_ = os.DevNull
}

// MountStatic 将前端构建产物挂载为静态资源并支持 SPA 前端路由。
// 不能使用 r.Static("/", ...) 注册 catch-all /*filepath：它会与已注册的
// /api、/v1 等顶层路径段冲突（gin radix tree 不允许 catch-all 与命名段共存），
// 启动时直接 panic。改用 NoRoute 兜底：命中真实文件则直接返回，其余回退到 index.html。
func (s *Server) MountStatic(r *gin.Engine, dist string) {
	if dist == "" {
		if info, err := os.Stat("web/dist"); err == nil && info.IsDir() {
			dist = "web/dist"
		} else if info, err := os.Stat("../frontend/dist"); err == nil && info.IsDir() {
			dist = "../frontend/dist"
		}
	}
	if dist == "" {
		return
	}

	indexPath := filepath.Join(dist, "index.html")
	fs := http.Dir(dist)
	fileServer := http.FileServer(fs)

	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path

		// API / 网关 / 健康检查路径不应落到前端，返回 JSON 404。
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/v1/") || p == "/healthz" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		// 命中真实静态文件则直接返回；目录交由 SPA 回退处理。
		if f, err := fs.Open(p); err == nil {
			if fi, statErr := f.Stat(); statErr == nil && !fi.IsDir() {
				f.Close()
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
			f.Close()
		}

		// SPA 回退：未命中的路径交给前端路由处理。
		c.Header("Cache-Control", "no-cache")
		c.File(indexPath)
	})
}
