// models.go 模型别名管理（REQ-006）：CRUD + 上游权重 + 调用统计 + 热加载。
package admin

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"llmrouter/internal/model"
)

type upstreamOut struct {
	ID            uint   `json:"id"`
	ProviderID    uint   `json:"provider_id"`
	ProviderName  string `json:"provider_name"`
	UpstreamModel string `json:"upstream_model"`
	Weight        int    `json:"weight"`
}

type aliasOut struct {
	ID        uint          `json:"id"`
	Alias     string        `json:"alias"`
	Enabled   bool          `json:"enabled"`
	Remark    string        `json:"remark"`
	Upstreams []upstreamOut `json:"upstreams"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

func (s *Server) listModels(c *gin.Context) {
	page, size := parsePage(c)
	q := s.db.Model(&model.ModelAlias{})
	if kw := c.Query("keyword"); kw != "" {
		q = q.Where("alias LIKE ?", "%"+kw+"%")
	}
	var total int64
	q.Count(&total)
	var aliases []model.ModelAlias
	q.Preload("Upstreams").Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&aliases)
	// provider name 批量解析
	ids := map[uint]bool{}
	for _, a := range aliases {
		for _, u := range a.Upstreams {
			ids[u.ProviderID] = true
		}
	}
	names := map[uint]string{}
	if len(ids) > 0 {
		var idList []uint
		for id := range ids {
			idList = append(idList, id)
		}
		var ps []model.Provider
		s.db.Where("id IN ?", idList).Find(&ps)
		for _, p := range ps {
			names[p.ID] = p.Name
		}
	}
	out := make([]aliasOut, 0, len(aliases))
	for _, a := range aliases {
		ups := make([]upstreamOut, 0, len(a.Upstreams))
		for _, u := range a.Upstreams {
			ups = append(ups, upstreamOut{ID: u.ID, ProviderID: u.ProviderID,
				ProviderName: names[u.ProviderID], UpstreamModel: u.UpstreamModel, Weight: u.Weight})
		}
		out = append(out, aliasOut{ID: a.ID, Alias: a.Alias, Enabled: a.Enabled, Remark: a.Remark,
			Upstreams: ups, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt})
	}
	okPaged(c, out, int(total), page, size)
}

func (s *Server) createModel(c *gin.Context) {
	var req struct {
		Alias     string `json:"alias"`
		Enabled   bool   `json:"enabled"`
		Remark    string `json:"remark"`
		Upstreams []struct {
			ProviderID    uint   `json:"provider_id"`
			UpstreamModel string `json:"upstream_model"`
			Weight        int    `json:"weight"`
		} `json:"upstreams"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Alias == "" || len(req.Upstreams) == 0 {
		s.fail(c, 400, 40001, "请填写别名并至少添加一个上游")
		return
	}
	a := model.ModelAlias{Alias: req.Alias, Enabled: req.Enabled, Remark: req.Remark}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&a).Error; err != nil {
			return err
		}
		for _, u := range req.Upstreams {
			w := u.Weight
			if w <= 0 {
				w = 1
			}
			if err := tx.Create(&model.AliasUpstream{AliasID: a.ID, ProviderID: u.ProviderID,
				UpstreamModel: u.UpstreamModel, Weight: w}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.fail(c, 500, 50001, "创建失败: "+err.Error())
		return
	}
	s.recordOp(c, "create", "model_alias", req.Alias, nil, req)
	s.mgr.Bump()
	s.db.Preload("Upstreams").First(&a, a.ID)
	s.ok(c, aliasOut{ID: a.ID, Alias: a.Alias, Enabled: a.Enabled, Remark: a.Remark,
		Upstreams: toUpstreamOut(a.Upstreams, s), CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt})
}

func (s *Server) updateModel(c *gin.Context) {
	id := c.Param("id")
	var a model.ModelAlias
	if err := s.db.First(&a, id).Error; err != nil {
		s.fail(c, 404, 40401, "模型别名不存在")
		return
	}
	var req struct {
		Alias     string `json:"alias"`
		Enabled   bool   `json:"enabled"`
		Remark    string `json:"remark"`
		Upstreams []struct {
			ProviderID    uint   `json:"provider_id"`
			UpstreamModel string `json:"upstream_model"`
			Weight        int    `json:"weight"`
		} `json:"upstreams"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Alias == "" || len(req.Upstreams) == 0 {
		s.fail(c, 400, 40001, "请填写别名并至少添加一个上游")
		return
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		a.Alias, a.Enabled, a.Remark = req.Alias, req.Enabled, req.Remark
		if err := tx.Save(&a).Error; err != nil {
			return err
		}
		if err := tx.Where("alias_id = ?", a.ID).Delete(&model.AliasUpstream{}).Error; err != nil {
			return err
		}
		for _, u := range req.Upstreams {
			w := u.Weight
			if w <= 0 {
				w = 1
			}
			if err := tx.Create(&model.AliasUpstream{AliasID: a.ID, ProviderID: u.ProviderID,
				UpstreamModel: u.UpstreamModel, Weight: w}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.fail(c, 500, 50001, "更新失败: "+err.Error())
		return
	}
	s.recordOp(c, "update", "model_alias", req.Alias, nil, req)
	s.mgr.Bump()
	s.db.Preload("Upstreams").First(&a, a.ID)
	s.ok(c, aliasOut{ID: a.ID, Alias: a.Alias, Enabled: a.Enabled, Remark: a.Remark,
		Upstreams: toUpstreamOut(a.Upstreams, s), CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt})
}

func (s *Server) deleteModel(c *gin.Context) {
	id := c.Param("id")
	var a model.ModelAlias
	if err := s.db.First(&a, id).Error; err != nil {
		s.fail(c, 404, 40401, "模型别名不存在")
		return
	}
	s.db.Where("alias_id = ?", a.ID).Delete(&model.AliasUpstream{})
	s.db.Delete(&a)
	s.recordOp(c, "delete", "model_alias", a.Alias, a, nil)
	s.mgr.Bump()
	s.ok(c, nil)
}

func (s *Server) modelStats(c *gin.Context) {
	id := c.Param("id")
	var a model.ModelAlias
	if err := s.db.First(&a, id).Error; err != nil {
		s.fail(c, 404, 40401, "模型别名不存在")
		return
	}
	since7 := time.Now().AddDate(0, 0, -7)
	todayStart := time.Now().Truncate(24 * time.Hour)
	var calls7, callsToday, blocked7 int64
	var latSum int64
	s.db.Model(&model.CallLog{}).Where("model_alias = ? AND created_at >= ?", a.Alias, since7).Count(&calls7)
	s.db.Model(&model.CallLog{}).Where("model_alias = ? AND created_at >= ?", a.Alias, todayStart).Count(&callsToday)
	s.db.Model(&model.CallLog{}).Where("model_alias = ? AND blocked = ? AND created_at >= ?", a.Alias, true, since7).Count(&blocked7)
	s.db.Model(&model.CallLog{}).Where("model_alias = ? AND status = ? AND created_at >= ?", a.Alias, "ok", since7).
		Select("COALESCE(SUM(latency_ms),0)").Row().Scan(&latSum)
	avg := int64(0)
	if calls7 > 0 {
		avg = latSum / calls7
	}
	s.ok(c, gin.H{"calls_7d": calls7, "calls_today": callsToday, "blocked_7d": blocked7, "avg_latency_ms": avg})
}

func toUpstreamOut(ups []model.AliasUpstream, s *Server) []upstreamOut {
	ids := map[uint]bool{}
	for _, u := range ups {
		ids[u.ProviderID] = true
	}
	names := map[uint]string{}
	for id := range ids {
		var p model.Provider
		if err := s.db.First(&p, id).Error; err == nil {
			names[id] = p.Name
		}
	}
	out := make([]upstreamOut, 0, len(ups))
	for _, u := range ups {
		out = append(out, upstreamOut{ID: u.ID, ProviderID: u.ProviderID,
			ProviderName: names[u.ProviderID], UpstreamModel: u.UpstreamModel, Weight: u.Weight})
	}
	return out
}
