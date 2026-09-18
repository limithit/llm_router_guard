import { Suspense, lazy } from 'react';
import { App as AntApp, ConfigProvider, Spin } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import enUS from 'antd/locale/en_US';
import { useTranslation } from 'react-i18next';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { isLoggedIn } from './store/auth';
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
function RequireAuth({ children }: { children: React.ReactNode }) {
  if (!isLoggedIn()) {
    return <Navigate to="/login" replace />;
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
