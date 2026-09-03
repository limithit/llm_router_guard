/**
 * 契约枚举的展示字典（i18n 版）：META 只存 value/color，文案经 i18n 键
 * `dicts.<ns>.<value>` 取词，语言切换时随组件重渲染自动更新。
 */
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
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

export interface DictMeta<V extends string = string> {
  value: V;
  color?: string;
}

export const PROTOCOL_META: DictMeta<ProviderProtocol>[] = [
  { value: 'openai_chat' },
  { value: 'openai_responses' },
  { value: 'anthropic' },
];

export const KEYWORD_CATEGORY_META: DictMeta<KeywordCategory>[] = [
  { value: 'political', color: 'red' },
  { value: 'porn', color: 'magenta' },
  { value: 'violence', color: 'volcano' },
  { value: 'illegal', color: 'orange' },
  { value: 'discrimination', color: 'gold' },
  { value: 'custom', color: 'blue' },
];

export const MATCH_MODE_META: DictMeta<MatchMode>[] = [
  { value: 'contains', color: 'blue' },
  { value: 'exact', color: 'geekblue' },
  { value: 'regex', color: 'purple' },
];

export const ACTION_META: DictMeta<RuleAction>[] = [
  { value: 'block', color: 'red' },
  { value: 'warn', color: 'orange' },
  { value: 'log', color: 'default' },
  { value: 'mask', color: 'cyan' },
];

export const QUOTA_TYPE_META: DictMeta<QuotaType>[] = [
  { value: 'requests' },
  { value: 'tokens' },
];

export const PERIOD_META: DictMeta<QuotaPeriod>[] = [
  { value: 'day' },
  { value: 'week' },
  { value: 'month' },
];

export const OVER_ACTION_META: DictMeta<OverAction>[] = [
  { value: 'reject' },
  { value: 'degrade' },
];

export const CALL_STATUS_META: DictMeta<CallStatus>[] = [
  { value: 'ok', color: 'green' },
  { value: 'error', color: 'red' },
  { value: 'blocked', color: 'volcano' },
  { value: 'rate_limited', color: 'orange' },
  { value: 'quota_exceeded', color: 'purple' },
];

export const LOG_LEVEL_META: DictMeta<LogLevel>[] = [
  { value: 'debug' },
  { value: 'info' },
  { value: 'warn' },
  { value: 'error' },
];

export const VIOLATION_STRATEGY_META: DictMeta<ViolationStrategy>[] = [
  { value: 'replace' },
  { value: 'block' },
  { value: 'log' },
];

export const ALERT_CHANNEL_META: DictMeta<AlertChannel>[] = [
  { value: 'ui' },
  { value: 'webhook' },
];

export const BACKOFF_META: DictMeta<BackoffStrategy>[] = [
  { value: 'fixed' },
  { value: 'exponential' },
];

export const PII_CATEGORY_META: DictMeta[] = [
  { value: 'phone', color: 'blue' },
  { value: 'id_card', color: 'geekblue' },
  { value: 'email', color: 'cyan' },
  { value: 'bank_card', color: 'purple' },
  { value: 'address', color: 'magenta' },
  { value: 'custom', color: 'default' },
];

export const OP_ACTION_META: DictMeta[] = [
  { value: 'create' },
  { value: 'update' },
  { value: 'delete' },
  { value: 'enable' },
  { value: 'disable' },
  { value: 'reload' },
  { value: 'rollback' },
  { value: 'reset' },
  { value: 'login' },
  { value: 'mfa_bind' },
  { value: 'mfa_unbind' },
  { value: 'backup' },
  { value: 'restore' },
];

export const OP_MODULE_META: DictMeta[] = [
  { value: 'provider' },
  { value: 'model_alias' },
  { value: 'failover' },
  { value: 'guard' },
  { value: 'quota' },
  { value: 'rate_limit' },
  { value: 'apikey' },
  { value: 'settings' },
  { value: 'security' },
  { value: 'backup' },
  { value: 'auth' },
];

/** 颜色查找（非 hook） */
export function colorOf(metas: DictMeta[], value?: string | null): string {
  if (value == null) return 'default';
  return metas.find((m) => m.value === value)?.color ?? 'default';
}

/**
 * 本地化 Select options：t 变化（语言切换）时重新计算。
 * label 取 `dicts.<ns>.<value>`。
 */
export function useDictOptions<V extends string>(ns: string, metas: DictMeta<V>[]): DictItem<V>[] {
  const { t } = useTranslation();
  return useMemo(
    () => metas.map((m) => ({ value: m.value, label: t(`dicts.${ns}.${m.value}`), color: m.color })),
    [t, ns, metas],
  );
}

/**
 * 本地化 label 查询函数（兜底：无映射时回显原始 value）。
 * 等价于旧的 labelOf(MAP.labels, v)。
 */
export function useDictLabel(ns: string): (value?: string | null) => string {
  const { t } = useTranslation();
  return useCallback(
    (value?: string | null) => {
      if (value == null || value === '') return '-';
      const key = `dicts.${ns}.${value}`;
      const label = t(key);
      return label === key ? value : label;
    },
    [t, ns],
  );
}

/** 取颜色（与语言无关，非 hook）。等价于旧的 colorOf(MAP.colors, v)。 */
export function dictColor<V extends string>(metas: DictMeta<V>[], value?: string | null): string {
  if (value == null) return 'default';
  return metas.find((m) => m.value === value)?.color ?? 'default';
}
