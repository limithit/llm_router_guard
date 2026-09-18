package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/adapter"
	"llmrouter/internal/metrics"
	"llmrouter/internal/model"
	"llmrouter/internal/quota"
	"llmrouter/internal/runtime"
	"llmrouter/internal/settings"
	"llmrouter/internal/slb"
)

// newTestServer 构造一个仅满足网关测试需要的最小 Server。
func newTestServer() *Server {
	return &Server{
		mx: metrics.New(),
		bl: slb.New(),
		qm: quota.NewQuotaManager(nil), // 测试用例快照中无配额规则，nil db 安全
	}
}

// guardSnap 返回一个启用关键词 "Google"（contains/block）的输出护栏快照。
func guardSnap(strategy string) *runtime.Snapshot {
	return &runtime.Snapshot{
		General: settings.General{GuardEnabled: true},
		Output: settings.OutputFilter{
			Enabled:              true,
			ViolationStrategy:    strategy,
			SafeMessage:          "[filtered]",
			StreamChunkThreshold: 256,
		},
		Keywords: []runtime.CompiledKeyword{
			{Raw: model.GuardKeyword{Word: "Google", MatchMode: "contains", Action: "block", Category: "custom", Enabled: true}},
		},
	}
}

// sseMockResponse 把若干 SSE data 行包装成 *http.Response。
func sseMockResponse(lines ...string) *http.Response {
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(strings.Join(lines, "\n"))),
	}
}

func newStreamCtxAndAp() (*gin.Context, *httptest.ResponseRecorder, *auditParams) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", nil)
	return c, w, &auditParams{}
}

// TestForwardStream_ShortOutput_Guarded 验证核心回归点：
// 流式输出短于阈值（默认 256）时，流结束前必须有一次兜底检测。
func TestForwardStream_ShortOutput_Guarded(t *testing.T) {
	s := newTestServer()
	c, w, ap := newStreamCtxAndAp()

	resp := sseMockResponse(
		`data: {"id":"x","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"content":"Google"}}]}`,
		`data: {"id":"x","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	)

	cr := &adapter.CanonicalRequest{Model: "gpt-4", Stream: true}
	up := runtime.ResolvedUpstream{ProviderID: 1, ProviderName: "mock", Protocol: adapter.ProtoOpenAIChat, UpstreamModel: "gpt-4"}
	snap := guardSnap("replace")

	fr := s.forwardStream(c, snap, adapter.ProtoOpenAIChat, cr, up, resp, "req-1", ap)

	if fr != frDone {
		t.Fatalf("forwardStream result = %v, want frDone", fr)
	}
	if ap.status != "blocked" {
		t.Fatalf("ap.status = %q, want blocked; short stream should be caught by final guard check", ap.status)
	}
	if !strings.Contains(w.Body.String(), "[filtered]") {
		t.Errorf("response should contain replacement message; body:\n%s", w.Body.String())
	}
}

// TestForwardStream_LongOutput_ThresholdGuarded 验证超过阈值的流式输出
// 仍能被阈值化检测正常拦截。
func TestForwardStream_LongOutput_ThresholdGuarded(t *testing.T) {
	s := newTestServer()
	c, w, ap := newStreamCtxAndAp()

	// 构造一段超过阈值 256 的文本，且包含关键词 Google。
	longPrefix := strings.Repeat("a", 260)
	resp := sseMockResponse(
		`data: {"id":"x","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"content":"`+longPrefix+`"}}]}`,
		`data: {"id":"x","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"content":" Google"}}]}`,
		`data: {"id":"x","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	)

	cr := &adapter.CanonicalRequest{Model: "gpt-4", Stream: true}
	up := runtime.ResolvedUpstream{ProviderID: 1, ProviderName: "mock", Protocol: adapter.ProtoOpenAIChat, UpstreamModel: "gpt-4"}
	snap := guardSnap("block")

	fr := s.forwardStream(c, snap, adapter.ProtoOpenAIChat, cr, up, resp, "req-2", ap)

	if fr != frFatal {
		t.Fatalf("forwardStream result = %v, want frFatal", fr)
	}
	if ap.status != "blocked" {
		t.Fatalf("ap.status = %q, want blocked", ap.status)
	}
	if !strings.Contains(w.Body.String(), "output blocked by content policy") {
		t.Errorf("response should contain block error event; body:\n%s", w.Body.String())
	}
}

// TestForwardStream_ShortOutputClean_NoFalsePositive 验证正常短输出不被误拦截。
func TestForwardStream_ShortOutputClean_NoFalsePositive(t *testing.T) {
	s := newTestServer()
	c, w, ap := newStreamCtxAndAp()

	resp := sseMockResponse(
		`data: {"id":"x","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"content":"hello world"}}]}`,
		`data: {"id":"x","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	)

	cr := &adapter.CanonicalRequest{Model: "gpt-4", Stream: true}
	up := runtime.ResolvedUpstream{ProviderID: 1, ProviderName: "mock", Protocol: adapter.ProtoOpenAIChat, UpstreamModel: "gpt-4"}
	snap := guardSnap("replace")

	fr := s.forwardStream(c, snap, adapter.ProtoOpenAIChat, cr, up, resp, "req-3", ap)

	if fr != frDone {
		t.Fatalf("forwardStream result = %v, want frDone", fr)
	}
	if ap.status != "ok" {
		t.Fatalf("ap.status = %q, want ok; clean output should not be blocked", ap.status)
	}
	if strings.Contains(w.Body.String(), "[filtered]") {
		t.Errorf("response should not contain replacement message; body:\n%s", w.Body.String())
	}
}

// TestResolveRequestID SEC-04 策略：主 ID 恒为服务端 32-hex 新生成；
// 客户端头只作为清洗后的 client_request_id 留存（不复活为可注入/可撞唯一键的主 ID）。
func TestResolveRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mk := func(h string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", nil)
		if h != "" {
			c.Request.Header.Set("X-Request-ID", h)
		}
		return c
	}
	sid, cid := resolveRequestID(mk("trace-abc-123"))
	if len(sid) != 32 {
		t.Errorf("server ID must always be freshly generated 32-hex, got %q", sid)
	}
	if cid != "trace-abc-123" {
		t.Errorf("client ID should be kept verbatim (sanitized), got %q", cid)
	}
	if sid2, _ := resolveRequestID(mk("trace-abc-123")); sid2 == sid {
		t.Error("two generated server IDs must differ")
	}
	// 换行/控制字符注入 → 替换为空格（防日志伪造）
	_, cid = resolveRequestID(mk("x\r\n[access] 200 FAKE"))
	if strings.ContainsAny(cid, "\r\n") {
		t.Errorf("control chars must be neutralized, got %q", cid)
	}
	// 超长截断到 128 字节
	long := strings.Repeat("x", 200)
	_, cid = resolveRequestID(mk(long))
	if len(cid) != 128 {
		t.Errorf("client ID must truncate to 128, got %d", len(cid))
	}
	if _, cid = resolveRequestID(mk("   ")); cid != "" {
		t.Errorf("blank header should yield empty client ID, got %q", cid)
	}
}

// TestForward_UpstreamReceivesRequestID 转发上游必须携带 X-Request-ID。
func TestForward_UpstreamReceivesRequestID(t *testing.T) {
	s := newTestServer()
	s.httpClient = &http.Client{Timeout: 5 * time.Second} // newTestServer 不带 httpClient
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", nil)
	c.Request.Header.Set("X-Request-ID", "trace-e2e-42")

	var seen string
	up := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-Request-ID")
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"id":"1","object":"chat.completion","model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"hi"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer up.Close()

	snap := &runtime.Snapshot{General: settings.General{DefaultTimeoutSeconds: 5}}
	cr := &adapter.CanonicalRequest{Model: "gpt-4", Messages: []adapter.Message{{Role: "user", Content: "hi"}}}
	ru := runtime.ResolvedUpstream{ProviderID: 1, ProviderName: "mock", Protocol: adapter.ProtoOpenAIChat,
		UpstreamModel: "gpt-4", BaseURL: up.URL, APIKey: "k"}
	ap := &auditParams{}

	if fr := s.forward(c, snap, adapter.ProtoOpenAIChat, cr, ru, "trace-e2e-42", ap); fr != frDone {
		t.Fatalf("forward result = %v, want frDone (errMsg=%s)", fr, ap.errMsg)
	}
	if seen != "trace-e2e-42" {
		t.Errorf("upstream saw X-Request-ID = %q, want trace-e2e-42", seen)
	}
	if w.Header().Get("X-Request-ID") != "" {
		t.Error("forward() itself should not set response header (Handle does)")
	}
}

// TestEstimateUsage_Fallback 上游未返回 usage 时，应通过 tokens.Count 精确估算。
func TestEstimateUsage_Fallback(t *testing.T) {
	input := "Hello, this is a test input."
	output := "And this is the model output."
	u := adapter.Usage{Prompt: 0, Completion: 0}
	got := estimateUsage(u, input, output)
	if got.Prompt <= 0 {
		t.Errorf("Prompt = %d, want > 0 for non-empty input", got.Prompt)
	}
	if got.Completion <= 0 {
		t.Errorf("Completion = %d, want > 0 for non-empty output", got.Completion)
	}
}

// TestEstimateUsage_PreserveUpstream 上游已返回 usage 时，不应覆盖。
func TestEstimateUsage_PreserveUpstream(t *testing.T) {
	u := adapter.Usage{Prompt: 42, Completion: 17}
	got := estimateUsage(u, "any input", "any output")
	if got.Prompt != 42 || got.Completion != 17 {
		t.Errorf("estimateUsage(%+v) = %+v, want {42,17} preserved", u, got)
	}
}
