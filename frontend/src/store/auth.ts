/**
 * zustand 登录态：token / user 持久化到 localStorage
 * 注意：本模块不得反向依赖 api/client 或 api/endpoints，避免循环引用。
 */
import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';
import type { User } from '../api/types';

const STORAGE_KEY = 'llm-router-guard.auth.v1';

interface AuthState {
  token: string | null;
  user: User | null;
  setAuth: (token: string, user: User) => void;
  clearAuth: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      setAuth: (token, user) => set({ token, user }),
      clearAuth: () => set({ token: null, user: null }),
    }),
    {
      name: STORAGE_KEY,
      storage: createJSONStorage(() => localStorage),
    }
  )
);

export const isLoggedIn = () => Boolean(useAuthStore.getState().token);
