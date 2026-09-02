// ===== 速率限制 (REQ-013) =====
// 规则列表 CRUD（api_key_id=0 显示“(全局)”）+ 启用/停用 + 命中统计展示
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
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
      message.success(editing ? '限流规则已更新' : '限流规则已创建');
      setModalOpen(false);
      invalidate();
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: rateLimitApi.remove,
    onSuccess: () => {
      message.success('已删除');
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
      title="速率限制"
      description="短周期速率限制规则，防止流量突增（REQ-013）。api_key_id=0 表示全局生效。"
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          新建限流规则
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
              r.api_key_id === 0 ? <Tag color="purple">{v || '(全局)'}</Tag> : v,
          },
          {
            title: '模型别名',
            dataIndex: 'model_alias',
            width: 130,
            render: (v: string) => (v === '*' ? <Tag color="blue">* 全部</Tag> : v),
          },
          { title: '窗口（秒）', dataIndex: 'window_seconds', width: 110 },
          { title: '最大请求数', dataIndex: 'max_requests', width: 120 },
          {
            title: '启用',
            dataIndex: 'enabled',
            width: 80,
            render: (_: unknown, record: RateLimit) => (
              <StatusSwitch checked={record.enabled} onChange={(c) => toggleEnabled(record, c)} />
            ),
          },
          {
            title: '累计命中',
            dataIndex: 'total_hits',
            width: 100,
            render: (v: number) => <Tag color="volcano">{fmtNumber(v)}</Tag>,
          },
          {
            title: '操作',
            key: 'action',
            width: 140,
            fixed: 'right',
            render: (_: unknown, record: RateLimit) => (
              <Space>
                <Button size="small" onClick={() => openEdit(record)}>
                  编辑
                </Button>
                <Popconfirm
                  title="删除限流规则"
                  description="确认删除该规则？"
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
        title={editing ? `编辑限流规则 #${editing.id}` : '新建限流规则'}
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
            label="作用对象（API Key）"
            rules={[{ required: true, message: '请选择 API Key' }]}
          >
            <Select
              showSearch
              optionFilterProp="label"
              options={[
                { value: 0, label: '0 — (全局) 对所有请求生效' },
                ...(apikeysData?.items ?? []).map((k) => ({
                  value: k.id,
                  label: `${k.id} — ${k.name}（${k.key_masked}）`,
                })),
              ]}
            />
          </Form.Item>
          <Form.Item
            name="model_alias"
            label="模型别名"
            tooltip="* 表示全部模型"
            rules={[{ required: true, message: '请输入模型别名' }]}
          >
            <Input placeholder="* 或模型别名" />
          </Form.Item>
          <Space size={16} align="start" wrap>
            <Form.Item
              name="window_seconds"
              label="时间窗口（秒）"
              rules={[{ required: true, message: '请输入时间窗口' }]}
            >
              <InputNumber min={1} max={86400} precision={0} style={{ width: 170 }} placeholder="60" />
            </Form.Item>
            <Form.Item
              name="max_requests"
              label="窗口内最大请求数"
              rules={[{ required: true, message: '请输入最大请求数' }]}
            >
              <InputNumber min={1} precision={0} style={{ width: 170 }} placeholder="60" />
            </Form.Item>
          </Space>
          <Form.Item name="enabled" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  );
}
