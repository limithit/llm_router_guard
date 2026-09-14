// backup.go 数据备份与恢复（REQ-019）。
package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/model"
	"llmrouter/internal/runtime"
)

func (s *Server) listBackups(c *gin.Context) {
	page, size := parsePage(c)
	var total int64
	s.db.Model(&model.BackupRecord{}).Count(&total)
	var rows []model.BackupRecord
	s.db.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows)
	okPaged(c, rows, int(total), page, size)
}

func (s *Server) createBackup(c *gin.Context) {
	b, err := s.mgr.ExportBundle()
	if err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "备份失败: "+err.Error())
		return
	}
	body, _ := json.Marshal(b)
	rec := model.BackupRecord{
		Filename:  fmt.Sprintf("backup-%s.json", time.Now().Format("20060102-150405")),
		Content:   string(body),
		SizeBytes: int64(len(body)),
	}
	if err := s.db.Create(&rec).Error; err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "备份失败")
		return
	}
	s.recordOp(c, "backup", "backup", rec.Filename, nil, gin.H{"id": rec.ID})
	s.ok(c, rec)
}

func (s *Server) downloadBackup(c *gin.Context) {
	id := c.Param("id")
	var r model.BackupRecord
	if err := s.db.First(&r, id).Error; err != nil {
		s.fail(c, http.StatusNotFound, 40401, "备份不存在")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", r.Filename))
	c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(r.Content))
}

func (s *Server) restoreBackup(c *gin.Context) {
	var req struct {
		ID      int    `json:"id"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	var data string
	src := "上传内容"
	if req.ID > 0 {
		var r model.BackupRecord
		if err := s.db.First(&r, req.ID).Error; err != nil {
			s.fail(c, http.StatusNotFound, 40401, "备份不存在")
			return
		}
		data = r.Content
		src = r.Filename
	} else {
		data = req.Content
	}
	b, err := runtime.LoadBundleJSON(data)
	if err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	if err := s.mgr.RestoreBundle(b); err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "恢复失败: "+err.Error())
		return
	}
	// M-32：全量覆盖是最高危的写操作——审计必须能回答"从哪恢复了什么规模的东西"
	// （旧记录 target=backup#0、前后值皆空，事后完全不可复核）。
	s.recordOp(c, "restore", "backup", src, nil, gin.H{
		"source": src, "bytes": len(data),
		"providers": len(b.Providers), "aliases": len(b.Aliases),
		"keywords": len(b.Keywords), "quotas": len(b.Quotas), "settings": len(b.Settings),
		"exported_at": b.ExportedAt,
	})
	s.mgr.Bump()
	s.ok(c, nil)
}

func (s *Server) deleteBackup(c *gin.Context) {
	id := c.Param("id")
	var r model.BackupRecord
	if err := s.db.First(&r, id).Error; err != nil {
		s.fail(c, http.StatusNotFound, 40401, "备份不存在")
		return
	}
	s.db.Delete(&r)
	s.recordOp(c, "delete", "backup", r.Filename, r, nil)
	s.ok(c, nil)
}
