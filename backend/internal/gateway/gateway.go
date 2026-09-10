// Package gateway 实现三协议统一接入与请求全链路（PRD 6.1 中间件链 + 协议适配层）：
// 认证 → 护栏输入检测 → 速率限制 → 配额 → SLB 选路 → 上游转发（含流式）→ 输出过滤 → 审计落库。
package gateway

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	mr "math/rand"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

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
	"llmrouter/internal/settings"
	"llmrouter/internal/slb"
	"llmrouter/internal/tokens"
)

type Server struct {
	db         *gorm.DB
	mgr        *runtime.Manager
	bl         *slb.Balancer
	rl         rateLimitAllow // 内存版 *quota.RateLimiter 或分布式 *quota.RedisLimiter
	qm         *quota.QuotaManager
	auditLog   *audit.Logger
	mx         *metrics.Metrics
	httpClient *http.Client
	lastUsed   sync.Map // apiKeyID -> unix seconds
}

// rateLimitAllow 限流判定接口：单机内存版与 Redis 分布式版实现同一签名，
// 由部署形态（REDIS_ADDR 是否配置）决定注入哪个实现。
type rateLimitAllow interface {
	Allow(snap *runtime.Snapshot, apiKeyID uint, alias string) (bool, *model.RateLimitRule)
}

func New(gdb *gorm.DB, mgr *runtime.Manager, bl *slb.Balancer, rl rateLimitAllow,
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

// resolveRequestID 链路追踪 ID：客户端带合法 X-Request-ID（≤128 字节）则透传复用
// （跨服务同一 ID 贯穿），否则生成。响应头回带同值，转发上游时也携带。
func resolveRequestID(c *gin.Context) string {
	if v := strings.TrimSpace(c.GetHeader("X-Request-ID")); v != "" && len(v) <= 128 {
		return v
	}
	return newRequestID()
}

// clientError 按客户端协议风格写错误响应。
func clientError(c *gin.Context, proto adapter.Protocol, status int, msg, errType, code string) {
	c.Data(status, "application/json", adapter.ErrorJSON(proto, status, msg, errType, code))
}

// ListModels GET /v1/models（及 /v1/{responses,messages,chat/completions}/models）
// 返回网关已配置的模型别名，供第三方 Agent 工具发现可用模型。
// 按协议返回干净 schema：路径含 /messages 或带 anthropic-version 头 → Anthropic 格式，
// 否则 OpenAI 格式。受 API Key 的模型限定过滤。
func (s *Server) ListModels(c *gin.Context) {
	snap := s.mgr.Get()
	rec := c.MustGet("apikey").(*model.APIKey) // AuthMiddleware 已注入

	names := make([]string, 0, len(snap.Aliases))
	for name := range snap.Aliases {
		if !rec.AllowsModel(name) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	// Anthropic：路径含 /messages（如 /v1/messages/models）或客户端带 anthropic-version 头。
	if strings.Contains(c.Request.URL.Path, "/messages") || c.GetHeader("anthropic-version") != "" {
		type anthModel struct {
			Type        string `json:"type"`
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			CreatedAt   string `json:"created_at"`
		}
		data := make([]anthModel, 0, len(names))
		for _, name := range names {
			data = append(data, anthModel{Type: "model", ID: name, DisplayName: name, CreatedAt: "1970-01-01T00:00:00Z"})
		}
		firstID, lastID := "", ""
		if len(names) > 0 {
			firstID, lastID = names[0], names[len(names)-1]
		}
		c.JSON(http.StatusOK, gin.H{"data": data, "has_more": false, "first_id": firstID, "last_id": lastID})
		return
	}

	// OpenAI Chat / Responses
	type oaiModel struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
		Created int64  `json:"created"`
	}
	data := make([]oaiModel, 0, len(names))
	for _, name := range names {
		owner := ""
		if ups := snap.Aliases[name]; len(ups) > 0 {
			owner = ups[0].ProviderName
		}
		data = append(data, oaiModel{ID: name, Object: "model", OwnedBy: owner, Created: 0})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
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
		if !rec.IPAllowed(c.ClientIP()) {
			clientError(c, proto, http.StatusForbidden,
				"ip not allowed for this api key", "forbidden", "ip_not_allowed")
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

// shouldAuditCall 判断本次调用是否应写入调用审计。
// 总开关关闭 → 不记；错误/拦截 → 始终记（便于排查）；成功调用受「仅错误」与采样控制。
func shouldAuditCall(g settings.General, status string) bool {
	if !g.CallAuditEnabled {
		return false
	}
	if status != "ok" {
		return true
	}
	if g.CallAuditOnlyErrors {
		return false
	}
	if g.CallAuditSampling <= 1 {
		return true
	}
	return mr.Intn(g.CallAuditSampling) == 0
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
		ErrorMsg: p.errMsg, GuardFindings: p.findings, FinishReason: p.finishReason,
	}
	s.auditLog.Write(cl)
}

type auditParams struct {
	requestID                                  string
	start                                      time.Time
	keyID                                      uint
	keyLabel                                   string
	proto                                      string
	alias                                      string
	upstreamProvider, upstreamModel            string
	input, output                              string
	promptTokens, completionTokens             int
	status, category, reason, errMsg, findings string
	finishReason                               string
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
		reqID := resolveRequestID(c)
		// 响应回带：客户端收到响应后可凭此 ID 在审计（request_id 列）中定位本次调用
		c.Header("X-Request-ID", reqID)

		ap := auditParams{
			requestID: reqID, start: start, keyID: apiKey.ID, keyLabel: apiKey.Name,
			proto: clientProto, status: "error",
		}
		defer func() {
			if shouldAuditCall(snap.General, ap.status) {
				s.writeLog(ap)
			}
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
		// 别名级默认 max_tokens：请求未显式携带（0）时注入，兜底推理模型的厂商默认补全上限
		// （如 sensenova glm 默认 ~1000 token，思维链即耗尽 → finish_reason 谎报 stop 的"截断"）。
		if cr.MaxTokens == 0 {
			if as := snap.AliasSettings[cr.Model]; as != nil && as.DefaultMaxTokens > 0 {
				cr.MaxTokens = as.DefaultMaxTokens
			}
		}
		if !apiKey.AllowsModel(cr.Model) {
			ap.errMsg = "model not allowed for api key"
			clientError(c, clientProto, http.StatusForbidden,
				fmt.Sprintf("model %q is not allowed for this api key", cr.Model),
				"forbidden", "model_not_allowed")
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
			up, ok5 := s.bl.Pick(ap.alias, ups, tried)
			if !ok5 {
				up, ok5 = s.bl.PickIgnoringCircuit(ap.alias, ups, tried)
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

	// 流式请求不套总超时：长 SSE 流可能远超 DefaultTimeoutSeconds（如大模型逐 token
	// 输出几十秒以上），套了会在中途 context.Canceled → scanner.Err() → 截断。
	// 这与 WriteTimeout=0（"SSE 长连接不设总写超时"）的设计一致：流式仅受客户端断开
	// 和上游自身行为约束。非流式仍用 DefaultTimeoutSeconds 总超时兜底慢上游。
	ctx := c.Request.Context()
	if !cr.Stream {
		timeout := time.Duration(snap.General.DefaultTimeoutSeconds) * time.Second
		if timeout <= 0 {
			timeout = 120 * time.Second
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
	}

	req, err := adapter.BuildUpstreamRequest(up.Protocol, cr, up.UpstreamModel, up.BaseURL, up.APIKey)
	if err != nil {
		ap.errMsg = err.Error()
		s.bl.RecordFailure(up.ProviderID, snap.Failover, err.Error())
		return frRetry
	}
	// 链路追踪透传（P1 #1）：上游可凭此头回查网关审计（call_logs.request_id）
	req.Header.Set("X-Request-ID", reqID)
	req = req.WithContext(ctx)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		ap.errMsg = err.Error() // 审计另有 UpstreamProvider 列；面向客户端的错误不泄漏上游名
		log.Printf("[upstream] %s %s: %v", up.ProviderName, up.UpstreamModel, err)
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
			ap.errMsg = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, msg)
			log.Printf("[upstream] %s %s returned %d: %s", up.ProviderName, up.UpstreamModel, resp.StatusCode, msg)
			s.bl.RecordFailure(up.ProviderID, snap.Failover, msg)
			return frRetry
		}
		ap.errMsg = msg
		ap.status = "error"
		clientError(c, clientProto, resp.StatusCode,
			fmt.Sprintf("upstream returned %d: %s", resp.StatusCode, msg),
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
	ap.finishReason = cr.FinishReason
	if len(cr.ToolCalls) > 0 {
		usage.Completion += len(cr.ToolCalls) / 4 // 工具参数 token 粗估并入配额
		cr.Usage = usage
		ap.completionTokens = usage.Completion
		ap.output = guard.MaskForLog(snap, cr.Content+" "+string(cr.ToolCalls))
	} else {
		ap.output = guard.MaskForLog(snap, cr.Content)
	}
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

	// 工具调用增量按 index 合并（id/name 在首个分片，arguments 跨块拼接），
	// 结束后并入审计文本，保证护栏与调用日志能看到模型调了什么工具。
	type toolCallBuf struct {
		id, name string
		args     strings.Builder
	}
	toolCalls := map[int]*toolCallBuf{}

	sawFinish := false
	finishReason := ""
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
			log.Printf("[stream] %s %s upstream error event: %s", up.ProviderName, up.UpstreamModel, chunk.Err)
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
		if chunk.ReasoningDelta != "" {
			_ = sw.Reasoning(chunk.ReasoningDelta)
		}
		if len(chunk.ToolCalls) > 0 {
			var dts []struct {
				Index    *int   `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			}
			if json.Unmarshal(chunk.ToolCalls, &dts) == nil {
				for _, dt := range dts {
					idx := 0
					if dt.Index != nil {
						idx = *dt.Index
					}
					buf := toolCalls[idx]
					if buf == nil {
						buf = &toolCallBuf{}
						toolCalls[idx] = buf
					}
					if dt.ID != "" {
						buf.id = dt.ID
					}
					if dt.Function.Name != "" {
						buf.name = dt.Function.Name
					}
					buf.args.WriteString(dt.Function.Arguments)
				}
			}
			_ = sw.ToolCalls(chunk.ToolCalls)
		}
		if chunk.TextDelta != "" {
			acc.WriteString(chunk.TextDelta)
			_ = sw.Delta(chunk.TextDelta)

			// 阈值化输出护栏（REQ-011 ④ 流式检测阈值）
			if snap.General.GuardEnabled && snap.Output.Enabled && acc.Len() >= nextCheck {
				stop, fatal := s.runStreamGuardCheck(snap, sw, reqID, ap, acc.String())
				if fatal {
					return frFatal
				}
				if stop {
					stopped = true
					break
				}
				nextCheck += threshold
			}
		}
		if chunk.Finish {
			sawFinish = true
			if chunk.FinishReason != "" {
				finishReason = chunk.FinishReason
			}
			break
		}
	}
	if err := scanner.Err(); err != nil {
		// 结束原因观测：正常 EOF 静默、真实错误也常被上层当"正常结束"，先在此显式留痕，
		// 否则"上游提前断开 / 客户端断开 / 护栏拦截"三种截断在日志里无法区分。
		log.Printf("[stream] %s %s finished early: read error=%v (accumulated %d bytes)", up.ProviderName, up.UpstreamModel, err, acc.Len())
		_ = sw.Fail("upstream stream interrupted: " + err.Error())
		ap.errMsg = err.Error()
		ap.status = "error"
		return frFatal
	}
	if !sawFinish {
		// 读到 EOF 也没见到 finish_reason/[DONE]：上游把流掐了（网络/网关/上游侧超时）。
		// 此前被当成功（RecordSuccess + usage 估算），排障时完全隐身——必须显式留痕。
		log.Printf("[stream] %s %s finished early: upstream closed before finish signal (accumulated %d bytes)", up.ProviderName, up.UpstreamModel, acc.Len())
	}

	// 工具调用并入累积文本：输出护栏与调用日志（output_text）由此可见模型实际调用了什么。
	if len(toolCalls) > 0 {
		idxs := make([]int, 0, len(toolCalls))
		for i := range toolCalls {
			idxs = append(idxs, i)
		}
		sort.Ints(idxs)
		for _, i := range idxs {
			tc := toolCalls[i]
			fmt.Fprintf(&acc, `<tool_call>{"name":%q,"arguments":%s}</tool_call>`, tc.name, tc.args.String())
		}
	}

	// 流结束兜底检测：短输出（不足阈值）或最后一个未达阈值的尾部此前未被检测，
	// 必须在此对完整累积文本补一次输出护栏，避免短回答（如“Google”）被放行。
	if !stopped && snap.General.GuardEnabled && snap.Output.Enabled && acc.Len() > 0 {
		if _, fatal := s.runStreamGuardCheck(snap, sw, reqID, ap, acc.String()); fatal {
			return frFatal
		}
	}

	usage = estimateUsage(usage, ap.input, acc.String())
	ap.promptTokens, ap.completionTokens = usage.Prompt, usage.Completion
	ap.finishReason = finishReason
	if ap.output == "" {
		ap.output = guard.MaskForLog(snap, acc.String())
	}
	if ap.status == "" || ap.status == "error" && ap.errMsg == "" {
		ap.status = "ok"
	}
	log.Printf("[stream] %s %s done: finish_reason=%s content_bytes=%d deltas_logged=%v", up.ProviderName, up.UpstreamModel, finishReason, acc.Len(), sawFinish)
	_ = sw.Finalize(usage, finishReason)
	s.bl.RecordSuccess(up.ProviderID)
	if ap.status == "ok" || ap.status == "blocked" {
		s.qm.Consume(snap, ap.keyID, ap.alias, usage.Prompt, usage.Completion)
	}
	return frDone
}

// runStreamGuardCheck 对流式累积输出做一次输出护栏检测，并按违规策略处理。
// 返回 stop（replace 后应终止循环）与 fatal（block 后调用方应立即返回 frFatal）。
func (s *Server) runStreamGuardCheck(snap *runtime.Snapshot, sw *adapter.SSEWriter,
	reqID string, ap *auditParams, text string) (stop, fatal bool) {
	vd := guard.CheckOutput(snap, text)
	if !vd.Blocked {
		if len(vd.Findings) > 0 {
			ap.findings = appendFindings(ap.findings, vd.Findings)
		}
		return false, false
	}
	ap.category = "output"
	ap.reason = vd.BlockReason
	switch snap.Output.ViolationStrategy {
	case "block":
		_ = sw.Fail("output blocked by content policy: " + vd.BlockReason)
		ap.status = "blocked"
		s.mx.RecordError(reqID, vd.BlockReason)
		return true, true
	case "log":
		if len(vd.Findings) > 0 {
			ap.findings = appendFindings(ap.findings, vd.Findings)
		}
		return false, false
	default: // replace
		msg := snap.Output.SafeMessage
		if msg == "" {
			msg = "抱歉，该回答包含不当内容。"
		}
		_ = sw.Delta("\n" + msg)
		ap.status = "blocked"
		ap.output = guard.MaskForLog(snap, msg)
		return true, false
	}
}

func estimateUsage(u adapter.Usage, input, output string) adapter.Usage {
	if u.Prompt == 0 && input != "" {
		u.Prompt = tokens.Count(input)
	}
	if u.Completion == 0 && output != "" {
		u.Completion = tokens.Count(output)
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
