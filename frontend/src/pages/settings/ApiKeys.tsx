// ===== API Key 管理 (REQ-002) =====
// 创建（一次性展示完整 Key）/ 启用禁用 / 删除 / 脱敏列表 / 最后使用时间
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
import {
  Alert,
  App,
  Button,
  Form,
  Input,
  Modal,
  Popconfirm,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd';
import { CopyOutlined, PlusOutlined } from '@ant-design/icons';
import PageContainer from '../../components/PageContainer';
import StatusSwitch from '../../components/StatusSwitch';
import { apikeyApi } from '../../api/endpoints';
import { fmtTime } from '../../utils/format';
import type { ApiKey, ApiKeyCreated } from '../../api/types';

interface KeyFormValues {
  name: string;
  remark?: string;
  enabled?: boolean;
}

export default function ApiKeys() {
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
      message.success('API Key 已更新');
      setEditing(null);
      invalidate();
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: apikeyApi.remove,
    onSuccess: () => {
      message.success('已删除');
      invalidate();
    },
    onError: () => undefined,
  });

  const toggleEnabled = async (row: ApiKey, enabled: boolean) => {
    await apikeyApi.update(row.id, { name: row.name, remark: row.remark, enabled });
    invalidate();
  };

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    setCreateOpen(true);
  };

  const openEdit = (row: ApiKey) => {
    setEditing(row);
    form.setFieldsValue({ name: row.name, remark: row.remark, enabled: row.enabled });
    setCreateOpen(false);
    setEditing(row);
  };

  const handleOk = async () => {
    const v = await form.validateFields();
    if (editing) {
      updateMutation.mutate({
        id: editing.id,
        body: { name: v.name, remark: v.remark, enabled: v.enabled ?? editing.enabled },
      });
    } else {
      createMutation.mutate({ name: v.name, remark: v.remark });
    }
  };

  return (
    <PageContainer
      title="API Key 管理"
      description="业务方调用网关的凭证（REQ-002）。Key 仅创建时完整展示一次，请妥善保存。"
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          创建 API Key
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
          { title: '名称', dataIndex: 'name', width: 160 },
          {
            title: 'Key（脱敏）',
            dataIndex: 'key_masked',
            width: 180,
            render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
          },
          { title: '备注', dataIndex: 'remark', ellipsis: true },
          {
            title: '启用',
            dataIndex: 'enabled',
            width: 80,
            render: (_: unknown, row: ApiKey) => (
              <StatusSwitch checked={row.enabled} onChange={(c) => toggleEnabled(row, c)} />
            ),
          },
          { title: '最后使用', dataIndex: 'last_used_at', width: 170, render: fmtTime },
          { title: '创建时间', dataIndex: 'created_at', width: 170, render: fmtTime },
          {
            title: '操作',
            key: 'action',
            width: 140,
            fixed: 'right',
            render: (_: unknown, row: ApiKey) => (
              <Space>
                <Button size="small" onClick={() => openEdit(row)}>
                  编辑
                </Button>
                <Popconfirm
                  title="删除 API Key"
                  description={`确认删除「${row.name}」？该 Key 将立即失效。`}
                  okButtonProps={{ danger: true }}
                  onConfirm={() => deleteMutation.mutate(row.id)}
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

      {/* 创建 / 编辑弹窗 */}
      <Modal
        title={editing ? `编辑 API Key「${editing.name}」` : '创建 API Key'}
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
            label="名称"
            rules={[{ required: true, message: '请输入 Key 名称（业务方标识）' }]}
          >
            <Input placeholder="如 team-a / 财务机器人" maxLength={64} />
          </Form.Item>
          <Form.Item name="remark" label="备注">
            <Input.TextArea rows={2} placeholder="用途说明" maxLength={255} />
          </Form.Item>
          {editing && (
            <Form.Item name="enabled" label="启用" valuePropName="checked">
              <Switch />
            </Form.Item>
          )}
        </Form>
      </Modal>

      {/* 创建成功：一次性展示完整 Key */}
      <Modal
        title="API Key 创建成功"
        open={!!created}
        onCancel={() => setCreated(null)}
        footer={
          <Button type="primary" onClick={() => setCreated(null)}>
            我已保存
          </Button>
        }
        width={560}
      >
        <Alert
          type="warning"
          showIcon
          message="完整 Key 仅展示这一次，关闭后无法再次查看，请立即复制保存。"
          style={{ marginBottom: 12 }}
        />
        <Space.Compact style={{ width: '100%' }}>
          <Input value={created?.key ?? ''} readOnly />
          <Button
            icon={<CopyOutlined />}
            onClick={async () => {
              if (created?.key) {
                await navigator.clipboard.writeText(created.key);
                message.success('已复制到剪贴板');
              }
            }}
          >
            复制
          </Button>
        </Space.Compact>
        <Typography.Paragraph type="secondary" style={{ marginTop: 8 }}>
          名称：{created?.name}（ID #{created?.id}）
        </Typography.Paragraph>
      </Modal>
    </PageContainer>
  );
}
