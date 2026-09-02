// configstatus.go 配置状态 + 热加载控制 + 回滚 + 导出（REQ-004A）。
package admin

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/model"
)

func (s *Server) configStatus(c *gin.Context) {
	info := s.mgr.Info()

	var logs []model.ConfigLoadLog
	s.db.Order("id DESC").Limit(50).Find(&logs)
	logOut := make([]gin.H, 0, len(logs))
	for _, l := range logs {
		logOut = append(logOut, gin.H{"time": l.Time, "module": l.Module, "status": l.Status, "message": l.Message})
	}

	var hist []model.ConfigVersion
	s.db.Order("id DESC").Limit(50).Find(&hist)
	histOut := make([]gin.H, 0, len(hist))
	for _, h := range hist {
		histOut = append(histOut, gin.H{
			"version": h.Version, "created_at": h.CreatedAt, "status": h.Status, "message": h.Message})
	}

	s.ok(c, gin.H{
		"version":        info.Version,
		"last_loaded_at": info.LastLoadedAt,
		"status":         info.Status,
		"error_message":  info.ErrMessage,
		"modules":        info.Modules,
		"logs":           logOut,
		"history":        histOut,
	})
}

func (s *Server) reloadConfig(c *gin.Context) {
	s.mgr.Bump()
	if _, err := s.mgr.ReloadIfChanged("manual"); err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "重载失败: "+err.Error())
		return
	}
	s.recordOp(c, "reload", "settings", "config", nil, gin.H{"version": s.mgr.Get().Version})
	s.ok(c, nil)
}

func (s *Server) rollbackConfig(c *gin.Context) {
	v, err := s.mgr.Rollback()
	if err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	s.recordOp(c, "rollback", "settings", "config", nil, gin.H{"rolled_back_to": v})
	s.ok(c, gin.H{"rolled_back_to": v})
}

func (s *Server) exportConfig(c *gin.Context) {
	b, err := s.mgr.ExportBundle()
	if err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "导出失败: "+err.Error())
		return
	}
	body, _ := json.MarshalIndent(b, "", "  ")
	c.Header("Content-Disposition", "attachment; filename=config.json")
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}
