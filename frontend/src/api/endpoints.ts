/**
 * API 函数（按模块组织）—— 请求路径与字段严格遵循 docs/api-contract.md
 * 统一返回已解包 envelope 后的 data。
 */
import { asPage, downloadFile, http } from './client';
import type {
  AdminUserRow,
  ApiKey,
  ApiKeyCreateBody,
  ApiKeyCreated,
  ApiKeyUpdateBody,
  BackupItem,
  BackupRestoreBody,
  CallLogDetail,
  CallLogFilter,
  CallLogRow,
  ChangePasswordBody,
  ConfigStatus,
  DashboardData,
  FailoverConfig,
  GuardTestResult,
  InjectionRule,
  InjectionRuleInput,
  InjectionRuleTemplate,
  Keyword,
  KeywordCategory,
  KeywordFilter,
  KeywordImportResult,
  KeywordInput,
  LoginBody,
  LoginResult,
  MatchMode,
  MfaDisableBody,
  MfaEnableBody,
  MfaEnableResult,
  MfaSetupResult,
  MfaStatus,
  ModelAlias,
  ModelAliasInput,
  ModelStats,
  OpLogFilter,
  OpLogRow,
  OutputConfig,
  PageData,
  PageParams,
  PiiRule,
  PiiRuleInput,
  PiiRuleTemplate,
  PiiTestResult,
  Provider,
  ProviderImportInput,
  ProviderImportResult,
  ProviderInput,
  ProviderTestResult,
  Quota,
  QuotaAlertConfig,
  QuotaBatchBody,
  QuotaInput,
  RateLimit,
  RateLimitInput,
  RollbackResult,
  RuleAction,
  RuntimeStatus,
  SecuritySettings,
  SystemSettings,
  SystemSettingsInput,
  TokenStatsData,
  TokenStatsFilter,
  User,
} from './types';

// ============ 1. 认证 / MFA ============
export const authApi = {
  login: (body: LoginBody) => http.post<LoginResult>('/auth/login', body),
  logout: () => http.post<null>('/auth/logout'),
  me: () => http.get<User>('/auth/me'),
  changePassword: (body: ChangePasswordBody) => http.put<null>('/auth/password', body),
};

export const accountMfaApi = {
  setup: () => http.post<MfaSetupResult>('/account/mfa/setup'),
  enable: (body: MfaEnableBody) => http.post<MfaEnableResult>('/account/mfa/enable', body),
  disable: (body: MfaDisableBody) => http.post<null>('/account/mfa/disable', body),
  status: () => http.get<MfaStatus>('/account/mfa/status'),
};

// ============ 2. Dashboard (REQ-016) ============
export const dashboardApi = {
  get: () => http.get<DashboardData>('/dashboard'),
};

// ============ 3. 供应商 (REQ-005) ============
export const providerApi = {
  list: (params: PageParams & { keyword?: string }) =>
    http.get<PageData<Provider>>('/providers', params),
  create: (body: ProviderInput) => http.post<Provider>('/providers', body),
  update: (id: number, body: ProviderInput) => http.put<Provider>(`/providers/${id}`, body),
  remove: (id: number) => http.delete<null>(`/providers/${id}`),
  test: (id: number) => http.post<ProviderTestResult>(`/providers/${id}/test`),
  importModels: (id: number, body: ProviderImportInput) =>
    http.post<ProviderImportResult>(`/providers/${id}/import-models`, body),
};

// ============ 4. 模型别名 (REQ-006) ============
export const modelApi = {
  list: (params: PageParams) => http.get<PageData<ModelAlias>>('/models', params),
  create: (body: ModelAliasInput) => http.post<ModelAlias>('/models', body),
  update: (id: number, body: ModelAliasInput) => http.put<ModelAlias>(`/models/${id}`, body),
  remove: (id: number) => http.delete<null>(`/models/${id}`),
  stats: (id: number) => http.get<ModelStats>(`/models/${id}/stats`),
};

// ============ 5. 故障转移 (REQ-007) ============
export const failoverApi = {
  get: () => http.get<FailoverConfig>('/failover'),
  save: (body: FailoverConfig) => http.put<FailoverConfig>('/failover', body),
};

// ============ 6. 护栏管理 ============

// 6.1 敏感词 (REQ-008)
export const keywordApi = {
  list: (params: KeywordFilter) => http.get<PageData<Keyword>>('/guard/keywords', params),
  create: (body: KeywordInput) => http.post<Keyword>('/guard/keywords', body),
  update: (id: number, body: KeywordInput) => http.put<Keyword>(`/guard/keywords/${id}`, body),
  remove: (id: number) => http.delete<null>(`/guard/keywords/${id}`),
  /** 粘贴 CSV 文本或上传文件 → 服务端解析（自动处理引号/换行） */
  importCsv: (csv: string) =>
    http.post<KeywordImportResult>('/guard/keywords/import', { csv } satisfies { csv: string }),
  test: (text: string) => http.post<GuardTestResult>('/guard/keywords/test', { text }),
  /** 导出 CSV（raw text/csv，不走 envelope），直接触发浏览器下载 */
  exportCsv: (params?: Partial<KeywordFilter>) =>
    downloadFile('/guard/keywords/export', {
      params: { format: 'csv', ...params },
      fallbackName: `keywords-${Date.now()}.csv`,
    }),
};

// 6.2 PII 规则 (REQ-009)
export const piiApi = {
  list: (params: PageParams) => http.get<PageData<PiiRule>>('/guard/pii-rules', params),
  create: (body: PiiRuleInput) => http.post<PiiRule>('/guard/pii-rules', body),
  update: (id: number, body: PiiRuleInput) => http.put<PiiRule>(`/guard/pii-rules/${id}`, body),
  remove: (id: number) => http.delete<null>(`/guard/pii-rules/${id}`),
  templates: () => http.get<PiiRuleTemplate[]>('/guard/pii-rules/templates'),
  test: (text: string) => http.post<PiiTestResult>('/guard/pii-rules/test', { text }),
};

// 6.3 注入规则 (REQ-010)
export const injectionApi = {
  list: (params: PageParams) => http.get<PageData<InjectionRule>>('/guard/injection-rules', params),
  create: (body: InjectionRuleInput) =>
    http.post<InjectionRule>('/guard/injection-rules', body),
  update: (id: number, body: InjectionRuleInput) =>
    http.put<InjectionRule>(`/guard/injection-rules/${id}`, body),
  remove: (id: number) => http.delete<null>(`/guard/injection-rules/${id}`),
  templates: () => http.get<InjectionRuleTemplate[]>('/guard/injection-rules/templates'),
};

// 6.4 输出过滤 (REQ-011)
export const outputApi = {
  get: () => http.get<OutputConfig>('/guard/output'),
  save: (body: OutputConfig) => http.put<OutputConfig>('/guard/output', body),
};

// ============ 7. 配额与速率限制 ============

// 7.1 配额 (REQ-012)
export const quotaApi = {
  list: (params: PageParams) => http.get<PageData<Quota>>('/quotas', params),
  create: (body: QuotaInput) => http.post<Quota>(`/quotas`, body),
  update: (id: number, body: QuotaInput) => http.put<Quota>(`/quotas/${id}`, body),
  remove: (id: number) => http.delete<null>(`/quotas/${id}`),
  reset: (id: number) => http.post<null>(`/quotas/${id}/reset`),
  batch: (body: QuotaBatchBody) => http.post<unknown>(`/quotas/batch`, body),
};

// 7.2 速率限制 (REQ-013)
export const rateLimitApi = {
  list: async (params: PageParams) =>
    asPage<RateLimit>(await http.get<unknown>('/rate-limits', params)),
  create: (body: RateLimitInput) => http.post<RateLimit>('/rate-limits', body),
  update: (id: number, body: RateLimitInput) => http.put<RateLimit>(`/rate-limits/${id}`, body),
  remove: (id: number) => http.delete<null>(`/rate-limits/${id}`),
};

// 7.3 配额预警 (REQ-014)
export const quotaAlertApi = {
  get: () => http.get<QuotaAlertConfig>('/quota-alerts'),
  save: (body: QuotaAlertConfig) => http.put<QuotaAlertConfig>('/quota-alerts', body),
};

// ============ 8. 审计日志 ============

// 8.1 调用日志 (REQ-015)
export const callLogApi = {
  list: (params: CallLogFilter) => http.get<PageData<CallLogRow>>('/audit/calls', params),
  detail: (requestId: string) =>
    http.get<CallLogDetail>(`/audit/calls/${encodeURIComponent(requestId)}`),
  exportCsv: (params: Omit<CallLogFilter, 'page' | 'page_size'>) =>
    downloadFile('/audit/calls/export', {
      params: { format: 'csv', ...params },
      fallbackName: `call-logs-${Date.now()}.csv`,
    }),
};

// 8.2 操作审计 (REQ-003)
export const opLogApi = {
  list: (params: OpLogFilter) => http.get<PageData<OpLogRow>>('/audit/operations', params),
  exportCsv: (params: Omit<OpLogFilter, 'page' | 'page_size'>) =>
    downloadFile('/audit/operations/export', {
      params: { format: 'csv', ...params },
      fallbackName: `operation-logs-${Date.now()}.csv`,
    }),
};

// 8.3 Token 用量统计（API Key 维度看板）
export const tokenStatsApi = {
  get: (params: TokenStatsFilter) => http.get<TokenStatsData>('/token-stats', params),
  exportCsv: (params: Omit<TokenStatsFilter, 'granularity'>) =>
    downloadFile('/token-stats/export', {
      params: { format: 'csv', ...params },
      fallbackName: `token-stats-${Date.now()}.csv`,
    }),
};

// ============ 9. 系统设置 ============

// 9.1 通用设置 (REQ-001)
export const settingsApi = {
  get: () => http.get<SystemSettings>('/settings'),
  save: (body: SystemSettingsInput) => http.put<SystemSettings>('/settings', body),
};

// 9.2 API Key (REQ-002)
export const apikeyApi = {
  list: (params: PageParams) => http.get<PageData<ApiKey>>('/apikeys', params),
  create: (body: ApiKeyCreateBody) => http.post<ApiKeyCreated>('/apikeys', body),
  update: (id: number, body: ApiKeyUpdateBody) => http.put<ApiKey>(`/apikeys/${id}`, body),
  remove: (id: number) => http.delete<null>(`/apikeys/${id}`),
};

// 9.3 安全设置 (REQ-020/023)
export const securityApi = {
  get: () => http.get<SecuritySettings>('/settings/security'),
  save: (body: SecuritySettings) => http.put<SecuritySettings>('/settings/security', body),
};

export const userApi = {
  list: (params: PageParams) => http.get<PageData<AdminUserRow>>('/users', params),
  unbindMfa: (id: number) => http.post<null>(`/users/${id}/unbind-mfa`),
  unlock: (id: number) => http.post<null>(`/users/${id}/unlock`),
};

// 9.4 配置状态 (REQ-004A)
export const configStatusApi = {
  get: () => http.get<ConfigStatus>('/config-status'),
  reload: () => http.post<null>('/config/reload'),
  rollback: () => http.post<RollbackResult>('/config/rollback'),
  /** 导出当前全量配置（raw JSON 文本，不走 envelope） */
  exportJson: () =>
    downloadFile('/config/export', { params: { format: 'json' }, fallbackName: 'config.json' }),
};

// ============ 10. 系统运维 ============

// 10.1 状态监控 (REQ-018)
export const runtimeStatusApi = {
  get: () => http.get<RuntimeStatus>('/status'),
};

// 10.2 数据备份 (REQ-019)
export const backupApi = {
  list: async () =>
    asPage<BackupItem>(await http.get<unknown>('/backups')),
  create: () => http.post<BackupItem | null>('/backups'),
  restore: (body: BackupRestoreBody) => http.post<null>('/backups/restore', body),
  remove: (id: number) => http.delete<null>(`/backups/${id}`),
  download: (id: number) =>
    downloadFile(`/backups/${id}/download`, { fallbackName: `backup-${id}.json` }),
};

// ============ 导出为类型重导出，供页面引用 ============
export type {
  KeywordCategory,
  MatchMode,
  RuleAction,
};
