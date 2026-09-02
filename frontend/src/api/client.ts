/**
 * axios 实例 + 拦截器
 * - 请求：自动携带 Authorization: Bearer <token>
 * - 响应：统一解 envelope（code===0 返回 data；code!==0 时 message.error 并 reject）
 * - HTTP 401：清空本地登录态并跳转 /login
 * - 导出类接口（responseType: 'blob'）：不做 envelope 解包，直接返回 Blob
 */
import axios, { AxiosError, type AxiosRequestConfig, type AxiosResponse } from 'axios';
import { message } from 'antd';
import { useAuthStore } from '../store/auth';
import type { ApiEnvelope } from './types';

/** 管理 API Base URL（见 api-contract.md） */
export const API_BASE = '/api/admin/v1';

const client = axios.create({
  baseURL: API_BASE,
  timeout: 20000,
});

// 401 跳转防抖
let redirectingToLogin = false;

function redirectToLogin() {
  if (redirectingToLogin) return;
  redirectingToLogin = true;
  const { clearAuth } = useAuthStore.getState();
  clearAuth();
  if (window.location.pathname !== '/login') {
    window.location.href = '/login';
  }
  window.setTimeout(() => {
    redirectingToLogin = false;
  }, 500);
}

client.interceptors.request.use((config) => {
  const { token } = useAuthStore.getState();
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

client.interceptors.response.use(
  (response: AxiosResponse) => {
    // 二进制/文本下载类响应不参与 envelope 解包
    if (response.config.responseType === 'blob' || response.config.responseType === 'text') {
      return response;
    }
    const body = response.data as ApiEnvelope<unknown> | undefined;
    if (body && typeof body.code === 'number') {
      if (body.code === 0) {
        return body.data as never;
      }
      // 业务错误：直接 toast（401 类错误静默，交给错误拦截器处理跳转）
      if (body.code !== 40101 && body.code !== 40102 && body.code !== 40103) {
        message.error(body.message || '请求失败');
      }
      const err = new Error(body.message || '请求失败') as Error & {
        code?: number;
        httpStatus?: number;
      };
      err.code = body.code;
      err.httpStatus = response.status;
      return Promise.reject(err);
    }
    // 非 envelope 结构（理论不应出现），透传
    return body as never;
  },
  (error: AxiosError<ApiEnvelope<unknown>>) => {
    const status = error.response?.status;
    const bizMessage = error.response?.data?.message;
    if (status === 401) {
      redirectToLogin();
      return Promise.reject(error);
    }
    if (bizMessage) {
      message.error(bizMessage);
    } else {
      message.error(error.message || '网络错误，请稍后重试');
    }
    return Promise.reject(error);
  }
);

/** 类型化 GET/POST/PUT/DELETE 辅助（拦截器已解包 envelope） */
async function request<T>(config: AxiosRequestConfig): Promise<T> {
  const res = await client.request(config);
  return res as unknown as T;
}

export const http = {
  get: <T>(url: string, params?: object) =>
    request<T>({ url, method: 'get', params }),
  post: <T>(url: string, data?: unknown, params?: object) =>
    request<T>({ url, method: 'post', data, params }),
  put: <T>(url: string, data?: unknown) => request<T>({ url, method: 'put', data }),
  delete: <T>(url: string) => request<T>({ url, method: 'delete' }),
};

/** 契约中若干列表接口可能直接返回数组（如 /rate-limits、/backups），统一规整为分页结构 */
export function asPage<T>(data: unknown): { items: T[]; total: number; page: number; page_size: number } {
  if (Array.isArray(data)) {
    const arr = data as T[];
    return { items: arr, total: arr.length, page: 1, page_size: Math.max(arr.length, 1) };
  }
  const d = (data ?? {}) as Partial<{ items: T[]; total: number; page: number; page_size: number }>;
  const items = Array.isArray(d.items) ? d.items : [];
  return {
    items,
    total: typeof d.total === 'number' ? d.total : items.length,
    page: d.page ?? 1,
    page_size: d.page_size ?? (items.length || 20),
  };
}

/** 下载类接口（CSV / JSON / 备份文件）：Blob + a[download] 触发浏览器下载 */
export async function downloadFile(
  url: string,
  opts?: { params?: Record<string, unknown>; fallbackName?: string }
): Promise<void> {
  const response = (await client.get(url, {
    params: opts?.params,
    responseType: 'blob',
  })) as unknown as AxiosResponse<Blob>;
  let filename = opts?.fallbackName || 'download';
  const cd = (response.headers?.['content-disposition'] as string | undefined) ?? '';
  const match = /filename\*?=(?:UTF-8'')?"?([^";]+)"?/i.exec(cd);
  if (match && match[1]) {
    try {
      filename = decodeURIComponent(match[1]);
    } catch {
      filename = match[1];
    }
  }
  const blobUrl = URL.createObjectURL(response.data);
  const a = document.createElement('a');
  a.href = blobUrl;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(blobUrl);
}
