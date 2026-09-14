// Package model 定义全部持久化实体（GORM 模型）。
// 覆盖 PRD：REQ-001~REQ-024 所需配置表与日志表。
package model

import (
	"encoding/json"
	"net"
	"strings"
	"time"
)

// ---------- 管理后台用户 & MFA (REQ-020~024) ----------

type AdminUser struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	Username          string     `gorm:"uniqueIndex;size:64" json:"username"`
	PasswordHash      string     `gorm:"size:128" json:"-"`
	MFASecret         string     `gorm:"size:128" json:"-"` // TOTP base32 secret（未启用也暂存，待 enable）
	MFAEnabled        bool       `json:"mfa_enabled"`
	MFABoundAt        *time.Time `json:"mfa_bound_at,omitempty"`
	RecoveryCodesJSON string     `gorm:"type:text" json:"-"` // sha256 后的恢复码 JSON 数组
	FailedLogins      int        `json:"-"`                  // 连续登录失败次数（含 MFA 错）
	LockedUntil       *time.Time `json:"-"`                  // 锁定期
	LastLoginAt       *time.Time `json:"last_login_at"`
	// SEC-02：首启生成的随机口令登录后，必须先改密（改密前仅发 scope=pw 受限会话）。
	MustChangePassword bool `json:"must_change_password"`
	// SEC-13：此时刻之前签发的全部 JWT 作废（登出/改密/解绑 MFA 时置为 now）。
	SessionsInvalidBefore *time.Time `json:"-"`
	// M-07：已成功核销的 TOTP 时间步（单调递增，杜绝 90s 窗口内重放）。
	LastTOTPStep int64     `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (u *AdminUser) IsLocked() bool {
	return u.LockedUntil != nil && time.Now().Before(*u.LockedUntil)
}

// ---------- 网关 API Key (REQ-002) ----------

type APIKey struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	Name      string `gorm:"uniqueIndex;size:64" json:"name"`
	KeyHash   string `gorm:"size:64;index" json:"-"` // sha256(hex)
	KeyMasked string `gorm:"size:64" json:"key_masked"`
	Remark    string `gorm:"size:255" json:"remark"`
	// AllowedModelsJSON: 允许的模型别名 JSON 数组；空=允许全部（默认）
	AllowedModelsJSON string `gorm:"type:text" json:"allowed_models_json"`
	// IPAllowlistJSON: IP/CIDR 白名单 JSON 数组；IPAllowlistEnabled=false 时忽略（默认放行）
	IPAllowlistJSON    string     `gorm:"type:text" json:"ip_allowlist_json"`
	IPAllowlistEnabled bool       `json:"ip_allowlist_enabled"`
	Enabled            bool       `json:"enabled"`
	LastUsedAt         *time.Time `json:"last_used_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// AllowsModel 判断该 Key 是否被授权访问指定别名；空列表表示允许全部。
// M-30：JSON 损坏时 fail-closed（拒绝全部）——此前放宽到全允许，坏记录=意外扩权。
func (k APIKey) AllowsModel(alias string) bool {
	if k.AllowedModelsJSON == "" {
		return true
	}
	var list []string
	if err := json.Unmarshal([]byte(k.AllowedModelsJSON), &list); err != nil {
		return false
	}
	if len(list) == 0 {
		return true
	}
	for _, m := range list {
		if m == alias {
			return true
		}
	}
	return false
}

// IPAllowed 判断请求 IP 是否在白名单内；未启用 IP 白名单时一律放行。
// 启用后，IP 不在列表内则拒绝（fail-closed）。
// L-02：条目支持裸 IP（按 /32 或 /128 精确匹配），此前只认 CIDR、裸 IP 静默永不匹配。
func (k APIKey) IPAllowed(ipStr string) bool {
	if !k.IPAllowlistEnabled {
		return true
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	var cidrs []string
	if err := json.Unmarshal([]byte(k.IPAllowlistJSON), &cidrs); err != nil {
		return false
	}
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if _, network, err := net.ParseCIDR(c); err == nil {
			if network.Contains(ip) {
				return true
			}
			continue
		}
		// 裸 IP：等价单机网段（v4→/32，v6→/128）
		if single := net.ParseIP(c); single != nil && single.Equal(ip) {
			return true
		}
	}
	return false
}

// ---------- 模型供应商 (REQ-005) ----------

// 协议类型
const (
	ProtoOpenAIChat      = "openai_chat"
	ProtoOpenAIResponses = "openai_responses"
	ProtoAnthropic       = "anthropic"
)

type Provider struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"uniqueIndex;size:64" json:"name"`
	Protocol     string    `gorm:"size:32" json:"protocol"`
	BaseURL      string    `gorm:"size:255" json:"base_url"`
	APIKeyEnc    string    `gorm:"type:text" json:"-"` // AES-GCM 加密存储 (REQ-005 ④)
	APIKey       string    `gorm:"-" json:"-"`         // 运行时内存明文（不入库）
	APIKeyMasked string    `gorm:"size:64" json:"api_key_masked"`
	Enabled      bool      `json:"enabled"`
	Remark       string    `gorm:"size:255" json:"remark"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ---------- 模型别名与上游 (REQ-006) ----------

type ModelAlias struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	Alias   string `gorm:"uniqueIndex;size:64" json:"alias"`
	Enabled bool   `json:"enabled"`
	Remark  string `gorm:"size:255" json:"remark"`
	// DefaultMaxTokens 别名级默认补全上限：客户端请求未带 max_tokens 时注入该值（0=不注入）。
	// 推理模型厂商默认补全上限常只有 ~1000 token（思维链即耗尽），必须在别名上兜底。
	DefaultMaxTokens int             `json:"default_max_tokens"`
	Upstreams        []AliasUpstream `gorm:"foreignKey:AliasID" json:"upstreams"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type AliasUpstream struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	AliasID       uint   `gorm:"index" json:"-"`
	ProviderID    uint   `gorm:"index" json:"provider_id"`
	UpstreamModel string `gorm:"size:64" json:"upstream_model"`
	Weight        int    `json:"weight"`
}

// ---------- 护栏规则 (REQ-008/009/010) ----------

type GuardKeyword struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Word      string    `gorm:"size:255;index" json:"word"`
	Category  string    `gorm:"size:32;index" json:"category"`
	MatchMode string    `gorm:"size:16" json:"match_mode"` // contains|exact|regex
	Action    string    `gorm:"size:16" json:"action"`     // block|warn|log
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PIIRule struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:64" json:"name"`
	Category    string    `gorm:"size:32;index" json:"category"`
	Pattern     string    `gorm:"size:512" json:"pattern"`
	Replacement string    `gorm:"size:64" json:"replacement"`
	Action      string    `gorm:"size:16" json:"action"` // mask|block|warn|log
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type InjectionRule struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64" json:"name"`
	Pattern   string    `gorm:"size:512" json:"pattern"`
	MatchMode string    `gorm:"size:16" json:"match_mode"` // contains|regex
	Action    string    `gorm:"size:16" json:"action"`     // block|warn|log
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ---------- 配额 (REQ-012) ----------

type Quota struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	APIKeyID     uint      `gorm:"index" json:"api_key_id"`
	ModelAlias   string    `gorm:"size:64;index" json:"model_alias"` // "*"=全部
	QuotaType    string    `gorm:"size:16" json:"quota_type"`        // requests|tokens
	Period       string    `gorm:"size:16" json:"period"`            // day|week|month
	LimitValue   int64     `json:"limit"`
	UsedValue    int64     `json:"used"`
	ResetAt      time.Time `json:"reset_at"`
	OverAction   string    `gorm:"size:16" json:"over_action"` // reject|degrade
	DegradeAlias string    `gorm:"size:64" json:"degrade_alias"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ---------- 速率限制 (REQ-013) ----------

type RateLimitRule struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	APIKeyID      uint      `gorm:"index" json:"api_key_id"` // 0=全局
	ModelAlias    string    `gorm:"size:64;index" json:"model_alias"`
	WindowSeconds int       `json:"window_seconds"`
	MaxRequests   int       `json:"max_requests"`
	TotalHits     int64     `json:"total_hits"` // 命中统计（异步落库）
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ---------- 系统参数 KV（通用设置/安全设置/输出过滤/故障转移/预警等） (REQ-001/007/011/014/020) ----------

type SystemSetting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	ValueJSON string    `gorm:"type:text" json:"value_json"`
	UpdatedAt time.Time `gorm:"index" json:"updated_at"`
}

// KV key 常量
const (
	SetKeyGeneral    = "general"       // REQ-001
	SetKeyFailover   = "failover"      // REQ-007
	SetKeyOutputFilt = "output_filter" // REQ-011
	SetKeyQuotaAlert = "quota_alerts"  // REQ-014
	SetKeySecurity   = "security"      // REQ-020
	SetKeyConfigMeta = "config_meta"   // 热加载版本戳/状态
)

// ---------- 配置版本与热加载日志 (REQ-004/004A) ----------

type ConfigVersion struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Version      string    `gorm:"size:32;index" json:"version"`
	SnapshotJSON string    `json:"-"`                     // 全量配置快照，用于回滚（无 size 的 string 由 GORM 按方言映射：mysql=longtext / pg=text / sqlite=text）
	Status       string    `gorm:"size:16" json:"status"` // ok|error
	Message      string    `gorm:"size:512" json:"message"`
	CreatedAt    time.Time `json:"created_at"`
}

type ConfigLoadLog struct {
	ID      uint      `gorm:"primaryKey" json:"id"`
	Time    time.Time `gorm:"index" json:"time"`
	Module  string    `gorm:"size:32" json:"module"`
	Status  string    `gorm:"size:16" json:"status"` // success|error
	Message string    `gorm:"size:512" json:"message"`
}

// ---------- 调用审计日志 (REQ-015) ----------

type CallLog struct {
	ID        uint   `gorm:"primaryKey" json:"-"`
	RequestID string `gorm:"uniqueIndex;size:40" json:"request_id"`
	// SEC-04：客户端自带的 X-Request-ID 记录于此（非唯一列，可伪造/重复），
	// request_id 恒为服务端生成——uniqueIndex 不再受客户端输入支配（防批删/顶号/日志注入）。
	ClientRequestID  string    `gorm:"size:128;index" json:"client_request_id,omitempty"`
	CreatedAt        time.Time `gorm:"index" json:"created_at"`
	APIKeyID         uint      `gorm:"index" json:"api_key_id"`
	APIKeyLabel      string    `gorm:"size:64" json:"api_key_label"`
	Protocol         string    `gorm:"size:32" json:"protocol"`
	ModelAlias       string    `gorm:"size:64;index" json:"model_alias"`
	UpstreamProvider string    `gorm:"size:64" json:"upstream_provider"`
	UpstreamModel    string    `gorm:"size:64" json:"upstream_model"`
	InputText        string    `json:"-"` // 已脱敏（NFR-009）；无 gorm type 标签：mysql→longtext / pg→text / sqlite→text，超长 prompt 不再 64KB 截断
	OutputText       string    `json:"-"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	LatencyMs        int64     `json:"latency_ms"`
	Status           string    `gorm:"size:24;index" json:"status"`  // ok|error|blocked|rate_limited|quota_exceeded
	FinishReason     string    `gorm:"size:32" json:"finish_reason"` // 上游 finish_reason（stop/length/...）：区分"模型自然结束"与"预算截断"的关键证据
	Blocked          bool      `gorm:"index" json:"blocked"`
	BlockCategory    string    `gorm:"size:64" json:"block_category"`
	BlockReason      string    `gorm:"size:512" json:"block_reason"`
	ErrorMsg         string    `gorm:"size:512" json:"error_msg"`
	GuardFindings    string    `gorm:"type:text" json:"-"` // JSON
}

// ---------- 操作审计日志 (REQ-003) ----------

type OperationLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
	Operator   string    `gorm:"size:64;index" json:"operator"`
	Action     string    `gorm:"size:24" json:"action"`
	Module     string    `gorm:"size:32;index" json:"module"`
	Target     string    `gorm:"size:128" json:"target"`
	BeforeJSON string    `gorm:"type:text" json:"before_json"`
	AfterJSON  string    `gorm:"type:text" json:"after_json"`
	IP         string    `gorm:"size:64" json:"ip"`
	Effective  bool      `json:"effective"` // 配置生效状态
}

// ---------- 备份记录 (REQ-019) ----------

type BackupRecord struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Filename  string    `gorm:"size:128" json:"filename"`
	Content   string    `json:"-"` // 备份内容（无 size 的 string 由 GORM 按方言映射，不用 MySQL 专属的 longtext 标签）
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

// AllModels 供 AutoMigrate 使用
func AllModels() []any {
	return []any{
		&AdminUser{}, &APIKey{}, &Provider{}, &ModelAlias{}, &AliasUpstream{},
		&GuardKeyword{}, &PIIRule{}, &InjectionRule{},
		&Quota{}, &RateLimitRule{}, &SystemSetting{},
		&ConfigVersion{}, &ConfigLoadLog{}, &CallLog{}, &OperationLog{}, &BackupRecord{},
	}
}
