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

// parseTimeQ 解析查询参数中的时间，并统一归一化到服务器本地时区。
//
// 为什么必须归一化：SQLite 把 datetime 列存为文本（本地时区墙钟，如
// "2026-09-03 14:39:47.791308+08:00"），比较按字典序进行。前端传的是 UTC
// ISO 串（"…Z"），若按 UTC 时区绑定（"…+00:00"），墙钟部分与库存值相差
// 时区偏移（如 8 小时），导致 `created_at <= end` 把最近 8 小时的记录全部
// 排除——成功日志明明存在，Token 统计却“看不到”。归一化到 time.Local 后，
// 绑定串与库存串同为本地墙钟，字典序比较才正确。MySQL/Postgres 为真实时间
// 类型，此归一化无副作用。
func parseTimeQ(v string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.In(time.Local), true // 带时区（如前端 UTC "Z"）→ 换算到服务器本地时区
	}
	for _, f := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(f, v, time.Local); err == nil {
			return t, true // 不带时区 → 按服务器本地时区解释
		}
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
		"client_request_id": r.ClientRequestID,
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
		// M-05：request_id/model/reason 均含租户可控成分，公式前缀必须转义
		w.Write([]string{r.RequestID, r.CreatedAt.Format(time.RFC3339), csvCell(r.APIKeyLabel), r.Protocol,
			csvCell(r.ModelAlias), csvCell(upstreamLabel(r.UpstreamProvider, r.UpstreamModel)),
			strconv.Itoa(r.PromptTokens + r.CompletionTokens),
			strconv.FormatInt(r.LatencyMs, 10), r.Status, strconv.FormatBool(r.Blocked), csvCell(r.BlockReason)})
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
		q = q.Where("model_alias LIKE ? ESCAPE '\\'", likeArg(v))
	}
	if v := c.Query("status"); v != "" {
		q = q.Where("status = ?", v)
	}
	if v := c.Query("blocked"); v != "" {
		q = q.Where("blocked = ?", v == "true")
	}
	if v := c.Query("category"); v != "" {
		q = q.Where("block_category LIKE ? ESCAPE '\\'", likeArg(v))
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
		w.Write([]string{strconv.Itoa(int(r.ID)), r.CreatedAt.Format(time.RFC3339), csvCell(r.Operator),
			csvCell(r.Action), csvCell(r.Module), csvCell(r.Target), csvCell(r.IP), strconv.FormatBool(r.Effective), csvCell(r.BeforeJSON), csvCell(r.AfterJSON)})
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
