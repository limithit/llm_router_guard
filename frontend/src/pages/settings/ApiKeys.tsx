// ===== API Key 管理 (REQ-002) =====
// 创建（一次性展示完整 Key）/ 启用禁用 / 删除 / 脱敏列表 / 最后使用时间
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  App,
  Button,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd';
import { CopyOutlined, PlusOutlined } from '@ant-design/icons';
import PageContainer from '../../components/PageContainer';
import StatusSwitch from '../../components/StatusSwitch';
import { apikeyApi, modelApi } from '../../api/endpoints';
import { fmtTime } from '../../utils/format';
import type { ApiKey, ApiKeyCreated, ModelAlias } from '../../api/types';

interface KeyFormValues {
  name: string;
  remark?: string;
  enabled?: boolean;
  allowed_models?: string[]; // 允许的别名数组（多选）；空=允许全部
  ip_allowlist_text?: string; // 一行一个 CIDR（textarea 原文）
  ip_allowlist_enabled?: boolean;
}

// 安全解析后端返回的 JSON 字符串数组（allowed_models_json / ip_allowlist_json）。
const parseStrArr = (s?: string | null): string[] => {
  if (!s) return [];
  try {
    const v = JSON.parse(s);
    return Array.isArray(v) ? v : [];
  } catch {
    return [];
  }
};

export default function ApiKeys() {
  const { t } = useTranslation();
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<ApiKey | null>(null);
  const [created, setCreated] = useState<ApiKeyCreated | null>(null);
  const [form] = Form.useForm<KeyFormValues>();

  const { data, isLoading } = useQuery({
    queryKey: ['apikeys', page, pageSize],
    queryFn: () => apikeyApi.list({ page, page_size: pageSize }),
    placeholderData: keepPreviousData,
  });

  // 别名列表，用于「限定模型」多选的下拉建议。
  const { data: modelsData } = useQuery({
    queryKey: ['models-all'],
    queryFn: () => modelApi.list({ page: 1, page_size: 1000 }),
    staleTime: 60_000,
  });
  const modelOptions = (modelsData?.items ?? []).map((m: ModelAlias) => ({
    label: m.alias,
    value: m.alias,
  }));

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['apikeys'] });
    queryClient.invalidateQueries({ queryKey: ['apikeys-all'] });
  };

  const createMutation = useMutation({
    mutationFn: apikeyApi.create,
    onSuccess: (res) => {
      setCreateOpen(false);
      form.resetFields();
      setCreated(res); // 一次性展示完整 Key
      invalidate();
    },
    onError: () => undefined,
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, body }: { id: number; body: Parameters<typeof apikeyApi.update>[1] }) =>
      apikeyApi.update(id, body),
    onSuccess: () => {
      message.success(t('apikeys.updated'));
      setEditing(null);
      invalidate();
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: apikeyApi.remove,
    onSuccess: () => {
      message.success(t('common.deleteSuccess'));
      invalidate();
    },
    onError: () => undefined,
  });

  const toggleEnabled = async (row: ApiKey, enabled: boolean) => {
    await apikeyApi.update(row.id, {
      name: row.name,
      remark: row.remark,
      enabled,
      allowed_models_json: row.allowed_models_json,
      ip_allowlist_json: row.ip_allowlist_json,
      ip_allowlist_enabled: row.ip_allowlist_enabled,
    });
    invalidate();
  };

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    setCreateOpen(true);
  };

  const openEdit = (row: ApiKey) => {
    setCreateOpen(false);
    setEditing(row);
    form.setFieldsValue({
      name: row.name,
      remark: row.remark,
      enabled: row.enabled,
      allowed_models: parseStrArr(row.allowed_models_json),
      ip_allowlist_text: parseStrArr(row.ip_allowlist_json).join('\n'),
      ip_allowlist_enabled: row.ip_allowlist_enabled,
    });
  };

  const handleOk = async () => {
    const v = await form.validateFields();
    const allowed_models_json =
      v.allowed_models && v.allowed_models.length ? JSON.stringify(v.allowed_models) : '';
    const ipList = (v.ip_allowlist_text || '')
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean);
    const ip_allowlist_json = ipList.length ? JSON.stringify(ipList) : '';
    const ip_allowlist_enabled = v.ip_allowlist_enabled ?? false;
    if (editing) {
      updateMutation.mutate({
        id: editing.id,
        body: {
          name: v.name,
          remark: v.remark,
          enabled: v.enabled ?? editing.enabled,
          allowed_models_json,
          ip_allowlist_json,
          ip_allowlist_enabled,
        },
      });
    } else {
      createMutation.mutate({
        name: v.name,
        remark: v.remark,
        allowed_models_json,
        ip_allowlist_json,
        ip_allowlist_enabled,
      });
    }
  };

  return (
    <PageContainer
      title={t('apikeys.title')}
      description={t('apikeys.desc')}
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          {t('apikeys.add')}
        </Button>
      }
    >
      <Table<ApiKey>
        rowKey="id"
        loading={isLoading}
        dataSource={data?.items ?? []}
        scroll={{ x: 900 }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 60, render: (v: number) => <Tag>{v}</Tag> },
          { title: t('apikeys.colName'), dataIndex: 'name', width: 160 },
          {
            title: t('apikeys.colKey'),
            dataIndex: 'key_masked',
            width: 180,
            render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
          },
          { title: t('common.remark'), dataIndex: 'remark', ellipsis: true },
          {
            title: t('common.status'),
            dataIndex: 'enabled',
            width: 80,
            render: (_: unknown, row: ApiKey) => (
              <StatusSwitch checked={row.enabled} onChange={(c) => toggleEnabled(row, c)} />
            ),
          },
          { title: t('apikeys.colLastUsed'), dataIndex: 'last_used_at', width: 170, render: fmtTime },
          { title: t('common.createdAt'), dataIndex: 'created_at', width: 170, render: fmtTime },
          {
            title: t('common.action'),
            key: 'action',
            width: 140,
            fixed: 'right',
            render: (_: unknown, row: ApiKey) => (
              <Space>
                <Button size="small" onClick={() => openEdit(row)}>
                  {t('common.edit')}
                </Button>
                <Popconfirm
                  title={t('apikeys.deleteTitle')}
                  description={t('apikeys.deleteConfirm', { name: row.name })}
                  okButtonProps={{ danger: true }}
                  onConfirm={() => deleteMutation.mutate(row.id)}
                >
                  <Button size="small" danger>
                    {t('common.delete')}
                  </Button>
                </Popconfirm>
              </Space>
            ),
          },
        ]}
        pagination={{
          current: page,
          pageSize,
          total: data?.total ?? 0,
          showSizeChanger: true,
          showTotal: (total) => t('common.total', { total }),
        }}
        onChange={(p) => {
          setPage(p.current ?? 1);
          setPageSize(p.pageSize ?? 10);
        }}
      />

      {/* 创建 / 编辑弹窗 */}
      <Modal
        title={editing ? t('apikeys.editTitle', { name: editing.name }) : t('apikeys.createTitle')}
        open={createOpen || !!editing}
        confirmLoading={createMutation.isPending || updateMutation.isPending}
        onOk={handleOk}
        onCancel={() => {
          setCreateOpen(false);
          setEditing(null);
        }}
        width={480}
        destroyOnClose
      >
        <Form<KeyFormValues> form={form} layout="vertical">
          <Form.Item
            name="name"
            label={t('apikeys.colName')}
            rules={[{ required: true, message: t('apikeys.nameReq') }]}
          >
            <Input placeholder={t('apikeys.namePh')} maxLength={64} />
          </Form.Item>
          <Form.Item name="remark" label={t('common.remark')}>
            <Input.TextArea rows={2} placeholder={t('apikeys.remarkPh')} maxLength={255} />
          </Form.Item>
          {editing && (
            <Form.Item name="enabled" label={t('common.status')} valuePropName="checked">
              <Switch />
            </Form.Item>
          )}
          <Form.Item
            name="allowed_models"
            label={t('apikeys.modelsLabel')}
            tooltip={t('apikeys.modelsTooltip')}
          >
            <Select
              mode="tags"
              allowClear
              placeholder={t('apikeys.modelsPh')}
              options={modelOptions}
              tokenSeparators={[',', '\n']}
            />
          </Form.Item>
          <Form.Item
            name="ip_allowlist_enabled"
            label={t('apikeys.ipEnabled')}
            valuePropName="checked"
            tooltip={t('apikeys.ipEnabledTooltip')}
          >
            <Switch />
          </Form.Item>
          <Form.Item
            name="ip_allowlist_text"
            label={t('apikeys.ipLabel')}
            tooltip={t('apikeys.ipTooltip')}
          >
            <Input.TextArea rows={3} placeholder={'10.0.0.0/8\n192.168.1.5\n2001:db8::/32'} />
          </Form.Item>
        </Form>
      </Modal>

      {/* 创建成功：一次性展示完整 Key */}
      <Modal
        title={t('apikeys.createdTitle')}
        open={!!created}
        onCancel={() => setCreated(null)}
        footer={
          <Button type="primary" onClick={() => setCreated(null)}>
            {t('apikeys.saved')}
          </Button>
        }
        width={560}
      >
        <Alert
          type="warning"
          showIcon
          message={t('apikeys.createdWarn')}
          style={{ marginBottom: 12 }}
        />
        <Space.Compact style={{ width: '100%' }}>
          <Input value={created?.key ?? ''} readOnly />
          <Button
            icon={<CopyOutlined />}
            onClick={async () => {
              if (created?.key) {
                await navigator.clipboard.writeText(created.key);
                message.success(t('apikeys.copied'));
              }
            }}
          >
            {t('apikeys.copy')}
          </Button>
        </Space.Compact>
        <Typography.Paragraph type="secondary" style={{ marginTop: 8 }}>
          {t('apikeys.meta', { name: created?.name ?? '', id: created?.id ?? '' })}
        </Typography.Paragraph>
      </Modal>
    </PageContainer>
  );
}
