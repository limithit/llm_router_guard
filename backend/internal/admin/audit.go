// audit.go 调用审计（REQ-015）与操作审计（REQ-003）查询 + CSV 导出。
package admin

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"llmrouter/internal/model"
)

func parseTimeQ(v string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02 15:04:05", v); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// callLogOut 列表行（预览截断）。
type callLogOut struct {
	model.CallLog
	InputPreview  string `json:"input_preview"`
	OutputPreview string `json:"output_preview"`
	Upstream      string `json:"upstream"`
}

func (s *Server) listCallLogs(c *gin.Context) {
	page, size := parsePage(c)
	q := s.callLogQuery(c)
	var total int64
	q.Count(&total)
	var rows []model.CallLog
	q.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&rows)
	out := make([]callLogOut, 0, len(rows))
	for _, r := range rows {
		out = append(out, callLogOut{CallLog: r,
			InputPreview: truncate(r.InputText, 200), OutputPreview: truncate(r.OutputText, 200),
			Upstream: upstreamLabel(r.UpstreamProvider, r.UpstreamModel)})
	}
	okPaged(c, out, int(total), page, size)
}

func (s *Server) callLogDetail(c *gin.Context) {
	reqID := c.Param("request_id")
	var r model.CallLog
	if err := s.db.Where("request_id = ?", reqID).First(&r).Error; err != nil {
		s.fail(c, http.StatusNotFound, 40401, "调用记录不存在")
		return
	}
	s.ok(c, gin.H{
		"request_id":        r.RequestID,
		"created_at":        r.CreatedAt,
		"api_key_label":     r.APIKeyLabel,
		"protocol":          r.Protocol,
		"model_alias":       r.ModelAlias,
		"upstream":          upstreamLabel(r.UpstreamProvider, r.UpstreamModel),
		"input_preview":     truncate(r.InputText, 200),
		"output_preview":    truncate(r.OutputText, 200),
		"input_text":        r.InputText,
		"output_text":       r.OutputText,
		"prompt_tokens":     r.PromptTokens,
		"completion_tokens": r.CompletionTokens,
		"latency_ms":        r.LatencyMs,
		"status":            r.Status,
		"blocked":           r.Blocked,
		"block_category":    r.BlockCategory,
		"block_reason":      r.BlockReason,
		"error_msg":         r.ErrorMsg,
		"guard_findings":    r.GuardFindings,
	})
}

func (s *Server) exportCallLogs(c *gin.Context) {
	var rows []model.CallLog
	s.callLogQuery(c).Order("created_at DESC").Limit(10000).Find(&rows)
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	w.Write([]string{"request_id", "created_at", "api_key", "protocol", "model", "upstream", "tokens", "latency_ms", "status", "blocked", "reason"})
	for _, r := range rows {
		w.Write([]string{r.RequestID, r.CreatedAt.Format(time.RFC3339), r.APIKeyLabel, r.Protocol,
			r.ModelAlias, upstreamLabel(r.UpstreamProvider, r.UpstreamModel),
			strconv.Itoa(r.PromptTokens + r.CompletionTokens),
			strconv.FormatInt(r.LatencyMs, 10), r.Status, strconv.FormatBool(r.Blocked), r.BlockReason})
	}
	w.Flush()
	c.Header("Content-Disposition", "attachment; filename=call-logs.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte("\uFEFF"+sb.String()))
}

// callLogQuery 构造带筛选条件的查询。
func (s *Server) callLogQuery(c *gin.Context) *gorm.DB {
	q := s.db.Model(&model.CallLog{})
	if st, ok := parseTimeQ(c.Query("start")); ok {
		q = q.Where("created_at >= ?", st)
	}
	if en, ok := parseTimeQ(c.Query("end")); ok {
		q = q.Where("created_at <= ?", en)
	}
	if v := c.Query("api_key_id"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q = q.Where("api_key_id = ?", n)
		}
	}
	if v := c.Query("model"); v != "" {
		q = q.Where("model_alias LIKE ?", "%"+v+"%")
	}
	if v := c.Query("status"); v != "" {
		q = q.Where("status = ?", v)
	}
	if v := c.Query("blocked"); v != "" {
		q = q.Where("blocked = ?", v == "true")
	}
	if v := c.Query("category"); v != "" {
		q = q.Where("block_category LIKE ?", "%"+v+"%")
	}
	return q
}

// ---- 操作审计 (REQ-003) ----

func (s *Server) listOpLogs(c *gin.Context) {
	page, size := parsePage(c)
	q := s.opLogFiltered(c)
	var total int64
	q.Count(&total)
	var rows []model.OperationLog
	q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows)
	okPaged(c, rows, int(total), page, size)
}

func (s *Server) opLogFiltered(c *gin.Context) *gorm.DB {
	q := s.db.Model(&model.OperationLog{})
	if st, ok := parseTimeQ(c.Query("start")); ok {
		q = q.Where("created_at >= ?", st)
	}
	if en, ok := parseTimeQ(c.Query("end")); ok {
		q = q.Where("created_at <= ?", en)
	}
	if v := c.Query("operator"); v != "" {
		q = q.Where("operator = ?", v)
	}
	if v := c.Query("module"); v != "" {
		q = q.Where("module = ?", v)
	}
	if v := c.Query("action"); v != "" {
		q = q.Where("action = ?", v)
	}
	return q
}

func (s *Server) exportOpLogs(c *gin.Context) {
	var rows []model.OperationLog
	s.opLogFiltered(c).Order("id DESC").Limit(10000).Find(&rows)
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	w.Write([]string{"id", "time", "operator", "action", "module", "target", "ip", "effective", "before", "after"})
	for _, r := range rows {
		w.Write([]string{strconv.Itoa(int(r.ID)), r.CreatedAt.Format(time.RFC3339), r.Operator,
			r.Action, r.Module, r.Target, r.IP, strconv.FormatBool(r.Effective), r.BeforeJSON, r.AfterJSON})
	}
	w.Flush()
	c.Header("Content-Disposition", "attachment; filename=operation-logs.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte("\uFEFF"+sb.String()))
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func upstreamLabel(p, m string) string {
	if p == "" && m == "" {
		return ""
	}
	if m == "" {
		return p
	}
	return p + "/" + m
}
