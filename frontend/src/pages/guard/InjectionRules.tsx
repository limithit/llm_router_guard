// ===== 注入规则管理 (REQ-010) =====
// 规则 CRUD + 启用/停用 + 「从预置模板添加」（勾选批量创建）
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
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
  ACTION_META,
  MATCH_MODE_META,
  colorOf,
  useDictLabel,
  useDictOptions,
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
  const { t } = useTranslation();
  const actionOpts = useDictOptions('action', ACTION_META);
  const modeOpts = useDictOptions('matchMode', MATCH_MODE_META);
  const actionLbl = useDictLabel('action');
  const modeLbl = useDictLabel('matchMode');
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
      message.success(editing ? t('injection.updated') : t('injection.created'));
      setModalOpen(false);
      invalidate();
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: injectionApi.remove,
    onSuccess: () => {
      message.success(t('common.deleteSuccess'));
      invalidate();
    },
    onError: () => undefined,
  });

  const createTemplates = useMutation({
    mutationFn: async () => {
      const list = templates ?? [];
      const selected = list.filter((t) => checkedTemplates.includes(t.name));
      if (!selected.length) throw new Error(t('injection.noTemplateChecked'));
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
          ? t('injection.tplCreated', { ok: r.ok, n: r.failed.length, failed: r.failed.join(', ') })
          : t('injection.tplCreatedAll', { ok: r.ok })
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
      title={t('injection.title')}
      description={t('injection.desc')}
      extra={
        <>
          <Button icon={<CheckSquareOutlined />} onClick={() => setTemplateOpen(true)}>
            {t('injection.fromTemplates')}
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            {t('injection.add')}
          </Button>
        </>
      }
    >
      <Space style={{ marginBottom: 12 }}>
        <Input.Search
          allowClear
          placeholder={t('injection.searchPh')}
          style={{ width: 240 }}
          onSearch={(v) => {
            setPage(1);
            setKeyword(v);
          }}
        />
        <Button onClick={() => setKeyword('')}>{t('common.reset')}</Button>
      </Space>

      <Table<InjectionRule>
        rowKey="id"
        loading={isLoading || deleteMutation.isPending}
        dataSource={data?.items ?? []}
        scroll={{ x: 860 }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 60, render: (v: number) => <Tag>{v}</Tag> },
          { title: t('injection.colName'), dataIndex: 'name', width: 160 },
          {
            title: t('injection.colMode'),
            dataIndex: 'match_mode',
            width: 110,
            render: (v: MatchMode) => modeLbl(v),
          },
          {
            title: t('injection.colPattern'),
            dataIndex: 'pattern',
            ellipsis: true,
            render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
          },
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
            render: (_: unknown, record: InjectionRule) => (
              <StatusSwitch checked={record.enabled} onChange={(c) => toggleEnabled(record, c)} />
            ),
          },
          { title: t('common.createdAt'), dataIndex: 'created_at', width: 160, render: (v: string) => fmtTime(v) },
          {
            title: t('common.action'),
            key: 'action',
            width: 140,
            fixed: 'right',
            render: (_: unknown, record: InjectionRule) => (
              <Space>
                <Button size="small" onClick={() => openEdit(record)}>
                  {t('common.edit')}
                </Button>
                <Popconfirm
                  title={t('injection.deleteTitle')}
                  description={t('injection.deleteConfirm', { name: record.name })}
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
        title={editing ? t('injection.editTitle', { name: editing.name }) : t('injection.createTitle')}
        open={modalOpen}
        confirmLoading={saveMutation.isPending}
        onOk={handleSave}
        onCancel={() => setModalOpen(false)}
        width={560}
        destroyOnClose
      >
        <Form<InjectionFormValues> form={form} layout="vertical">
          <Space size={16} align="start" wrap>
            <Form.Item name="name" label={t('injection.nameLabel')} rules={[{ required: true, message: t('injection.nameReq') }]}>
              <Input placeholder={t('injection.namePh')} style={{ width: 220 }} />
            </Form.Item>
            <Form.Item name="match_mode" label={t('injection.modeLabel')} rules={[{ required: true }]}>
              <Select style={{ width: 150 }} options={modeOpts} />
            </Form.Item>
          </Space>
          <Form.Item
            name="pattern"
            label={t('injection.patternLabel')}
            tooltip={t('injection.patternTooltip')}
            rules={[{ required: true, message: t('injection.patternReq') }]}
          >
            <Input placeholder={t('injection.patternPh')} />
          </Form.Item>
          <Space size={16} align="start" wrap>
            <Form.Item name="action" label={t('injection.actionLabel')} rules={[{ required: true }]}>
              <Select style={{ width: 150 }} options={actionOpts} />
            </Form.Item>
            <Form.Item name="enabled" label={t('common.status')} valuePropName="checked">
              <Switch />
            </Form.Item>
          </Space>
        </Form>
      </Modal>

      <Modal
        title={t('injection.templateTitle')}
        open={templateOpen}
        onOk={() => createTemplates.mutate()}
        confirmLoading={createTemplates.isPending}
        onCancel={() => setTemplateOpen(false)}
        width={680}
      >
        <Typography.Paragraph type="secondary">
          {t('injection.tplTip')}
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
          {t('injection.selectAll')}
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
            { title: t('injection.colName'), dataIndex: 'name', width: 160 },
            {
              title: t('injection.colMode'),
              dataIndex: 'match_mode',
              width: 110,
              render: (v: MatchMode) => modeLbl(v),
            },
            {
              title: t('injection.tplColPattern'),
              dataIndex: 'pattern',
              ellipsis: true,
              render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
            },
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
    </PageContainer>
  );
}
