import { Suspense, lazy, useEffect } from 'react';
import { App as AntApp, ConfigProvider, Spin } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import enUS from 'antd/locale/en_US';
import { useTranslation } from 'react-i18next';
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query';
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom';
import { useAuthStore, type AuthScope } from './store/auth';
import { authApi } from './api/endpoints';
import MainLayout from './layouts/MainLayout';
import Login from './pages/Login';
import ChangePassword from './pages/ChangePassword';
import NotFound from './pages/NotFound';

/**
 * 路由级代码分割：业务页面全部 React.lazy 懒加载，
 * 按路由自动拆 chunk，减小首屏体积；Login/NotFound 为轻量页保持静态引入。
 */
const Dashboard = lazy(() => import('./pages/Dashboard'));
const Providers = lazy(() => import('./pages/providers/Providers'));
const Models = lazy(() => import('./pages/models/Models'));
const Failover = lazy(() => import('./pages/models/Failover'));
const Keywords = lazy(() => import('./pages/guard/Keywords'));
const PiiRules = lazy(() => import('./pages/guard/PiiRules'));
const InjectionRules = lazy(() => import('./pages/guard/InjectionRules'));
const OutputFilter = lazy(() => import('./pages/guard/OutputFilter'));
const Quotas = lazy(() => import('./pages/quota/Quotas'));
const RateLimits = lazy(() => import('./pages/quota/RateLimits'));
const QuotaAlerts = lazy(() => import('./pages/quota/QuotaAlerts'));
const CallLogs = lazy(() => import('./pages/audit/Calls'));
const Operations = lazy(() => import('./pages/audit/Operations'));
const TokenStats = lazy(() => import('./pages/usage/TokenStats'));
const GeneralSettings = lazy(() => import('./pages/settings/General'));
const ApiKeys = lazy(() => import('./pages/settings/ApiKeys'));
const SecuritySettings = lazy(() => import('./pages/settings/Security'));
const ConfigStatus = lazy(() => import('./pages/settings/ConfigStatus'));
const RuntimeStatus = lazy(() => import('./pages/settings/RuntimeStatus'));
const Backup = lazy(() => import('./pages/settings/Backup'));
const AccountSecurity = lazy(() => import('./pages/account/AccountSecurity'));
const Help = lazy(() => import('./pages/Help'));

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 15_000,
    },
  },
});

/** 懒加载页面 chunk 加载时的内容区兜底（侧栏/顶栏已就绪，不闪白整页） */
function RouteFallback() {
  return (
    <div
      style={{
        minHeight: 320,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      <Spin size="large" />
    </div>
  );
}

/** 登录守卫：未登录访问任何业务页 → /login */
/**
 * 会话守卫：登录态 + 受限会话（scope）强制引导。
 *
 * 后端对 scope=mfa / scope=pw 的令牌只放行白名单路由（中间件硬门禁），
 * 但仅靠服务端会出现「外壳看起来正常、点哪都报错」的观感：
 * 此前前端只在登录那一刻跳转一次，刷新页面后守卫即失效。
 * 这里按持久化的 scope 做同步守卫，并用 /auth/me 回显的 scope 校正
 * （例如已绑定 MFA 后旧 scope 需要清掉，或换标签页登录改变了会话）。
 */
function RequireAuth({ children }: { children: React.ReactNode }) {
  const { token, scope, setScope } = useAuthStore();
  const location = useLocation();

  const { data: me } = useQuery({
    queryKey: ['auth-me'],
    queryFn: () => authApi.me(),
    enabled: Boolean(token),
    staleTime: 30_000,
    retry: false,
  });

  useEffect(() => {
    if (me) setScope((me.scope ?? '') as AuthScope);
  }, [me, setScope]);

  if (!token) {
    return <Navigate to="/login" replace />;
  }
  // 受限会话：除引导页外的任何路由都弹回去
  if (scope === 'mfa' && location.pathname !== '/account/security') {
    return <Navigate to="/account/security" replace />;
  }
  if (scope === 'pw' && location.pathname !== '/change-password') {
    return <Navigate to="/change-password" replace />;
  }
  return <>{children}</>;
}

export default function App() {
  const { i18n } = useTranslation();
  const antdLocale = i18n.language === 'en-US' ? enUS : zhCN;
  return (
    <ConfigProvider locale={antdLocale} theme={{ token: { colorPrimary: '#1677ff' } }}>
      <AntApp>
        <QueryClientProvider client={queryClient}>
          <BrowserRouter>
            <Suspense fallback={<RouteFallback />}>
              <Routes>
                <Route path="/login" element={<Login />} />
                <Route path="/change-password" element={<ChangePassword />} />
                <Route
                  element={
                    <RequireAuth>
                      <MainLayout />
                    </RequireAuth>
                  }
                >
                  <Route path="/" element={<Dashboard />} />
                  {/* 模型管理 */}
                  <Route path="/providers" element={<Providers />} />
                  <Route path="/models" element={<Models />} />
                  <Route path="/models/failover" element={<Failover />} />
                  {/* 护栏管理 */}
                  <Route path="/guard/keywords" element={<Keywords />} />
                  <Route path="/guard/pii" element={<PiiRules />} />
                  <Route path="/guard/injection" element={<InjectionRules />} />
                  <Route path="/guard/output" element={<OutputFilter />} />
                  {/* 配额管理 */}
                  <Route path="/quota" element={<Quotas />} />
                  <Route path="/quota/ratelimit" element={<RateLimits />} />
                  <Route path="/quota/alert" element={<QuotaAlerts />} />
                  {/* 审计日志 */}
                  <Route path="/audit/calls" element={<CallLogs />} />
                  <Route path="/audit/operations" element={<Operations />} />
                  {/* 用量统计 */}
                  <Route path="/usage/token" element={<TokenStats />} />
                  {/* 系统设置 */}
                  <Route path="/settings/general" element={<GeneralSettings />} />
                  <Route path="/settings/apikeys" element={<ApiKeys />} />
                  <Route path="/settings/security" element={<SecuritySettings />} />
                  <Route path="/settings/config-status" element={<ConfigStatus />} />
                  <Route path="/settings/status" element={<RuntimeStatus />} />
                  <Route path="/settings/backup" element={<Backup />} />
                  {/* 个人账户 */}
                  <Route path="/account/security" element={<AccountSecurity />} />
                  {/* 使用帮助 */}
                  <Route path="/help" element={<Help />} />
                </Route>
                <Route path="/404" element={<NotFound />} />
                <Route path="*" element={<NotFound />} />
              </Routes>
            </Suspense>
          </BrowserRouter>
        </QueryClientProvider>
      </AntApp>
    </ConfigProvider>
  );
}
