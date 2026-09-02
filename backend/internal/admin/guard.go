// guard.go 护栏管理（REQ-008/009/010）：敏感词、PII 规则、注入规则的 CRUD +
// 批量导入导出 + 测试工具 + 预置模板；变更后 Bump 热加载。
package admin

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/guard"
	"llmrouter/internal/model"
	"llmrouter/internal/runtime"
)

// ---- 敏感词 (REQ-008) ----

func (s *Server) listKeywords(c *gin.Context) {
	page, size := parsePage(c)
	q := s.db.Model(&model.GuardKeyword{})
	if kw := c.Query("keyword"); kw != "" {
		q = q.Where("word LIKE ?", "%"+kw+"%")
	}
	if v := c.Query("category"); v != "" {
		q = q.Where("category = ?", v)
	}
	if v := c.Query("action"); v != "" {
		q = q.Where("action = ?", v)
	}
	if v := c.Query("enabled"); v != "" {
		q = q.Where("enabled = ?", v == "true")
	}
	var total int64
	q.Count(&total)
	var rows []model.GuardKeyword
	q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows)
	s.okPaged(c, rows, int(total), page, size)
}

func (s *Server) createKeyword(c *gin.Context) {
	var req model.GuardKeyword
	if err := c.ShouldBindJSON(&req); err != nil || req.Word == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请填写敏感词")
		return
	}
	if err := validateRule(req.MatchMode, req.Word); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	if err := s.db.Create(&req).Error; err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "创建失败")
		return
	}
	s.recordOp(c, "create", "guard", fmt.Sprintf("keyword#%d", req.ID), nil, req)
	s.mgr.Bump()
	s.ok(c, req)
}

func (s *Server) updateKeyword(c *gin.Context) {
	id := c.Param("id")
	var kw model.GuardKeyword
	if err := s.db.First(&kw, id).Error; err != nil {
		s.fail(c, 404, 40401, "敏感词不存在")
		return
	}
	var req model.GuardKeyword
	if err := c.ShouldBindJSON(&req); err != nil || req.Word == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if err := validateRule(req.MatchMode, req.Word); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	before := kw
	kw.Word, kw.Category, kw.MatchMode, kw.Action, kw.Enabled = req.Word, req.Category, req.MatchMode, req.Action, req.Enabled
	if err := s.db.Save(&kw).Error; err != nil {
		s.fail(c, 500, 50001, "更新失败")
		return
	}
	s.recordOp(c, "update", "guard", fmt.Sprintf("keyword#%d", kw.ID), before, kw)
	s.mgr.Bump()
	s.ok(c, kw)
}

func (s *Server) deleteKeyword(c *gin.Context) {
	id := c.Param("id")
	var kw model.GuardKeyword
	if err := s.db.First(&kw, id).Error; err != nil {
		s.fail(c, 404, 40401, "敏感词不存在")
		return
	}
	s.db.Delete(&kw)
	s.recordOp(c, "delete", "guard", fmt.Sprintf("keyword#%d", kw.ID), kw, nil)
	s.mgr.Bump()
	s.ok(c, nil)
}

func (s *Server) batchKeywords(c *gin.Context) {
	var req struct {
		Items []model.GuardKeyword `json:"items"`
		Mode  string               `json:"mode"` // merge|replace
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if req.Mode == "replace" {
		s.db.Where("1 = 1").Delete(&model.GuardKeyword{})
	}
	var created int
	for _, it := range req.Items {
		if it.Word == "" {
			continue
		}
		it.ID = 0
		if err := s.db.Create(&it).Error; err == nil {
			created++
		}
	}
	s.recordOp(c, "create", "guard", fmt.Sprintf("keyword batch (%d)", created), nil, gin.H{"count": created})
	s.mgr.Bump()
	s.ok(c, gin.H{"created": created, "skipped": len(req.Items) - created})
}

// exportKeywords CSV 导出（text/csv，不走 envelope）。
func (s *Server) exportKeywords(c *gin.Context) {
	var rows []model.GuardKeyword
	s.db.Order("id").Find(&rows)
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	w.Write([]string{"word", "category", "match_mode", "action", "enabled"})
	for _, r := range rows {
		w.Write([]string{r.Word, r.Category, r.MatchMode, r.Action, strconv.FormatBool(r.Enabled)})
	}
	w.Flush()
	c.Header("Content-Disposition", "attachment; filename=keywords.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte("﻿"+sb.String()))
}

// importKeywords 接收 CSV 文本（word,category,match_mode,action 四列，无表头或有表头均可）。
func (s *Server) importKeywords(c *gin.Context) {
	var req struct {
		CSV string `json:"csv"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.CSV) == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请粘贴 CSV 内容")
		return
	}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(req.CSV, "﻿")))
	records, err := r.ReadAll()
	if err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "CSV 解析失败: "+err.Error())
		return
	}
	var created, skipped int
	for _, rec := range records {
		if len(rec) < 1 || strings.TrimSpace(rec[0]) == "" || rec[0] == "word" {
			continue
		}
		word := strings.TrimSpace(rec[0])
		cat, mode, act := "custom", "contains", "block"
		en := true
		if len(rec) > 1 && rec[1] != "" {
			cat = strings.TrimSpace(rec[1])
		}
		if len(rec) > 2 && rec[2] != "" {
			mode = strings.TrimSpace(rec[2])
		}
		if len(rec) > 3 && rec[3] != "" {
			act = strings.TrimSpace(rec[3])
		}
		if len(rec) > 4 && (rec[4] == "false" || rec[4] == "0") {
			en = false
		}
		if err := validateRule(mode, word); err != nil {
			skipped++
			continue
		}
		if err := s.db.Create(&model.GuardKeyword{Word: word, Category: cat, MatchMode: mode, Action: act, Enabled: en}).Error; err == nil {
			created++
		} else {
			skipped++
		}
	}
	s.recordOp(c, "create", "guard", "keyword csv import", nil, gin.H{"created": created})
	s.mgr.Bump()
	s.ok(c, gin.H{"created": created, "skipped": skipped})
}

func (s *Server) testKeywords(c *gin.Context) {
	var req struct{ Text string }
	if err := c.ShouldBindJSON(&req); err != nil || req.Text == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请输入测试文本")
		return
	}
	snap := s.mgr.Get()
	var hits []gin.H
	blocked := false
	for _, kw := range snap.Keywords {
		if keywordHit(kw, req.Text) {
			hits = append(hits, gin.H{"type": "keyword", "value": kw.Raw.Word,
				"category": kw.Raw.Category, "action": kw.Raw.Action})
			if kw.Raw.Action == "block" {
				blocked = true
			}
		}
	}
	s.ok(c, gin.H{"blocked": blocked, "hits": hits})
}

// ---- PII 规则 (REQ-009) ----

func (s *Server) listPiiRules(c *gin.Context) {
	page, size := parsePage(c)
	q := s.db.Model(&model.PIIRule{})
	if kw := c.Query("keyword"); kw != "" {
		q = q.Where("name LIKE ?", "%"+kw+"%")
	}
	var total int64
	q.Count(&total)
	var rows []model.PIIRule
	q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows)
	s.okPaged(c, rows, int(total), page, size)
}

func (s *Server) createPiiRule(c *gin.Context) {
	var req model.PIIRule
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.Pattern == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请填写规则名称与正则")
		return
	}
	if err := validateRegex(req.Pattern); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	if err := s.db.Create(&req).Error; err != nil {
		s.fail(c, 500, 50001, "创建失败")
		return
	}
	s.recordOp(c, "create", "guard", fmt.Sprintf("pii#%d", req.ID), nil, req)
	s.mgr.Bump()
	s.ok(c, req)
}

func (s *Server) updatePiiRule(c *gin.Context) {
	id := c.Param("id")
	var r model.PIIRule
	if err := s.db.First(&r, id).Error; err != nil {
		s.fail(c, 404, 40401, "规则不存在")
		return
	}
	var req model.PIIRule
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.Pattern == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if err := validateRegex(req.Pattern); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	before := r
	r.Name, r.Category, r.Pattern, r.Replacement, r.Action, r.Enabled =
		req.Name, req.Category, req.Pattern, req.Replacement, req.Action, req.Enabled
	if err := s.db.Save(&r).Error; err != nil {
		s.fail(c, 500, 50001, "更新失败")
		return
	}
	s.recordOp(c, "update", "guard", fmt.Sprintf("pii#%d", r.ID), before, r)
	s.mgr.Bump()
	s.ok(c, r)
}

func (s *Server) deletePiiRule(c *gin.Context) {
	id := c.Param("id")
	var r model.PIIRule
	if err := s.db.First(&r, id).Error; err != nil {
		s.fail(c, 404, 40401, "规则不存在")
		return
	}
	s.db.Delete(&r)
	s.recordOp(c, "delete", "guard", fmt.Sprintf("pii#%d", r.ID), r, nil)
	s.mgr.Bump()
	s.ok(c, nil)
}

func (s *Server) piiTemplates(c *gin.Context) {
	tpl := []map[string]any{
		{"name": "手机号", "category": "phone", "pattern": `1[3-9]\d{9}`, "replacement": "[PHONE]", "action": "mask"},
		{"name": "身份证号", "category": "id_card", "pattern": `\d{17}[\dXx]`, "replacement": "[ID_CARD]", "action": "mask"},
		{"name": "邮箱", "category": "email", "pattern": `[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`, "replacement": "[EMAIL]", "action": "mask"},
		{"name": "银行卡号", "category": "bank_card", "pattern": `\d{16,19}`, "replacement": "[BANK_CARD]", "action": "mask"},
		{"name": "地址关键词", "category": "address", "pattern": `\d{1,6}(省|市|区|县|镇|村|路|街|号)\w{0,20}`, "replacement": "[ADDRESS]", "action": "mask"},
	}
	s.ok(c, tpl)
}

func (s *Server) testPii(c *gin.Context) {
	var req struct{ Text string }
	if err := c.ShouldBindJSON(&req); err != nil || req.Text == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请输入测试文本")
		return
	}
	snap := s.mgr.Get()
	vd := guard.CheckInput(snap, req.Text)
	// 提取 PII 类命中
	var findings []gin.H
	for _, f := range vd.Findings {
		if f.Type == "pii" {
			findings = append(findings, gin.H{"rule": f.Value, "category": strings.TrimPrefix(f.Category, "pii."), "sample": maskSample(f.Value)})
		}
	}
	s.ok(c, gin.H{"blocked": vd.Blocked, "findings": findings, "masked_text": vd.Text})
}

// ---- 注入规则 (REQ-010) ----

func (s *Server) listInjectionRules(c *gin.Context) {
	page, size := parsePage(c)
	q := s.db.Model(&model.InjectionRule{})
	if kw := c.Query("keyword"); kw != "" {
		q = q.Where("name LIKE ?", "%"+kw+"%")
	}
	var total int64
	q.Count(&total)
	var rows []model.InjectionRule
	q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows)
	s.okPaged(c, rows, int(total), page, size)
}

func (s *Server) createInjectionRule(c *gin.Context) {
	var req model.InjectionRule
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.Pattern == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请填写规则名称与模式")
		return
	}
	if req.MatchMode == "regex" {
		if err := validateRegex(req.Pattern); err != nil {
			s.fail(c, http.StatusBadRequest, 40001, err.Error())
			return
		}
	}
	if err := s.db.Create(&req).Error; err != nil {
		s.fail(c, 500, 50001, "创建失败")
		return
	}
	s.recordOp(c, "create", "guard", fmt.Sprintf("injection#%d", req.ID), nil, req)
	s.mgr.Bump()
	s.ok(c, req)
}

func (s *Server) updateInjectionRule(c *gin.Context) {
	id := c.Param("id")
	var r model.InjectionRule
	if err := s.db.First(&r, id).Error; err != nil {
		s.fail(c, 404, 40401, "规则不存在")
		return
	}
	var req model.InjectionRule
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.Pattern == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if req.MatchMode == "regex" {
		if err := validateRegex(req.Pattern); err != nil {
			s.fail(c, http.StatusBadRequest, 40001, err.Error())
			return
		}
	}
	before := r
	r.Name, r.Pattern, r.MatchMode, r.Action, r.Enabled = req.Name, req.Pattern, req.MatchMode, req.Action, req.Enabled
	if err := s.db.Save(&r).Error; err != nil {
		s.fail(c, 500, 50001, "更新失败")
		return
	}
	s.recordOp(c, "update", "guard", fmt.Sprintf("injection#%d", r.ID), before, r)
	s.mgr.Bump()
	s.ok(c, r)
}

func (s *Server) deleteInjectionRule(c *gin.Context) {
	id := c.Param("id")
	var r model.InjectionRule
	if err := s.db.First(&r, id).Error; err != nil {
		s.fail(c, 404, 40401, "规则不存在")
		return
	}
	s.db.Delete(&r)
	s.recordOp(c, "delete", "guard", fmt.Sprintf("injection#%d", r.ID), r, nil)
	s.mgr.Bump()
	s.ok(c, nil)
}

func (s *Server) injectionTemplates(c *gin.Context) {
	tpl := []map[string]any{
		{"name": "忽略系统指令", "pattern": `ignore (all |the )?previous instructions`, "match_mode": "regex", "action": "block"},
		{"name": "越狱提示", "pattern": `do anything now|DAN mode|jailbreak`, "match_mode": "regex", "action": "block"},
		{"name": "伪装身份", "pattern": `你现在是|忽略之前的`, "match_mode": "contains", "action": "warn"},
		{"name": "指令覆盖", "pattern": `(override|ignore|forget).{0,10}(instructions|system prompt)`, "match_mode": "regex", "action": "block"},
	}
	s.ok(c, tpl)
}

// ---- 工具 ----

func validateRule(mode, value string) error {
	if mode == "regex" {
		return validateRegex(value)
	}
	return nil
}

func validateRegex(pat string) error {
	if pat == "" {
		return fmt.Errorf("正则不能为空")
	}
	if _, err := regexp.Compile(pat); err != nil {
		return fmt.Errorf("正则非法: %v", err)
	}
	return nil
}

func keywordHit(kw runtime.CompiledKeyword, text string) bool {
	switch kw.Raw.MatchMode {
	case "regex":
		return kw.Re != nil && kw.Re.MatchString(text)
	case "exact":
		return strings.EqualFold(strings.TrimSpace(text), strings.TrimSpace(kw.Raw.Word))
	default:
		return strings.Contains(strings.ToLower(text), strings.ToLower(kw.Raw.Word))
	}
}

func maskSample(name string) string {
	if len(name) > 20 {
		return name[:20] + "…"
	}
	return name
}
