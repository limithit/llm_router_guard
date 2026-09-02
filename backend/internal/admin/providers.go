// providers.go 供应商管理（REQ-005）：CRUD + 测试连接 + 删除引用检查 + 热加载。
package admin

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/crypto"
	"llmrouter/internal/model"
	"llmrouter/internal/runtime"
)

type providerOut struct {
	model.Provider
	APIKey string `json:"-"` // 不输出明文
}

func (s *Server) listProviders(c *gin.Context) {
	page, size := parsePage(c)
	q := s.db.Model(&model.Provider{})
	if kw := c.Query("keyword"); kw != "" {
		q = q.Where("name LIKE ? OR remark LIKE ?", "%"+kw+"%", "%"+kw+"%")
	}
	var total int64
	q.Count(&total)
	var rows []model.Provider
	q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows)
	okPaged(c, rows, int(total), page, size)
}

func (s *Server) createProvider(c *gin.Context) {
	var req struct {
		Name     string `json:"name"`
		Protocol string `json:"protocol"`
		BaseURL  string `json:"base_url"`
		APIKey   string `json:"api_key"`
		Enabled  bool   `json:"enabled"`
		Remark   string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.BaseURL == "" {
		s.fail(c, 400, 40001, "请填写供应商名称、协议类型与端点 URL")
		return
	}
	enc, err := s.enc.Encrypt(req.APIKey)
	if err != nil {
		s.fail(c, 500, 50001, "密钥加密失败")
		return
	}
	p := model.Provider{Name: req.Name, Protocol: req.Protocol, BaseURL: req.BaseURL,
		APIKeyEnc: enc, APIKeyMasked: crypto.MaskKey(req.APIKey), Enabled: req.Enabled, Remark: req.Remark}
	if err := s.db.Create(&p).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "unique") {
			s.fail(c, 409, 40901, "供应商名称已存在")
			return
		}
		s.fail(c, 500, 50001, "创建失败: "+err.Error())
		return
	}
	s.recordOp(c, "create", "provider", p.Name, nil, p)
	s.mgr.Bump()
	p.APIKeyEnc = ""
	p.APIKeyMasked = crypto.MaskKey(req.APIKey)
	s.ok(c, p)
}

func (s *Server) updateProvider(c *gin.Context) {
	id := c.Param("id")
	var p model.Provider
	if err := s.db.First(&p, id).Error; err != nil {
		s.fail(c, 404, 40401, "供应商不存在")
		return
	}
	var req struct {
		Name     string `json:"name"`
		Protocol string `json:"protocol"`
		BaseURL  string `json:"base_url"`
		APIKey   string `json:"api_key"`
		Enabled  bool   `json:"enabled"`
		Remark   string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, 400, 40001, "请求参数错误")
		return
	}
	before := p
	p.Name, p.Protocol, p.BaseURL, p.Enabled, p.Remark = req.Name, req.Protocol, req.BaseURL, req.Enabled, req.Remark
	if req.APIKey != "" {
		enc, err := s.enc.Encrypt(req.APIKey)
		if err != nil {
			s.fail(c, 500, 50001, "密钥加密失败")
			return
		}
		p.APIKeyEnc = enc
		p.APIKeyMasked = crypto.MaskKey(req.APIKey)
	}
	if err := s.db.Save(&p).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "unique") {
			s.fail(c, 409, 40901, "供应商名称已存在")
			return
		}
		s.fail(c, 500, 50001, "更新失败: "+err.Error())
		return
	}
	s.recordOp(c, "update", "provider", p.Name, before, p)
	s.mgr.Bump()
	p.APIKeyEnc = ""
	s.ok(c, p)
}

func (s *Server) deleteProvider(c *gin.Context) {
	id := c.Param("id")
	var count int64
	s.db.Model(&model.AliasUpstream{}).Where("provider_id = ?", id).Count(&count)
	if count > 0 {
		s.fail(c, 409, 40901, "该供应商仍被模型别名引用，无法删除")
		return
	}
	var p model.Provider
	if err := s.db.First(&p, id).Error; err != nil {
		s.fail(c, 404, 40401, "供应商不存在")
		return
	}
	s.db.Delete(&p)
	s.recordOp(c, "delete", "provider", p.Name, p, nil)
	s.mgr.Bump()
	s.ok(c, nil)
}

// testProvider 测试连接：openai 系 GET /models；anthropic 发最小请求看是否 401。
func (s *Server) testProvider(c *gin.Context) {
	id := c.Param("id")
	var p model.Provider
	if err := s.db.First(&p, id).Error; err != nil {
		s.fail(c, 404, 40401, "供应商不存在")
		return
	}
	key, _ := s.enc.Decrypt(p.APIKeyEnc)
	start := time.Now()
	client := &http.Client{Timeout: 15 * time.Second}
	var req *http.Request
	var ok bool
	var msg string
	switch p.Protocol {
	case runtime.ProtoAnthropic:
		base := strings.TrimSuffix(strings.TrimRight(p.BaseURL, "/"), "/v1")
		if !strings.HasSuffix(base, "/v1") {
			base = p.BaseURL
			if !strings.HasSuffix(strings.TrimRight(base, "/"), "/v1") {
				base = strings.TrimRight(base, "/") + "/v1"
			}
		}
		req, _ = http.NewRequest("POST", strings.TrimRight(base, "/")+"/messages",
			strings.NewReader(`{"model":"claude-3-haiku-20240307","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("Content-Type", "application/json")
	default: // openai_chat / openai_responses
		base := strings.TrimRight(p.BaseURL, "/")
		if !strings.HasSuffix(base, "/v1") {
			base += "/v1"
		}
		req, _ = http.NewRequest("GET", base+"/models", nil)
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		s.ok(c, gin.H{"ok": false, "latency_ms": latency, "message": "连接失败: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		ok, msg = false, "认证失败（API Key 无效）"
	case resp.StatusCode < 500:
		ok, msg = true, "连接成功"
	default:
		ok, msg = false, "上游返回 " + resp.Status
	}
	s.ok(c, gin.H{"ok": ok, "latency_ms": latency, "message": msg})
}
