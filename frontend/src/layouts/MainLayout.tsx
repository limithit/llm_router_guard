import { useEffect, useMemo, useState } from 'react';
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { Avatar, Breadcrumb, Dropdown, Layout, Menu, Space, Typography } from 'antd';
import type { MenuProps } from 'antd';
import {
  ApartmentOutlined,
  AuditOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  DeploymentUnitOutlined,
  LogoutOutlined,
  LockOutlined,
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

const { Header, Sider, Content } = Layout;

type MenuItem = Required<MenuProps>['items'][number];

/** 侧边树形菜单（PRD 4.1）：key 即路由路径 */
const MENU: MenuItem[] = [
  { key: '/', label: '首页', icon: <DashboardOutlined /> },
  {
    key: 'group-models',
    label: '模型管理',
    icon: <ApartmentOutlined />,
    children: [
      { key: '/providers', label: '供应商管理', icon: <DeploymentUnitOutlined /> },
      { key: '/models', label: '模型别名', icon: <ApartmentOutlined /> },
      { key: '/models/failover', label: '故障转移设置', icon: <RetweetOutlined /> },
    ],
  },
  {
    key: 'group-guard',
    label: '护栏管理',
    icon: <SecurityScanOutlined />,
    children: [
      { key: '/guard/keywords', label: '敏感词', icon: <ProfileOutlined /> },
      { key: '/guard/pii', label: 'PII 规则', icon: <ProfileOutlined /> },
      { key: '/guard/injection', label: '注入规则', icon: <ProfileOutlined /> },
      { key: '/guard/output', label: '输出过滤', icon: <ProfileOutlined /> },
    ],
  },
  {
    key: 'group-quota',
    label: '配额管理',
    icon: <AuditOutlined />,
    children: [
      { key: '/quota', label: '配额列表', icon: <AuditOutlined /> },
      { key: '/quota/ratelimit', label: '速率限制', icon: <AuditOutlined /> },
      { key: '/quota/alert', label: '预警设置', icon: <AuditOutlined /> },
    ],
  },
  {
    key: 'group-audit',
    label: '审计日志',
    icon: <AuditOutlined />,
    children: [
      { key: '/audit/calls', label: '调用日志', icon: <AuditOutlined /> },
      { key: '/audit/operations', label: '操作审计', icon: <AuditOutlined /> },
    ],
  },
  {
    key: 'group-settings',
    label: '系统设置',
    icon: <SettingOutlined />,
    children: [
      { key: '/settings/general', label: '通用设置', icon: <SettingOutlined /> },
      { key: '/settings/apikeys', label: 'API Key', icon: <SafetyCertificateOutlined /> },
      { key: '/settings/security', label: '安全设置', icon: <LockOutlined /> },
      { key: '/settings/config-status', label: '配置状态', icon: <ProfileOutlined /> },
      { key: '/settings/status', label: '状态监控', icon: <AuditOutlined /> },
      { key: '/settings/backup', label: '数据备份', icon: <DatabaseOutlined /> },
    ],
  },
  { key: '/help', label: '使用帮助', icon: <QuestionCircleOutlined /> },
];

/** 页面路由 → 展示名称（面包屑，PRD 4.2） */
const PAGE_NAMES: Record<string, string> = {
  '/': '概览 Dashboard',
  '/providers': '供应商管理',
  '/models': '模型别名管理',
  '/models/failover': '故障转移设置',
  '/guard/keywords': '敏感词管理',
  '/guard/pii': 'PII 规则管理',
  '/guard/injection': '注入规则管理',
  '/guard/output': '输出过滤配置',
  '/quota': '配额管理',
  '/quota/ratelimit': '速率限制',
  '/quota/alert': '预警设置',
  '/audit/calls': '调用日志',
  '/audit/operations': '操作审计',
  '/settings/general': '通用设置',
  '/settings/apikeys': 'API Key 管理',
  '/settings/security': '安全设置',
  '/settings/config-status': '配置状态',
  '/settings/status': '状态监控',
  '/settings/backup': '数据备份',
  '/account/security': '个人安全设置',
  '/help': '使用帮助',
};

export default function MainLayout() {
  const location = useLocation();
  const navigate = useNavigate();
  const { user, clearAuth } = useAuthStore();
  const username = user?.username ?? 'admin';
  const path = location.pathname;

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
  }, [path]);

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
    const items: Array<{ title: React.ReactNode }> = [
      {
        title:
          path === '/' ? (
            'AI 网关管理平台'
          ) : (
            <Link to="/">首页</Link>
          ),
      },
    ];
    if (path !== '/') {
      if (groupName) items.push({ title: groupName });
      items.push({ title: PAGE_NAMES[path] ?? path });
    }
    return items;
  }, [path, groupName]);

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
      label: <Link to="/account/security">账户安全</Link>,
    },
    { type: 'divider' },
    { key: 'logout', icon: <LogoutOutlined />, label: '退出登录' },
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
            AI 网关管理平台
          </Typography.Text>
        </Link>
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
