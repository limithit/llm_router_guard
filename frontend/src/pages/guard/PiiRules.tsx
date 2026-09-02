// ===== PII 规则管理 (REQ-009) =====
// 规则 CRUD + 启用/停用 + 「从预置模板添加」（勾选批量创建）+ 测试工具（识别/脱敏预览）
import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
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
import StatusSwitch from '../../components/StatusSwitch';
import { piiApi } from '../../api/endpoints';
import { ACTION_MAP, ACTION_OPTIONS, colorOf, labelOf, PII_CATEGORY_OPTIONS } from '../../constants/dicts';
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
      message.success(editing ? '规则已更新' : '规则已创建');
      setModalOpen(false);
      invalidate();
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: piiApi.remove,
    onSuccess: () => {
      message.success('已删除');
      invalidate();
    },
    onError: () => undefined,
  });

  /** 批量创建模板：逐个调用创建接口（契约未提供批量端点） */
  const createTemplates = useMutation({
    mutationFn: async () => {
      const list = templates ?? [];
      const selected = list.filter((t) => checkedTemplates.includes(t.name));
      if (!selected.length) throw new Error('未勾选任何模板');
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
          ? `已创建 ${r.ok} 条，失败 ${r.failed.length} 条：${r.failed.join('、')}`
          : `已从预置模板创建 ${r.ok} 条规则`
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
      message.warning('请输入要测试的文本');
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
      title="PII 规则管理"
      description="配置个人身份信息（手机号/身份证/邮箱等）识别与处理策略（REQ-009）。"
      extra={
        <>
          <Button icon={<ExperimentOutlined />} onClick={() => setTestOpen(true)}>
            测试工具
          </Button>
          <Button icon={<CheckSquareOutlined />} onClick={openTemplateModal}>
            从预置模板添加
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            新建规则
          </Button>
        </>
      }
    >
      <Space style={{ marginBottom: 12 }}>
        <Input.Search
          allowClear
          placeholder="按规则名搜索"
          style={{ width: 240 }}
          onSearch={(v) => {
            setPage(1);
            setKeyword(v);
          }}
        />
        <Button onClick={() => setKeyword('')}>重置</Button>
      </Space>

      <Table<PiiRule>
        rowKey="id"
        loading={isLoading || deleteMutation.isPending}
        dataSource={data?.items ?? []}
        scroll={{ x: 900 }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 60, render: (v: number) => <Tag>{v}</Tag> },
          { title: '规则名', dataIndex: 'name', width: 140 },
          {
            title: '类别',
            dataIndex: 'category',
            width: 120,
            render: (v: string) => labelOf(Object.fromEntries(PII_CATEGORY_OPTIONS.map((o) => [o.value, o.label])), v),
          },
          {
            title: '正则表达式',
            dataIndex: 'pattern',
            ellipsis: true,
            render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
          },
          { title: '替换符', dataIndex: 'replacement', width: 130, render: (v: string) => v || '-' },
          {
            title: '动作',
            dataIndex: 'action',
            width: 90,
            render: (v: RuleAction) => (
              <Tag color={colorOf(ACTION_MAP.colors, v)}>{labelOf(ACTION_MAP.labels, v)}</Tag>
            ),
          },
          {
            title: '启用',
            dataIndex: 'enabled',
            width: 80,
            render: (_: unknown, record: PiiRule) => (
              <StatusSwitch checked={record.enabled} onChange={(c) => toggleEnabled(record, c)} />
            ),
          },
          { title: '创建时间', dataIndex: 'created_at', width: 160, render: (v: string) => fmtTime(v) },
          {
            title: '操作',
            key: 'action',
            width: 140,
            fixed: 'right',
            render: (_: unknown, record: PiiRule) => (
              <Space>
                <Button size="small" onClick={() => openEdit(record)}>
                  编辑
                </Button>
                <Popconfirm
                  title="删除规则"
                  description={`确认删除「${record.name}」？`}
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

      {/* 新增 / 编辑 */}
      <Modal
        title={editing ? `编辑规则：${editing.name}` : '新建 PII 规则'}
        open={modalOpen}
        confirmLoading={saveMutation.isPending}
        onOk={handleSave}
        onCancel={() => setModalOpen(false)}
        width={560}
        destroyOnClose
      >
        <Form<PiiFormValues> form={form} layout="vertical">
          <Space size={16} align="start" wrap>
            <Form.Item name="name" label="规则名称" rules={[{ required: true, message: '请输入规则名' }]}>
              <Input placeholder="例如 手机号" style={{ width: 200 }} />
            </Form.Item>
            <Form.Item name="category" label="类别" rules={[{ required: true, message: '请输入类别' }]}>
              <Select
                style={{ width: 180 }}
                showSearch
                placeholder="选择或输入类别"
                options={PII_CATEGORY_OPTIONS}
              />
            </Form.Item>
          </Space>
          <Form.Item
            name="pattern"
            label="正则表达式"
            tooltip="用于识别 PII 的正则；合法性由后端校验"
            rules={[{ required: true, message: '请输入正则表达式' }]}
          >
            <Input placeholder="例如 1[3-9]\\d{9}" />
          </Form.Item>
          <Space size={16} align="start" wrap>
            <Form.Item name="replacement" label="替换符（脱敏展示）">
              <Input placeholder="例如 [PHONE]" style={{ width: 200 }} />
            </Form.Item>
            <Form.Item name="action" label="处理动作" rules={[{ required: true }]}>
              <Select
                style={{ width: 140 }}
                options={ACTION_OPTIONS}
              />
            </Form.Item>
            <Form.Item name="enabled" label="启用" valuePropName="checked">
              <Switch />
            </Form.Item>
          </Space>
        </Form>
      </Modal>

      {/* 预置模板批量添加 */}
      <Modal
        title="从预置模板添加 PII 规则"
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
            全选
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
            { title: '规则名', dataIndex: 'name' },
            { title: '类别', dataIndex: 'category', width: 120 },
            {
              title: '正则表达式',
              dataIndex: 'pattern',
              ellipsis: true,
              render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
            },
            { title: '替换符', dataIndex: 'replacement', width: 130 },
            {
              title: '动作',
              dataIndex: 'action',
              width: 90,
              render: (v: RuleAction) => (
                <Tag color={colorOf(ACTION_MAP.colors, v)}>{labelOf(ACTION_MAP.labels, v)}</Tag>
              ),
            },
          ]}
        />
      </Modal>

      {/* 测试工具 */}
      <Drawer
        title="PII 识别测试"
        open={testOpen}
        onClose={() => setTestOpen(false)}
        width={560}
        extra={
          <Button type="primary" loading={testLoading} onClick={runTest}>
            开始测试
          </Button>
        }
      >
        <Typography.Paragraph type="secondary">
          输入一段文本，预览识别到的 PII 与脱敏后的结果。
        </Typography.Paragraph>
        <Input.TextArea rows={6} placeholder="例如：我的手机号是 13800138000…" value={testText} onChange={(e) => setTestText(e.target.value)} />
        {testResult ? (
          <div style={{ marginTop: 16 }}>
            {testResult.findings.length === 0 ? (
              <Typography.Paragraph type="secondary">未识别到 PII。</Typography.Paragraph>
            ) : (
              <Table<PiiFinding>
                style={{ marginBottom: 12 }}
                size="small"
                pagination={false}
                rowKey={(r, i) => `${r.rule}-${r.sample}-${i}`}
                dataSource={testResult.findings}
                columns={[
                  { title: '规则', dataIndex: 'rule' },
                  { title: '类别', dataIndex: 'category', width: 140 },
                  { title: '命中示例', dataIndex: 'sample' },
                ]}
              />
            )}
            <Typography.Text strong>脱敏结果：</Typography.Text>
            <Typography.Paragraph style={{ marginTop: 4 }} copyable>
              {testResult.masked_text || '(无脱敏输出)'}
            </Typography.Paragraph>
          </div>
        ) : null}
      </Drawer>
    </PageContainer>
  );
}

function AlertTip() {
  return (
    <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
      <ProfileOutlined /> 勾选预置模板（手机号/身份证/邮箱/银行卡/地址）后点击确定批量创建，已存在同名规则会由后端去重提示。
    </Typography.Paragraph>
  );
}
