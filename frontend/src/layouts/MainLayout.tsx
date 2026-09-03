import { useEffect, useMemo, useState } from 'react';
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Avatar, Breadcrumb, Dropdown, Layout, Menu, Space, Typography } from 'antd';
import type { MenuProps } from 'antd';
import {
  ApartmentOutlined,
  AuditOutlined,
  BarChartOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  DeploymentUnitOutlined,
  LockOutlined,
  LogoutOutlined,
  ProfileOutlined,
  QuestionCircleOutlined,
  RetweetOutlined,
  SafetyCertificateOutlined,
  SecurityScanOutlined,
  SettingOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { useAuthStore } from '../store/auth';
import { authApi } from '../api/endpoints';
import LanguageSwitch from '../components/LanguageSwitch';

const { Header, Sider, Content } = Layout;

type MenuItem = Required<MenuProps>['items'][number];

/** 页面路由 → 面包屑展示文案键（PRD 4.2） */
const PAGE_NAME_KEYS: Record<string, string> = {
  '/': 'pageName.dashboard',
  '/usage/token': 'pageName.tokenUsage',
  '/providers': 'pageName.providers',
  '/models': 'pageName.models',
  '/models/failover': 'pageName.failover',
  '/guard/keywords': 'pageName.keywords',
  '/guard/pii': 'pageName.piiRules',
  '/guard/injection': 'pageName.injectionRules',
  '/guard/output': 'pageName.outputFilter',
  '/quota': 'pageName.quotas',
  '/quota/ratelimit': 'pageName.rateLimits',
  '/quota/alert': 'pageName.quotaAlerts',
  '/audit/calls': 'pageName.callLogs',
  '/audit/operations': 'pageName.operationLogs',
  '/settings/general': 'pageName.general',
  '/settings/apikeys': 'pageName.apiKeys',
  '/settings/security': 'pageName.security',
  '/settings/config-status': 'pageName.configStatus',
  '/settings/status': 'pageName.runtimeStatus',
  '/settings/backup': 'pageName.backup',
  '/account/security': 'pageName.accountSecurity',
  '/help': 'pageName.help',
};

export default function MainLayout() {
  const { t } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();
  const { user, clearAuth } = useAuthStore();
  const username = user?.username ?? 'admin';
  const path = location.pathname;

  /** 侧边树形菜单（PRD 4.1）：key 即路由路径，语言切换时随 t 重建 */
  const MENU: MenuItem[] = useMemo(
    () => [
      { key: '/', label: t('menu.home'), icon: <DashboardOutlined /> },
      { key: '/usage/token', label: t('menu.tokenUsage'), icon: <BarChartOutlined /> },
      {
        key: 'group-models',
        label: t('menu.models'),
        icon: <ApartmentOutlined />,
        children: [
          { key: '/providers', label: t('menu.providers'), icon: <DeploymentUnitOutlined /> },
          { key: '/models', label: t('menu.modelAliases'), icon: <ApartmentOutlined /> },
          { key: '/models/failover', label: t('menu.failover'), icon: <RetweetOutlined /> },
        ],
      },
      {
        key: 'group-guard',
        label: t('menu.guard'),
        icon: <SecurityScanOutlined />,
        children: [
          { key: '/guard/keywords', label: t('menu.keywords'), icon: <ProfileOutlined /> },
          { key: '/guard/pii', label: t('menu.piiRules'), icon: <ProfileOutlined /> },
          { key: '/guard/injection', label: t('menu.injectionRules'), icon: <ProfileOutlined /> },
          { key: '/guard/output', label: t('menu.outputFilter'), icon: <ProfileOutlined /> },
        ],
      },
      {
        key: 'group-quota',
        label: t('menu.quota'),
        icon: <AuditOutlined />,
        children: [
          { key: '/quota', label: t('menu.quotas'), icon: <AuditOutlined /> },
          { key: '/quota/ratelimit', label: t('menu.rateLimits'), icon: <AuditOutlined /> },
          { key: '/quota/alert', label: t('menu.quotaAlerts'), icon: <AuditOutlined /> },
        ],
      },
      {
        key: 'group-audit',
        label: t('menu.audit'),
        icon: <AuditOutlined />,
        children: [
          { key: '/audit/calls', label: t('menu.callLogs'), icon: <AuditOutlined /> },
          { key: '/audit/operations', label: t('menu.operationLogs'), icon: <AuditOutlined /> },
        ],
      },
      {
        key: 'group-settings',
        label: t('menu.settings'),
        icon: <SettingOutlined />,
        children: [
          { key: '/settings/general', label: t('menu.general'), icon: <SettingOutlined /> },
          { key: '/settings/apikeys', label: t('menu.apiKeys'), icon: <SafetyCertificateOutlined /> },
          { key: '/settings/security', label: t('menu.security'), icon: <LockOutlined /> },
          { key: '/settings/config-status', label: t('menu.configStatus'), icon: <ProfileOutlined /> },
          { key: '/settings/status', label: t('menu.runtimeStatus'), icon: <AuditOutlined /> },
          { key: '/settings/backup', label: t('menu.backup'), icon: <DatabaseOutlined /> },
        ],
      },
      { key: '/help', label: t('menu.help'), icon: <QuestionCircleOutlined /> },
    ],
    [t],
  );

  const groupName = useMemo(() => {
    for (const item of MENU) {
      if (item && 'children' in item && item.children) {
        for (const child of item.children) {
          if (child && 'key' in child && child.key === path) {
            return 'label' in item && typeof item.label === 'string' ? item.label : undefined;
          }
        }
      }
    }
    return undefined;
  }, [path, MENU]);

  const openKeysOf = (p: string): string[] => {
    for (const item of MENU) {
      if (item && 'children' in item && item.children) {
        if (item.children.some((c) => c && 'key' in c && c.key === p)) {
          return item.key != null ? [String(item.key)] : [];
        }
      }
    }
    return [];
  };

  const [openKeys, setOpenKeys] = useState<string[]>(() => openKeysOf(path));

  // 路由变化时自动展开所在分组
  useEffect(() => {
    const keys = openKeysOf(path);
    if (keys.length) {
      setOpenKeys((prev) => Array.from(new Set([...prev, ...keys])));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path]);

  const selectedKeys = useMemo(() => [path], [path]);

  const breadcrumbItems = useMemo(() => {
    const pageKey = PAGE_NAME_KEYS[path];
    const items: Array<{ title: React.ReactNode }> = [
      {
        title:
          path === '/' ? (
            t('menu.brand')
          ) : (
            <Link to="/">{t('menu.home')}</Link>
          ),
      },
    ];
    if (path !== '/') {
      if (groupName) items.push({ title: groupName });
      items.push({ title: pageKey ? t(pageKey) : path });
    }
    return items;
  }, [path, groupName, t]);

  const handleLogout = async () => {
    try {
      await authApi.logout();
    } catch {
      // 后端登出失败也继续清理本地状态
    }
    clearAuth();
    navigate('/login', { replace: true });
  };

  const userMenuItems: MenuProps['items'] = [
    {
      key: 'account',
      icon: <LockOutlined />,
      label: <Link to="/account/security">{t('menu.account')}</Link>,
    },
    { type: 'divider' },
    { key: 'logout', icon: <LogoutOutlined />, label: t('menu.logout') },
  ];

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          background: '#001529',
          padding: '0 24px',
          position: 'sticky',
          top: 0,
          zIndex: 100,
        }}
      >
        <Link to="/" style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <Avatar size={30} style={{ background: '#1677ff' }} icon={<SafetyCertificateOutlined />} />
          <Typography.Text strong style={{ color: '#fff', fontSize: 17 }}>
            {t('menu.brand')}
          </Typography.Text>
        </Link>
        <Space size={20}>
          <LanguageSwitch />
          <Dropdown
            menu={{
              items: userMenuItems,
              onClick: ({ key }) => {
                if (key === 'logout') handleLogout();
              },
            }}
          >
            <Space style={{ cursor: 'pointer' }}>
              <Avatar size="small" icon={<UserOutlined />} style={{ background: '#1677ff' }} />
              <span style={{ color: '#fff' }}>{username}</span>
            </Space>
          </Dropdown>
        </Space>
      </Header>
      <Layout>
        <Sider
          width={216}
          theme="dark"
          style={{
            position: 'sticky',
            top: 64,
            height: 'calc(100vh - 64px)',
            overflow: 'auto',
            left: 0,
          }}
        >
          <Menu
            theme="dark"
            mode="inline"
            items={MENU}
            selectedKeys={selectedKeys}
            openKeys={openKeys}
            onOpenChange={(keys) => setOpenKeys(keys)}
            onClick={({ key }) => {
              if (key.startsWith('/')) navigate(key);
            }}
          />
        </Sider>
        <Content style={{ padding: 16, background: '#f0f2f5', minWidth: 0 }}>
          <Breadcrumb style={{ marginBottom: 12 }} items={breadcrumbItems} />
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}
