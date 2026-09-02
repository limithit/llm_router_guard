/**
 * 契约枚举的展示字典（label 映射 + Select options + Tag 颜色）。
 * 枚举取值严格来自 docs/api-contract.md「通用枚举」，仅补充中文展示文案。
 */
import type {
  AlertChannel,
  BackoffStrategy,
  CallStatus,
  KeywordCategory,
  LogLevel,
  MatchMode,
  OverAction,
  ProviderProtocol,
  QuotaPeriod,
  QuotaType,
  RuleAction,
  ViolationStrategy,
} from '../api/types';

export interface DictItem<V extends string = string> {
  value: V;
  label: string;
  color?: string;
}

export const PROTOCOL_OPTIONS: DictItem<ProviderProtocol>[] = [
  { value: 'openai_chat', label: 'OpenAI Chat Completions' },
  { value: 'openai_responses', label: 'OpenAI Responses' },
  { value: 'anthropic', label: 'Anthropic Messages' },
];

export const KEYWORD_CATEGORY_OPTIONS: DictItem<KeywordCategory>[] = [
  { value: 'political', label: '涉政', color: 'red' },
  { value: 'porn', label: '色情', color: 'magenta' },
  { value: 'violence', label: '暴力', color: 'volcano' },
  { value: 'illegal', label: '违法', color: 'orange' },
  { value: 'discrimination', label: '歧视', color: 'gold' },
  { value: 'custom', label: '自定义', color: 'blue' },
];

export const MATCH_MODE_OPTIONS: DictItem<MatchMode>[] = [
  { value: 'contains', label: '包含匹配', color: 'blue' },
  { value: 'exact', label: '精确匹配', color: 'geekblue' },
  { value: 'regex', label: '正则匹配', color: 'purple' },
];

export const ACTION_OPTIONS: DictItem<RuleAction>[] = [
  { value: 'block', label: '拦截', color: 'red' },
  { value: 'warn', label: '告警', color: 'orange' },
  { value: 'log', label: '仅记录', color: 'default' },
  { value: 'mask', label: '脱敏', color: 'cyan' },
];

export const QUOTA_TYPE_OPTIONS: DictItem<QuotaType>[] = [
  { value: 'requests', label: '请求次数' },
  { value: 'tokens', label: 'Token 数' },
];

export const PERIOD_OPTIONS: DictItem<QuotaPeriod>[] = [
  { value: 'day', label: '按天' },
  { value: 'week', label: '按周' },
  { value: 'month', label: '按月' },
];

export const OVER_ACTION_OPTIONS: DictItem<OverAction>[] = [
  { value: 'reject', label: '拒绝请求' },
  { value: 'degrade', label: '降级到其他模型' },
];

export const CALL_STATUS_OPTIONS: DictItem<CallStatus>[] = [
  { value: 'ok', label: '成功', color: 'green' },
  { value: 'error', label: '错误', color: 'red' },
  { value: 'blocked', label: '拦截', color: 'volcano' },
  { value: 'rate_limited', label: '限流', color: 'orange' },
  { value: 'quota_exceeded', label: '超配额', color: 'purple' },
];

export const LOG_LEVEL_OPTIONS: DictItem<LogLevel>[] = [
  { value: 'debug', label: 'debug' },
  { value: 'info', label: 'info' },
  { value: 'warn', label: 'warn' },
  { value: 'error', label: 'error' },
];

export const VIOLATION_STRATEGY_OPTIONS: DictItem<ViolationStrategy>[] = [
  { value: 'replace', label: '替换为安全提示语' },
  { value: 'block', label: '阻断输出' },
  { value: 'log', label: '仅记录' },
];

export const ALERT_CHANNEL_OPTIONS: DictItem<AlertChannel>[] = [
  { value: 'ui', label: '页面内通知' },
  { value: 'webhook', label: 'Webhook' },
];

export const BACKOFF_OPTIONS: DictItem<BackoffStrategy>[] = [
  { value: 'fixed', label: '固定间隔' },
  { value: 'exponential', label: '指数退避' },
];

export const PII_CATEGORY_OPTIONS: DictItem[] = [
  { value: 'phone', label: '手机号', color: 'blue' },
  { value: 'id_card', label: '身份证号', color: 'geekblue' },
  { value: 'email', label: '邮箱', color: 'cyan' },
  { value: 'bank_card', label: '银行卡号', color: 'purple' },
  { value: 'address', label: '地址', color: 'magenta' },
  { value: 'custom', label: '自定义', color: 'default' },
];

export const OP_ACTION_OPTIONS: DictItem[] = [
  { value: 'create', label: '创建' },
  { value: 'update', label: '修改' },
  { value: 'delete', label: '删除' },
  { value: 'enable', label: '启用' },
  { value: 'disable', label: '禁用' },
  { value: 'reload', label: '重载配置' },
  { value: 'rollback', label: '配置回滚' },
  { value: 'reset', label: '配额重置' },
  { value: 'login', label: '登录' },
  { value: 'mfa_bind', label: 'MFA 绑定' },
  { value: 'mfa_unbind', label: 'MFA 解绑' },
  { value: 'backup', label: '备份' },
  { value: 'restore', label: '恢复' },
];

export const OP_MODULE_OPTIONS: DictItem[] = [
  { value: 'provider', label: '供应商' },
  { value: 'model_alias', label: '模型别名' },
  { value: 'failover', label: '故障转移' },
  { value: 'guard', label: '护栏' },
  { value: 'quota', label: '配额' },
  { value: 'rate_limit', label: '速率限制' },
  { value: 'apikey', label: 'API Key' },
  { value: 'settings', label: '系统设置' },
  { value: 'security', label: '安全设置' },
  { value: 'backup', label: '数据备份' },
  { value: 'auth', label: '认证' },
];

const toRecord = (list: DictItem[]) => {
  const labels: Record<string, string> = {};
  const colors: Record<string, string> = {};
  list.forEach((it) => {
    labels[it.value] = it.label;
    if (it.color) colors[it.value] = it.color;
  });
  return { labels, colors };
};

export const PROTOCOL_MAP = toRecord(PROTOCOL_OPTIONS);
export const CATEGORY_MAP = toRecord(KEYWORD_CATEGORY_OPTIONS);
export const MATCH_MODE_MAP = toRecord(MATCH_MODE_OPTIONS);
export const ACTION_MAP = toRecord(ACTION_OPTIONS);
export const QUOTA_TYPE_MAP = toRecord(QUOTA_TYPE_OPTIONS);
export const PERIOD_MAP = toRecord(PERIOD_OPTIONS);
export const OVER_ACTION_MAP = toRecord(OVER_ACTION_OPTIONS);
export const CALL_STATUS_MAP = toRecord(CALL_STATUS_OPTIONS);
export const LOG_LEVEL_MAP = toRecord(LOG_LEVEL_OPTIONS);
export const VIOLATION_STRATEGY_MAP = toRecord(VIOLATION_STRATEGY_OPTIONS);
export const ALERT_CHANNEL_MAP = toRecord(ALERT_CHANNEL_OPTIONS);
export const BACKOFF_MAP = toRecord(BACKOFF_OPTIONS);
export const OP_ACTION_MAP = toRecord(OP_ACTION_OPTIONS);
export const OP_MODULE_MAP = toRecord(OP_MODULE_OPTIONS);

/** 兜底 label（如 PII 的 category 为自定义值时原样展示） */
export const labelOf = (map: Record<string, string>, value?: string | null) =>
  value == null || value === '' ? '-' : map[value] ?? value;

export const colorOf = (map: Record<string, string>, value?: string | null) =>
  value == null ? 'default' : map[value] ?? 'default';
