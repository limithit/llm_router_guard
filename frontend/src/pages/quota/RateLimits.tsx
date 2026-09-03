// ===== 速率限制 (REQ-013) =====
// 规则列表 CRUD（api_key_id=0 显示"(全局)"）+ 启用/停用 + 命中统计展示
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import {
  App,
  Button,
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
} from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import PageContainer from '../../components/PageContainer';
import StatusSwitch from '../../components/StatusSwitch';
import { apikeyApi, rateLimitApi } from '../../api/endpoints';
import { fmtNumber } from '../../utils/format';
import type { RateLimit, RateLimitInput } from '../../api/types';

interface RateLimitFormValues {
  api_key_id: number;
  model_alias: string;
  window_seconds: number;
  max_requests: number;
  enabled: boolean;
}

export default function RateLimits() {
  const { t } = useTranslation();
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<RateLimit | null>(null);
  const [form] = Form.useForm<RateLimitFormValues>();

  const { data, isLoading } = useQuery({
    queryKey: ['rate-limits', page, pageSize],
    queryFn: () => rateLimitApi.list({ page, page_size: pageSize }),
    placeholderData: keepPreviousData,
  });

  const { data: apikeysData } = useQuery({
    queryKey: ['apikeys-all'],
    queryFn: () => apikeyApi.list({ page: 1, page_size: 500 }),
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['rate-limits'] });

  const saveMutation = useMutation({
    mutationFn: (body: RateLimitInput) =>
      editing ? rateLimitApi.update(editing.id, body) : rateLimitApi.create(body),
    onSuccess: () => {
      message.success(editing ? t('rateLimits.updated') : t('rateLimits.created'));
      setModalOpen(false);
      invalidate();
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: rateLimitApi.remove,
    onSuccess: () => {
      message.success(t('common.deleteSuccess'));
      invalidate();
    },
    onError: () => undefined,
  });

  const toggleEnabled = async (record: RateLimit, enabled: boolean) => {
    await rateLimitApi.update(record.id, {
      api_key_id: record.api_key_id,
      model_alias: record.model_alias,
      window_seconds: record.window_seconds,
      max_requests: record.max_requests,
      enabled,
    });
    invalidate();
  };

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ api_key_id: 0, model_alias: '*', enabled: true });
    setModalOpen(true);
  };

  const openEdit = (record: RateLimit) => {
    setEditing(record);
    form.setFieldsValue({
      api_key_id: record.api_key_id,
      model_alias: record.model_alias,
      window_seconds: record.window_seconds,
      max_requests: record.max_requests,
      enabled: record.enabled,
    });
    setModalOpen(true);
  };

  const handleSave = async () => {
    const v = await form.validateFields();
    saveMutation.mutate({
      api_key_id: v.api_key_id,
      model_alias: v.model_alias.trim() || '*',
      window_seconds: v.window_seconds,
      max_requests: v.max_requests,
      enabled: v.enabled ?? true,
    });
  };

  return (
    <PageContainer
      title={t('rateLimits.title')}
      description={t('rateLimits.desc')}
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          {t('rateLimits.add')}
        </Button>
      }
    >
      <Table<RateLimit>
        rowKey="id"
        loading={isLoading || deleteMutation.isPending}
        dataSource={data?.items ?? []}
        scroll={{ x: 860 }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 60, render: (v: number) => <Tag>{v}</Tag> },
          {
            title: 'API Key',
            dataIndex: 'api_key_label',
            width: 160,
            render: (v: string, r: RateLimit) =>
              r.api_key_id === 0 ? <Tag color="purple">{v || t('rateLimits.global')}</Tag> : v,
          },
          {
            title: t('rateLimits.colModel'),
            dataIndex: 'model_alias',
            width: 130,
            render: (v: string) => (v === '*' ? <Tag color="blue">{t('rateLimits.allModels')}</Tag> : v),
          },
          { title: t('rateLimits.colWindow'), dataIndex: 'window_seconds', width: 110 },
          { title: t('rateLimits.colMaxReq'), dataIndex: 'max_requests', width: 120 },
          {
            title: t('common.status'),
            dataIndex: 'enabled',
            width: 80,
            render: (_: unknown, record: RateLimit) => (
              <StatusSwitch checked={record.enabled} onChange={(c) => toggleEnabled(record, c)} />
            ),
          },
          {
            title: t('rateLimits.colHits'),
            dataIndex: 'total_hits',
            width: 100,
            render: (v: number) => <Tag color="volcano">{fmtNumber(v)}</Tag>,
          },
          {
            title: t('common.action'),
            key: 'action',
            width: 140,
            fixed: 'right',
            render: (_: unknown, record: RateLimit) => (
              <Space>
                <Button size="small" onClick={() => openEdit(record)}>
                  {t('common.edit')}
                </Button>
                <Popconfirm
                  title={t('rateLimits.deleteTitle')}
                  description={t('rateLimits.deleteConfirm')}
                  okButtonProps={{ danger: true }}
                  onConfirm={() => deleteMutation.mutate(record.id)}
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

      <Modal
        title={editing ? t('rateLimits.editTitle', { id: editing.id }) : t('rateLimits.createTitle')}
        open={modalOpen}
        confirmLoading={saveMutation.isPending}
        onOk={handleSave}
        onCancel={() => setModalOpen(false)}
        width={480}
        destroyOnClose
      >
        <Form<RateLimitFormValues> form={form} layout="vertical">
          <Form.Item
            name="api_key_id"
            label={t('rateLimits.targetLabel')}
            rules={[{ required: true, message: t('rateLimits.targetReq') }]}
          >
            <Select
              showSearch
              optionFilterProp="label"
              options={[
                { value: 0, label: t('rateLimits.globalOption') },
                ...(apikeysData?.items ?? []).map((k) => ({
                  value: k.id,
                  label: `${k.id} — ${k.name} (${k.key_masked})`,
                })),
              ]}
            />
          </Form.Item>
          <Form.Item
            name="model_alias"
            label={t('rateLimits.modelLabel')}
            tooltip={t('rateLimits.modelTooltip')}
            rules={[{ required: true, message: t('rateLimits.modelReq') }]}
          >
            <Input placeholder={t('rateLimits.modelPh')} />
          </Form.Item>
          <Space size={16} align="start" wrap>
            <Form.Item
              name="window_seconds"
              label={t('rateLimits.windowLabel')}
              rules={[{ required: true, message: t('rateLimits.windowReq') }]}
            >
              <InputNumber min={1} max={86400} precision={0} style={{ width: 170 }} placeholder="60" />
            </Form.Item>
            <Form.Item
              name="max_requests"
              label={t('rateLimits.maxReqLabel')}
              rules={[{ required: true, message: t('rateLimits.maxReqReq') }]}
            >
              <InputNumber min={1} precision={0} style={{ width: 170 }} placeholder="60" />
            </Form.Item>
          </Space>
          <Form.Item name="enabled" label={t('common.status')} valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  );
}
