// providers.go 供应商管理（REQ-005）：CRUD + 测试连接 + 删除引用检查 + 热加载。
package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"llmrouter/internal/adapter"
	"llmrouter/internal/crypto"
	"llmrouter/internal/model"
)

type providerOut struct {
	model.Provider
	APIKey string `json:"-"` // 不输出明文
}

// validProtocol 校验供应商协议枚举（第九轮实测发现：错误值会一路存库，直到转发时才 502）。
func validProtocol(p string) bool {
	switch p {
	case adapter.ProtoOpenAIChat, adapter.ProtoOpenAIResponses, adapter.ProtoAnthropic:
		return true
	}
	return false
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
	if !validProtocol(req.Protocol) {
		s.fail(c, 400, 40001, "protocol 必须是 openai_chat / openai_responses / anthropic 之一")
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
	if !validProtocol(req.Protocol) {
		s.fail(c, 400, 40001, "protocol 必须是 openai_chat / openai_responses / anthropic 之一")
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

// testProvider 测试连接：向厂商 GET /v1/models，真实验证「端点可达 + API Key 有效 + 返回标准模型列表」。
// 严格判据：仅 HTTP 200 且响应体含 data/models 列表才算成功；401/403 报认证失败，404 报端点不存在，
// 其余状态原样返回——不再因任意 4xx 误报"连接成功"。成功时一并返回模型 ID 列表，供前端勾选导入。
func (s *Server) testProvider(c *gin.Context) {
	id := c.Param("id")
	var p model.Provider
	if err := s.db.First(&p, id).Error; err != nil {
		s.fail(c, 404, 40401, "供应商不存在")
		return
	}
	key, _ := s.enc.Decrypt(p.APIKeyEnc)

	// 归一化 base：保证以 /v1 结尾（与 adapter.BuildUpstreamRequest 一致），再追加 /models。
	base := strings.TrimRight(p.BaseURL, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	req, err := http.NewRequest(http.MethodGet, base+"/models", nil)
	if err != nil {
		s.fail(c, 400, 40001, "Base URL 非法: "+err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if p.Protocol == "anthropic" {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else { // openai_chat / openai_responses
		req.Header.Set("Authorization", "Bearer "+key)
	}

	start := time.Now()
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		s.ok(c, gin.H{"ok": false, "latency_ms": latency, "message": "连接失败: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	ok, msg := false, ""
	var models []string
	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		ok, msg = false, "认证失败（API Key 无效或无权限）"+errSnippet(body)
	case resp.StatusCode == 200:
		if ids, has := parseModelList(body); has {
			ok = true
			models = ids
			msg = fmt.Sprintf("连接成功，返回 %d 个模型", len(ids))
		} else {
			ok, msg = false, "端点可达，但响应非标准模型列表格式（Base URL 可能不正确）"
		}
	default: // 404 / 5xx / 其它
		if p.Protocol == "anthropic" {
			// /v1/models 不被该端点支持（常见于第三方 Anthropic 兼容代理未实现该端点）
			// → 回退到最小 /messages 请求，至少验证端点可达性与鉴权。
			ok, msg = probeAnthropicMessages(base, key)
		} else if resp.StatusCode == 404 {
			ok, msg = false, "端点不存在（404）：请检查 Base URL 是否正确"
		} else {
			ok, msg = false, fmt.Sprintf("上游返回 %s%s", resp.Status, errSnippet(body))
		}
	}
	s.ok(c, gin.H{"ok": ok, "latency_ms": latency, "message": msg, "models": models})
}

// probeAnthropicMessages 在 /v1/models 不可用时（多为第三方 Anthropic 兼容代理未实现该端点）
// 用最小 /messages 请求验证端点可达性与鉴权。返回 (ok, message)；不返回模型列表。
func probeAnthropicMessages(base, key string) (bool, string) {
	req, err := http.NewRequest(http.MethodPost, base+"/messages",
		strings.NewReader(`{"model":"claude-3-haiku-20240307","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		return false, "构造探测请求失败: " + err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, "连接失败: " + err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	switch {
	case resp.StatusCode == 200:
		return true, "连接成功（/v1/models 不支持，/messages 探测通过）"
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return false, "认证失败（API Key 无效或无权限）" + errSnippet(body)
	case resp.StatusCode == 400 || resp.StatusCode == 404:
		// /messages 是 Anthropic 核心端点；400/404 多为模型不存在或请求被拒，
		// 此类错误能返回即说明端点可达且鉴权已通过。
		return true, "连接成功（端点可达，/v1/models 不支持）"
	default:
		return false, fmt.Sprintf("上游返回 %s%s", resp.Status, errSnippet(body))
	}
}

// parseModelList 从 /models 响应中解析模型 ID 列表。
// 兼容 OpenAI/Anthropic 的 {"data":[{"id":"..."}]} 与部分第三方的 {"models":[...]}、顶层数组。
// has=false 表示响应不含可作为数组的标准模型列表字段（端点可疑）。
func parseModelList(body []byte) (ids []string, has bool) {
	type item struct {
		ID   string `json:"id"`
		Name string `json:"name"` // 个别厂商用 name 而非 id
	}
	collect := func(arr []item) []string {
		out := make([]string, 0, len(arr))
		for _, m := range arr {
			if m.ID != "" {
				out = append(out, m.ID)
			} else if m.Name != "" {
				out = append(out, m.Name)
			}
		}
		return out
	}
	var std struct {
		Data []item `json:"data"`
	}
	if err := json.Unmarshal(body, &std); err == nil && std.Data != nil {
		return collect(std.Data), true
	}
	var alt struct {
		Models []item `json:"models"`
	}
	if err := json.Unmarshal(body, &alt); err == nil && alt.Models != nil {
		return collect(alt.Models), true
	}
	var arr []item
	if err := json.Unmarshal(body, &arr); err == nil && arr != nil {
		return collect(arr), true
	}
	return nil, false
}

// importProviderModels 从厂商返回的模型列表批量创建模型别名（别名=上游模型名，单上游=本供应商）。
// 已存在的别名跳过并在 skipped_names 中返回，避免重复创建与手输——勾选即启用。
func (s *Server) importProviderModels(c *gin.Context) {
	id := c.Param("id")
	var p model.Provider
	if err := s.db.First(&p, id).Error; err != nil {
		s.fail(c, 404, 40401, "供应商不存在")
		return
	}
	var req struct {
		Models  []string `json:"models"`
		Enabled bool     `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Models) == 0 {
		s.fail(c, 400, 40001, "请至少选择一个模型")
		return
	}
	created, added, skippedNames := 0, 0, []string{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, raw := range req.Models {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}
			var a model.ModelAlias
			if err := tx.Where("alias = ?", name).First(&a).Error; err == nil {
				// 别名已存在 → 把本供应商作为额外上游加入（同供应商+同上游模型去重）。
				// 这样同一模型名可挂多个账号/平台，SLB 在多上游间轮询与故障转移。
				var dup model.AliasUpstream
				if derr := tx.Where("alias_id = ? AND provider_id = ? AND upstream_model = ?",
					a.ID, p.ID, name).First(&dup).Error; derr == nil {
					skippedNames = append(skippedNames, name)
					continue
				} else if derr != gorm.ErrRecordNotFound {
					return derr
				}
				if err := tx.Create(&model.AliasUpstream{AliasID: a.ID, ProviderID: p.ID,
					UpstreamModel: name, Weight: 1}).Error; err != nil {
					return err
				}
				added++
			} else if err == gorm.ErrRecordNotFound {
				// 别名不存在 → 新建 + 单上游指向本供应商
				a = model.ModelAlias{Alias: name, Enabled: req.Enabled}
				if err := tx.Create(&a).Error; err != nil {
					return err
				}
				if err := tx.Create(&model.AliasUpstream{AliasID: a.ID, ProviderID: p.ID,
					UpstreamModel: name, Weight: 1}).Error; err != nil {
					return err
				}
				created++
			} else {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.fail(c, 500, 50001, "导入失败: "+err.Error())
		return
	}
	s.recordOp(c, "import", "model_alias", p.Name, nil,
		gin.H{"created": created, "added": added, "skipped": len(skippedNames)})
	s.mgr.Bump()
	s.ok(c, gin.H{"created": created, "added": added,
		"skipped": len(skippedNames), "skipped_names": skippedNames})
}

// errSnippet 从错误响应中提取 OpenAI/Anthropic 风格的 error.message，否则取前 120 字。
func errSnippet(body []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		return "：" + e.Error.Message
	}
	s := strings.TrimSpace(string(body))
	if s == "" {
		return ""
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return "：" + s
}
