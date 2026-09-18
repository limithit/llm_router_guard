/**
 * zustand 登录态：token / user / scope 持久化到 localStorage
 *
 * scope 说明（与后端 JWT 的 scope 声明一致）：
 *   ''    完整会话
 *   'mfa' 仅可绑定 MFA 的受限会话（登录时被要求绑定，服务端白名单门禁）
 *   'pw'  仅可改密的受限会话（首启一次性口令账户）
 * 持久化 scope 是为了让「刷新页面」后前端守卫依然生效——此前只在登录那一刻
 * 跳转一次，刷新后用户即可自由浏览整个后台（页面接口虽 403，但外壳看起来正常）。
 *
 * 注意：本模块不得反向依赖 api/client 或 api/endpoints，避免循环引用。
 */
import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';
import type { User } from '../api/types';

const STORAGE_KEY = 'llm-router-guard.auth.v1';

/** 受限会话 scope 取值 */
export type AuthScope = '' | 'mfa' | 'pw';

interface AuthState {
  token: string | null;
  user: User | null;
  scope: AuthScope;
  setAuth: (token: string, user: User, scope?: AuthScope) => void;
  setScope: (scope: AuthScope) => void;
  clearAuth: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      scope: '',
      setAuth: (token, user, scope = '') => set({ token, user, scope }),
      setScope: (scope) => set({ scope }),
      clearAuth: () => set({ token: null, user: null, scope: '' }),
    }),
    {
      name: STORAGE_KEY,
      storage: createJSONStorage(() => localStorage),
    }
  )
);

export const isLoggedIn = () => Boolean(useAuthStore.getState().token);

/** 当前会话是否被限制在 MFA 绑定引导页（服务端 scope=mfa） */
export const isMfaRestricted = () => useAuthStore.getState().scope === 'mfa';