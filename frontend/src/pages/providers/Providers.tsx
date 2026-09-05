// ===== 供应商管理 (REQ-005) =====
// 列表 CRUD + Modal 表单（编辑时 api_key 留空=不修改）+ 启用 Switch + 删除确认 + 测试连接
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
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
import { tableLoading } from '../../components/TableSkeleton';
import StatusSwitch from '../../components/StatusSwitch';
import { providerApi } from '../../api/endpoints';
import { PROTOCOL_META, useDictLabel, useDictOptions } from '../../constants/dicts';
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
  const { t } = useTranslation();
  const protoOpts = useDictOptions('protocol', PROTOCOL_META);
  const protoLbl = useDictLabel('protocol');
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
      message.success(editing ? t('providers.updated') : t('providers.created'));
      setModalOpen(false);
      invalidate();
    },
    onError: () => undefined, // 拦截器已 toast
  });

  const deleteMutation = useMutation({
    mutationFn: providerApi.remove,
    onSuccess: () => {
      message.success(t('common.deleteSuccess'));
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
    message.success(enabled ? t('providers.enabledOk') : t('providers.disabledOk'));
    invalidate();
  };

  const handleTest = async (record: Provider) => {
    setTestingId(record.id);
    try {
      const res = await providerApi.test(record.id);
      if (res.ok) {
        const extra = res.models?.length ? t('providers.testModels', { n: res.models.length }) : '';
        message.success(t('providers.testOk', { ms: res.latency_ms }) + extra);
        // 连接成功但未返回模型列表 → 提示手动添加上游
        if (!res.models || res.models.length === 0) {
          message.info(t('providers.testNoModels'));
        }
        if (res.models && res.models.length) {
          setImportProvider(record);
          setModelList(res.models);
          setSelectedModels(res.models); // 默认全选
          setImportModalOpen(true);
        }
      } else {
        message.error(t('providers.testFail', { msg: res.message }));
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
      let msg = t('providers.importOk', { created: res.created });
      if (res.added) msg += t('providers.importAdded', { n: res.added });
      if (res.skipped) msg += t('providers.importSkipped', { n: res.skipped });
      message.success(msg);
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
      title={t('providers.title')}
      description={t('providers.desc')}
      extra={
        <>
          <Button icon={<ReloadOutlined />} onClick={() => invalidate()}>
            {t('providers.refresh')}
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            {t('providers.add')}
          </Button>
        </>
      }
    >
      <Space style={{ marginBottom: 12 }}>
        <Input.Search
          allowClear
          placeholder={t('providers.searchPh')}
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
          {t('common.reset')}
        </Button>
      </Space>

      <Table<Provider>
        rowKey="id"
        loading={tableLoading(isLoading, deleteMutation.isPending, (data?.items?.length ?? 0) > 0)}
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
          { title: t('providers.colName'), dataIndex: 'name', width: 140 },
          {
            title: t('providers.colProtocol'),
            dataIndex: 'protocol',
            width: 180,
            render: (v: ProviderProtocol) => protoLbl(v),
          },
          { title: t('providers.colEndpoint'), dataIndex: 'base_url', ellipsis: true },
          {
            title: 'API Key',
            dataIndex: 'api_key_masked',
            width: 150,
            render: (v: string) => <Typography.Text code>{v || '-'}</Typography.Text>,
          },
          {
            title: t('common.status'),
            dataIndex: 'enabled',
            width: 80,
            render: (_: unknown, record: Provider) => (
              <StatusSwitch checked={record.enabled} onChange={(c) => toggleEnabled(record, c)} />
            ),
          },
          {
            title: t('common.remark'),
            dataIndex: 'remark',
            width: 120,
            ellipsis: true,
            render: (v: string) => v || '-',
          },
          {
            title: t('providers.colUpdatedAt'),
            dataIndex: 'updated_at',
            width: 160,
            render: (v: string) => fmtTime(v),
          },
          {
            title: t('common.action'),
            key: 'action',
            width: 220,
            fixed: 'right',
            render: (_: unknown, record: Provider) => (
              <Space>
                <Button size="small" loading={testingId === record.id} onClick={() => handleTest(record)}>
                  {t('providers.test')}
                </Button>
                <Button size="small" onClick={() => openEdit(record)}>
                  {t('common.edit')}
                </Button>
                <Popconfirm
                  title={t('providers.deleteTitle')}
                  description={t('providers.deleteConfirm', { name: record.name })}
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
        title={editing ? t('providers.editTitle', { name: editing.name }) : t('providers.createTitle')}
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
            label={t('providers.nameLabel')}
            rules={[{ required: true, message: t('providers.nameReq') }]}
          >
            <Input placeholder={t('providers.namePh')} />
          </Form.Item>
          <Form.Item
            name="protocol"
            label={t('providers.protocolLabel')}
            rules={[{ required: true, message: t('providers.protocolReq') }]}
          >
            <Select options={protoOpts} />
          </Form.Item>
          <Form.Item
            name="base_url"
            label={t('providers.urlLabel')}
            rules={[
              { required: true, message: t('providers.urlReq') },
              { type: 'url', message: t('providers.urlInvalid') },
            ]}
          >
            <Input placeholder="https://api.openai.com/v1" />
          </Form.Item>
          <Form.Item
            name="api_key"
            label="API Key"
            extra={editing ? t('providers.keyExtraEdit') : t('providers.keyExtraNew')}
            rules={editing ? [] : [{ required: true, message: t('providers.keyReq') }]}
          >
            <Input.Password placeholder={editing ? t('providers.keyPhKeep') : 'sk-...'} autoComplete="new-password" />
          </Form.Item>
          <Form.Item name="enabled" label={t('common.status')} valuePropName="checked">
            <Switch checkedChildren={t('providers.on')} unCheckedChildren={t('providers.off')} />
          </Form.Item>
          <Form.Item name="remark" label={t('common.remark')}>
            <Input.TextArea rows={2} placeholder={t('providers.remarkPh')} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={t('providers.importTitle', { name: importProvider?.name ?? '' })}
        open={importModalOpen}
        confirmLoading={importing}
        okText={t('providers.importOkText', { n: selectedModels.length })}
        okButtonProps={{ disabled: selectedModels.length === 0 }}
        cancelText={t('common.cancel')}
        onOk={handleImport}
        onCancel={() => setImportModalOpen(false)}
        width={560}
        destroyOnClose
      >
        <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
          {t('providers.importHint')}
        </Typography.Paragraph>
        <Table
          size="small"
          rowKey="name"
          dataSource={modelList.map((m) => ({ name: m }))}
          columns={[{ title: t('providers.colModel'), dataIndex: 'name' }]}
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
