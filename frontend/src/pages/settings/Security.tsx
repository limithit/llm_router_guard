// ===== 安全设置 (REQ-020 / REQ-023) =====
// MFA 全局配置：启用开关 / 强制所有用户 / 恢复码数量 / 宽限期（保存后热加载立即生效）
// 用户 MFA 设备管理：查看所有用户 MFA 状态、强制解绑（记审计）、解锁账户
import { useEffect } from 'react';
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
  const { t } = useTranslation();
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
    onSuccess: () => message.success(t('security.saveOk')),
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

  const unlockMutation = useMutation({
    mutationFn: userApi.unlock,
    onSuccess: () => {
      message.success(t('security.unlockOk'));
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
    onError: () => undefined,
  });

  return (
    <PageContainer
      title={t('security.title')}
      description={t('security.desc')}
      extra={
        <Button
          type="primary"
          icon={<SaveOutlined />}
          loading={saveMutation.isPending}
          onClick={async () => saveMutation.mutate(await form.validateFields())}
        >
          {t('security.save')}
        </Button>
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
          <Alert
            type="info"
            showIcon
            message={t('security.totpTip')}
          />
        </Form>
      </Card>

      <Divider />

      <Card size="small" title={t('security.usersCard')}>
        <Table<AdminUserRow>
          rowKey="id"
          loading={usersQuery.isLoading}
          dataSource={usersQuery.data?.items ?? []}
          scroll={{ x: 800 }}
          columns={[
            { title: 'ID', dataIndex: 'id', width: 60, render: (v: number) => <Tag>{v}</Tag> },
            { title: t('security.colUsername'), dataIndex: 'username', width: 160 },
            {
              title: t('security.colMfa'),
              dataIndex: 'mfa_enabled',
              width: 110,
              render: (v: boolean) =>
                v ? <Tag color="green">{t('security.mfaBound')}</Tag> : <Tag color="default">{t('security.mfaUnbound')}</Tag>,
            },
            {
              title: t('security.colLock'),
              dataIndex: 'locked',
              width: 100,
              render: (v: boolean) =>
                v ? <Tag color="red">{t('security.locked')}</Tag> : <Tag color="blue">{t('security.normal')}</Tag>,
            },
            { title: t('security.colLastLogin'), dataIndex: 'last_login_at', width: 170, render: fmtTime },
            {
              title: t('common.action'),
              key: 'action',
              fixed: 'right',
              width: 220,
              render: (_: unknown, row: AdminUserRow) => (
                <Space>
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
    </PageContainer>
  );
}
