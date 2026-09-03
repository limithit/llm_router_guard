import dayjs from 'dayjs';
import i18n from '../i18n';

/** RFC3339 → 'YYYY-MM-DD HH:mm:ss'（契约要求时间字段统一 RFC3339） */
export function fmtTime(value?: string | number | null): string {
  if (value == null || value === '') return '-';
  const d = dayjs(value);
  return d.isValid() ? d.format('YYYY-MM-DD HH:mm:ss') : String(value);
}

export function fmtTimeShort(value?: string | null): string {
  if (value == null || value === '') return '-';
  const d = dayjs(value);
  return d.isValid() ? d.format('HH:mm:ss') : String(value);
}

export function fmtBytes(bytes?: number): string {
  if (bytes == null || Number.isNaN(bytes)) return '-';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`;
}

export function fmtDuration(totalSeconds?: number): string {
  if (totalSeconds == null || Number.isNaN(totalSeconds)) return '-';
  const s = Math.max(0, Math.floor(totalSeconds));
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  const D = i18n.t('common.day'), H = i18n.t('common.hour'), M = i18n.t('common.minute'), S = i18n.t('common.second');
  if (d > 0) return `${d}${D}${h}${H}${m}${M}`;
  if (h > 0) return `${h}${H}${m}${M}${sec}${S}`;
  if (m > 0) return `${m}${M}${sec}${S}`;
  return `${sec}${S}`;
}

export function fmtNumber(n?: number | null): string {
  if (n == null || Number.isNaN(n)) return '-';
  return n.toLocaleString(i18n.language === 'zh-CN' ? 'zh-CN' : 'en-US');
}

/** 使用率 → antd Progress 颜色（超额自动变色） */
export function usagePercentColor(percent: number): 'normal' | 'exception' | 'success' | 'active' {
  if (percent > 100) return 'exception';
  if (percent >= 90) return 'exception';
  if (percent >= 70) return 'normal';
  return 'success';
}

export function clampPercent(used?: number, limit?: number): number {
  if (!limit) return 0;
  return Math.round(((used ?? 0) / limit) * 10000) / 100;
}
