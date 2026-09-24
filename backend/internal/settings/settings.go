// Package settings 定义以 KV 形式存于数据库的各配置模块结构体（含默认值），
// 对应 REQ-001(通用) / REQ-007(故障转移) / REQ-011(输出过滤) / REQ-014(预警) / REQ-020(安全)。
package settings

import (
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"llmrouter/internal/model"
)

// General REQ-001：网关全局运行参数（监听端口为启动参数，仅回显）。
type General struct {
	LogLevel              string `json:"log_level"`
	AuditRetentionDays    int    `json:"audit_retention_days"`
	DefaultTimeoutSeconds int    `json:"default_timeout_seconds"`
	MaxConnections        int    `json:"max_connections"`
	GuardEnabled          bool   `json:"guard_enabled"`
	HotReloadSeconds      int    `json:"hot_reload_seconds"`
	ListenPort            int    `json:"listen_port"`
	ListenPortNote        string `json:"listen_port_note,omitempty"`
	// 调用审计写入控制：默认开启、默认全记（采样=1）、默认不只记错误。错误/拦截始终记录。
	CallAuditEnabled    bool `json:"call_audit_enabled"`
	CallAuditOnlyErrors bool `json:"call_audit_only_errors"`
	CallAuditSampling   int  `json:"call_audit_sampling"`
	// 思维链（reasoning_content）写入审计 output_text：默认关闭（占库高，仅调试时开）。
	// 关闭时 output_text 只含最终回答；token 计数始终含 reasoning，不受此开关影响。
	LogReasoning bool `json:"log_reasoning"`
}

func DefaultGeneral() General {
	return General{LogLevel: "info", AuditRetentionDays: 90, DefaultTimeoutSeconds: 120,
		MaxConnections: 1000, GuardEnabled: true, HotReloadSeconds: 3, ListenPort: 8080,
		CallAuditEnabled: true, CallAuditSampling: 1}
}

// Failover REQ-007：故障转移与熔断。
type Failover struct {
	Enabled                 bool   `json:"enabled"`
	RetryCount              int    `json:"retry_count"`
	Backoff                 string `json:"backoff"` // fixed|exponential
	RetryIntervalMs         int    `json:"retry_interval_ms"`
	TriggerStatusCodes      []int  `json:"trigger_status_codes"`
	CircuitFailureThreshold int    `json:"circuit_failure_threshold"`
	CircuitResetSeconds     int    `json:"circuit_reset_seconds"`
}

func DefaultFailover() Failover {
	return Failover{Enabled: true, RetryCount: 2, Backoff: "exponential", RetryIntervalMs: 200,
		TriggerStatusCodes: []int{408, 429, 500, 502, 503, 504}, CircuitFailureThreshold: 5, CircuitResetSeconds: 30}
}

// OutputFilter REQ-011：输出过滤策略。
type OutputFilter struct {
	Enabled              bool   `json:"enabled"`
	ViolationStrategy    string `json:"violation_strategy"` // replace|block|log
	SafeMessage          string `json:"safe_message"`
	StreamChunkThreshold int    `json:"stream_chunk_threshold"`
}

func DefaultOutputFilter() OutputFilter {
	return OutputFilter{Enabled: true, ViolationStrategy: "replace",
		SafeMessage: "抱歉，该回答包含不当内容，已被安全策略处理。", StreamChunkThreshold: 256}
}

// QuotaAlerts REQ-014。
type QuotaAlerts struct {
	Enabled    bool   `json:"enabled"`
	Thresholds []int  `json:"thresholds"`
	Channel    string `json:"channel"` // ui|webhook
	WebhookURL string `json:"webhook_url"`
	Receivers  string `json:"receivers"`
}

func DefaultQuotaAlerts() QuotaAlerts {
	return QuotaAlerts{Enabled: false, Thresholds: []int{80, 95}, Channel: "ui"}
}

// Security REQ-020：MFA 全局配置。
type Security struct {
	MFAEnabled        bool `json:"mfa_enabled"`
	MFARequiredAll    bool `json:"mfa_required_for_all"`
	RecoveryCodeCount int  `json:"recovery_code_count"`
	GraceDays         int  `json:"grace_days"`
}

func DefaultSecurity() Security {
	return Security{MFAEnabled: true, MFARequiredAll: false, RecoveryCodeCount: 10, GraceDays: 7}
}

// ConfigMeta 热加载版本戳（REQ-004：多实例轮询依据）。
type ConfigMeta struct {
	Counter    int64  `json:"counter"`
	Version    string `json:"version"`
	Status     string `json:"status"`
	ErrMessage string `json:"error_message"`
}

// LoadKV 读取一个 KV 配置模块并反序列化到 dst；不存在时保持 dst 默认值。
func LoadKV[T any](gdb *gorm.DB, key string, dst *T, def T) error {
	*dst = def // 先填默认值，缺失字段保留默认（前向兼容，避免新增字段被旧记录清零）
	var row model.SystemSetting
	// 用结构体条件而非裸 "key = ?"：key 是 MySQL 保留字（原生 SQL 不加反引号会 1064），
	// 结构体条件由 GORM 按方言加引号，sqlite/mysql/postgres 通吃。
	err := gdb.Where(model.SystemSetting{Key: key}).First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	if row.ValueJSON == "" {
		return nil
	}
	return json.Unmarshal([]byte(row.ValueJSON), dst)
}

// SaveKV 序列化并 upsert 一个 KV 配置模块。
func SaveKV[T any](gdb *gorm.DB, key string, val T) error {
	b, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", key, err)
	}
	return gdb.Save(&model.SystemSetting{Key: key, ValueJSON: string(b)}).Error
}

// EnsureDefaults 首次启动时写入各模块默认配置。
func EnsureDefaults(gdb *gorm.DB, port int) error {
	var g General
	if err := LoadKV(gdb, model.SetKeyGeneral, &g, DefaultGeneral()); err != nil {
		return err
	}
	if g.ListenPort == 0 {
		g.ListenPort = port
	}
	if err := SaveKV(gdb, model.SetKeyGeneral, g); err != nil {
		return err
	}
	pairs := []struct {
		key string
		val any
	}{
		{model.SetKeyFailover, DefaultFailover()},
		{model.SetKeyOutputFilt, DefaultOutputFilter()},
		{model.SetKeyQuotaAlert, DefaultQuotaAlerts()},
		{model.SetKeySecurity, DefaultSecurity()},
		{model.SetKeyConfigMeta, ConfigMeta{Status: "ok"}},
	}
	for _, p := range pairs {
		var cnt int64
		gdb.Model(&model.SystemSetting{}).Where(model.SystemSetting{Key: p.key}).Count(&cnt)
		if cnt == 0 {
			if err := SaveKV(gdb, p.key, p.val); err != nil {
				return err
			}
		}
	}
	return nil
}
