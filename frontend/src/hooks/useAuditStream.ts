/**
 * useAuditStream — 实时调用审计 WebSocket 订阅（P2 #5）。
 * 后端：GET /api/admin/v1/audit/ws?token=<JWT>（服务端推送 model.CallLog JSON 帧）。
 * 特性：断线 3s 自动重连（页面挂载期间无限重试）、最多保留 max 条（新事件在前）。
 */
import { useEffect, useRef, useState } from 'react';
import { useAuthStore } from '../store/auth';
import { i18n } from '../i18n';

/** 服务端推送行（model.CallLog JSON，字段与 /audit/calls 列表略有差异：无 preview/upstream 合成字段） */
export interface AuditLiveRow {
  request_id: string;
  created_at: string;
  api_key_label: string;
  protocol: string;
  model_alias: string;
  upstream_provider: string;
  upstream_model: string;
  prompt_tokens: number;
  completion_tokens: number;
  latency_ms: number;
  status: string;
  blocked: boolean;
  block_category: string;
  block_reason: string;
}

export interface AuditStreamState {
  /** WebSocket 连接状态：connecting | live | disconnected */
  phase: 'connecting' | 'live' | 'disconnected';
  events: AuditLiveRow[];
  /** 已收事件总数（会话累计，用于角标） */
  total: number;
  /** 清空已收列表（连接保持） */
  reset: () => void;
}

const RETRY_MS = 3000;

export function useAuditStream(enabled: boolean, max = 30): AuditStreamState {
  const [phase, setPhase] = useState<AuditStreamState['phase']>('connecting');
  const [events, setEvents] = useState<AuditLiveRow[]>([]);
  const [total, setTotal] = useState(0);
  const enabledRef = useRef(enabled);
  enabledRef.current = enabled;
  const maxRef = useRef(max);
  maxRef.current = max;

  const reset = () => {
    setEvents([]);
    setTotal(0);
  };

  useEffect(() => {
    if (!enabled) return;
    let ws: WebSocket | null = null;
    let retryTimer: number | undefined;
    let closedByUs = false;

    const connect = () => {
      if (!enabledRef.current) return;
      const token = useAuthStore.getState().token;
      if (!token) return;
      const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const url = `${proto}//${window.location.host}/api/admin/v1/audit/ws?token=${encodeURIComponent(token)}`;
      setPhase((p) => (p === 'live' ? p : 'connecting'));
      ws = new WebSocket(url);

      ws.onopen = () => setPhase('live');
      ws.onmessage = (ev) => {
        try {
          const row = JSON.parse(ev.data as string) as AuditLiveRow;
          setEvents((prev) => [row, ...prev].slice(0, maxRef.current));
          setTotal((n) => n + 1);
        } catch {
          // 非 JSON 帧忽略（不应出现）
        }
      };
      ws.onclose = () => {
        setPhase('disconnected');
        if (!closedByUs && enabledRef.current) {
          retryTimer = window.setTimeout(connect, RETRY_MS);
        }
      };
      ws.onerror = () => {
        // onclose 随后触发，统一在那里重连
      };
    };

    connect();
    return () => {
      closedByUs = true;
      if (retryTimer) window.clearTimeout(retryTimer);
      ws?.close();
    };
  }, [enabled, max]);

  return { phase, events, total, reset };
}

/** 连接状态的本地化文案 */
export function auditStreamPhaseText(phase: AuditStreamState['phase']): string {
  switch (phase) {
    case 'live':
      return i18n.t('calls.live.live');
    case 'connecting':
      return i18n.t('calls.live.connecting');
    default:
      return i18n.t('calls.live.disconnected');
  }
}
