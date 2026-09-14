// apikeys.go API Key 管理（REQ-002）：创建一次性返回完整 Key，之后仅脱敏展示。
package admin

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/crypto"
	"llmrouter/internal/model"
)

// validateApiKeyLists M-30 写侧：两个 JSON 列表字段落库前强制合法
// （读侧已 fail-closed；写侧不校验的话，一次坏数据就把整把 Key 静默锁死/放开）。
func validateApiKeyLists(modelsJSON, ipJSON string) error {
	if modelsJSON != "" {
		var l []string
		if err := json.Unmarshal([]byte(modelsJSON), &l); err != nil {
			return fmt.Errorf("allowed_models_json 必须是 JSON 字符串数组")
		}
		if len(l) > 500 {
			return fmt.Errorf("授权别名最多 500 个")
		}
	}
	if ipJSON != "" {
		var l []string
		if err := json.Unmarshal([]byte(ipJSON), &l); err != nil {
			return fmt.Errorf("ip_allowlist_json 必须是 JSON 字符串数组")
		}
		if len(l) > 200 {
			return fmt.Errorf("IP 白名单最多 200 条")
		}
		for _, c := range l {
			c = strings.TrimSpace(c)
			if _, _, err := net.ParseCIDR(c); err == nil {
				continue
			}
			if net.ParseIP(c) == nil { // L-02：裸 IP 合法（等 /32、/128）
				return fmt.Errorf("IP 白名单条目无法解析：%s（需 CIDR 或 IP）", c)
			}
		}
	}
	return nil
}

func (s *Server) listApiKeys(c *gin.Context) {
	page, size := parsePage(c)
	q := s.db.Model(&model.APIKey{})
	if kw := c.Query("keyword"); kw != "" {
		q = q.Where("name LIKE ? ESCAPE '\\'", likeArg(kw))
	}
	var total int64
	q.Count(&total)
	var rows []model.APIKey
	q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows)
	okPaged(c, rows, int(total), page, size)
}

func (s *Server) createApiKey(c *gin.Context) {
	var req struct {
		Name               string `json:"name"`
		Remark             string `json:"remark"`
		AllowedModelsJSON  string `json:"allowed_models_json"`
		IPAllowlistJSON    string `json:"ip_allowlist_json"`
		IPAllowlistEnabled bool   `json:"ip_allowlist_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请输入 Key 名称")
		return
	}
	if err := validateApiKeyLists(req.AllowedModelsJSON, req.IPAllowlistJSON); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	key := "sk-" + crypto.RandomHex(16)
	rec := model.APIKey{Name: req.Name, KeyHash: crypto.Sha256Hex(key),
		KeyMasked: crypto.MaskKey(key), Remark: req.Remark,
		AllowedModelsJSON: req.AllowedModelsJSON, IPAllowlistJSON: req.IPAllowlistJSON,
		IPAllowlistEnabled: req.IPAllowlistEnabled, Enabled: true}
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
		Name               string `json:"name"`
		Remark             string `json:"remark"`
		Enabled            bool   `json:"enabled"`
		AllowedModelsJSON  string `json:"allowed_models_json"`
		IPAllowlistJSON    string `json:"ip_allowlist_json"`
		IPAllowlistEnabled bool   `json:"ip_allowlist_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if err := validateApiKeyLists(req.AllowedModelsJSON, req.IPAllowlistJSON); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	before := k
	k.Name, k.Remark, k.Enabled = req.Name, req.Remark, req.Enabled
	k.AllowedModelsJSON = req.AllowedModelsJSON
	k.IPAllowlistJSON = req.IPAllowlistJSON
	k.IPAllowlistEnabled = req.IPAllowlistEnabled
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
