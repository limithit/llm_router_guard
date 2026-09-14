// quota.go 配额（REQ-012）与速率限制（REQ-013）管理。
package admin

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/model"
	"llmrouter/internal/quota"
)

// validateQuotaReq M-17：枚举字段严格化（脏 period 静默按 day 重置、脏 over_action
// 静默按 reject 处理都是隐性行为漂移），上限必须为正。
func validateQuotaReq(q *model.Quota) error {
	if q.LimitValue <= 0 {
		return fmt.Errorf("配额上限必须大于 0")
	}
	switch q.QuotaType {
	case "requests", "tokens":
	default:
		return fmt.Errorf("配额类型仅支持 requests/tokens")
	}
	switch q.Period {
	case "day", "week", "month":
	default:
		return fmt.Errorf("重置周期仅支持 day/week/month")
	}
	switch q.OverAction {
	case "reject", "degrade", "":
	default:
		return fmt.Errorf("超限动作仅支持 reject/degrade")
	}
	if q.OverAction == "degrade" && q.DegradeAlias == "" {
		return fmt.Errorf("降级动作必须指定目标别名")
	}
	if q.ModelAlias == "" {
		q.ModelAlias = "*"
	}
	return nil
}

// ---- 配额 ----

type quotaOut struct {
	model.Quota
	APIKeyLabel string `json:"api_key_label"`
}

func (s *Server) listQuotas(c *gin.Context) {
	page, size := parsePage(c)
	var total int64
	s.db.Model(&model.Quota{}).Count(&total)
	var rows []model.Quota
	s.db.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows)
	keyNames := s.apiKeyNames()
	out := make([]quotaOut, 0, len(rows))
	now := time.Now()
	for _, r := range rows {
		label := keyNames[r.APIKeyID]
		if label == "" && r.APIKeyID != 0 {
			label = "(已删除 Key)"
		}
		// 周期已过 → 显示时按重置后处理
		if now.After(r.ResetAt) {
			r.UsedValue = 0
		}
		out = append(out, quotaOut{Quota: r, APIKeyLabel: label})
	}
	okPaged(c, out, int(total), page, size)
}

func (s *Server) createQuota(c *gin.Context) {
	var req model.Quota
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if err := validateQuotaReq(&req); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	req.ResetAt = quota.NextReset(req.Period, time.Now())
	if err := s.db.Create(&req).Error; err != nil {
		s.fail(c, 500, 50001, "创建失败")
		return
	}
	s.recordOp(c, "create", "quota", req.ModelAlias, nil, req)
	s.mgr.Bump()
	s.ok(c, req)
}

func (s *Server) updateQuota(c *gin.Context) {
	id := c.Param("id")
	var q model.Quota
	if err := s.db.First(&q, id).Error; err != nil {
		s.fail(c, 404, 40401, "配额不存在")
		return
	}
	var req model.Quota
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	before := q
	q.APIKeyID, q.ModelAlias, q.QuotaType, q.Period, q.LimitValue =
		req.APIKeyID, req.ModelAlias, req.QuotaType, req.Period, req.LimitValue
	q.OverAction, q.DegradeAlias, q.Enabled = req.OverAction, req.DegradeAlias, req.Enabled
	if err := validateQuotaReq(&q); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	if err := s.db.Save(&q).Error; err != nil {
		s.fail(c, 500, 50001, "更新失败")
		return
	}
	s.recordOp(c, "update", "quota", q.ModelAlias, before, q)
	s.mgr.Bump()
	s.ok(c, q)
}

func (s *Server) deleteQuota(c *gin.Context) {
	id := c.Param("id")
	var q model.Quota
	if err := s.db.First(&q, id).Error; err != nil {
		s.fail(c, 404, 40401, "配额不存在")
		return
	}
	s.db.Delete(&q)
	s.recordOp(c, "delete", "quota", q.ModelAlias, q, nil)
	s.mgr.Bump()
	s.ok(c, nil)
}

func (s *Server) resetQuota(c *gin.Context) {
	id := c.Param("id")
	var q model.Quota
	if err := s.db.First(&q, id).Error; err != nil {
		s.fail(c, 404, 40401, "配额不存在")
		return
	}
	q.UsedValue = 0
	q.ResetAt = quota.NextReset(q.Period, time.Now())
	s.db.Save(&q)
	s.recordOp(c, "reset", "quota", q.ModelAlias, nil, q)
	s.mgr.Bump()
	s.ok(c, q)
}

func (s *Server) batchQuotas(c *gin.Context) {
	var req struct {
		Items []model.Quota `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if len(req.Items) > maxBatchItems { // M-25：批量上限
		s.fail(c, http.StatusBadRequest, 40001, fmt.Sprintf("单次批量上限 %d 条", maxBatchItems))
		return
	}
	var created int
	for _, it := range req.Items {
		it.ID = 0
		if it.ModelAlias == "" {
			it.ModelAlias = "*"
		}
		if it.LimitValue <= 0 || validateQuotaReq(&it) != nil {
			continue
		}
		it.ResetAt = quota.NextReset(it.Period, time.Now())
		if err := s.db.Create(&it).Error; err == nil {
			created++
		}
	}
	s.recordOp(c, "create", "quota", "batch", nil, gin.H{"created": created})
	s.mgr.Bump()
	s.ok(c, gin.H{"created": created})
}

// ---- 速率限制 (REQ-013) ----

type rateLimitOut struct {
	model.RateLimitRule
	APIKeyLabel string `json:"api_key_label"`
}

func (s *Server) listRateLimits(c *gin.Context) {
	page, size := parsePage(c)
	var total int64
	s.db.Model(&model.RateLimitRule{}).Count(&total)
	var rows []model.RateLimitRule
	s.db.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows)
	keyNames := s.apiKeyNames()
	out := make([]rateLimitOut, 0, len(rows))
	for _, r := range rows {
		label := keyNames[r.APIKeyID]
		if label == "" && r.APIKeyID != 0 {
			label = "(已删除 Key)"
		}
		out = append(out, rateLimitOut{RateLimitRule: r, APIKeyLabel: label})
	}
	okPaged(c, out, int(total), page, size)
}

func (s *Server) createRateLimit(c *gin.Context) {
	var req model.RateLimitRule
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if req.WindowSeconds <= 0 || req.WindowSeconds > 86400 || req.MaxRequests <= 0 || req.MaxRequests > 10_000_000 {
		s.fail(c, http.StatusBadRequest, 40001, "窗口需为 1-86400 秒、上限需为 1-10000000")
		return
	}
	if req.ModelAlias == "" {
		req.ModelAlias = "*"
	}
	if err := s.db.Create(&req).Error; err != nil {
		s.fail(c, 500, 50001, "创建失败")
		return
	}
	s.recordOp(c, "create", "rate_limit", req.ModelAlias, nil, req)
	s.mgr.Bump()
	s.ok(c, req)
}

func (s *Server) updateRateLimit(c *gin.Context) {
	id := c.Param("id")
	var r model.RateLimitRule
	if err := s.db.First(&r, id).Error; err != nil {
		s.fail(c, 404, 40401, "限流规则不存在")
		return
	}
	var req model.RateLimitRule
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	before := r
	r.APIKeyID, r.ModelAlias, r.WindowSeconds, r.MaxRequests, r.Enabled =
		req.APIKeyID, req.ModelAlias, req.WindowSeconds, req.MaxRequests, req.Enabled
	// M-20：更新此前毫无校验——负窗口/零上限可直接把该 Key 全量锁死或全量放开
	if r.WindowSeconds <= 0 || r.WindowSeconds > 86400 || r.MaxRequests <= 0 || r.MaxRequests > 10_000_000 {
		s.fail(c, http.StatusBadRequest, 40001, "窗口需为 1-86400 秒、上限需为 1-10000000")
		return
	}
	if r.ModelAlias == "" {
		r.ModelAlias = "*"
	}
	if err := s.db.Save(&r).Error; err != nil {
		s.fail(c, 500, 50001, "更新失败")
		return
	}
	s.recordOp(c, "update", "rate_limit", r.ModelAlias, before, r)
	s.mgr.Bump()
	s.ok(c, r)
}

func (s *Server) deleteRateLimit(c *gin.Context) {
	id := c.Param("id")
	var r model.RateLimitRule
	if err := s.db.First(&r, id).Error; err != nil {
		s.fail(c, 404, 40401, "限流规则不存在")
		return
	}
	s.db.Delete(&r)
	s.recordOp(c, "delete", "rate_limit", r.ModelAlias, r, nil)
	s.mgr.Bump()
	s.ok(c, nil)
}

// apiKeyNames 返回 id → name 映射。
func (s *Server) apiKeyNames() map[uint]string {
	var keys []model.APIKey
	s.db.Find(&keys)
	m := map[uint]string{}
	for _, k := range keys {
		m[k.ID] = k.Name
	}
	return m
}
