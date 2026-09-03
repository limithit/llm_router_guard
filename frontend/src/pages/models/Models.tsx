// ===== 模型别名管理 (REQ-006) =====
// CRUD + upstreams 动态表单行（供应商下拉 + 上游模型名 + 权重）+ 启用/停用 + 查看统计 Drawer
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
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
import StatusSwitch from '../../components/StatusSwitch';
import { modelApi, providerApi } from '../../api/endpoints';
import { fmtTime, fmtNumber } from '../../utils/format';
import type { ModelAlias, ModelStats, UpstreamInput } from '../../api/types';

interface ModelFormValues {
  alias: string;
  enabled: boolean;
  remark?: string;
  upstreams: Array<UpstreamInput & { provider_name?: string }>;
}

export default function Models() {
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
      message.success(editing ? '模型别名已更新' : '模型别名已创建');
      setModalOpen(false);
      invalidate();
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: modelApi.remove,
    onSuccess: () => {
      message.success('已删除');
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
    message.success(enabled ? '已启用' : '已禁用');
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
      title="模型别名管理"
      description="将业务模型别名映射到多个上游供应商（加权 SLB）（REQ-006）。"
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          新建模型别名
        </Button>
      }
    >
      <Space style={{ marginBottom: 12 }}>
        <Input.Search
          allowClear
          placeholder="按别名搜索"
          style={{ width: 260 }}
          onSearch={(v) => {
            setPage(1);
            setSearchText(v);
          }}
        />
        <Button onClick={() => setSearchText('')}>重置</Button>
      </Space>

      <Table<ModelAlias>
        rowKey="id"
        loading={isLoading}
        dataSource={data?.items ?? []}
        scroll={{ x: 900 }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 64, render: (v: number) => <Tag>{v}</Tag> },
          { title: '别名', dataIndex: 'alias', width: 160 },
          {
            title: '上游（供应商 / 模型名 × 权重）',
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
                    {!u.provider_enabled && '（已停用）'}
                  </Tag>
                ))}
                {record.upstreams.length === 0 ? <Typography.Text type="secondary">无上游</Typography.Text> : null}
              </Space>
            ),
          },
          {
            title: '启用',
            dataIndex: 'enabled',
            width: 80,
            render: (_: unknown, record: ModelAlias) => (
              <StatusSwitch checked={record.enabled} onChange={(c) => toggleEnabled(record, c)} />
            ),
          },
          {
            title: '备注',
            dataIndex: 'remark',
            ellipsis: true,
            width: 120,
            render: (v: string) => v || '-',
          },
          { title: '更新时间', dataIndex: 'updated_at', width: 160, render: (v: string) => fmtTime(v) },
          {
            title: '操作',
            key: 'action',
            width: 250,
            fixed: 'right',
            render: (_: unknown, record: ModelAlias) => (
              <Space>
                <Button size="small" icon={<LineChartOutlined />} onClick={() => setStatsTarget(record)}>
                  统计
                </Button>
                <Button size="small" onClick={() => openEdit(record)}>
                  编辑
                </Button>
                <Popconfirm
                  title="删除模型别名"
                  description={`确认删除「${record.alias}」？`}
                  okButtonProps={{ danger: true }}
                  onConfirm={() => deleteMutation.mutate(record.id)}
                >
                  <Button size="small" danger>
                    删除
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
          showTotal: (t) => `共 ${t} 条`,
        }}
        onChange={(p) => {
          setPage(p.current ?? 1);
          setPageSize(p.pageSize ?? 10);
        }}
      />

      <Modal
        title={editing ? `编辑模型别名：${editing.alias}` : '新建模型别名'}
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
              label="别名名称"
              rules={[{ required: true, message: '请输入别名' }]}
            >
              <Input placeholder="例如 gpt-4" style={{ width: 240 }} />
            </Form.Item>
            <Form.Item name="enabled" label="启用" valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item name="remark" label="备注" style={{ width: 200 }}>
              <Input placeholder="备注（可选）" />
            </Form.Item>
          </Space>

          <Typography.Text strong>上游供应商与权重</Typography.Text>
          <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
            按权重比例分发流量（权重为正整数），可添加多个上游。
          </Typography.Paragraph>
          <Form.List name="upstreams">
            {(fields, { add, remove }) => (
              <>
                {fields.map((field) => (
                  <Space key={field.key} align="baseline" wrap style={{ marginBottom: 4 }}>
                    <Form.Item
                      name={[field.name, 'provider_id']}
                      rules={[{ required: true, message: '选择供应商' }]}
                    >
                      <Select
                        showSearch
                        optionFilterProp="label"
                        placeholder="供应商"
                        style={{ width: 190 }}
                        options={(providersData?.items ?? []).map((p) => ({
                          value: p.id,
                          label: `${p.name}（${p.protocol}）`,
                          disabled: !p.enabled,
                        }))}
                      />
                    </Form.Item>
                    <Form.Item
                      name={[field.name, 'upstream_model']}
                      rules={[{ required: true, message: '上游模型名' }]}
                    >
                      <Input placeholder="上游模型名，如 gpt-4-0613" style={{ width: 230 }} />
                    </Form.Item>
                    <Form.Item
                      name={[field.name, 'weight']}
                      rules={[
                        { required: true, message: '权重' },
                        { type: 'number', min: 1, message: '正整数' },
                      ]}
                    >
                      <InputNumber min={1} precision={0} placeholder="权重" style={{ width: 100 }} />
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
                  添加上游
                </Button>
              </>
            )}
          </Form.List>
        </Form>
      </Modal>

      <Drawer
        title={`调用统计：${statsTarget?.alias ?? ''}`}
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
                { key: 'alias', label: '别名', children: statsTarget.alias },
                {
                  key: 'upstreams',
                  label: '上游数',
                  children: `${statsTarget.upstreams.length} 个`,
                },
                { key: 'enabled', label: '状态', children: statsTarget.enabled ? '启用' : '停用' },
                { key: 'created', label: '创建时间', children: fmtTime(statsTarget.created_at) },
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
              <Statistic title="近 7 天调用" value={fmtNumber(stats?.calls_7d)} loading={!stats} />
              <Statistic title="今日调用" value={fmtNumber(stats?.calls_today)} loading={!stats} />
              <Statistic
                title="近 7 天拦截"
                value={fmtNumber(stats?.blocked_7d)}
                valueStyle={{ color: '#ff4d4f' }}
                loading={!stats}
              />
              <Statistic
                title="平均延迟"
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
