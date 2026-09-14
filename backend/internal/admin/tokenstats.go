// tokenstats.go API Key Token 用量统计看板：按 Key / 模型聚合 + 时间趋势分桶，
// 支持时间范围历史查询与 CSV 导出。数据源为 call_logs（已脱敏的审计表）。
package admin

import (
	"encoding/csv"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"llmrouter/internal/model"
)

// ---- 聚合中间结构（列别名 ↔ 字段对应；GORM Scan 不展开嵌入结构，故全部展平） ----

type tokenAgg struct {
	Prompt     int64 `gorm:"column:prompt"`
	Completion int64 `gorm:"column:completion"`
	CallCount  int64 `gorm:"column:call_count"`
}

func (a tokenAgg) total() int64 { return a.Prompt + a.Completion }

type keyTokenRow struct {
	APIKeyID    uint   `gorm:"column:api_key_id"`
	APIKeyLabel string `gorm:"column:api_key_label"`
	CallCount   int64  `gorm:"column:call_count"`
	Prompt      int64  `gorm:"column:prompt"`
	Completion  int64  `gorm:"column:completion"`
}

func (r keyTokenRow) total() int64 { return r.Prompt + r.Completion }

type modelTokenRow struct {
	Model      string `gorm:"column:model"`
	CallCount  int64  `gorm:"column:call_count"`
	Prompt     int64  `gorm:"column:prompt"`
	Completion int64  `gorm:"column:completion"`
}

func (r modelTokenRow) total() int64 { return r.Prompt + r.Completion }

type trendRow struct {
	Bucket     string `gorm:"column:bucket"`
	CallCount  int64  `gorm:"column:call_count"`
	Prompt     int64  `gorm:"column:prompt"`
	Completion int64  `gorm:"column:completion"`
}

func (r trendRow) total() int64 { return r.Prompt + r.Completion }

const tokenSumSelect = "COUNT(*) AS call_count, COALESCE(SUM(prompt_tokens),0) AS prompt, COALESCE(SUM(completion_tokens),0) AS completion"

// dateBucketExpr 跨数据库时间分桶表达式（sqlite / mysql / postgres）。
// 第十五轮起 sqlite 统一存 UTC 文本：strftime 按 UTC 读取后经 'localtime'
// 修饰符落到服务器本地时区桶，与 mysql DATE_FORMAT / postgres to_char
// （均按会话本地时区）行为一致，前端按本地日历日展示。
func dateBucketExpr(dialect, granularity string) string {
	if granularity == "hour" {
		switch dialect {
		case "mysql":
			return "DATE_FORMAT(created_at, '%Y-%m-%d %H:00')"
		case "postgres":
			return "to_char(created_at, 'YYYY-MM-DD HH24:00')"
		default: // sqlite
			return "strftime('%Y-%m-%d %H:00', created_at, 'localtime')"
		}
	}
	switch dialect {
	case "mysql":
		return "DATE_FORMAT(created_at, '%Y-%m-%d')"
	case "postgres":
		return "to_char(created_at, 'YYYY-MM-DD')"
	default: // sqlite
		return "strftime('%Y-%m-%d', created_at, 'localtime')"
	}
}

// tokenStatsParams 解析筛选参数并给出默认时间范围（近 30 天）。
type tokenStatsParams struct {
	start       time.Time
	end         time.Time
	apiKeyID    uint
	modelAlias  string
	granularity string
}

func (s *Server) parseTokenStatsParams(c *gin.Context) tokenStatsParams {
	p := tokenStatsParams{
		start:       time.Now().AddDate(0, 0, -30).Truncate(24 * time.Hour),
		end:         time.Now(),
		granularity: "day",
	}
	if v, ok := parseTimeQ(c.Query("start")); ok {
		p.start = v
	}
	if v, ok := parseTimeQ(c.Query("end")); ok {
		p.end = v
	}
	if v := c.Query("api_key_id"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			p.apiKeyID = uint(n)
		}
	}
	p.modelAlias = c.Query("model")
	if g := c.Query("granularity"); g == "hour" {
		p.granularity = "hour"
	}
	if p.end.Before(p.start) {
		p.end = p.start
	}
	return p
}

// applyTokenFilters 给查询追加统一的时间/Key/模型筛选条件。
func applyTokenFilters(q *gorm.DB, p tokenStatsParams) *gorm.DB {
	q = q.Where("created_at >= ? AND created_at <= ?", p.start, p.end)
	if p.apiKeyID != 0 {
		q = q.Where("api_key_id = ?", p.apiKeyID)
	}
	if p.modelAlias != "" {
		q = q.Where("model_alias LIKE ? ESCAPE '\\'", likeArg(p.modelAlias))
	}
	return q
}

func (s *Server) tokenStats(c *gin.Context) {
	p := s.parseTokenStatsParams(c)
	dialect := s.db.Dialector.Name()

	// 1) 全局汇总
	var summary tokenAgg
	applyTokenFilters(s.db.Model(&model.CallLog{}), p).
		Select(tokenSumSelect).Scan(&summary)

	// 2) 按 Key 聚合（仅统计有调用记录的 Key）
	var keyAggs []keyTokenRow
	applyTokenFilters(s.db.Model(&model.CallLog{}), p).
		Select("api_key_id, MAX(api_key_label) AS api_key_label, " + tokenSumSelect).
		Group("api_key_id").Scan(&keyAggs)

	// 3) 合并全部 Key（含零用量），保证“每个 Key 消耗了多少”完整可见
	aggByID := make(map[uint]keyTokenRow, len(keyAggs))
	for _, r := range keyAggs {
		aggByID[r.APIKeyID] = r
	}
	var allKeys []model.APIKey
	keyQ := s.db.Model(&model.APIKey{})
	if p.apiKeyID != 0 {
		keyQ = keyQ.Where("id = ?", p.apiKeyID)
	}
	keyQ.Find(&allKeys)

	byKey := make([]gin.H, 0, len(allKeys)+len(keyAggs))
	seen := map[uint]bool{}
	for _, k := range allKeys {
		seen[k.ID] = true
		row := aggByID[k.ID] // 不存在则为零值
		byKey = append(byKey, gin.H{
			"api_key_id": k.ID, "api_key_label": k.Name,
			"calls": row.CallCount, "prompt_tokens": row.Prompt,
			"completion_tokens": row.Completion, "total_tokens": row.total(),
		})
	}
	// 已删除但仍有历史日志的 Key
	for _, r := range keyAggs {
		if seen[r.APIKeyID] {
			continue
		}
		label := r.APIKeyLabel
		if label == "" {
			label = "(已删除 Key)"
		}
		byKey = append(byKey, gin.H{
			"api_key_id": r.APIKeyID, "api_key_label": label,
			"calls": r.CallCount, "prompt_tokens": r.Prompt,
			"completion_tokens": r.Completion, "total_tokens": r.total(),
		})
	}
	sort.Slice(byKey, func(i, j int) bool {
		return byKey[i]["total_tokens"].(int64) > byKey[j]["total_tokens"].(int64)
	})
	keysWithUsage := len(keyAggs)

	// 4) 按模型聚合
	var modelAggs []modelTokenRow
	applyTokenFilters(s.db.Model(&model.CallLog{}), p).
		Select("model_alias AS model, " + tokenSumSelect).
		Group("model_alias").Scan(&modelAggs)
	sort.Slice(modelAggs, func(i, j int) bool { return modelAggs[i].total() > modelAggs[j].total() })
	byModel := make([]gin.H, 0, len(modelAggs))
	for _, m := range modelAggs {
		byModel = append(byModel, gin.H{
			"model": m.Model, "calls": m.CallCount, "prompt_tokens": m.Prompt,
			"completion_tokens": m.Completion, "total_tokens": m.total(),
		})
	}

	// 5) 趋势分桶
	var trend []trendRow
	expr := dateBucketExpr(dialect, p.granularity)
	applyTokenFilters(s.db.Model(&model.CallLog{}), p).
		Select(expr + " AS bucket, " + tokenSumSelect).
		Group("bucket").Order("bucket ASC").Scan(&trend)
	trendOut := make([]gin.H, 0, len(trend))
	for _, t := range trend {
		trendOut = append(trendOut, gin.H{
			"bucket": t.Bucket, "calls": t.CallCount, "prompt_tokens": t.Prompt,
			"completion_tokens": t.Completion, "total_tokens": t.total(),
		})
	}

	s.ok(c, gin.H{
		"range": gin.H{"start": p.start.Format(time.RFC3339), "end": p.end.Format(time.RFC3339),
			"granularity": p.granularity},
		"summary": gin.H{
			"calls": summary.CallCount, "prompt_tokens": summary.Prompt,
			"completion_tokens": summary.Completion, "total_tokens": summary.total(),
			"keys_with_usage": keysWithUsage, "total_keys": len(allKeys),
		},
		"by_key":   byKey,
		"by_model": byModel,
		"trend":    trendOut,
	})
}

// exportTokenStats 按 Key 用量表导出 CSV（不包 envelope）。
func (s *Server) exportTokenStats(c *gin.Context) {
	p := s.parseTokenStatsParams(c)

	var keyAggs []keyTokenRow
	applyTokenFilters(s.db.Model(&model.CallLog{}), p).
		Select("api_key_id, MAX(api_key_label) AS api_key_label, " + tokenSumSelect).
		Group("api_key_id").Scan(&keyAggs)

	aggByID := make(map[uint]keyTokenRow, len(keyAggs))
	for _, r := range keyAggs {
		aggByID[r.APIKeyID] = r
	}
	var allKeys []model.APIKey
	s.db.Model(&model.APIKey{}).Find(&allKeys)

	// 合并（含零用量 + 已删除 Key），按 total 降序
	type mergedRow struct {
		id    uint
		label string
		row   keyTokenRow
	}
	seen := map[uint]bool{}
	rows := make([]mergedRow, 0, len(allKeys)+len(keyAggs))
	for _, k := range allKeys {
		seen[k.ID] = true
		rows = append(rows, mergedRow{id: k.ID, label: k.Name, row: aggByID[k.ID]})
	}
	for _, r := range keyAggs {
		if seen[r.APIKeyID] {
			continue
		}
		lbl := r.APIKeyLabel
		if lbl == "" {
			lbl = "(已删除 Key)"
		}
		rows = append(rows, mergedRow{id: r.APIKeyID, label: lbl, row: r})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].row.total() > rows[j].row.total() })

	var sb strings.Builder
	w := csv.NewWriter(&sb)
	w.Write([]string{"api_key_id", "api_key", "calls", "prompt_tokens", "completion_tokens", "total_tokens"})
	for _, r := range rows {
		w.Write([]string{
			strconv.Itoa(int(r.id)), csvCell(r.label),
			strconv.FormatInt(r.row.CallCount, 10),
			strconv.FormatInt(r.row.Prompt, 10),
			strconv.FormatInt(r.row.Completion, 10),
			strconv.FormatInt(r.row.total(), 10),
		})
	}
	w.Flush()
	c.Header("Content-Disposition", "attachment; filename=token-stats.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte("\uFEFF"+sb.String()))
}
