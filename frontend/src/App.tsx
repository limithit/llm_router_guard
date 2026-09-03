import { App as AntApp, ConfigProvider } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { isLoggedIn } from './store/auth';
import MainLayout from './layouts/MainLayout';
import Login from './pages/Login';
import NotFound from './pages/NotFound';
import Dashboard from './pages/Dashboard';
import Providers from './pages/providers/Providers';
import Models from './pages/models/Models';
import Failover from './pages/models/Failover';
import Keywords from './pages/guard/Keywords';
import PiiRules from './pages/guard/PiiRules';
import InjectionRules from './pages/guard/InjectionRules';
import OutputFilter from './pages/guard/OutputFilter';
import Quotas from './pages/quota/Quotas';
import RateLimits from './pages/quota/RateLimits';
import QuotaAlerts from './pages/quota/QuotaAlerts';
import CallLogs from './pages/audit/Calls';
import Operations from './pages/audit/Operations';
import TokenStats from './pages/usage/TokenStats';
import GeneralSettings from './pages/settings/General';
import ApiKeys from './pages/settings/ApiKeys';
import SecuritySettings from './pages/settings/Security';
import ConfigStatus from './pages/settings/ConfigStatus';
import RuntimeStatus from './pages/settings/RuntimeStatus';
import Backup from './pages/settings/Backup';
import AccountSecurity from './pages/account/AccountSecurity';
import Help from './pages/Help';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 15_000,
    },
  },
});

/** 登录守卫：未登录访问任何业务页 → /login */
function RequireAuth({ children }: { children: React.ReactNode }) {
  if (!isLoggedIn()) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

export default function App() {
  return (
    <ConfigProvider locale={zhCN} theme={{ token: { colorPrimary: '#1677ff' } }}>
      <AntApp>
        <QueryClientProvider client={queryClient}>
          <BrowserRouter>
            <Routes>
              <Route path="/login" element={<Login />} />
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
          </BrowserRouter>
        </QueryClientProvider>
      </AntApp>
    </ConfigProvider>
  );
}
