// apikeys.go API Key 管理（REQ-002）：创建一次性返回完整 Key，之后仅脱敏展示。
package admin

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/crypto"
	"llmrouter/internal/model"
)

func (s *Server) listApiKeys(c *gin.Context) {
	page, size := parsePage(c)
	q := s.db.Model(&model.APIKey{})
	if kw := c.Query("keyword"); kw != "" {
		q = q.Where("name LIKE ?", "%"+kw+"%")
	}
	var total int64
	q.Count(&total)
	var rows []model.APIKey
	q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows)
	okPaged(c, rows, int(total), page, size)
}

func (s *Server) createApiKey(c *gin.Context) {
	var req struct {
		Name   string `json:"name"`
		Remark string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请输入 Key 名称")
		return
	}
	key := "sk-" + crypto.RandomHex(16)
	rec := model.APIKey{Name: req.Name, KeyHash: crypto.Sha256Hex(key),
		KeyMasked: crypto.MaskKey(key), Remark: req.Remark, Enabled: true}
	if err := s.db.Create(&rec).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "unique") {
			s.fail(c, http.StatusConflict, 40901, "名称已存在")
			return
		}
		s.fail(c, http.StatusInternalServerError, 50001, "创建失败")
		return
	}
	s.recordOp(c, "create", "apikey", req.Name, nil, gin.H{"id": rec.ID, "name": req.Name})
	s.mgr.Bump()
	s.ok(c, gin.H{"id": rec.ID, "name": rec.Name, "key": key}) // 仅此一次返回完整 Key
}

func (s *Server) updateApiKey(c *gin.Context) {
	id := c.Param("id")
	var k model.APIKey
	if err := s.db.First(&k, id).Error; err != nil {
		s.fail(c, 404, 40401, "API Key 不存在")
		return
	}
	var req struct {
		Name    string `json:"name"`
		Remark  string `json:"remark"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	before := k
	k.Name, k.Remark, k.Enabled = req.Name, req.Remark, req.Enabled
	if err := s.db.Save(&k).Error; err != nil {
		s.fail(c, 500, 50001, "更新失败")
		return
	}
	s.recordOp(c, "update", "apikey", k.Name, before, k)
	s.mgr.Bump()
	s.ok(c, k)
}

func (s *Server) deleteApiKey(c *gin.Context) {
	id := c.Param("id")
	var k model.APIKey
	if err := s.db.First(&k, id).Error; err != nil {
		s.fail(c, 404, 40401, "API Key 不存在")
		return
	}
	s.db.Delete(&k)
	s.recordOp(c, "delete", "apikey", k.Name, k, nil)
	s.mgr.Bump()
	s.ok(c, nil)
}
