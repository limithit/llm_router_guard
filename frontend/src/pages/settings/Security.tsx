// ===== 安全设置 (REQ-020 / REQ-023) =====
// MFA 全局配置：启用开关 / 强制所有用户 / 恢复码数量 / 宽限期（保存后热加载立即生效）
// 用户管理：列表 / 创建 / 改密 / 改角色 / 删除 / 强制解绑 MFA / 解锁
import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  App,
  Button,
  Card,
  Divider,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
} from 'antd';
import {
  DeleteOutlined,
  KeyOutlined,
  PlusOutlined,
  QuestionCircleOutlined,
  SaveOutlined,
  UserAddOutlined,
} from '@ant-design/icons';
import PageContainer from '../../components/PageContainer';
import { securityApi, userApi } from '../../api/endpoints';
import { fmtTime } from '../../utils/format';
import type { AdminUserRow, SecuritySettings, UserCreateBody, UserChangePasswordBody } from '../../api/types';

export default function SecuritySettingsPage() {
  const { t } = useTranslation();
  const { message, modal } = App.useApp();
  const queryClient = useQueryClient();
  const [form] = Form.useForm<SecuritySettings>();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  // 弹窗状态
  const [createOpen, setCreateOpen] = useState(false);
  const [pwdOpen, setPwdOpen] = useState(false);
  const [pwdTarget, setPwdTarget] = useState<AdminUserRow | null>(null);
  const [createForm] = Form.useForm<UserCreateBody>();
  const [pwdForm] = Form.useForm<{ password: string; force_change_password: boolean }>();

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
    onSuccess: () => message.success(t('security.saveOk')),
    onError: () => undefined,
  });

  const createMutation = useMutation({
    mutationFn: (body: UserCreateBody) => userApi.create(body),
    onSuccess: () => {
      message.success(t('security.createOk'));
      queryClient.invalidateQueries({ queryKey: ['users'] });
      setCreateOpen(false);
      createForm.resetFields();
    },
    onError: () => undefined,
  });

  const pwdMutation = useMutation({
    mutationFn: ({ id, body }: { id: number; body: UserChangePasswordBody }) =>
      userApi.changePassword(id, body),
    onSuccess: () => {
      message.success(t('security.pwdOk'));
      queryClient.invalidateQueries({ queryKey: ['users'] });
      setPwdOpen(false);
      pwdForm.resetFields();
      setPwdTarget(null);
    },
    onError: () => undefined,
  });

  const roleMutation = useMutation({
    mutationFn: ({ id, role }: { id: number; role: 'admin' | 'viewer' }) =>
      userApi.changeRole(id, { role }),
    onSuccess: () => {
      message.success(t('security.roleOk'));
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: (id: number) => userApi.remove(id),
    onSuccess: () => {
      message.success(t('security.deleteOk'));
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
    onError: () => undefined,
  });

  const unbindMutation = useMutation({
    mutationFn: userApi.unbindMfa,
    onSuccess: () => {
      message.success(t('security.unbindOk'));
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
    onError: () => undefined,
  });

  // 单用户强制 MFA（不影响其他用户）
  const mfaRequiredMutation = useMutation({
    mutationFn: ({ id, required }: { id: number; required: boolean }) =>
      userApi.setMfaRequired(id, { mfa_required: required }),
    onSuccess: (_d, v) => {
      message.success(v.required ? t('security.mfaRequiredOnOk') : t('security.mfaRequiredOffOk'));
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
    onError: () => undefined,
  });

  const unlockMutation = useMutation({
    mutationFn: userApi.unlock,
    onSuccess: () => {
      message.success(t('security.unlockOk'));
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
    onError: () => undefined,
  });

  const roleTag = (role: string) =>
    role === 'viewer' ? (
      <Tag color="orange">{t('security.roleViewer')}</Tag>
    ) : (
      <Tag color="purple">{t('security.roleAdmin')}</Tag>
    );

  return (
    <PageContainer
      title={t('security.title')}
      description={t('security.desc')}
      extra={
        <Space>
          <Button
            icon={<PlusOutlined />}
            onClick={() => {
              createForm.resetFields();
              setCreateOpen(true);
            }}
          >
            {t('security.addUser')}
          </Button>
          <Button
            type="primary"
            icon={<SaveOutlined />}
            loading={saveMutation.isPending}
            onClick={async () => saveMutation.mutate(await form.validateFields())}
          >
            {t('security.save')}
          </Button>
        </Space>
      }
    >
      <Card size="small" title={t('security.mfaCard')} loading={isLoading}>
        <Form<SecuritySettings> form={form} layout="vertical" style={{ maxWidth: 520 }}>
          <Form.Item name="mfa_enabled" label={t('security.mfaEnabled')} valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item
            name="mfa_required_for_all"
            label={t('security.mfaRequired')}
            valuePropName="checked"
            tooltip={t('security.mfaRequiredTooltip')}
          >
            <Switch />
          </Form.Item>
          <Space size={16}>
            <Form.Item
              name="recovery_code_count"
              label={t('security.recoveryCount')}
              rules={[{ required: true, message: t('security.recoveryReq') }]}
            >
              <InputNumber min={4} max={20} precision={0} style={{ width: 140 }} />
            </Form.Item>
            <Form.Item
              name="grace_days"
              label={t('security.graceDays')}
              rules={[{ required: true, message: t('security.graceReq') }]}
            >
              <InputNumber min={0} max={90} precision={0} style={{ width: 140 }} addonAfter={t('security.dayUnit')} />
            </Form.Item>
          </Space>
          <Alert type="info" showIcon message={t('security.totpTip')} />
        </Form>
      </Card>

      <Divider />

      <Card size="small" title={t('security.usersCard')}>
        <Table<AdminUserRow>
          rowKey="id"
          loading={usersQuery.isLoading}
          dataSource={usersQuery.data?.items ?? []}
          scroll={{ x: 900 }}
          columns={[
            { title: 'ID', dataIndex: 'id', width: 60, render: (v: number) => <Tag>{v}</Tag> },
            { title: t('security.colUsername'), dataIndex: 'username', width: 140 },
            {
              title: t('security.colRole'),
              dataIndex: 'role',
              width: 100,
              render: (v: string) => roleTag(v),
            },
            {
              title: t('security.colMfa'),
              dataIndex: 'mfa_enabled',
              width: 110,
              render: (v: boolean) =>
                v ? <Tag color="green">{t('security.mfaBound')}</Tag> : <Tag color="default">{t('security.mfaUnbound')}</Tag>,
            },
            {
              // 单用户强制 MFA：只对个别高权限账户要求二次验证，不影响其他人
              title: (
                <Space size={4}>
                  {t('security.colMfaRequired')}
                  <Tooltip title={t('security.colMfaRequiredTip')}>
                    <QuestionCircleOutlined style={{ color: '#8c8c8c' }} />
                  </Tooltip>
                </Space>
              ),
              dataIndex: 'mfa_required',
              width: 130,
              render: (v: boolean, row: AdminUserRow) => (
                <Switch
                  size="small"
                  checked={v}
                  loading={mfaRequiredMutation.isPending}
                  onChange={(checked) => mfaRequiredMutation.mutate({ id: row.id, required: checked })}
                />
              ),
            },
            {
              title: t('security.colLock'),
              dataIndex: 'locked',
              width: 90,
              render: (v: boolean) =>
                v ? <Tag color="red">{t('security.locked')}</Tag> : <Tag color="blue">{t('security.normal')}</Tag>,
            },
            { title: t('security.colLastLogin'), dataIndex: 'last_login_at', width: 170, render: fmtTime },
            {
              title: t('common.action'),
              key: 'action',
              fixed: 'right',
              width: 360,
              render: (_: unknown, row: AdminUserRow) => (
                <Space size="small" wrap>
                  <Button
                    size="small"
                    icon={<KeyOutlined />}
                    onClick={() => {
                      pwdForm.resetFields();
                      setPwdTarget(row);
                      setPwdOpen(true);
                    }}
                  >
                    {t('security.changePwd')}
                  </Button>
                  <Popconfirm
                    title={t('security.changeRoleTitle')}
                    description={t('security.changeRoleConfirm', {
                      username: row.username,
                      role: t(row.role === 'viewer' ? 'security.roleAdmin' : 'security.roleViewer'),
                    })}
                    onConfirm={() =>
                      roleMutation.mutate({
                        id: row.id,
                        role: row.role === 'viewer' ? 'admin' : 'viewer',
                      })
                    }
                  >
                    <Button size="small">
                      {t('security.changeRole')}
                    </Button>
                  </Popconfirm>
                  <Popconfirm
                    title={t('security.unbindTitle')}
                    description={t('security.unbindConfirm', { username: row.username })}
                    okButtonProps={{ danger: true }}
                    disabled={!row.mfa_enabled}
                    onConfirm={() => unbindMutation.mutate(row.id)}
                  >
                    <Button size="small" danger disabled={!row.mfa_enabled}>
                      {t('security.unbind')}
                    </Button>
                  </Popconfirm>
                  <Popconfirm
                    title={t('security.unlockTitle')}
                    description={t('security.unlockConfirm', { username: row.username })}
                    disabled={!row.locked}
                    onConfirm={() => unlockMutation.mutate(row.id)}
                  >
                    <Button size="small" disabled={!row.locked}>
                      {t('security.unlock')}
                    </Button>
                  </Popconfirm>
                  <Popconfirm
                    title={t('security.deleteTitle')}
                    description={t('security.deleteConfirm', { username: row.username })}
                    okButtonProps={{ danger: true }}
                    onConfirm={() => deleteMutation.mutate(row.id)}
                  >
                    <Button size="small" danger icon={<DeleteOutlined />}>
                      {t('security.delete')}
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
            showTotal: (total) => t('common.total', { total }),
          }}
          onChange={(p) => {
            setPage(p.current ?? 1);
            setPageSize(p.pageSize ?? 10);
          }}
        />
      </Card>

      {/* 创建用户弹窗 */}
      <Modal
        title={
          <span>
            <UserAddOutlined style={{ marginRight: 8 }} />
            {t('security.createUser')}
          </span>
        }
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        confirmLoading={createMutation.isPending}
        onOk={async () => {
          const values = await createForm.validateFields();
          createMutation.mutate(values);
        }}
      >
        <Form<UserCreateBody> form={createForm} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            name="username"
            label={t('security.colUsername')}
            rules={[
              { required: true, message: t('security.usernameReq') },
              { max: 64, message: t('security.usernameMax') },
            ]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            name="password"
            label={t('security.passwordLabel')}
            extra={t('security.pwdPolicy')}
            rules={[
              { required: true, message: t('security.passwordReq') },
              { min: 10, message: t('security.passwordMin') },
            ]}
          >
            <Input.Password />
          </Form.Item>
          <Form.Item name="role" label={t('security.colRole')} initialValue="admin">
            <Select
              options={[
                { value: 'admin', label: t('security.roleAdmin') },
                { value: 'viewer', label: t('security.roleViewer') },
              ]}
            />
          </Form.Item>
          <Form.Item
            name="force_change_password"
            label={t('security.forceChange')}
            valuePropName="checked"
            initialValue={true}
            tooltip={t('security.forceChangeTooltip')}
          >
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      {/* 改密弹窗 */}
      <Modal
        title={
          <span>
            <KeyOutlined style={{ marginRight: 8 }} />
            {t('security.changePwdTitle')} — {pwdTarget?.username}
          </span>
        }
        open={pwdOpen}
        onCancel={() => {
          setPwdOpen(false);
          setPwdTarget(null);
        }}
        confirmLoading={pwdMutation.isPending}
        onOk={async () => {
          if (!pwdTarget) return;
          const values = await pwdForm.validateFields();
          pwdMutation.mutate({
            id: pwdTarget.id,
            body: {
              password: values.password,
              force_change_password: values.force_change_password ?? false,
            },
          });
        }}
      >
        <Form layout="vertical" form={pwdForm} style={{ marginTop: 16 }}>
          <Form.Item
            name="password"
            label={t('security.newPassword')}
            extra={t('security.pwdPolicy')}
            rules={[
              { required: true, message: t('security.passwordReq') },
              { min: 10, message: t('security.passwordMin') },
            ]}
          >
            <Input.Password />
          </Form.Item>
          <Form.Item
            name="force_change_password"
            label={t('security.forceChange')}
            valuePropName="checked"
            tooltip={t('security.forceChangeTooltip')}
          >
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  );
}
