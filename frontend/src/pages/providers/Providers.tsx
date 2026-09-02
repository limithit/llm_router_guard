// ===== 供应商管理 (REQ-005) =====
// 列表 CRUD + Modal 表单（编辑时 api_key 留空=不修改）+ 启用 Switch + 删除确认 + 测试连接
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
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
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { keepPreviousData } from '@tanstack/react-query';
import PageContainer from '../../components/PageContainer';
import StatusSwitch from '../../components/StatusSwitch';
import { providerApi } from '../../api/endpoints';
import { PROTOCOL_MAP, PROTOCOL_OPTIONS, labelOf } from '../../constants/dicts';
import { fmtTime } from '../../utils/format';
import type { Provider, ProviderInput, ProviderProtocol } from '../../api/types';

interface ProviderFormValues {
  name: string;
  protocol: ProviderProtocol;
  base_url: string;
  api_key?: string;
  enabled: boolean;
  remark?: string;
}

export default function Providers() {
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [keyword, setKeyword] = useState('');
  const [searchText, setSearchText] = useState('');

  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<Provider | null>(null);
  const [form] = Form.useForm<ProviderFormValues>();
  const [testingId, setTestingId] = useState<number | null>(null);

  // 测试连接返回模型列表后，弹窗让用户勾选导入为模型别名
  const [importModalOpen, setImportModalOpen] = useState(false);
  const [importProvider, setImportProvider] = useState<Provider | null>(null);
  const [modelList, setModelList] = useState<string[]>([]);
  const [selectedModels, setSelectedModels] = useState<string[]>([]);
  const [importing, setImporting] = useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ['providers', page, pageSize, searchText],
    queryFn: () =>
      providerApi.list({ page, page_size: pageSize, keyword: searchText || undefined }),
    placeholderData: keepPreviousData,
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['providers'] });

  const saveMutation = useMutation({
    mutationFn: (body: ProviderInput) =>
      editing ? providerApi.update(editing.id, body) : providerApi.create(body),
    onSuccess: () => {
      message.success(editing ? '供应商已更新' : '供应商已创建');
      setModalOpen(false);
      invalidate();
    },
    onError: () => undefined, // 拦截器已 toast
  });

  const deleteMutation = useMutation({
    mutationFn: providerApi.remove,
    onSuccess: () => {
      message.success('已删除');
      invalidate();
    },
    onError: () => undefined,
  });

  const toggleEnabled = async (record: Provider, enabled: boolean) => {
    await providerApi.update(record.id, {
      name: record.name,
      protocol: record.protocol,
      base_url: record.base_url,
      api_key: '', // 空 = 不修改
      enabled,
      remark: record.remark,
    });
    message.success(enabled ? '已启用' : '已禁用');
    invalidate();
  };

  const handleTest = async (record: Provider) => {
    setTestingId(record.id);
    try {
      const res = await providerApi.test(record.id);
      if (res.ok) {
        const extra = res.models?.length ? `，返回 ${res.models.length} 个模型` : '';
        message.success(`连接成功，延迟 ${res.latency_ms} ms${extra}`);
        // 返回了模型列表 → 弹窗勾选导入为模型别名
        if (res.models && res.models.length) {
          setImportProvider(record);
          setModelList(res.models);
          setSelectedModels(res.models); // 默认全选
          setImportModalOpen(true);
        }
      } else {
        message.error(`连接失败 — ${res.message}`);
      }
    } catch {
      // 拦截器已 toast
    } finally {
      setTestingId(null);
    }
  };

  const handleImport = async () => {
    if (!importProvider || selectedModels.length === 0) return;
    setImporting(true);
    try {
      const res = await providerApi.importModels(importProvider.id, {
        models: selectedModels,
        enabled: true,
      });
      const skippedNote = res.skipped ? `，跳过 ${res.skipped} 个已存在` : '';
      message.success(`已导入 ${res.created} 个模型别名${skippedNote}`);
      setImportModalOpen(false);
      queryClient.invalidateQueries({ queryKey: ['models'] });
    } catch {
      // 拦截器已 toast
    } finally {
      setImporting(false);
    }
  };

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ protocol: 'openai_chat', enabled: true });
    setModalOpen(true);
  };

  const openEdit = (record: Provider) => {
    setEditing(record);
    form.setFieldsValue({
      name: record.name,
      protocol: record.protocol,
      base_url: record.base_url,
      api_key: undefined,
      enabled: record.enabled,
      remark: record.remark,
    });
    setModalOpen(true);
  };

  const handleOk = async () => {
    const values = await form.validateFields();
    saveMutation.mutate({
      name: values.name.trim(),
      protocol: values.protocol,
      base_url: values.base_url.trim(),
      api_key: values.api_key?.trim() || '',
      enabled: values.enabled ?? true,
      remark: values.remark?.trim() || '',
    });
  };

  return (
    <PageContainer
      title="供应商管理"
      description="管理上游 AI 模型供应商接入信息（REQ-005），变更通过热加载 ≤3s 内生效。"
      extra={
        <>
          <Button icon={<ReloadOutlined />} onClick={() => invalidate()}>
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            新建供应商
          </Button>
        </>
      }
    >
      <Space style={{ marginBottom: 12 }}>
        <Input.Search
          allowClear
          placeholder="按名称/端点搜索"
          style={{ width: 260 }}
          onSearch={(v) => {
            setPage(1);
            setKeyword(v);
            setSearchText(v);
          }}
          onChange={(e) => setKeyword(e.target.value)}
          value={keyword}
        />
        <Button
          onClick={() => {
            setKeyword('');
            setSearchText('');
            setPage(1);
          }}
        >
          重置
        </Button>
      </Space>

      <Table<Provider>
        rowKey="id"
        loading={isLoading || deleteMutation.isPending}
        dataSource={data?.items ?? []}
        size="middle"
        scroll={{ x: 980 }}
        columns={[
          {
            title: 'ID',
            dataIndex: 'id',
            width: 64,
            render: (v: number) => <Tag>{v}</Tag>,
          },
          { title: '名称', dataIndex: 'name', width: 140 },
          {
            title: '协议',
            dataIndex: 'protocol',
            width: 180,
            render: (v: ProviderProtocol) => labelOf(PROTOCOL_MAP.labels, v),
          },
          { title: 'API 端点', dataIndex: 'base_url', ellipsis: true },
          {
            title: 'API Key',
            dataIndex: 'api_key_masked',
            width: 150,
            render: (v: string) => <Typography.Text code>{v || '-'}</Typography.Text>,
          },
          {
            title: '启用',
            dataIndex: 'enabled',
            width: 80,
            render: (_: unknown, record: Provider) => (
              <StatusSwitch checked={record.enabled} onChange={(c) => toggleEnabled(record, c)} />
            ),
          },
          {
            title: '备注',
            dataIndex: 'remark',
            width: 120,
            ellipsis: true,
            render: (v: string) => v || '-',
          },
          {
            title: '更新时间',
            dataIndex: 'updated_at',
            width: 160,
            render: (v: string) => fmtTime(v),
          },
          {
            title: '操作',
            key: 'action',
            width: 220,
            fixed: 'right',
            render: (_: unknown, record: Provider) => (
              <Space>
                <Button size="small" loading={testingId === record.id} onClick={() => handleTest(record)}>
                  测试连接
                </Button>
                <Button size="small" onClick={() => openEdit(record)}>
                  编辑
                </Button>
                <Popconfirm
                  title="删除供应商"
                  description={`确认删除「${record.name}」？如被模型别名引用将被后端拒绝。`}
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
        title={editing ? `编辑供应商：${editing.name}` : '新建供应商'}
        open={modalOpen}
        confirmLoading={saveMutation.isPending}
        onOk={handleOk}
        onCancel={() => setModalOpen(false)}
        width={560}
        destroyOnClose
      >
        <Form<ProviderFormValues> form={form} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item
            name="name"
            label="供应商名称（唯一标识）"
            rules={[{ required: true, message: '请输入供应商名称' }]}
          >
            <Input placeholder="例如 openai-main" />
          </Form.Item>
          <Form.Item
            name="protocol"
            label="协议类型"
            rules={[{ required: true, message: '请选择协议类型' }]}
          >
            <Select options={PROTOCOL_OPTIONS} />
          </Form.Item>
          <Form.Item
            name="base_url"
            label="API 端点 URL"
            rules={[
              { required: true, message: '请输入 API 端点 URL' },
              { type: 'url', message: '请输入合法的 URL（含 http(s)://）' },
            ]}
          >
            <Input placeholder="https://api.openai.com/v1" />
          </Form.Item>
          <Form.Item
            name="api_key"
            label="API Key"
            extra={editing ? '编辑时留空表示不修改原 Key' : '将加密存储，列表中只展示脱敏值'}
            rules={editing ? [] : [{ required: true, message: '请输入 API Key' }]}
          >
            <Input.Password placeholder={editing ? '留空表示不修改' : 'sk-...'} autoComplete="new-password" />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked">
            <Switch checkedChildren="启用" unCheckedChildren="停用" />
          </Form.Item>
          <Form.Item name="remark" label="备注">
            <Input.TextArea rows={2} placeholder="备注（可选）" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={`导入模型别名 — ${importProvider?.name ?? ''}`}
        open={importModalOpen}
        confirmLoading={importing}
        okText={`导入选中（${selectedModels.length}）`}
        okButtonProps={{ disabled: selectedModels.length === 0 }}
        cancelText="取消"
        onOk={handleImport}
        onCancel={() => setImportModalOpen(false)}
        width={560}
        destroyOnClose
      >
        <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
          勾选要启用的模型，将作为「模型别名」导入（别名 = 上游模型名，单上游指向本供应商）。已存在的别名会自动跳过。
        </Typography.Paragraph>
        <Table
          size="small"
          rowKey="name"
          dataSource={modelList.map((m) => ({ name: m }))}
          columns={[{ title: '模型', dataIndex: 'name' }]}
          pagination={false}
          scroll={{ y: 360 }}
          rowSelection={{
            selectedRowKeys: selectedModels,
            onChange: (keys) => setSelectedModels(keys as string[]),
          }}
        />
      </Modal>
    </PageContainer>
  );
}
