// ===== 安全设置 (REQ-020 / REQ-023) =====
// MFA 全局配置：启用开关 / 强制所有用户 / 恢复码数量 / 宽限期（保存后热加载立即生效）
// 用户 MFA 设备管理：查看所有用户 MFA 状态、强制解绑（记审计）、解锁账户
import { useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
import {
  Alert,
  App,
  Button,
  Card,
  Divider,
  Form,
  InputNumber,
  Popconfirm,
  Space,
  Switch,
  Table,
  Tag,
} from 'antd';
import { SaveOutlined } from '@ant-design/icons';
import { useState } from 'react';
import PageContainer from '../../components/PageContainer';
import { securityApi, userApi } from '../../api/endpoints';
import { fmtTime } from '../../utils/format';
import type { AdminUserRow, SecuritySettings } from '../../api/types';

export default function SecuritySettingsPage() {
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [form] = Form.useForm<SecuritySettings>();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const { data, isLoading } = useQuery({
    queryKey: ['settings-security'],
    queryFn: () => securityApi.get(),
  });

  useEffect(() => {
    if (data) form.setFieldsValue(data);
  }, [data, form]);

  const usersQuery = useQuery({
    queryKey: ['users', page, pageSize],
    queryFn: () => userApi.list({ page, page_size: pageSize }),
    placeholderData: keepPreviousData,
  });

  const saveMutation = useMutation({
    mutationFn: (v: SecuritySettings) => securityApi.save(v),
    onSuccess: () => message.success('MFA 全局配置已保存，立即生效（REQ-020）'),
    onError: () => undefined,
  });

  const unbindMutation = useMutation({
    mutationFn: userApi.unbindMfa,
    onSuccess: () => {
      message.success('已强制解绑该用户的 MFA（已记录操作审计）');
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
    onError: () => undefined,
  });

  const unlockMutation = useMutation({
    mutationFn: userApi.unlock,
    onSuccess: () => {
      message.success('账户已解锁');
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
    onError: () => undefined,
  });

  return (
    <PageContainer
      title="安全设置"
      description="管理后台 MFA（TOTP）全局策略与用户设备管理（REQ-020 / REQ-023）。"
      extra={
        <Button
          type="primary"
          icon={<SaveOutlined />}
          loading={saveMutation.isPending}
          onClick={async () => saveMutation.mutate(await form.validateFields())}
        >
          保存 MFA 配置
        </Button>
      }
    >
      <Card size="small" title="MFA 全局配置" loading={isLoading}>
        <Form<SecuritySettings> form={form} layout="vertical" style={{ maxWidth: 520 }}>
          <Form.Item name="mfa_enabled" label="启用 MFA" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item
            name="mfa_required_for_all"
            label="强制所有用户启用 MFA"
            valuePropName="checked"
            tooltip="开启后未绑定 MFA 的用户登录后将被引导至绑定流程"
          >
            <Switch />
          </Form.Item>
          <Space size={16}>
            <Form.Item
              name="recovery_code_count"
              label="备用恢复码数量"
              rules={[{ required: true, message: '请输入数量' }]}
            >
              <InputNumber min={4} max={20} precision={0} style={{ width: 140 }} />
            </Form.Item>
            <Form.Item
              name="grace_days"
              label="强制启用宽限期"
              rules={[{ required: true, message: '请输入天数' }]}
            >
              <InputNumber min={0} max={90} precision={0} style={{ width: 140 }} addonAfter="天" />
            </Form.Item>
          </Space>
          <Alert
            type="info"
            showIcon
            message="验证方式：TOTP（Google Authenticator / Microsoft Authenticator 等）"
          />
        </Form>
      </Card>

      <Divider />

      <Card size="small" title="用户 MFA 设备管理">
        <Table<AdminUserRow>
          rowKey="id"
          loading={usersQuery.isLoading}
          dataSource={usersQuery.data?.items ?? []}
          scroll={{ x: 800 }}
          columns={[
            { title: 'ID', dataIndex: 'id', width: 60, render: (v: number) => <Tag>{v}</Tag> },
            { title: '用户名', dataIndex: 'username', width: 160 },
            {
              title: 'MFA 状态',
              dataIndex: 'mfa_enabled',
              width: 110,
              render: (v: boolean) =>
                v ? <Tag color="green">已绑定</Tag> : <Tag color="default">未绑定</Tag>,
            },
            {
              title: '账户状态',
              dataIndex: 'locked',
              width: 100,
              render: (v: boolean) =>
                v ? <Tag color="red">已锁定</Tag> : <Tag color="blue">正常</Tag>,
            },
            { title: '最后登录', dataIndex: 'last_login_at', width: 170, render: fmtTime },
            {
              title: '操作',
              key: 'action',
              fixed: 'right',
              width: 220,
              render: (_: unknown, row: AdminUserRow) => (
                <Space>
                  <Popconfirm
                    title="强制解绑 MFA"
                    description={`确认解绑「${row.username}」的 MFA 设备？该操作将记录审计。`}
                    okButtonProps={{ danger: true }}
                    disabled={!row.mfa_enabled}
                    onConfirm={() => unbindMutation.mutate(row.id)}
                  >
                    <Button size="small" danger disabled={!row.mfa_enabled}>
                      强制解绑
                    </Button>
                  </Popconfirm>
                  <Popconfirm
                    title="解锁账户"
                    description={`确认解锁「${row.username}」？`}
                    disabled={!row.locked}
                    onConfirm={() => unlockMutation.mutate(row.id)}
                  >
                    <Button size="small" disabled={!row.locked}>
                      解锁
                    </Button>
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
          pagination={{
            current: page,
            pageSize,
            total: usersQuery.data?.total ?? 0,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 条`,
          }}
          onChange={(p) => {
            setPage(p.current ?? 1);
            setPageSize(p.pageSize ?? 10);
          }}
        />
      </Card>
    </PageContainer>
  );
}
