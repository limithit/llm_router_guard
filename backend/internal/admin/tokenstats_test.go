package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/model"
)

// seedAPIKey 建一个 API Key，返回 ID。
func (s *Server) seedAPIKey(t *testing.T, name string) uint {
	t.Helper()
	k := model.APIKey{Name: name, KeyHash: "hash-" + name, KeyMasked: "sk-***" + name, Enabled: true}
	if err := s.db.Create(&k).Error; err != nil {
		t.Fatalf("create apikey: %v", err)
	}
	return k.ID
}

// seedCallLog 建一条调用日志。
func (s *Server) seedCallLog(t *testing.T, keyID uint, keyLabel, alias string, at time.Time, prompt, completion int) {
	t.Helper()
	cl := model.CallLog{
		RequestID: "req-" + at.Format("20060102150405") + "-" + keyLabel + "-" + alias,
		CreatedAt: at, APIKeyID: keyID, APIKeyLabel: keyLabel, Protocol: model.ProtoOpenAIChat,
		ModelAlias: alias, Status: "ok", PromptTokens: prompt, CompletionTokens: completion,
	}
	if err := s.db.Create(&cl).Error; err != nil {
		t.Fatalf("create calllog: %v", err)
	}
}

type tokenStatsResp struct {
	Code int `json:"code"`
	Data struct {
		Range struct {
			Start       string `json:"start"`
			End         string `json:"end"`
			Granularity string `json:"granularity"`
		} `json:"range"`
		Summary struct {
			Calls           int64 `json:"calls"`
			PromptTokens    int64 `json:"prompt_tokens"`
			CompletionTokns int64 `json:"completion_tokens"`
			TotalTokens     int64 `json:"total_tokens"`
			KeysWithUsage   int   `json:"keys_with_usage"`
			TotalKeys       int   `json:"total_keys"`
		} `json:"summary"`
		ByKey []struct {
			APIKeyID         uint   `json:"api_key_id"`
			APIKeyLabel      string `json:"api_key_label"`
			Calls            int64  `json:"calls"`
			PromptTokens     int64  `json:"prompt_tokens"`
			CompletionTokens int64  `json:"completion_tokens"`
			TotalTokens      int64  `json:"total_tokens"`
		} `json:"by_key"`
		ByModel []struct {
			Model            string `json:"model"`
			Calls            int64  `json:"calls"`
			PromptTokens     int64  `json:"prompt_tokens"`
			CompletionTokens int64  `json:"completion_tokens"`
			TotalTokens      int64  `json:"total_tokens"`
		} `json:"by_model"`
		Trend []struct {
			Bucket           string `json:"bucket"`
			Calls            int64  `json:"calls"`
			PromptTokens     int64  `json:"prompt_tokens"`
			CompletionTokens int64  `json:"completion_tokens"`
			TotalTokens      int64  `json:"total_tokens"`
		} `json:"trend"`
	} `json:"data"`
}

func callTokenStats(t *testing.T, s *Server, query string) tokenStatsResp {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/token-stats", s.tokenStats)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/token-stats"+query, nil)
	r.ServeHTTP(w, req)
	var resp tokenStatsResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	return resp
}

func TestTokenStats_Aggregation(t *testing.T) {
	s := newTestServer(t)

	keyA := s.seedAPIKey(t, "team-a")
	keyB := s.seedAPIKey(t, "team-b")
	keyC := s.seedAPIKey(t, "team-c-zero-usage") // 零用量 Key，也应出现在 by_key

	base := time.Now().Add(-2 * 24 * time.Hour)
	// key-a: gpt-4o 两条（100+200, 50+50），claude 一条（10+10）
	s.seedCallLog(t, keyA, "team-a", "gpt-4o", base, 100, 200)
	s.seedCallLog(t, keyA, "team-a", "gpt-4o", base.Add(time.Hour), 50, 50)
	s.seedCallLog(t, keyA, "team-a", "claude-3", base.Add(2*time.Hour), 10, 10)
	// key-b: gpt-4o 一条（7+3）
	s.seedCallLog(t, keyB, "team-b", "gpt-4o", base.Add(3*time.Hour), 7, 3)
	_ = keyC

	start := base.Add(-time.Hour).Format("2006-01-02 15:04:05")
	end := base.Add(24 * time.Hour).Format("2006-01-02 15:04:05")
	resp := callTokenStats(t, s, "?start="+urlEncode(start)+"&end="+urlEncode(end))

	if resp.Code != 0 {
		t.Fatalf("code = %d, want 0", resp.Code)
	}
	sum := resp.Data.Summary
	if sum.Calls != 4 {
		t.Errorf("calls = %d, want 4", sum.Calls)
	}
	if sum.PromptTokens != 167 {
		t.Errorf("prompt_tokens = %d, want 167", sum.PromptTokens)
	}
	if sum.CompletionTokns != 263 {
		t.Errorf("completion_tokens = %d, want 263", sum.CompletionTokns)
	}
	if sum.TotalTokens != 430 {
		t.Errorf("total_tokens = %d, want 430", sum.TotalTokens)
	}
	if sum.KeysWithUsage != 2 {
		t.Errorf("keys_with_usage = %d, want 2", sum.KeysWithUsage)
	}
	if sum.TotalKeys != 3 {
		t.Errorf("total_keys = %d, want 3", sum.TotalKeys)
	}

	// by_key 应包含 3 个 Key（含零用量），按 total 降序：team-a(420) > team-b(10) > team-c(0)
	if len(resp.Data.ByKey) != 3 {
		t.Fatalf("by_key len = %d, want 3", len(resp.Data.ByKey))
	}
	first := resp.Data.ByKey[0]
	if first.APIKeyLabel != "team-a" || first.TotalTokens != 420 || first.Calls != 3 {
		t.Errorf("by_key[0] = %+v, want team-a 420 tokens 3 calls", first)
	}
	if resp.Data.ByKey[1].APIKeyLabel != "team-b" || resp.Data.ByKey[1].TotalTokens != 10 {
		t.Errorf("by_key[1] = %+v, want team-b 10 tokens", resp.Data.ByKey[1])
	}
	if resp.Data.ByKey[2].APIKeyLabel != "team-c-zero-usage" || resp.Data.ByKey[2].TotalTokens != 0 {
		t.Errorf("by_key[2] = %+v, want team-c zero usage", resp.Data.ByKey[2])
	}

	// by_model：gpt-4o(410 = team-a 400 + team-b 10) > claude-3(20)
	if len(resp.Data.ByModel) != 2 {
		t.Fatalf("by_model len = %d, want 2", len(resp.Data.ByModel))
	}
	if resp.Data.ByModel[0].Model != "gpt-4o" || resp.Data.ByModel[0].TotalTokens != 410 {
		t.Errorf("by_model[0] = %+v, want gpt-4o 410", resp.Data.ByModel[0])
	}
	if resp.Data.ByModel[1].Model != "claude-3" || resp.Data.ByModel[1].TotalTokens != 20 {
		t.Errorf("by_model[1] = %+v, want claude-3 20", resp.Data.ByModel[1])
	}

	// trend：日粒度 → 全部记录同一天一个桶
	if len(resp.Data.Trend) != 1 {
		t.Fatalf("trend len = %d, want 1 (same day bucket)", len(resp.Data.Trend))
	}
	if resp.Data.Trend[0].TotalTokens != 430 {
		t.Errorf("trend[0].total = %d, want 430", resp.Data.Trend[0].TotalTokens)
	}
}

func TestTokenStats_FilterByAPIKey(t *testing.T) {
	s := newTestServer(t)
	keyA := s.seedAPIKey(t, "team-a")
	keyB := s.seedAPIKey(t, "team-b")
	base := time.Now().Add(-time.Hour)
	s.seedCallLog(t, keyA, "team-a", "gpt-4o", base, 100, 100)
	s.seedCallLog(t, keyB, "team-b", "gpt-4o", base.Add(time.Minute), 7, 3)

	start := base.Add(-time.Hour).Format("2006-01-02 15:04:05")
	end := base.Add(time.Hour).Format("2006-01-02 15:04:05")
	resp := callTokenStats(t, s, "?start="+urlEncode(start)+"&end="+urlEncode(end)+"&api_key_id="+uintToStr(keyA))

	if resp.Data.Summary.TotalTokens != 200 {
		t.Errorf("filtered total = %d, want 200 (only team-a)", resp.Data.Summary.TotalTokens)
	}
	if len(resp.Data.ByKey) != 1 || resp.Data.ByKey[0].APIKeyLabel != "team-a" {
		t.Errorf("by_key should contain only team-a, got %+v", resp.Data.ByKey)
	}
}

func TestTokenStats_FilterByModel(t *testing.T) {
	s := newTestServer(t)
	keyA := s.seedAPIKey(t, "team-a")
	base := time.Now().Add(-time.Hour)
	s.seedCallLog(t, keyA, "team-a", "gpt-4o", base, 100, 100)
	s.seedCallLog(t, keyA, "team-a", "claude-3", base.Add(time.Minute), 7, 3)

	start := base.Add(-time.Hour).Format("2006-01-02 15:04:05")
	end := base.Add(time.Hour).Format("2006-01-02 15:04:05")
	resp := callTokenStats(t, s, "?start="+urlEncode(start)+"&end="+urlEncode(end)+"&model=gpt")

	if resp.Data.Summary.TotalTokens != 200 {
		t.Errorf("model-filtered total = %d, want 200", resp.Data.Summary.TotalTokens)
	}
	if len(resp.Data.ByModel) != 1 || resp.Data.ByModel[0].Model != "gpt-4o" {
		t.Errorf("by_model should contain only gpt-4o, got %+v", resp.Data.ByModel)
	}
}

func TestTokenStats_HistoricalTrend(t *testing.T) {
	s := newTestServer(t)
	keyA := s.seedAPIKey(t, "team-a")
	now := time.Now()
	// 3 天各一条，验证按日分桶的历史序列
	for i := 3; i >= 1; i-- {
		day := now.AddDate(0, 0, -i).Truncate(24 * time.Hour).Add(12 * time.Hour)
		s.seedCallLog(t, keyA, "team-a", "gpt-4o", day, i*10, i*5)
	}

	start := now.AddDate(0, 0, -5).Format("2006-01-02")
	end := now.Format("2006-01-02 15:04:05")
	resp := callTokenStats(t, s, "?start="+start+"&end="+urlEncode(end))

	if len(resp.Data.Trend) != 3 {
		t.Fatalf("trend len = %d, want 3 day buckets; trend=%+v", len(resp.Data.Trend), resp.Data.Trend)
	}
	// 升序按日期：3 天前 i=3 → total 45；随后 30；1 天前 i=1 → 15
	want := []int64{45, 30, 15}
	for i, w := range want {
		if resp.Data.Trend[i].TotalTokens != w {
			t.Errorf("trend[%d].total = %d, want %d", i, resp.Data.Trend[i].TotalTokens, w)
		}
	}
	// 桶格式应为 YYYY-MM-DD
	if len(resp.Data.Trend[0].Bucket) != 10 || resp.Data.Trend[0].Bucket[4] != '-' {
		t.Errorf("bucket format = %q, want YYYY-MM-DD", resp.Data.Trend[0].Bucket)
	}
}

func TestTokenStats_HourGranularity(t *testing.T) {
	s := newTestServer(t)
	keyA := s.seedAPIKey(t, "team-a")
	base := time.Now().Truncate(time.Hour)
	s.seedCallLog(t, keyA, "team-a", "gpt-4o", base.Add(-2*time.Hour), 10, 10)
	s.seedCallLog(t, keyA, "team-a", "gpt-4o", base.Add(-1*time.Hour), 20, 20)
	s.seedCallLog(t, keyA, "team-a", "gpt-4o", base.Add(-1*time.Hour+time.Minute), 5, 5)

	start := base.Add(-3 * time.Hour).Format("2006-01-02 15:04:05")
	end := base.Format("2006-01-02 15:04:05")
	resp := callTokenStats(t, s, "?start="+urlEncode(start)+"&end="+urlEncode(end)+"&granularity=hour")

	if resp.Data.Range.Granularity != "hour" {
		t.Errorf("granularity = %q, want hour", resp.Data.Range.Granularity)
	}
	if len(resp.Data.Trend) != 2 {
		t.Fatalf("trend len = %d, want 2 hour buckets; trend=%+v", len(resp.Data.Trend), resp.Data.Trend)
	}
	if resp.Data.Trend[0].TotalTokens != 20 || resp.Data.Trend[1].TotalTokens != 50 {
		t.Errorf("hour buckets = %+v, want 20 then 50", resp.Data.Trend)
	}
	if !strings.HasSuffix(resp.Data.Trend[0].Bucket, ":00") {
		t.Errorf("hour bucket format = %q, want '... HH:00'", resp.Data.Trend[0].Bucket)
	}
}

func TestTokenStats_DefaultRange30Days(t *testing.T) {
	s := newTestServer(t)
	keyA := s.seedAPIKey(t, "team-a")
	// 60 天前的记录不应命中默认近 30 天窗口
	s.seedCallLog(t, keyA, "team-a", "gpt-4o", time.Now().AddDate(0, 0, -60), 100, 100)
	// 昨天的记录应命中
	s.seedCallLog(t, keyA, "team-a", "gpt-4o", time.Now().AddDate(0, 0, -1), 7, 3)

	resp := callTokenStats(t, s, "")
	if resp.Data.Summary.TotalTokens != 10 {
		t.Errorf("default-range total = %d, want 10 (only yesterday)", resp.Data.Summary.TotalTokens)
	}
}

func TestTokenStats_DeletedKeyHistory(t *testing.T) {
	s := newTestServer(t)
	keyA := s.seedAPIKey(t, "team-a")
	base := time.Now().Add(-time.Hour)
	s.seedCallLog(t, keyA, "team-a", "gpt-4o", base, 100, 100)
	// 删除 Key，历史日志仍在
	s.db.Delete(&model.APIKey{}, keyA)

	start := base.Add(-time.Hour).Format("2006-01-02 15:04:05")
	end := base.Add(time.Hour).Format("2006-01-02 15:04:05")
	resp := callTokenStats(t, s, "?start="+urlEncode(start)+"&end="+urlEncode(end))

	if len(resp.Data.ByKey) != 1 {
		t.Fatalf("by_key len = %d, want 1 (deleted key history preserved)", len(resp.Data.ByKey))
	}
	if resp.Data.ByKey[0].TotalTokens != 200 {
		t.Errorf("deleted key total = %d, want 200", resp.Data.ByKey[0].TotalTokens)
	}
	if resp.Data.ByKey[0].APIKeyLabel != "team-a" {
		t.Errorf("label = %q, want historical label team-a", resp.Data.ByKey[0].APIKeyLabel)
	}
}

func TestExportTokenStats_CSV(t *testing.T) {
	s := newTestServer(t)
	keyA := s.seedAPIKey(t, "team-a")
	base := time.Now().Add(-time.Hour)
	s.seedCallLog(t, keyA, "team-a", "gpt-4o", base, 100, 200)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/token-stats/export", s.exportTokenStats)
	w := httptest.NewRecorder()
	start := base.Add(-time.Hour).Format("2006-01-02 15:04:05")
	end := base.Add(time.Hour).Format("2006-01-02 15:04:05")
	req := httptest.NewRequest(http.MethodGet, "/token-stats/export?start="+urlEncode(start)+"&end="+urlEncode(end), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "\uFEFF") {
		t.Error("CSV should start with BOM for Excel")
	}
	if !strings.Contains(body, "api_key_id,api_key,calls,prompt_tokens,completion_tokens,total_tokens") {
		t.Errorf("CSV header missing; body = %q", body)
	}
	if !strings.Contains(body, "team-a") || !strings.Contains(body, "300") {
		t.Errorf("CSV should contain team-a row with total 300; body = %q", body)
	}
}

// ---- 小工具 ----

func urlEncode(s string) string {
	return strings.NewReplacer(" ", "%20", ":", "%3A").Replace(s)
}

func uintToStr(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
