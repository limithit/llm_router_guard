// ===== PII 规则管理 (REQ-009) =====
// 规则 CRUD + 启用/停用 + 「从预置模板添加」（勾选批量创建）+ 测试工具（识别/脱敏预览）
import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import {
  App,
  Button,
  Checkbox,
  Drawer,
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
import {
  CheckSquareOutlined,
  ExperimentOutlined,
  PlusOutlined,
  ProfileOutlined,
} from '@ant-design/icons';
import PageContainer from '../../components/PageContainer';
import { tableLoading } from '../../components/TableSkeleton';
import StatusSwitch from '../../components/StatusSwitch';
import { piiApi } from '../../api/endpoints';
import { ACTION_META, PII_CATEGORY_META, colorOf, useDictLabel, useDictOptions } from '../../constants/dicts';
import { fmtTime } from '../../utils/format';
import type { PiiFinding, PiiRule, PiiRuleInput, PiiRuleTemplate, RuleAction } from '../../api/types';

interface PiiFormValues {
  name: string;
  category: string;
  pattern: string;
  replacement?: string;
  action: RuleAction;
  enabled: boolean;
}

export default function PiiRules() {
  const { t } = useTranslation();
  const catOpts = useDictOptions('piiCategory', PII_CATEGORY_META);
  const actionOpts = useDictOptions('action', ACTION_META);
  const catLbl = useDictLabel('piiCategory');
  const actionLbl = useDictLabel('action');
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [keyword, setKeyword] = useState('');

  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<PiiRule | null>(null);
  const [form] = Form.useForm<PiiFormValues>();

  const [templateOpen, setTemplateOpen] = useState(false);
  const [checkedTemplates, setCheckedTemplates] = useState<string[]>([]);

  const [testOpen, setTestOpen] = useState(false);
  const [testText, setTestText] = useState('');
  const [testLoading, setTestLoading] = useState(false);
  const [testResult, setTestResult] = useState<{
    blocked: boolean;
    findings: PiiFinding[];
    masked_text: string;
  } | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ['pii-rules', page, pageSize, keyword],
    queryFn: () => piiApi.list({ page, page_size: pageSize, keyword: keyword || undefined }),
    placeholderData: keepPreviousData,
  });

  const { data: templates } = useQuery({
    queryKey: ['pii-templates'],
    queryFn: piiApi.templates,
    enabled: templateOpen,
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['pii-rules'] });

  const saveMutation = useMutation({
    mutationFn: (body: PiiRuleInput) =>
      editing ? piiApi.update(editing.id, body) : piiApi.create(body),
    onSuccess: () => {
      message.success(editing ? t('pii.updated') : t('pii.created'));
      setModalOpen(false);
      invalidate();
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: piiApi.remove,
    onSuccess: () => {
      message.success(t('common.deleteSuccess'));
      invalidate();
    },
    onError: () => undefined,
  });

  /** 批量创建模板：逐个调用创建接口（契约未提供批量端点） */
  const createTemplates = useMutation({
    mutationFn: async () => {
      const list = templates ?? [];
      const selected = list.filter((t) => checkedTemplates.includes(t.name));
      if (!selected.length) throw new Error(t('pii.noTemplateChecked'));
      let ok = 0;
      const errors: string[] = [];
      for (const t of selected) {
        try {
          await piiApi.create({
            name: t.name,
            category: t.category,
            pattern: t.pattern,
            replacement: t.replacement,
            action: t.action === 'mask' ? 'mask' : t.action,
            enabled: true,
          });
          ok += 1;
        } catch {
          errors.push(t.name);
        }
      }
      return { ok, failed: errors };
    },
    onSuccess: (r) => {
      message.success(
        r.failed.length
          ? t('pii.tplCreated', { ok: r.ok, n: r.failed.length, failed: r.failed.join(', ') })
          : t('pii.tplCreatedAll', { ok: r.ok })
      );
      setTemplateOpen(false);
      setCheckedTemplates([]);
      invalidate();
    },
    onError: (e) => {
      if (e instanceof Error && e.message) message.error(e.message);
    },
  });

  const toggleEnabled = async (record: PiiRule, enabled: boolean) => {
    await piiApi.update(record.id, {
      name: record.name,
      category: record.category,
      pattern: record.pattern,
      replacement: record.replacement,
      action: record.action,
      enabled,
    });
    invalidate();
  };

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ action: 'mask', enabled: true });
    setModalOpen(true);
  };

  const openEdit = (record: PiiRule) => {
    setEditing(record);
    form.setFieldsValue({
      name: record.name,
      category: record.category,
      pattern: record.pattern,
      replacement: record.replacement,
      action: record.action,
      enabled: record.enabled,
    });
    setModalOpen(true);
  };

  const handleSave = async () => {
    const v = await form.validateFields();
    saveMutation.mutate({
      name: v.name.trim(),
      category: v.category.trim(),
      pattern: v.pattern,
      replacement: v.replacement?.trim() || '',
      action: v.action,
      enabled: v.enabled ?? true,
    });
  };

  const openTemplateModal = () => {
    setCheckedTemplates([]);
    setTemplateOpen(true);
  };

  const runTest = async () => {
    if (!testText.trim()) {
      message.warning(t('pii.testInputReq'));
      return;
    }
    setTestLoading(true);
    try {
      setTestResult(await piiApi.test(testText));
    } catch {
      // 拦截器已提示
    } finally {
      setTestLoading(false);
    }
  };

  const templateNames = useMemo(() => (templates ?? []).map((t) => t.name), [templates]);

  return (
    <PageContainer
      title={t('pii.title')}
      description={t('pii.desc')}
      extra={
        <>
          <Button icon={<ExperimentOutlined />} onClick={() => setTestOpen(true)}>
            {t('pii.testTool')}
          </Button>
          <Button icon={<CheckSquareOutlined />} onClick={openTemplateModal}>
            {t('pii.fromTemplates')}
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            {t('pii.add')}
          </Button>
        </>
      }
    >
      <Space style={{ marginBottom: 12 }}>
        <Input.Search
          allowClear
          placeholder={t('pii.searchPh')}
          style={{ width: 240 }}
          onSearch={(v) => {
            setPage(1);
            setKeyword(v);
          }}
        />
        <Button onClick={() => setKeyword('')}>{t('common.reset')}</Button>
      </Space>

      <Table<PiiRule>
        rowKey="id"
        loading={tableLoading(isLoading, deleteMutation.isPending, (data?.items?.length ?? 0) > 0)}
        dataSource={data?.items ?? []}
        scroll={{ x: 900 }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 60, render: (v: number) => <Tag>{v}</Tag> },
          { title: t('pii.colName'), dataIndex: 'name', width: 140 },
          {
            title: t('pii.colCategory'),
            dataIndex: 'category',
            width: 120,
            render: (v: string) => catLbl(v),
          },
          {
            title: t('pii.colPattern'),
            dataIndex: 'pattern',
            ellipsis: true,
            render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
          },
          { title: t('pii.colReplacement'), dataIndex: 'replacement', width: 130, render: (v: string) => v || '-' },
          {
            title: t('pii.colAction'),
            dataIndex: 'action',
            width: 90,
            render: (v: RuleAction) => (
              <Tag color={colorOf(ACTION_META, v)}>{actionLbl(v)}</Tag>
            ),
          },
          {
            title: t('common.status'),
            dataIndex: 'enabled',
            width: 80,
            render: (_: unknown, record: PiiRule) => (
              <StatusSwitch checked={record.enabled} onChange={(c) => toggleEnabled(record, c)} />
            ),
          },
          { title: t('common.createdAt'), dataIndex: 'created_at', width: 160, render: (v: string) => fmtTime(v) },
          {
            title: t('common.action'),
            key: 'action',
            width: 140,
            fixed: 'right',
            render: (_: unknown, record: PiiRule) => (
              <Space>
                <Button size="small" onClick={() => openEdit(record)}>
                  {t('common.edit')}
                </Button>
                <Popconfirm
                  title={t('pii.deleteTitle')}
                  description={t('pii.deleteConfirm', { name: record.name })}
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

      {/* 新增 / 编辑 */}
      <Modal
        title={editing ? t('pii.editTitle', { name: editing.name }) : t('pii.createTitle')}
        open={modalOpen}
        confirmLoading={saveMutation.isPending}
        onOk={handleSave}
        onCancel={() => setModalOpen(false)}
        width={560}
        destroyOnClose
      >
        <Form<PiiFormValues> form={form} layout="vertical">
          <Space size={16} align="start" wrap>
            <Form.Item name="name" label={t('pii.nameLabel')} rules={[{ required: true, message: t('pii.nameReq') }]}>
              <Input placeholder={t('pii.namePh')} style={{ width: 200 }} />
            </Form.Item>
            <Form.Item name="category" label={t('pii.colCategory')} rules={[{ required: true, message: t('pii.categoryReq') }]}>
              <Select
                style={{ width: 180 }}
                showSearch
                placeholder={t('pii.categoryPh')}
                options={catOpts}
              />
            </Form.Item>
          </Space>
          <Form.Item
            name="pattern"
            label={t('pii.colPattern')}
            tooltip={t('pii.patternTooltip')}
            rules={[{ required: true, message: t('pii.patternReq') }]}
          >
            <Input placeholder={t('pii.patternPh')} />
          </Form.Item>
          <Space size={16} align="start" wrap>
            <Form.Item name="replacement" label={t('pii.replacementLabel')}>
              <Input placeholder={t('pii.replacementPh')} style={{ width: 200 }} />
            </Form.Item>
            <Form.Item name="action" label={t('pii.actionLabel')} rules={[{ required: true }]}>
              <Select
                style={{ width: 140 }}
                options={actionOpts}
              />
            </Form.Item>
            <Form.Item name="enabled" label={t('common.status')} valuePropName="checked">
              <Switch />
            </Form.Item>
          </Space>
        </Form>
      </Modal>

      {/* 预置模板批量添加 */}
      <Modal
        title={t('pii.templateTitle')}
        open={templateOpen}
        onOk={() => createTemplates.mutate()}
        confirmLoading={createTemplates.isPending}
        onCancel={() => setTemplateOpen(false)}
        width={640}
      >
        <AlertTip />
        <div style={{ margin: '12px 0' }}>
          <Checkbox
            indeterminate={
              checkedTemplates.length > 0 && checkedTemplates.length < templateNames.length
            }
            checked={checkedTemplates.length === templateNames.length && templateNames.length > 0}
            onChange={(e) => {
              setCheckedTemplates(e.target.checked ? templateNames : []);
            }}
          >
            {t('pii.selectAll')}
          </Checkbox>
        </div>
        <Table<PiiRuleTemplate>
          rowKey="name"
          size="small"
          pagination={false}
          dataSource={templates ?? []}
          loading={!templates}
          rowSelection={{
            selectedRowKeys: checkedTemplates,
            onChange: (keys) => setCheckedTemplates(keys.map(String)),
          }}
          columns={[
            { title: t('pii.colName'), dataIndex: 'name' },
            { title: t('pii.colCategory'), dataIndex: 'category', width: 120 },
            {
              title: t('pii.colPattern'),
              dataIndex: 'pattern',
              ellipsis: true,
              render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
            },
            { title: t('pii.colReplacement'), dataIndex: 'replacement', width: 130 },
            {
              title: t('pii.colAction'),
              dataIndex: 'action',
              width: 90,
              render: (v: RuleAction) => (
                <Tag color={colorOf(ACTION_META, v)}>{actionLbl(v)}</Tag>
              ),
            },
          ]}
        />
      </Modal>

      {/* 测试工具 */}
      <Drawer
        title={t('pii.testTitle')}
        open={testOpen}
        onClose={() => setTestOpen(false)}
        width={560}
        extra={
          <Button type="primary" loading={testLoading} onClick={runTest}>
            {t('pii.testRun')}
          </Button>
        }
      >
        <Typography.Paragraph type="secondary">
          {t('pii.testHint')}
        </Typography.Paragraph>
        <Input.TextArea rows={6} placeholder={t('pii.testPh')} value={testText} onChange={(e) => setTestText(e.target.value)} />
        {testResult ? (
          <div style={{ marginTop: 16 }}>
            {testResult.findings.length === 0 ? (
              <Typography.Paragraph type="secondary">{t('pii.testNone')}</Typography.Paragraph>
            ) : (
              <Table<PiiFinding>
                style={{ marginBottom: 12 }}
                size="small"
                pagination={false}
                rowKey={(r, i) => `${r.rule}-${r.sample}-${i}`}
                dataSource={testResult.findings}
                columns={[
                  { title: t('pii.testColRule'), dataIndex: 'rule' },
                  { title: t('pii.colCategory'), dataIndex: 'category', width: 140 },
                  { title: t('pii.testColSample'), dataIndex: 'sample' },
                ]}
              />
            )}
            <Typography.Text strong>{t('pii.testMasked')}</Typography.Text>
            <Typography.Paragraph style={{ marginTop: 4 }} copyable>
              {testResult.masked_text || t('pii.testMaskedNone')}
            </Typography.Paragraph>
          </div>
        ) : null}
      </Drawer>
    </PageContainer>
  );
}

function AlertTip() {
  const { t } = useTranslation();
  return (
    <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
      <ProfileOutlined /> {t('pii.tplTip')}
    </Typography.Paragraph>
  );
}
