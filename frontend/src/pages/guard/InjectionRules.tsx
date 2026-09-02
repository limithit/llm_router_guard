// ===== 注入规则管理 (REQ-010) =====
// 规则 CRUD + 启用/停用 + 「从预置模板添加」（勾选批量创建）
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
import {
  App,
  Button,
  Checkbox,
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
import { CheckSquareOutlined, PlusOutlined } from '@ant-design/icons';
import PageContainer from '../../components/PageContainer';
import StatusSwitch from '../../components/StatusSwitch';
import { injectionApi } from '../../api/endpoints';
import {
  ACTION_MAP,
  ACTION_OPTIONS,
  colorOf,
  labelOf,
  MATCH_MODE_MAP,
  MATCH_MODE_OPTIONS,
} from '../../constants/dicts';
import { fmtTime } from '../../utils/format';
import type { InjectionRule, InjectionRuleInput, InjectionRuleTemplate, MatchMode, RuleAction } from '../../api/types';

interface InjectionFormValues {
  name: string;
  pattern: string;
  match_mode: MatchMode;
  action: RuleAction;
  enabled: boolean;
}

export default function InjectionRules() {
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [keyword, setKeyword] = useState('');

  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<InjectionRule | null>(null);
  const [form] = Form.useForm<InjectionFormValues>();

  const [templateOpen, setTemplateOpen] = useState(false);
  const [checkedTemplates, setCheckedTemplates] = useState<string[]>([]);

  const { data, isLoading } = useQuery({
    queryKey: ['injection-rules', page, pageSize, keyword],
    queryFn: () => injectionApi.list({ page, page_size: pageSize, keyword: keyword || undefined }),
    placeholderData: keepPreviousData,
  });

  const { data: templates } = useQuery({
    queryKey: ['injection-templates'],
    queryFn: injectionApi.templates,
    enabled: templateOpen,
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['injection-rules'] });

  const saveMutation = useMutation({
    mutationFn: (body: InjectionRuleInput) =>
      editing ? injectionApi.update(editing.id, body) : injectionApi.create(body),
    onSuccess: () => {
      message.success(editing ? '规则已更新' : '规则已创建');
      setModalOpen(false);
      invalidate();
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: injectionApi.remove,
    onSuccess: () => {
      message.success('已删除');
      invalidate();
    },
    onError: () => undefined,
  });

  const createTemplates = useMutation({
    mutationFn: async () => {
      const list = templates ?? [];
      const selected = list.filter((t) => checkedTemplates.includes(t.name));
      if (!selected.length) throw new Error('未勾选任何模板');
      let ok = 0;
      const errors: string[] = [];
      for (const t of selected) {
        try {
          await injectionApi.create({
            name: t.name,
            pattern: t.pattern,
            match_mode: t.match_mode,
            action: t.action,
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

  const toggleEnabled = async (record: InjectionRule, enabled: boolean) => {
    await injectionApi.update(record.id, {
      name: record.name,
      pattern: record.pattern,
      match_mode: record.match_mode,
      action: record.action,
      enabled,
    });
    invalidate();
  };

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ match_mode: 'regex', action: 'block', enabled: true });
    setModalOpen(true);
  };

  const openEdit = (record: InjectionRule) => {
    setEditing(record);
    form.setFieldsValue({
      name: record.name,
      pattern: record.pattern,
      match_mode: record.match_mode,
      action: record.action,
      enabled: record.enabled,
    });
    setModalOpen(true);
  };

  const handleSave = async () => {
    const v = await form.validateFields();
    saveMutation.mutate({
      name: v.name.trim(),
      pattern: v.pattern,
      match_mode: v.match_mode,
      action: v.action,
      enabled: v.enabled ?? true,
    });
  };

  return (
    <PageContainer
      title="注入规则管理"
      description="配置提示词注入 / 越狱攻击检测规则（REQ-010）。"
      extra={
        <>
          <Button icon={<CheckSquareOutlined />} onClick={() => setTemplateOpen(true)}>
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

      <Table<InjectionRule>
        rowKey="id"
        loading={isLoading || deleteMutation.isPending}
        dataSource={data?.items ?? []}
        scroll={{ x: 860 }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 60, render: (v: number) => <Tag>{v}</Tag> },
          { title: '规则名', dataIndex: 'name', width: 160 },
          {
            title: '匹配模式',
            dataIndex: 'match_mode',
            width: 110,
            render: (v: MatchMode) => labelOf(MATCH_MODE_MAP.labels, v),
          },
          {
            title: '正则/关键词',
            dataIndex: 'pattern',
            ellipsis: true,
            render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
          },
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
            render: (_: unknown, record: InjectionRule) => (
              <StatusSwitch checked={record.enabled} onChange={(c) => toggleEnabled(record, c)} />
            ),
          },
          { title: '创建时间', dataIndex: 'created_at', width: 160, render: (v: string) => fmtTime(v) },
          {
            title: '操作',
            key: 'action',
            width: 140,
            fixed: 'right',
            render: (_: unknown, record: InjectionRule) => (
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

      <Modal
        title={editing ? `编辑规则：${editing.name}` : '新建注入规则'}
        open={modalOpen}
        confirmLoading={saveMutation.isPending}
        onOk={handleSave}
        onCancel={() => setModalOpen(false)}
        width={560}
        destroyOnClose
      >
        <Form<InjectionFormValues> form={form} layout="vertical">
          <Space size={16} align="start" wrap>
            <Form.Item name="name" label="规则名称" rules={[{ required: true, message: '请输入规则名' }]}>
              <Input placeholder="例如 忽略系统指令" style={{ width: 220 }} />
            </Form.Item>
            <Form.Item name="match_mode" label="匹配模式" rules={[{ required: true }]}>
              <Select style={{ width: 150 }} options={MATCH_MODE_OPTIONS} />
            </Form.Item>
          </Space>
          <Form.Item
            name="pattern"
            label="检测表达式"
            tooltip="正则或关键词；正则合法性由后端校验"
            rules={[{ required: true, message: '请输入检测表达式' }]}
          >
            <Input placeholder="例如 ignore (all |the )?previous instructions" />
          </Form.Item>
          <Space size={16} align="start" wrap>
            <Form.Item name="action" label="处理动作" rules={[{ required: true }]}>
              <Select style={{ width: 150 }} options={ACTION_OPTIONS} />
            </Form.Item>
            <Form.Item name="enabled" label="启用" valuePropName="checked">
              <Switch />
            </Form.Item>
          </Space>
        </Form>
      </Modal>

      <Modal
        title="从预置模板添加注入规则"
        open={templateOpen}
        onOk={() => createTemplates.mutate()}
        confirmLoading={createTemplates.isPending}
        onCancel={() => setTemplateOpen(false)}
        width={680}
      >
        <Typography.Paragraph type="secondary">
          勾选预置模板后点击确定批量创建（提示词注入 / 越狱攻击常用模式）。
        </Typography.Paragraph>
        <Checkbox
          style={{ marginBottom: 8 }}
          indeterminate={
            (templates ?? []).length > 0 &&
            checkedTemplates.length > 0 &&
            checkedTemplates.length < (templates ?? []).length
          }
          checked={
            (templates ?? []).length > 0 && checkedTemplates.length === (templates ?? []).length
          }
          onChange={(e) => {
            setCheckedTemplates(e.target.checked ? (templates ?? []).map((t) => t.name) : []);
          }}
        >
          全选
        </Checkbox>
        <Table<InjectionRuleTemplate>
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
            { title: '规则名', dataIndex: 'name', width: 160 },
            {
              title: '匹配模式',
              dataIndex: 'match_mode',
              width: 110,
              render: (v: MatchMode) => labelOf(MATCH_MODE_MAP.labels, v),
            },
            {
              title: '表达式',
              dataIndex: 'pattern',
              ellipsis: true,
              render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
            },
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
    </PageContainer>
  );
}
