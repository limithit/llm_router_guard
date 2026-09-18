// settings.go KV 形式配置：通用设置(REQ-001)/故障转移(REQ-007)/输出过滤(REQ-011)/
// 配额预警(REQ-014)/安全设置(REQ-020)，保存后触发 Manager.Bump 热加载。
package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/model"
	"llmrouter/internal/settings"
)

// 设置写入校验（M-27 数值界限 / M-16 webhook scheme）。
// 原则：越界值在「保存时」拒绝——此前只信任前端，直接写库即可把网关打成不可用
// （retention=0 → 审计被清空；chunk_threshold=0 → 每 token 全量重扫；webhook=file:// → 内网探测）。
func validateGeneral(g *settings.General) error {
	if g.AuditRetentionDays < 1 || g.AuditRetentionDays > 3650 {
		return fmt.Errorf("审计保留天数需为 1-3650")
	}
	if g.DefaultTimeoutSeconds < 5 || g.DefaultTimeoutSeconds > 3600 {
		return fmt.Errorf("默认超时需为 5-3600 秒")
	}
	if g.MaxConnections < 1 || g.MaxConnections > 1_000_000 {
		return fmt.Errorf("最大连接数需为 1-1000000")
	}
	if g.HotReloadSeconds < 1 || g.HotReloadSeconds > 600 {
		return fmt.Errorf("热加载间隔需为 1-600 秒")
	}
	if g.CallAuditSampling < 1 || g.CallAuditSampling > 1000 {
		return fmt.Errorf("采样率需为 1-1000")
	}
	if g.LogLevel != "debug" && g.LogLevel != "info" && g.LogLevel != "warn" && g.LogLevel != "error" {
		g.LogLevel = "info"
	}
	return nil
}

func validateFailover(f *settings.Failover) error {
	if f.RetryCount < 0 || f.RetryCount > 10 {
		return fmt.Errorf("重试次数需为 0-10")
	}
	if f.RetryIntervalMs < 0 || f.RetryIntervalMs > 60000 {
		return fmt.Errorf("重试间隔需为 0-60000 毫秒")
	}
	if f.CircuitFailureThreshold < 1 || f.CircuitFailureThreshold > 100 {
		return fmt.Errorf("熔断失败阈值需为 1-100")
	}
	if f.CircuitResetSeconds < 1 || f.CircuitResetSeconds > 3600 {
		return fmt.Errorf("熔断恢复时间需为 1-3600 秒")
	}
	if len(f.TriggerStatusCodes) > 32 {
		return fmt.Errorf("触发状态码最多 32 个")
	}
	for _, c := range f.TriggerStatusCodes {
		if c < 400 || c > 599 {
			return fmt.Errorf("触发状态码需为 400-599，收到 %d", c)
		}
	}
	if f.Backoff != "fixed" && f.Backoff != "exponential" {
		f.Backoff = "fixed"
	}
	return nil
}

func validateOutput(o *settings.OutputFilter) error {
	switch o.ViolationStrategy {
	case "replace", "block", "log":
	default:
		return fmt.Errorf("违规处理策略仅支持 replace/block/log")
	}
	if o.StreamChunkThreshold < 64 || o.StreamChunkThreshold > 1_000_000 {
		return fmt.Errorf("流式检测阈值需为 64-1000000 字节")
	}
	if len(o.SafeMessage) > 2000 {
		return fmt.Errorf("安全提示语不超过 2000 字节")
	}
	return nil
}

func validateQuotaAlerts(a *settings.QuotaAlerts) error {
	if len(a.Thresholds) > 20 {
		return fmt.Errorf("预警阈值最多 20 个")
	}
	for _, t := range a.Thresholds {
		if t < 1 || t > 100 {
			return fmt.Errorf("预警阈值需为 1-100 百分比，收到 %d", t)
		}
	}
	if a.Channel != "ui" && a.Channel != "webhook" {
		a.Channel = "ui"
	}
	if a.Channel == "webhook" && a.Enabled {
		return validateWebhookURL(a.WebhookURL)
	}
	return nil
}

// validateWebhookURL M-16：仅允许 http/https 绝对地址（拒绝 file/gopher/内部协议与空值）。
// 注意：这不能替代内网地址限制——单节点部署的 webhook 目标由管理员自行负责，
// 多租户场景请在反代/防火墙侧收敛出网策略（见 SECURITY_AUDIT.md 残余风险）。
func validateWebhookURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("启用 webhook 通道时必须填写回调地址")
	}
	return validateHTTPURL("回调地址", raw)
}

// validateHTTPURL 通用 http/https 绝对地址校验（webhook / provider base_url 共用）。
func validateHTTPURL(label, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%s无法解析: %w", label, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s仅支持 http/https，收到 %q", label, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("%s缺少主机名", label)
	}
	return nil
}

func validateSecurity(sec *settings.Security) error {
	if sec.RecoveryCodeCount < 1 || sec.RecoveryCodeCount > 100 {
		return fmt.Errorf("恢复码数量需为 1-100")
	}
	if sec.GraceDays < 0 || sec.GraceDays > 365 {
		return fmt.Errorf("宽限期需为 0-365 天")
	}
	// 「强制所有用户启用 MFA」本身即蕴含启用 MFA：把主开关一并置位，
	// 避免出现「只开强制、不开主开关」的矛盾配置（此前该组合会静默失效，无任何提示）。
	if sec.MFARequiredAll {
		sec.MFAEnabled = true
	}
	return nil
}

// ---- 通用设置 (REQ-001) ----

func (s *Server) getGeneral(c *gin.Context) {
	var g settings.General
	settings.LoadKV(s.db, model.SetKeyGeneral, &g, settings.DefaultGeneral())
	g.ListenPort = s.port
	g.ListenPortNote = "仅启动参数生效（PORT 环境变量），修改需重启服务"
	s.ok(c, g)
}

func (s *Server) saveGeneral(c *gin.Context) {
	var g settings.General
	if err := c.ShouldBindJSON(&g); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if err := validateGeneral(&g); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	g.ListenPort = s.port
	if err := settings.SaveKV(s.db, model.SetKeyGeneral, g); err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "保存失败")
		return
	}
	s.recordOp(c, "update", "settings", "general", nil, g)
	s.mgr.Bump()
	s.ok(c, g)
}

// ---- 故障转移 (REQ-007) ----

func (s *Server) getFailover(c *gin.Context) {
	var f settings.Failover
	settings.LoadKV(s.db, model.SetKeyFailover, &f, settings.DefaultFailover())
	s.ok(c, f)
}

func (s *Server) saveFailover(c *gin.Context) {
	var f settings.Failover
	if err := c.ShouldBindJSON(&f); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if err := validateFailover(&f); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	if err := settings.SaveKV(s.db, model.SetKeyFailover, f); err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "保存失败")
		return
	}
	s.recordOp(c, "update", "failover", "failover", nil, f)
	s.mgr.Bump()
	s.ok(c, f)
}

// ---- 输出过滤 (REQ-011) ----

func (s *Server) getOutput(c *gin.Context) {
	var o settings.OutputFilter
	settings.LoadKV(s.db, model.SetKeyOutputFilt, &o, settings.DefaultOutputFilter())
	s.ok(c, o)
}

func (s *Server) saveOutput(c *gin.Context) {
	var o settings.OutputFilter
	if err := c.ShouldBindJSON(&o); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if err := validateOutput(&o); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	if err := settings.SaveKV(s.db, model.SetKeyOutputFilt, o); err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "保存失败")
		return
	}
	s.recordOp(c, "update", "guard", "output_filter", nil, o)
	s.mgr.Bump()
	s.ok(c, o)
}

// ---- 配额预警 (REQ-014) ----

func (s *Server) getQuotaAlerts(c *gin.Context) {
	var a settings.QuotaAlerts
	settings.LoadKV(s.db, model.SetKeyQuotaAlert, &a, settings.DefaultQuotaAlerts())
	s.ok(c, a)
}

func (s *Server) saveQuotaAlerts(c *gin.Context) {
	var a settings.QuotaAlerts
	if err := c.ShouldBindJSON(&a); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if err := validateQuotaAlerts(&a); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	if err := settings.SaveKV(s.db, model.SetKeyQuotaAlert, a); err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "保存失败")
		return
	}
	s.recordOp(c, "update", "quota", "alerts", nil, a)
	s.mgr.Bump()
	s.ok(c, a)
}

// ---- 安全设置 (REQ-020) ----

func (s *Server) getSecurity(c *gin.Context) {
	s.ok(c, s.securitySettings())
}

func (s *Server) saveSecurity(c *gin.Context) {
	var sec settings.Security
	if err := c.ShouldBindJSON(&sec); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if err := validateSecurity(&sec); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	if err := settings.SaveKV(s.db, model.SetKeySecurity, sec); err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "保存失败")
		return
	}
	s.recordOp(c, "update", "security", "mfa", nil, sec)
	s.mgr.Bump()
	s.ok(c, sec)
}
