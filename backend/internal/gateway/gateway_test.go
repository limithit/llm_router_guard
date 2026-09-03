package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
