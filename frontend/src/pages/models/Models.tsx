// ===== 模型别名管理 (REQ-006) =====
// CRUD + upstreams 动态表单行（供应商下拉 + 上游模型名 + 权重）+ 启用/停用 + 查看统计 Drawer
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import {
  App,
  Button,
  Descriptions,
  Drawer,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Statistic,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd';
import { DeleteOutlined, LineChartOutlined, MinusCircleOutlined, PlusOutlined } from '@ant-design/icons';
import PageContainer from '../../components/PageContainer';
import { tableLoading } from '../../components/TableSkeleton';
import StatusSwitch from '../../components/StatusSwitch';
import { modelApi, providerApi } from '../../api/endpoints';
import { PROTOCOL_META, useDictLabel } from '../../constants/dicts';
import { fmtTime, fmtNumber } from '../../utils/format';
import type { ModelAlias, ModelStats, UpstreamInput } from '../../api/types';

interface ModelFormValues {
  alias: string;
  enabled: boolean;
  remark?: string;
  upstreams: Array<UpstreamInput & { provider_name?: string }>;
}

export default function Models() {
  const { t } = useTranslation();
  const protoLbl = useDictLabel('protocol');
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [searchText, setSearchText] = useState('');
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<ModelAlias | null>(null);
  const [statsTarget, setStatsTarget] = useState<ModelAlias | null>(null);
  const [form] = Form.useForm<ModelFormValues>();

  const { data, isLoading } = useQuery({
    queryKey: ['models', page, pageSize, searchText],
    queryFn: () => modelApi.list({ page, page_size: pageSize, keyword: searchText || undefined }),
    placeholderData: keepPreviousData,
  });

  const { data: providersData } = useQuery({
    queryKey: ['providers-all'],
    queryFn: () => providerApi.list({ page: 1, page_size: 500 }),
  });

  const { data: stats } = useQuery({
    queryKey: ['model-stats', statsTarget?.id],
    queryFn: () => modelApi.stats(statsTarget!.id),
    enabled: Boolean(statsTarget),
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['models'] });

  const saveMutation = useMutation({
    mutationFn: (body: Parameters<typeof modelApi.create>[0]) =>
      editing ? modelApi.update(editing.id, body) : modelApi.create(body),
    onSuccess: () => {
      message.success(editing ? t('models.updated') : t('models.created'));
      setModalOpen(false);
      invalidate();
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: modelApi.remove,
    onSuccess: () => {
      message.success(t('common.deleteSuccess'));
      invalidate();
    },
    onError: () => undefined,
  });

  const toggleEnabled = async (record: ModelAlias, enabled: boolean) => {
    await modelApi.update(record.id, {
      alias: record.alias,
      enabled,
      remark: record.remark,
      upstreams: record.upstreams.map((u) => ({
        provider_id: u.provider_id,
        upstream_model: u.upstream_model,
        weight: u.weight,
      })),
    });
    message.success(enabled ? t('models.enabledOk') : t('models.disabledOk'));
    invalidate();
  };

  const providerName = (id: number) =>
    providersData?.items.find((p) => p.id === id)?.name ?? String(id);

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({
      enabled: true,
      upstreams: [{ provider_id: undefined as unknown as number, upstream_model: '', weight: 1 }],
    });
    setModalOpen(true);
  };

  const openEdit = (record: ModelAlias) => {
    setEditing(record);
    form.setFieldsValue({
      alias: record.alias,
      enabled: record.enabled,
      remark: record.remark,
      upstreams: record.upstreams.map((u) => ({
        provider_id: u.provider_id,
        upstream_model: u.upstream_model,
        weight: u.weight,
      })),
    });
    setModalOpen(true);
  };

  const handleOk = async () => {
    const values = await form.validateFields();
    saveMutation.mutate({
      alias: values.alias.trim(),
      enabled: values.enabled ?? true,
      remark: values.remark?.trim() || '',
      upstreams: (values.upstreams ?? [])
        .filter((u) => u.provider_id != null && u.upstream_model)
        .map((u) => ({ provider_id: u.provider_id, upstream_model: u.upstream_model.trim(), weight: u.weight })),
    });
  };

  return (
    <PageContainer
      title={t('models.title')}
      description={t('models.desc')}
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          {t('models.add')}
        </Button>
      }
    >
      <Space style={{ marginBottom: 12 }}>
        <Input.Search
          allowClear
          placeholder={t('models.searchPh')}
          style={{ width: 260 }}
          onSearch={(v) => {
            setPage(1);
            setSearchText(v);
          }}
        />
        <Button onClick={() => setSearchText('')}>{t('common.reset')}</Button>
      </Space>

      <Table<ModelAlias>
        rowKey="id"
        loading={tableLoading(isLoading, undefined, (data?.items?.length ?? 0) > 0)}
        dataSource={data?.items ?? []}
        scroll={{ x: 900 }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 64, render: (v: number) => <Tag>{v}</Tag> },
          { title: t('models.colAlias'), dataIndex: 'alias', width: 160 },
          {
            title: t('models.colUpstreams'),
            key: 'upstreams',
            render: (_: unknown, record: ModelAlias) => (
              <Space size={[4, 4]} wrap>
                {record.upstreams.map((u, i) => (
                  <Tag
                    key={`${u.provider_id}-${i}`}
                    color={u.provider_enabled ? 'geekblue' : 'default'}
                    style={u.provider_enabled ? undefined : { opacity: 0.6 }}
                  >
                    {providerName(u.provider_id)} / {u.upstream_model} ×{u.weight}
                    {!u.provider_enabled && t('models.disabled')}
                  </Tag>
                ))}
                {record.upstreams.length === 0 ? <Typography.Text type="secondary">{t('models.noUpstreams')}</Typography.Text> : null}
              </Space>
            ),
          },
          {
            title: t('common.status'),
            dataIndex: 'enabled',
            width: 80,
            render: (_: unknown, record: ModelAlias) => (
              <StatusSwitch checked={record.enabled} onChange={(c) => toggleEnabled(record, c)} />
            ),
          },
          {
            title: t('models.colRemark'),
            dataIndex: 'remark',
            ellipsis: true,
            width: 120,
            render: (v: string) => v || '-',
          },
          { title: t('common.updatedAt'), dataIndex: 'updated_at', width: 160, render: (v: string) => fmtTime(v) },
          {
            title: t('common.action'),
            key: 'action',
            width: 250,
            fixed: 'right',
            render: (_: unknown, record: ModelAlias) => (
              <Space>
                <Button size="small" icon={<LineChartOutlined />} onClick={() => setStatsTarget(record)}>
                  {t('models.stats')}
                </Button>
                <Button size="small" onClick={() => openEdit(record)}>
                  {t('common.edit')}
                </Button>
                <Popconfirm
                  title={t('models.deleteTitle')}
                  description={t('models.deleteConfirm', { alias: record.alias })}
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
        title={editing ? t('models.editTitle', { alias: editing.alias }) : t('models.createTitle')}
        open={modalOpen}
        confirmLoading={saveMutation.isPending}
        onOk={handleOk}
        onCancel={() => setModalOpen(false)}
        width={720}
        destroyOnClose
      >
        <Form<ModelFormValues> form={form} layout="vertical">
          <Space size={16} align="start" wrap>
            <Form.Item
              name="alias"
              label={t('models.aliasLabel')}
              rules={[{ required: true, message: t('models.aliasReq') }]}
            >
              <Input placeholder={t('models.aliasPh')} style={{ width: 240 }} />
            </Form.Item>
            <Form.Item name="enabled" label={t('common.status')} valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item name="remark" label={t('common.remark')} style={{ width: 200 }}>
              <Input placeholder={t('models.remarkPh')} />
            </Form.Item>
          </Space>

          <Typography.Text strong>{t('models.upstreamsTitle')}</Typography.Text>
          <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
            {t('models.upstreamsHint')}
          </Typography.Paragraph>
          <Form.List name="upstreams">
            {(fields, { add, remove }) => (
              <>
                {fields.map((field) => (
                  <Space key={field.key} align="baseline" wrap style={{ marginBottom: 4 }}>
                    <Form.Item
                      name={[field.name, 'provider_id']}
                      rules={[{ required: true, message: t('models.providerReq') }]}
                    >
                      <Select
                        showSearch
                        optionFilterProp="label"
                        placeholder={t('models.providerPh')}
                        style={{ width: 190 }}
                        options={(providersData?.items ?? []).map((p) => ({
                          value: p.id,
                          label: `${p.name} (${protoLbl(p.protocol)})`,
                          disabled: !p.enabled,
                        }))}
                      />
                    </Form.Item>
                    <Form.Item
                      name={[field.name, 'upstream_model']}
                      rules={[{ required: true, message: t('models.upstreamModelReq') }]}
                    >
                      <Input placeholder={t('models.upstreamModelPh')} style={{ width: 230 }} />
                    </Form.Item>
                    <Form.Item
                      name={[field.name, 'weight']}
                      rules={[
                        { required: true, message: t('models.weightReq') },
                        { type: 'number', min: 1, message: t('models.weightInt') },
                      ]}
                    >
                      <InputNumber min={1} precision={0} placeholder={t('models.weightPh')} style={{ width: 100 }} />
                    </Form.Item>
                    <MinusCircleOutlined onClick={() => remove(field.name)} />
                  </Space>
                ))}
                <Button
                  type="dashed"
                  block
                  icon={<PlusOutlined />}
                  onClick={() => add({ provider_id: undefined, upstream_model: '', weight: 1 })}
                >
                  {t('models.addUpstream')}
                </Button>
              </>
            )}
          </Form.List>
        </Form>
      </Modal>

      <Drawer
        title={t('models.statsTitle', { alias: statsTarget?.alias ?? '' })}
        open={Boolean(statsTarget)}
        onClose={() => setStatsTarget(null)}
        width={420}
      >
        {statsTarget ? (
          <>
            <Descriptions
              column={1}
              bordered
              size="small"
              items={[
                { key: 'alias', label: t('models.colAlias'), children: statsTarget.alias },
                {
                  key: 'upstreams',
                  label: t('models.upstreamCount'),
                  children: t('models.upstreamCountUnit', { n: statsTarget.upstreams.length }),
                },
                { key: 'enabled', label: t('models.statusLabel'), children: statsTarget.enabled ? t('models.on') : t('models.off') },
                { key: 'created', label: t('common.createdAt'), children: fmtTime(statsTarget.created_at) },
              ]}
            />
            <div
              style={{
                display: 'grid',
                gridTemplateColumns: '1fr 1fr',
                gap: 12,
                marginTop: 16,
              }}
            >
              <Statistic title={t('models.calls7d')} value={fmtNumber(stats?.calls_7d)} loading={!stats} />
              <Statistic title={t('models.callsToday')} value={fmtNumber(stats?.calls_today)} loading={!stats} />
              <Statistic
                title={t('models.blocked7d')}
                value={fmtNumber(stats?.blocked_7d)}
                valueStyle={{ color: '#ff4d4f' }}
                loading={!stats}
              />
              <Statistic
                title={t('models.avgLatency')}
                value={stats?.avg_latency_ms}
                suffix="ms"
                loading={!stats}
              />
            </div>
          </>
        ) : null}
      </Drawer>
    </PageContainer>
  );
}
