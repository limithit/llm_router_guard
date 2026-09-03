/**
 * API 契约类型定义 —— 严格对应 docs/api-contract.md
 * 所有路径 / 字段名 / 枚举值均以契约为唯一依据，不得自创字段。
 */

// ============ 统一响应包裹 & 分页 ============
export interface ApiEnvelope<T = unknown> {
  code: number;
  message: string;
  data: T;
}

export interface PageData<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

export interface PageParams {
  page?: number;
  page_size?: number;
  keyword?: string;
}

// ============ 通用枚举字符串类型（与契约一致） ============
export type ProviderProtocol = 'openai_chat' | 'openai_responses' | 'anthropic';
export type KeywordCategory =
  | 'political'
  | 'porn'
  | 'violence'
  | 'illegal'
  | 'discrimination'
  | 'custom';
export type MatchMode = 'contains' | 'exact' | 'regex';
export type RuleAction = 'block' | 'warn' | 'log' | 'mask';
export type QuotaType = 'requests' | 'tokens';
export type QuotaPeriod = 'day' | 'week' | 'month';
export type OverAction = 'reject' | 'degrade';
export type CallStatus = 'ok' | 'error' | 'blocked' | 'rate_limited' | 'quota_exceeded';
export type LogLevel = 'debug' | 'info' | 'warn' | 'error';
export type ViolationStrategy = 'replace' | 'block' | 'log';
export type AlertChannel = 'ui' | 'webhook';
export type BackoffStrategy = 'fixed' | 'exponential';

// ============ 认证 / MFA ============
export interface User {
  id: number;
  username: string;
  mfa_enabled: boolean;
  last_login_at?: string | null;
  created_at?: string | null;
}

export interface LoginBody {
  username: string;
  password: string;
  totp_code?: string;
  recovery_code?: string;
  mfa_token?: string;
}

export interface LoginOk {
  token: string;
  user: User;
  need_bind_mfa?: boolean;
}

export interface LoginMfaRequired {
  mfa_required: true;
  mfa_token: string;
}

export type LoginResult = LoginOk | LoginMfaRequired;

export interface ChangePasswordBody {
  old_password: string;
  new_password: string;
}

export interface MfaSetupResult {
  secret: string;
  otpauth_url: string;
  qr_png_base64: string;
}

export interface MfaEnableBody {
  code: string;
}

export interface MfaEnableResult {
  recovery_codes: string[];
}

export interface MfaDisableBody {
  code: string;
}

export interface MfaStatus {
  enabled: boolean;
  bound_at?: string | null;
  recovery_codes_left?: number;
}

// ============ Dashboard (REQ-016) ============
export interface DashboardToday {
  calls: number;
  success: number;
  blocked: number;
  errors: number;
  avg_latency_ms: number;
}

export interface TrendPoint {
  date: string;
  calls: number;
  blocked: number;
}

export interface ModelRank {
  model: string;
  calls: number;
}

export interface BlockCategoryItem {
  category: string;
  count: number;
}

export interface UpstreamHealth {
  provider: string;
  protocol: string;
  healthy: boolean;
  fail_count: number;
}

export interface QuotaUsageItem {
  name: string;
  used: number;
  limit: number;
}

export interface DashboardSystem {
  version: string;
  uptime_seconds: number;
  config_status: string;
  go_version: string;
}

export interface DashboardData {
  today: DashboardToday;
  trend_7d: TrendPoint[];
  top_models: ModelRank[];
  block_categories: BlockCategoryItem[];
  upstream_health: UpstreamHealth[];
  quota_usage: QuotaUsageItem[];
  system: DashboardSystem;
}

// ============ 供应商 (REQ-005) ============
export interface Provider {
  id: number;
  name: string;
  protocol: ProviderProtocol;
  base_url: string;
  api_key_masked: string;
  enabled: boolean;
  remark: string;
  created_at?: string | null;
  updated_at?: string | null;
}

export interface ProviderInput {
  name: string;
  protocol: ProviderProtocol;
  base_url: string;
  /** 编辑时传空字符串 = 不修改 */
  api_key?: string;
  enabled?: boolean;
  remark?: string;
}

export interface ProviderTestResult {
  ok: boolean;
  latency_ms: number;
  message: string;
  models?: string[];
}

export interface ProviderImportInput {
  models: string[];
  enabled: boolean;
}

export interface ProviderImportResult {
  created: number;
  added: number;
  skipped: number;
  skipped_names: string[];
}

// ============ 模型别名 (REQ-006) ============
export interface Upstream {
  provider_id: number;
  provider_name: string;
  provider_enabled: boolean;
  upstream_model: string;
  weight: number;
}

export interface UpstreamInput {
  provider_id: number;
  upstream_model: string;
  weight: number;
}

export interface ModelAlias {
  id: number;
  alias: string;
  enabled: boolean;
  remark: string;
  upstreams: Upstream[];
  created_at?: string | null;
  updated_at?: string | null;
}

export interface ModelAliasInput {
  alias: string;
  enabled: boolean;
  remark?: string;
  upstreams: UpstreamInput[];
}

export interface ModelStats {
  calls_7d: number;
  calls_today: number;
  blocked_7d: number;
  avg_latency_ms: number;
}

// ============ 故障转移 (REQ-007) ============
export interface FailoverConfig {
  enabled: boolean;
  retry_count: number;
  backoff: BackoffStrategy;
  retry_interval_ms: number;
  trigger_status_codes: number[];
  circuit_failure_threshold: number;
  circuit_reset_seconds: number;
}

// ============ 敏感词 (REQ-008) ============
export interface KeywordFilter extends PageParams {
  category?: KeywordCategory | '';
  enabled?: boolean;
  action?: RuleAction | '';
}

export interface Keyword {
  id: number;
  word: string;
  category: KeywordCategory;
  match_mode: MatchMode;
  action: RuleAction;
  enabled: boolean;
  created_at?: string | null;
}

export interface KeywordInput {
  word: string;
  category: KeywordCategory;
  match_mode: MatchMode;
  action: RuleAction;
  enabled: boolean;
}

export interface KeywordImportBody {
  csv: string;
}

export interface KeywordImportResult {
  created: number;
  skipped: number;
}

export interface GuardHit {
  type: string;
  value: string;
  category: string;
  action: string;
}

export interface GuardTestResult {
  blocked: boolean;
  hits: GuardHit[];
}

// ============ PII 规则 (REQ-009) ============
export interface PiiRule {
  id: number;
  name: string;
  category: string;
  pattern: string;
  replacement: string;
  action: RuleAction;
  enabled: boolean;
  created_at?: string | null;
}

export interface PiiRuleInput {
  name: string;
  category: string;
  pattern: string;
  replacement?: string;
  action: RuleAction;
  enabled: boolean;
}

export type PiiRuleTemplate = Omit<PiiRule, 'id' | 'enabled' | 'created_at'>;

export interface PiiFinding {
  rule: string;
  category: string;
  sample: string;
}

export interface PiiTestResult {
  blocked: boolean;
  findings: PiiFinding[];
  masked_text: string;
}

// ============ 注入规则 (REQ-010) ============
export interface InjectionRule {
  id: number;
  name: string;
  pattern: string;
  match_mode: MatchMode;
  action: RuleAction;
  enabled: boolean;
  created_at?: string | null;
}

export interface InjectionRuleInput {
  name: string;
  pattern: string;
  match_mode: MatchMode;
  action: RuleAction;
  enabled: boolean;
}

export type InjectionRuleTemplate = Omit<InjectionRule, 'id' | 'enabled' | 'created_at'>;

// ============ 输出过滤 (REQ-011) ============
export interface OutputConfig {
  enabled: boolean;
  violation_strategy: ViolationStrategy;
  safe_message: string;
  stream_chunk_threshold: number;
}

// ============ 配额 (REQ-012) ============
export interface Quota {
  id: number;
  api_key_id: number;
  api_key_label: string;
  model_alias: string;
  quota_type: QuotaType;
  period: QuotaPeriod;
  limit: number;
  used: number;
  over_action: OverAction;
  degrade_alias: string;
  enabled: boolean;
  reset_at?: string | null;
  created_at?: string | null;
}

export interface QuotaInput {
  api_key_id: number;
  model_alias: string;
  quota_type: QuotaType;
  period: QuotaPeriod;
  limit: number;
  over_action: OverAction;
  degrade_alias?: string;
  enabled: boolean;
}

export interface QuotaBatchBody {
  items: QuotaInput[];
}

// ============ 速率限制 (REQ-013) ============
export interface RateLimit {
  id: number;
  api_key_id: number;
  api_key_label: string;
  model_alias: string;
  window_seconds: number;
  max_requests: number;
  enabled: boolean;
  total_hits: number;
}

export interface RateLimitInput {
  api_key_id: number;
  model_alias: string;
  window_seconds: number;
  max_requests: number;
  enabled: boolean;
}

// ============ 配额预警 (REQ-014) ============
export interface QuotaAlertConfig {
  enabled: boolean;
  thresholds: number[];
  channel: AlertChannel;
  webhook_url: string;
  receivers: string;
}

// ============ 审计日志 (REQ-015 / REQ-003) ============
export interface CallLogFilter {
  start?: string;
  end?: string;
  api_key_id?: number;
  model?: string;
  status?: CallStatus | '';
  blocked?: boolean;
  category?: string;
  page?: number;
  page_size?: number;
}

export interface CallLogRow {
  request_id: string;
  created_at: string;
  api_key_label: string;
  protocol: string;
  model_alias: string;
  upstream: string;
  input_preview: string;
  output_preview: string;
  prompt_tokens: number;
  completion_tokens: number;
  latency_ms: number;
  status: CallStatus;
  blocked: boolean;
  block_category: string;
  block_reason: string;
}

export interface CallLogDetail extends CallLogRow {
  input_text: string;
  output_text: string;
  error_msg: string;
  /** JSON 字符串，展示时前端自行 parse */
  guard_findings: string;
}

export interface OpLogFilter {
  start?: string;
  end?: string;
  operator?: string;
  module?: string;
  action?: string;
  page?: number;
  page_size?: number;
}

export interface OpLogRow {
  id: number;
  created_at: string;
  operator: string;
  action: string;
  module: string;
  target: string;
  before_json: string;
  after_json: string;
  ip: string;
  effective: boolean;
}

// ============ Token 用量统计（API Key 维度看板） ============
export interface TokenStatsFilter {
  /** RFC3339；缺省后端默认近 30 天 */
  start?: string;
  /** RFC3339 */
  end?: string;
  api_key_id?: number;
  /** 模型别名（模糊匹配） */
  model?: string;
  /** day（默认）| hour（短时窗用小时分桶） */
  granularity?: 'day' | 'hour';
}

export interface TokenStatsSummary {
  calls: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  keys_with_usage: number;
  total_keys: number;
}

export interface TokenUsageByKey {
  api_key_id: number;
  api_key_label: string;
  calls: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

export interface TokenUsageByModel {
  model: string;
  calls: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

export interface TokenUsageTrendPoint {
  /** day 粒度：YYYY-MM-DD；hour 粒度：YYYY-MM-DD HH:00 */
  bucket: string;
  calls: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

export interface TokenStatsData {
  range: { start: string; end: string; granularity: 'day' | 'hour' };
  summary: TokenStatsSummary;
  by_key: TokenUsageByKey[];
  by_model: TokenUsageByModel[];
  trend: TokenUsageTrendPoint[];
}

// ============ 系统设置 (REQ-001) ============
export interface SystemSettings {
  log_level: LogLevel;
  audit_retention_days: number;
  default_timeout_seconds: number;
  max_connections: number;
  guard_enabled: boolean;
  hot_reload_seconds: number;
  call_audit_enabled: boolean;
  call_audit_only_errors: boolean;
  call_audit_sampling: number;
  /** 只读：仅启动参数生效 */
  listen_port: number;
  listen_port_note: string;
}

export interface SystemSettingsInput {
  log_level: LogLevel;
  audit_retention_days: number;
  default_timeout_seconds: number;
  max_connections: number;
  guard_enabled: boolean;
  hot_reload_seconds: number;
  call_audit_enabled: boolean;
  call_audit_only_errors: boolean;
  call_audit_sampling: number;
}

// ============ API Key (REQ-002) ============
export interface ApiKey {
  id: number;
  name: string;
  key_masked: string;
  remark: string;
  allowed_models_json: string; // JSON 数组 of 别名；空=允许全部（默认）
  ip_allowlist_json: string; // JSON 数组 of CIDR
  ip_allowlist_enabled: boolean;
  enabled: boolean;
  last_used_at?: string | null;
  created_at?: string | null;
}

export interface ApiKeyCreateBody {
  name: string;
  remark?: string;
  allowed_models_json?: string;
  ip_allowlist_json?: string;
  ip_allowlist_enabled?: boolean;
}

/** 创建成功时额外一次性返回完整 key */
export interface ApiKeyCreated {
  id: number;
  name: string;
  key: string;
}

export interface ApiKeyUpdateBody {
  name: string;
  remark?: string;
  enabled: boolean;
  allowed_models_json?: string;
  ip_allowlist_json?: string;
  ip_allowlist_enabled?: boolean;
}

// ============ 安全设置 (REQ-020 / REQ-023) ============
export interface SecuritySettings {
  mfa_enabled: boolean;
  mfa_required_for_all: boolean;
  recovery_code_count: number;
  grace_days: number;
}

export interface AdminUserRow {
  id: number;
  username: string;
  mfa_enabled: boolean;
  last_login_at?: string | null;
  locked: boolean;
}

// ============ 配置状态 (REQ-004A) ============
export interface ConfigLoadLog {
  time: string;
  module: string;
  status: 'success' | 'error';
  message: string;
}

export interface ConfigHistory {
  version: string;
  created_at: string;
  status: string;
  message: string;
}

export interface ConfigStatus {
  version: string;
  last_loaded_at: string;
  status: string;
  error_message: string;
  modules: Record<string, number>;
  logs: ConfigLoadLog[];
  history: ConfigHistory[];
}

export interface RollbackResult {
  rolled_back_to: string;
}

// ============ 状态监控 (REQ-018) ============
export interface UpstreamRuntime {
  provider: string;
  healthy: boolean;
  fail_count: number;
  last_error: string;
}

export interface QpsByModel {
  model: string;
  qps: number;
  errors_1m: number;
}

export interface RecentError {
  time: string;
  request_id: string;
  message: string;
}

export interface RuntimeStatus {
  running: boolean;
  uptime_seconds: number;
  version: string;
  memory_alloc_kb: number;
  cpu_percent: number;
  goroutines: number;
  open_connections: number;
  upstreams: UpstreamRuntime[];
  qps_by_model: QpsByModel[];
  recent_errors: RecentError[];
  config_status: {
    version: string;
    last_loaded_at: string;
    status: string;
  };
}

// ============ 备份 (REQ-019) ============
export interface BackupItem {
  id: number;
  filename: string;
  created_at: string;
  size_bytes: number;
}

export interface BackupRestoreBody {
  id?: number;
  content?: string;
}
