// Package gateway 实现三协议统一接入与请求全链路（PRD 6.1 中间件链 + 协议适配层）：
// 认证 → 护栏输入检测 → 速率限制 → 配额 → SLB 选路 → 上游转发（含流式）→ 输出过滤 → 审计落库。
package gateway

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"llmrouter/internal/adapter"
	"llmrouter/internal/audit"
	"llmrouter/internal/crypto"
	"llmrouter/internal/guard"
	"llmrouter/internal/metrics"
	"llmrouter/internal/model"
	"llmrouter/internal/quota"
	"llmrouter/internal/runtime"
	"llmrouter/internal/slb"
)

type Server struct {
	db         *gorm.DB
	mgr        *runtime.Manager
	bl         *slb.Balancer
	rl         *quota.RateLimiter
	qm         *quota.QuotaManager
	auditLog   *audit.Logger
	mx         *metrics.Metrics
	httpClient *http.Client
	lastUsed   sync.Map // apiKeyID -> unix seconds
}

func New(gdb *gorm.DB, mgr *runtime.Manager, bl *slb.Balancer, rl *quota.RateLimiter,
	qm *quota.QuotaManager, al *audit.Logger, mx *metrics.Metrics) *Server {
	return &Server{
		db: gdb, mgr: mgr, bl: bl, rl: rl, qm: qm, auditLog: al, mx: mx,
		httpClient: &http.Client{Transport: &http.Transport{
			MaxIdleConns: 200, MaxIdleConnsPerHost: 64, IdleConnTimeout: 90 * time.Second,
		}},
	}
}

func protoFromPath(path string) adapter.Protocol {
	switch {
	case strings.HasSuffix(path, "/messages"):
		return adapter.ProtoAnthropic
	case strings.HasSuffix(path, "/responses"):
		return adapter.ProtoOpenAIResponses
	default:
		return adapter.ProtoOpenAIChat
	}
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// clientError 按客户端协议风格写错误响应。
func clientError(c *gin.Context, proto adapter.Protocol, status int, msg, errType, code string) {
	c.Data(status, "application/json", adapter.ErrorJSON(proto, status, msg, errType, code))
}

// AuthMiddleware 网关端点 API Key 认证（REQ-002）。
func (s *Server) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		snap := s.mgr.Get()
		proto := protoFromPath(c.Request.URL.Path)

		// 并发连接上限（REQ-001 ⑤）
		if int64(snap.General.MaxConnections) > 0 && s.mx.Conns() >= int64(snap.General.MaxConnections) {
			clientError(c, proto, http.StatusServiceUnavailable,
				"gateway is overloaded: too many connections", "overloaded_error", "too_many_connections")
			c.Abort()
			return
		}

		key := ""
		if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
			key = strings.TrimSpace(h[len("Bearer "):])
		}
		if key == "" {
			key = strings.TrimSpace(c.GetHeader("x-api-key"))
		}
		if key == "" {
			clientError(c, proto, http.StatusUnauthorized,
				"missing API key: use Authorization: Bearer sk-... or x-api-key", "authentication_error", "invalid_api_key")
			c.Abort()
			return
		}
		rec, ok := snap.APIKeys[crypto.Sha256Hex(key)]
		if !ok || !rec.Enabled {
			clientError(c, proto, http.StatusUnauthorized,
				"invalid or disabled API key", "authentication_error", "invalid_api_key")
			c.Abort()
			return
		}
		c.Set("apikey", rec)
		c.Next()
	}
}

// touchLastUsed 节流更新 Key 最后使用时间（≤ 每分钟一次）。
func (s *Server) touchLastUsed(id uint) {
	now := time.Now().Unix()
	if v, ok := s.lastUsed.Load(id); ok && now-v.(int64) < 60 {
		return
	}
	s.lastUsed.Store(id, now)
	go func() {
		t := time.Now()
		s.db.Model(&model.APIKey{}).Where("id = ?", id).Update("last_used_at", &t)
	}()
}

// writeLog 组装并异步入库调用审计（REQ-015）。
func (s *Server) writeLog(p auditParams) {
	cl := &model.CallLog{
		RequestID: p.requestID, CreatedAt: p.start, APIKeyID: p.keyID, APIKeyLabel: p.keyLabel,
		Protocol: p.proto, ModelAlias: p.alias, UpstreamProvider: p.upstreamProvider,
		UpstreamModel: p.upstreamModel, InputText: p.input, OutputText: p.output,
		PromptTokens: p.promptTokens, CompletionTokens: p.completionTokens,
		LatencyMs: time.Since(p.start).Milliseconds(), Status: p.status,
		Blocked: p.status == "blocked", BlockCategory: p.category, BlockReason: p.reason,
		ErrorMsg: p.errMsg, GuardFindings: p.findings,
	}
	s.auditLog.Write(cl)
}

type auditParams struct {
	requestID                                        string
	start                                            time.Time
	keyID                                            uint
	keyLabel                                         string
	proto                                            string
	alias                                            string
	upstreamProvider, upstreamModel                  string
	input, output                                    string
	promptTokens, completionTokens                   int
	status, category, reason, errMsg, findings       string
}

// Handle 返回一个协议端点的 gin 处理器。
func (s *Server) Handle(clientProto adapter.Protocol) gin.HandlerFunc {
	return func(c *gin.Context) {
		s.mx.ConnInc()
		defer s.mx.ConnDec()
		start := time.Now()
		snap := s.mgr.Get()
		apiKey := c.MustGet("apikey").(*model.APIKey)
		s.touchLastUsed(apiKey.ID)
		reqID := newRequestID()

		ap := auditParams{
			requestID: reqID, start: start, keyID: apiKey.ID, keyLabel: apiKey.Name,
			proto: clientProto, status: "error",
		}
		defer func() {
			s.writeLog(ap)
			s.mx.Observe(ap.alias, ap.status != "ok")
		}()

		// 1. 解析协议体 → 规范模型
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 16<<20))
		if err != nil {
			ap.errMsg = "read request body: " + err.Error()
			clientError(c, clientProto, http.StatusBadRequest, ap.errMsg, "invalid_request_error", "bad_body")
			return
		}
		cr, err := adapter.ParseRequest(clientProto, body)
		if err != nil {
			ap.errMsg = err.Error()
			clientError(c, clientProto, http.StatusBadRequest, err.Error(), "invalid_request_error", "bad_request")
			return
		}
		ap.alias = cr.Model
		ups, ok := snap.Aliases[cr.Model]
		if !ok {
			ap.errMsg = "unknown model alias: " + cr.Model
			clientError(c, clientProto, http.StatusNotFound,
				fmt.Sprintf("unknown model %q: not a configured alias", cr.Model),
				"invalid_request_error", "model_not_found")
			return
		}

		// 2. 护栏输入检测（逐条消息，PII 就地脱敏）
		var allFindings []guard.Finding
		blockedReason, blockedCategory := "", ""
		if snap.General.GuardEnabled {
			for i := range cr.Messages {
				vd := guard.CheckInput(snap, cr.Messages[i].Content)
				allFindings = append(allFindings, vd.Findings...)
				cr.Messages[i].Content = vd.Text
				if vd.Blocked && blockedReason == "" {
					blockedReason = vd.BlockReason
					for _, f := range vd.Findings {
						if f.Action == "block" {
							blockedCategory = f.Category
							break
						}
					}
				}
			}
		}
		ap.findings = guard.FindingsJSON(allFindings)
		ap.input = guard.MaskForLog(snap, cr.InputPlainText())
		if blockedReason != "" {
			ap.status = "blocked"
			ap.category = blockedCategory
			ap.reason = blockedReason
			clientError(c, clientProto, http.StatusBadRequest,
				"request blocked by content policy: "+blockedReason,
				"content_policy_violation", "content_filtered")
			return
		}

		// 3. 速率限制
		if ok2, hit := s.rl.Allow(snap, apiKey.ID, ap.alias); !ok2 {
			ap.status = "rate_limited"
			ap.reason = fmt.Sprintf("rate limit rule#%d: %d reqs/%ds", hit.ID, hit.MaxRequests, hit.WindowSeconds)
			clientError(c, clientProto, http.StatusTooManyRequests,
				ap.reason, "rate_limit_error", "rate_limit_exceeded")
			return
		}

		// 4. 配额（超限可降级到其他别名）
		if ok3, over := s.qm.Check(snap, apiKey.ID, ap.alias); !ok3 {
			degraded := false
			if over.OverAction == "degrade" && over.DegradeAlias != "" {
				if ups2, ok4 := snap.Aliases[over.DegradeAlias]; ok4 {
					ups = ups2
					ap.alias = over.DegradeAlias
					degraded = true
				}
			}
			if !degraded {
				ap.status = "quota_exceeded"
				ap.reason = fmt.Sprintf("quota#%d (%s/%s) 已达上限 %d/%d",
					over.QuotaID, over.Kind, "current", over.Used, over.Limit)
				clientError(c, clientProto, http.StatusTooManyRequests,
					fmt.Sprintf("quota exceeded (%s): %d of %d used", over.Kind, over.Used, over.Limit),
					"rate_limit_error", "quota_exceeded")
				return
			}
		}

		// 5. SLB + 故障转移转发
		fo := snap.Failover
		maxAttempts := 1
		if fo.Enabled && fo.RetryCount > 0 {
			maxAttempts = 1 + fo.RetryCount
		}
		tried := map[uint]bool{}
		lastErr := "no available upstream"
		for attempt := 0; attempt < maxAttempts; attempt++ {
			up, ok5 := s.bl.Pick(ups, tried)
			if !ok5 {
				up, ok5 = s.bl.PickIgnoringCircuit(ups, tried)
			}
			if !ok5 {
				break
			}
			tried[up.ProviderID] = true
			if attempt > 0 && fo.RetryIntervalMs > 0 {
				wait := fo.RetryIntervalMs
				if fo.Backoff == "exponential" {
					wait = fo.RetryIntervalMs * (1 << (attempt - 1))
				}
				time.Sleep(time.Duration(wait) * time.Millisecond)
			}
			res := s.forward(c, snap, clientProto, cr, up, reqID, &ap)
			switch res {
			case frDone, frFatal:
				return // 响应已写出（成功或终结性错误），ap 已填充
			case frRetry:
				lastErr = ap.errMsg
				continue
			}
		}
		ap.status = "error"
		ap.errMsg = lastErr
		s.mx.RecordError(reqID, lastErr)
		clientError(c, clientProto, http.StatusBadGateway,
			"all upstreams unavailable: "+lastErr, "upstream_error", "bad_gateway")
	}
}

type forwardResult int

const (
	frDone  forwardResult = iota // 成功
	frFatal                      // 终结性失败（响应已写）
	frRetry                      // 可重试
)

// forward 单次上游尝试（流式与非流式统一入口）。
func (s *Server) forward(c *gin.Context, snap *runtime.Snapshot, clientProto adapter.Protocol,
	cr *adapter.CanonicalRequest, up runtime.ResolvedUpstream, reqID string, ap *auditParams) forwardResult {

	timeout := time.Duration(snap.General.DefaultTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	req, err := adapter.BuildUpstreamRequest(up.Protocol, cr, up.UpstreamModel, up.BaseURL, up.APIKey)
	if err != nil {
		ap.errMsg = err.Error()
		s.bl.RecordFailure(up.ProviderID, snap.Failover, err.Error())
		return frRetry
	}
	req = req.WithContext(ctx)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		ap.errMsg = fmt.Sprintf("%s: %v", up.ProviderName, err)
		s.bl.RecordFailure(up.ProviderID, snap.Failover, err.Error())
		return frRetry
	}
	defer resp.Body.Close()

	ap.upstreamProvider = up.ProviderName
	ap.upstreamModel = up.UpstreamModel

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		msg := adapter.ExtractError(up.Protocol, body)
		retryable := snap.Failover.Enabled && (resp.StatusCode >= 500 || containsInt(snap.Failover.TriggerStatusCodes, resp.StatusCode))
		if retryable {
			ap.errMsg = fmt.Sprintf("%s returned %d: %s", up.ProviderName, resp.StatusCode, msg)
			s.bl.RecordFailure(up.ProviderID, snap.Failover, msg)
			return frRetry
		}
		ap.errMsg = msg
		ap.status = "error"
		clientError(c, clientProto, resp.StatusCode,
			fmt.Sprintf("upstream %s returned %d: %s", up.ProviderName, resp.StatusCode, msg),
			"upstream_error", "upstream_error")
		return frFatal
	}

	if cr.Stream {
		return s.forwardStream(c, snap, clientProto, cr, up, resp, reqID, ap)
	}
	return s.forwardBuffered(c, snap, clientProto, up, resp, ap)
}

// forwardBuffered 非流式：读取完整响应 → 输出护栏 → 按客户端协议序列化。
func (s *Server) forwardBuffered(c *gin.Context, snap *runtime.Snapshot, clientProto adapter.Protocol,
	up runtime.ResolvedUpstream, resp *http.Response, ap *auditParams) forwardResult {

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		ap.errMsg = "read upstream response: " + err.Error()
		s.bl.RecordFailure(up.ProviderID, snap.Failover, err.Error())
		return frRetry
	}
	cr, err := adapter.ParseCompletion(up.Protocol, body)
	if err != nil {
		ap.errMsg = "parse upstream completion: " + err.Error()
		s.bl.RecordFailure(up.ProviderID, snap.Failover, err.Error())
		return frRetry
	}

	// 输出护栏
	if snap.General.GuardEnabled && snap.Output.Enabled {
		vd := guard.CheckOutput(snap, cr.Content)
		if len(vd.Findings) > 0 {
			ap.findings = appendFindings(ap.findings, vd.Findings)
		}
		if vd.Blocked {
			final, blocked := guard.ApplyOutputStrategy(snap, vd)
			if blocked {
				ap.status = "blocked"
				ap.category = "output"
				ap.reason = vd.BlockReason
				clientError(c, clientProto, http.StatusBadRequest,
					"response blocked by content policy: "+vd.BlockReason,
					"content_policy_violation", "content_filtered")
				return frFatal
			}
			cr.Content = final
		} else {
			cr.Content = vd.Text
		}
	}

	usage := estimateUsage(cr.Usage, ap.input, cr.Content)
	cr.Usage = usage
	ap.promptTokens, ap.completionTokens = usage.Prompt, usage.Completion
	ap.output = guard.MaskForLog(snap, cr.Content)
	ap.status = "ok"

	out := adapter.BuildCompletionJSON(clientProto, cr)
	c.Data(http.StatusOK, "application/json", out)
	s.bl.RecordSuccess(up.ProviderID)
	s.qm.Consume(snap, ap.keyID, ap.alias, usage.Prompt, usage.Completion)
	return frDone
}

// forwardStream 流式：逐块转发 + 阈值化输出检测。
func (s *Server) forwardStream(c *gin.Context, snap *runtime.Snapshot, clientProto adapter.Protocol,
	cr *adapter.CanonicalRequest, up runtime.ResolvedUpstream, resp *http.Response,
	reqID string, ap *auditParams) forwardResult {

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	sw := adapter.NewSSEWriter(clientProto, c.Writer, reqID, cr.Model)
	_ = sw.Open()

	threshold := snap.Output.StreamChunkThreshold
	if threshold < 64 {
		threshold = 256
	}

	var acc strings.Builder
	var usage adapter.Usage
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	nextCheck := threshold
	stopped := false

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		chunk := adapter.ParseUpstreamData(up.Protocol, strings.TrimPrefix(line, "data:"))
		if chunk.Err != "" {
			_ = sw.Fail(chunk.Err)
			ap.errMsg = chunk.Err
			ap.status = "error"
			s.mx.RecordError(reqID, chunk.Err)
			return frFatal
		}
		if chunk.Usage != nil {
			if chunk.Usage.Prompt > 0 {
				usage.Prompt = chunk.Usage.Prompt
			}
			if chunk.Usage.Completion > 0 {
				usage.Completion = chunk.Usage.Completion
			}
		}
		if chunk.TextDelta != "" {
			acc.WriteString(chunk.TextDelta)
			_ = sw.Delta(chunk.TextDelta)

			// 阈值化输出护栏（REQ-011 ④ 流式检测阈值）
			if snap.General.GuardEnabled && snap.Output.Enabled && acc.Len() >= nextCheck {
				vd := guard.CheckOutput(snap, acc.String())
				if vd.Blocked {
					ap.category = "output"
					ap.reason = vd.BlockReason
					switch snap.Output.ViolationStrategy {
					case "block":
						_ = sw.Fail("output blocked by content policy: " + vd.BlockReason)
						ap.status = "blocked"
						s.mx.RecordError(reqID, vd.BlockReason)
						return frFatal
					case "replace":
						msg := snap.Output.SafeMessage
						if msg == "" {
							msg = "抱歉，该回答包含不当内容。"
						}
						_ = sw.Delta("\n" + msg)
						ap.status = "blocked"
						ap.output = guard.MaskForLog(snap, msg)
						stopped = true
					default: // log
						if len(vd.Findings) > 0 {
							ap.findings = appendFindings(ap.findings, vd.Findings)
						}
					}
					if stopped {
						break
					}
				}
				nextCheck += threshold
			}
		}
		if chunk.Finish {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		_ = sw.Fail("upstream stream interrupted: " + err.Error())
		ap.errMsg = err.Error()
		ap.status = "error"
		return frFatal
	}

	usage = estimateUsage(usage, ap.input, acc.String())
	ap.promptTokens, ap.completionTokens = usage.Prompt, usage.Completion
	if ap.output == "" {
		ap.output = guard.MaskForLog(snap, acc.String())
	}
	if ap.status == "" || ap.status == "error" && ap.errMsg == "" {
		ap.status = "ok"
	}
	_ = sw.Finalize(usage)
	s.bl.RecordSuccess(up.ProviderID)
	if ap.status == "ok" || ap.status == "blocked" {
		s.qm.Consume(snap, ap.keyID, ap.alias, usage.Prompt, usage.Completion)
	}
	return frDone
}

func estimateUsage(u adapter.Usage, input, output string) adapter.Usage {
	if u.Prompt == 0 && input != "" {
		u.Prompt = utf8.RuneCountInString(input)/2 + 1
	}
	if u.Completion == 0 && output != "" {
		u.Completion = utf8.RuneCountInString(output)/2 + 1
	}
	return u
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func appendFindings(existing string, fs []guard.Finding) string {
	merged := guard.FindingsJSONRaw(existing, fs)
	return merged
}
