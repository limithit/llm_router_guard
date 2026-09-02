// settings.go KV 形式配置：通用设置(REQ-001)/故障转移(REQ-007)/输出过滤(REQ-011)/
// 配额预警(REQ-014)/安全设置(REQ-020)，保存后触发 Manager.Bump 热加载。
package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/model"
	"llmrouter/internal/settings"
)

// ---- 通用设置 (REQ-001) ----

func (s *Server) getGeneral(c *gin.Context) {
	var g settings.General
	settings.LoadKV(s.db, model.SetKeyGeneral, &g, settings.DefaultGeneral())
	g.ListenPort = s.port
	g.ListenPortNote = "仅启动参数生效（PORT 环境变量），修改需重启服务"
	s.ok(c, g)
}

func (s *Server) saveGeneral(c *gin.Context) {
	var g settings.General
	if err := c.ShouldBindJSON(&g); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	g.ListenPort = s.port
	if err := settings.SaveKV(s.db, model.SetKeyGeneral, g); err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "保存失败")
		return
	}
	s.recordOp(c, "update", "settings", "general", nil, g)
	s.mgr.Bump()
	s.ok(c, g)
}

// ---- 故障转移 (REQ-007) ----

func (s *Server) getFailover(c *gin.Context) {
	var f settings.Failover
	settings.LoadKV(s.db, model.SetKeyFailover, &f, settings.DefaultFailover())
	s.ok(c, f)
}

func (s *Server) saveFailover(c *gin.Context) {
	var f settings.Failover
	if err := c.ShouldBindJSON(&f); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if err := settings.SaveKV(s.db, model.SetKeyFailover, f); err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "保存失败")
		return
	}
	s.recordOp(c, "update", "failover", "failover", nil, f)
	s.mgr.Bump()
	s.ok(c, f)
}

// ---- 输出过滤 (REQ-011) ----

func (s *Server) getOutput(c *gin.Context) {
	var o settings.OutputFilter
	settings.LoadKV(s.db, model.SetKeyOutputFilt, &o, settings.DefaultOutputFilter())
	s.ok(c, o)
}

func (s *Server) saveOutput(c *gin.Context) {
	var o settings.OutputFilter
	if err := c.ShouldBindJSON(&o); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if err := settings.SaveKV(s.db, model.SetKeyOutputFilt, o); err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "保存失败")
		return
	}
	s.recordOp(c, "update", "guard", "output_filter", nil, o)
	s.mgr.Bump()
	s.ok(c, o)
}

// ---- 配额预警 (REQ-014) ----

func (s *Server) getQuotaAlerts(c *gin.Context) {
	var a settings.QuotaAlerts
	settings.LoadKV(s.db, model.SetKeyQuotaAlert, &a, settings.DefaultQuotaAlerts())
	s.ok(c, a)
}

func (s *Server) saveQuotaAlerts(c *gin.Context) {
	var a settings.QuotaAlerts
	if err := c.ShouldBindJSON(&a); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if err := settings.SaveKV(s.db, model.SetKeyQuotaAlert, a); err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "保存失败")
		return
	}
	s.recordOp(c, "update", "quota", "alerts", nil, a)
	s.mgr.Bump()
	s.ok(c, a)
}

// ---- 安全设置 (REQ-020) ----

func (s *Server) getSecurity(c *gin.Context) {
	s.ok(c, s.securitySettings())
}

func (s *Server) saveSecurity(c *gin.Context) {
	var sec settings.Security
	if err := c.ShouldBindJSON(&sec); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if err := settings.SaveKV(s.db, model.SetKeySecurity, sec); err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "保存失败")
		return
	}
	s.recordOp(c, "update", "security", "mfa", nil, sec)
	s.mgr.Bump()
	s.ok(c, sec)
}
