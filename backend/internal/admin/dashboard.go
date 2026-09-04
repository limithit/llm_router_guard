// dashboard.go 概览 Dashboard（REQ-016）：今日指标 / 7 天趋势 / 模型排行 /
// 拦截类别分布 / 上游健康 / 配额使用率 / 系统状态。
package admin

import (
	"runtime"
	"time"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/model"
)

// ---- 概览 Dashboard (REQ-016) ----

func (s *Server) dashboard(c *gin.Context) {
	snap := s.mgr.Get()
	now := time.Now()
	today := now.Truncate(24 * time.Hour)

	var todayCalls, todaySuccess, todayBlocked, todayErrors int64
	var todayLat int64
	s.db.Model(&model.CallLog{}).Where("created_at >= ?", today).Count(&todayCalls)
	s.db.Model(&model.CallLog{}).Where("created_at >= ? AND status = ?", today, "ok").Count(&todaySuccess)
	s.db.Model(&model.CallLog{}).Where("created_at >= ? AND status = ?", today, "blocked").Count(&todayBlocked)
	s.db.Model(&model.CallLog{}).Where("created_at >= ? AND status = ?", today, "error").Count(&todayErrors)
	s.db.Model(&model.CallLog{}).Where("created_at >= ? AND status = ?", today, "ok").
		Select("COALESCE(SUM(latency_ms),0)").Row().Scan(&todayLat)
	avgLat := int64(0)
	if todaySuccess > 0 {
		avgLat = todayLat / todaySuccess
	}

	// 7 天趋势
	var trend []gin.H
	for i := 6; i >= 0; i-- {
		day := today.AddDate(0, 0, -i)
		next := day.AddDate(0, 0, 1)
		var calls, blocked int64
		s.db.Model(&model.CallLog{}).Where("created_at >= ? AND created_at < ?", day, next).Count(&calls)
		s.db.Model(&model.CallLog{}).Where("created_at >= ? AND created_at < ? AND blocked = ?", day, next, true).Count(&blocked)
		trend = append(trend, gin.H{"date": day.Format("2006-01-02"), "calls": calls, "blocked": blocked})
	}

	// 模型排行（7 天）
	type modelRank struct {
		Model string
		Calls int64
	}
	var top []modelRank
	s.db.Model(&model.CallLog{}).
		Select("model_alias as model, COUNT(*) as calls").
		Where("created_at >= ?", today.AddDate(0, 0, -7)).
		Group("model_alias").Order("calls DESC").Limit(10).Scan(&top)

	// 拦截类别分布（7 天）
	type catRank struct {
		Category string
		Count    int64
	}
	var cats []catRank
	s.db.Model(&model.CallLog{}).
		Select("block_category as category, COUNT(*) as count").
		Where("created_at >= ? AND blocked = ?", today.AddDate(0, 0, -7), true).
		Group("block_category").Order("count DESC").Scan(&cats)

	// 配额使用率
	var quotas []model.Quota
	s.db.Where("enabled = ?", true).Find(&quotas)
	quotaUsage := make([]gin.H, 0, len(quotas))
	for _, q := range quotas {
		used := q.UsedValue
		if time.Now().After(q.ResetAt) {
			used = 0
		}
		name := q.ModelAlias + " / " + q.Period + "-" + q.QuotaType
		quotaUsage = append(quotaUsage, gin.H{"name": name, "used": used, "limit": q.LimitValue})
	}

	// 上游健康
	health := s.bl.HealthList(snap)
	upstreams := make([]gin.H, 0, len(health))
	for _, h := range health {
		upstreams = append(upstreams, gin.H{
			"provider": h.Provider, "protocol": h.Protocol,
			"healthy": h.Healthy, "fail_count": h.FailCount})
	}

	s.ok(c, gin.H{
		"today": gin.H{
			"calls": todayCalls, "success": todaySuccess,
			"blocked": todayBlocked, "errors": todayErrors, "avg_latency_ms": avgLat},
		"trend_7d": trend,
		"top_models": func() []gin.H {
			out := make([]gin.H, 0, len(top))
			for _, t := range top {
				out = append(out, gin.H{"model": t.Model, "calls": t.Calls})
			}
			return out
		}(),
		"block_categories": func() []gin.H {
			out := make([]gin.H, 0, len(cats))
			for _, ct := range cats {
				out = append(out, gin.H{"category": ct.Category, "count": ct.Count})
			}
			return out
		}(),
		"upstream_health": upstreams,
		"quota_usage":     quotaUsage,
		"system": gin.H{"version": "1.0.0", "uptime_seconds": int64(s.mx.Uptime().Seconds()),
			"config_status": s.mgr.Status, "go_version": "go1.24"},
	})
}

// status.go REQ-018
func (s *Server) status(c *gin.Context) {
	snap := s.mgr.Get()
	health := s.bl.HealthList(snap)
	ups := make([]gin.H, 0, len(health))
	for _, h := range health {
		ups = append(ups, gin.H{"provider": h.Provider, "healthy": h.Healthy,
			"fail_count": h.FailCount, "last_error": h.LastError})
	}
	recent := make([]gin.H, 0, len(s.mx.RecentErrors()))
	for _, e := range s.mx.RecentErrors() {
		recent = append(recent, gin.H{"time": e.Time.Format("2006-01-02T15:04:05Z07:00"),
			"request_id": e.RequestID, "message": e.Message})
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	s.ok(c, gin.H{
		"running": true, "uptime_seconds": int64(s.mx.Uptime().Seconds()),
		"version":         "1.0.0",
		"memory_alloc_kb": ms.Alloc / 1024, "cpu_percent": 0.0,
		"goroutines": runtime.NumGoroutine(), "open_connections": s.mx.Conns(),
		"upstreams":     ups,
		"qps_by_model":  s.mx.QPS(),
		"recent_errors": recent,
		"config_status": gin.H{
			"version": s.mgr.Get().Version, "last_loaded_at": s.mgr.Get().LoadedAt, "status": s.mgr.Status},
	})
}
