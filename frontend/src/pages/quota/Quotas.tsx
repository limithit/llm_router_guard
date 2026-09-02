// ===== 配额管理 (REQ-012) =====
// 配额列表（进度条展示 used/limit，超额自动变色）+ CRUD（api_key 下拉 / model_alias 输入或下拉）
// + 手动重置 + 批量创建
import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
import {
  App,
  AutoComplete,
  Button,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Progress,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd';
import { PlusOutlined, RedoOutlined } from '@ant-design/icons';
import PageContainer from '../../components/PageContainer';
import StatusSwitch from '../../components/StatusSwitch';
import { apikeyApi, modelApi, quotaApi } from '../../api/endpoints';
import {
  OVER_ACTION_MAP,
  OVER_ACTION_OPTIONS,
  PERIOD_MAP,
  PERIOD_OPTIONS,
  QUOTA_TYPE_MAP,
  QUOTA_TYPE_OPTIONS,
  labelOf,
} from '../../constants/dicts';
import { clampPercent, fmtNumber, fmtTime } from '../../utils/format';
import type {
  OverAction,
  Quota,
  QuotaInput,
  QuotaPeriod,
  QuotaType,
} from '../../api/types';

interface QuotaFormValues {
  api_key_id: number;
  model_alias: string;
  quota_type: QuotaType;
  period: QuotaPeriod;
  limit: number;
  over_action: OverAction;
  degrade_alias?: string;
  enabled: boolean;
}

export default function Quotas() {
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<Quota | null>(null);
  const [form] = Form.useForm<QuotaFormValues>();
  const overAction = Form.useWatch('over_action', form);

  const [batchOpen, setBatchOpen] = useState(false);
  const [batchText, setBatchText] = useState('');
  const [batchError, setBatchError] = useState('');

  const { data, isLoading } = useQuery({
    queryKey: ['quotas', page, pageSize],
    queryFn: () => quotaApi.list({ page, page_size: pageSize }),
    placeholderData: keepPreviousData,
  });

  const { data: apikeysData } = useQuery({
    queryKey: ['apikeys-all'],
    queryFn: () => apikeyApi.list({ page: 1, page_size: 500 }),
  });

  const { data: modelsData } = useQuery({
    queryKey: ['models-all'],
    queryFn: () => modelApi.list({ page: 1, page_size: 500 }),
  });

  const aliasOptions = useMemo(() => {
    const aliases = (modelsData?.items ?? []).map((m) => m.alias);
    return ['*', ...Array.from(new Set(aliases))].map((a) => ({ value: a, label: a === '*' ? '*（全部模型）' : a }));
  }, [modelsData]);

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['quotas'] });

  const saveMutation = useMutation({
    mutationFn: (body: QuotaInput) =>
      editing ? quotaApi.update(editing.id, body) : quotaApi.create(body),
    onSuccess: () => {
      message.success(editing ? '配额已更新' : '配额已创建');
      setModalOpen(false);
      invalidate();
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: quotaApi.remove,
    onSuccess: () => {
      message.success('已删除');
      invalidate();
    },
    onError: () => undefined,
  });

  const resetMutation = useMutation({
    mutationFn: quotaApi.reset,
    onSuccess: () => {
      message.success('已重置，使用量归零');
      invalidate();
    },
    onError: () => undefined,
  });

  const batchMutation = useMutation({
    mutationFn: quotaApi.batch,
    onSuccess: () => {
      message.success('批量创建完成');
      setBatchOpen(false);
      setBatchText('');
      invalidate();
    },
    onError: () => undefined,
  });

  const toggleEnabled = async (record: Quota, enabled: boolean) => {
    await quotaApi.update(record.id, {
      api_key_id: record.api_key_id,
      model_alias: record.model_alias,
      quota_type: record.quota_type,
      period: record.period,
      limit: record.limit,
      over_action: record.over_action,
      degrade_alias: record.degrade_alias,
      enabled,
    });
    invalidate();
  };

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ quota_type: 'requests', period: 'day', over_action: 'reject', enabled: true });
    setModalOpen(true);
  };

  const openEdit = (record: Quota) => {
    setEditing(record);
    form.setFieldsValue({
      api_key_id: record.api_key_id,
      model_alias: record.model_alias,
      quota_type: record.quota_type,
      period: record.period,
      limit: record.limit,
      over_action: record.over_action,
      degrade_alias: record.degrade_alias || undefined,
      enabled: record.enabled,
    });
    setModalOpen(true);
  };

  const handleSave = async () => {
    const v = await form.validateFields();
    saveMutation.mutate({
      api_key_id: v.api_key_id,
      model_alias: v.model_alias.trim() || '*',
      quota_type: v.quota_type,
      period: v.period,
      limit: v.limit,
      over_action: v.over_action,
      degrade_alias: v.over_action === 'degrade' ? v.degrade_alias?.trim() : '',
      enabled: v.enabled ?? true,
    });
  };

  /** 批量创建：每行 api_key名称或ID,model_alias,quota_type,period,limit[,over_action] */
  const parseBatch = (): QuotaInput[] => {
    const apiKeyIdOf = (token: string): number | null => {
      if (/^\d+$/.test(token.trim())) return Number(token.trim());
      const found = (apikeysData?.items ?? []).find((k) => k.name === token.trim());
      return found ? found.id : null;
    };
    const lines = batchText
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l && !l.startsWith('#'));
    if (!lines.length) {
      setBatchError('请输入至少一行配置');
      return [];
    }
    const items: QuotaInput[] = [];
    const errors: string[] = [];
    lines.forEach((line, idx) => {
      const parts = line.split(/[,，\t]/).map((s) => s.trim());
      const [keyToken, alias, qtype, period, limitStr, over = 'reject'] = parts;
      const keyId = apiKeyIdOf(keyToken);
      if (keyId == null) {
        errors.push(`第 ${idx + 1} 行：找不到 API Key「${keyToken}」`);
        return;
      }
      if (!alias || !qtype || !period || !limitStr) {
        errors.push(`第 ${idx + 1} 行：字段不足（需要 apiKey,modelAlias,type,period,limit）`);
        return;
      }
      if ((qtype !== 'requests' && qtype !== 'tokens') || (period !== 'day' && period !== 'week' && period !== 'month')) {
        errors.push(`第 ${idx + 1} 行：枚举值不合法（type: requests/tokens, period: day/week/month）`);
        return;
      }
      const limit = Number(limitStr);
      if (!Number.isInteger(limit) || limit <= 0) {
        errors.push(`第 ${idx + 1} 行：limit 必须为正整数`);
        return;
      }
      items.push({
        api_key_id: keyId,
        model_alias: alias === '*' ? '*' : alias,
        quota_type: qtype as QuotaType,
        period: period as QuotaPeriod,
        limit,
        over_action: (over === 'degrade' ? 'degrade' : 'reject') as OverAction,
        enabled: true,
      });
    });
    if (errors.length) {
      setBatchError(errors.join('\n'));
      return [];
    }
    setBatchError('');
    return items;
  };

  const doBatch = () => {
    const items = parseBatch();
    if (items.length) {
      batchMutation.mutate({ items });
    }
  };

  return (
    <PageContainer
      title="配额管理"
      description="按 API Key 与模型别名配置请求次数 / Token 配额，支持周期自动重置（REQ-012）。"
      extra={
        <>
          <Button icon={<RedoOutlined />} onClick={() => invalidate()}>
            刷新
          </Button>
          <Button onClick={() => setBatchOpen(true)}>批量创建</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            新建配额
          </Button>
        </>
      }
    >
      <Table<Quota>
        rowKey="id"
        loading={isLoading || deleteMutation.isPending || resetMutation.isPending}
        dataSource={data?.items ?? []}
        scroll={{ x: 1100 }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 60, render: (v: number) => <Tag>{v}</Tag> },
          { title: 'API Key', dataIndex: 'api_key_label', width: 130 },
          {
            title: '模型别名',
            dataIndex: 'model_alias',
            width: 110,
            render: (v: string) => (v === '*' ? <Tag color="blue">* 全部模型</Tag> : v),
          },
          {
            title: '配额类型',
            dataIndex: 'quota_type',
            width: 100,
            render: (v: QuotaType) => labelOf(QUOTA_TYPE_MAP.labels, v),
          },
          {
            title: '周期',
            dataIndex: 'period',
            width: 80,
            render: (v: QuotaPeriod) => labelOf(PERIOD_MAP.labels, v),
          },
          {
            title: '使用情况',
            key: 'usage',
            width: 220,
            render: (_: unknown, r: Quota) => {
              const pct = clampPercent(r.used, r.limit);
              const over = r.used >= r.limit;
              return (
                <div>
                  <Progress
                    percent={Math.min(100, pct)}
                    size="small"
                    status={over || pct >= 90 ? 'exception' : pct > 0 ? 'active' : undefined}
                    format={() => `${fmtNumber(r.used)}/${fmtNumber(r.limit)}`}
                  />
                  <Typography.Text type={over ? 'danger' : 'secondary'} style={{ fontSize: 12 }}>
                    {over ? `已超额 ${fmtNumber(r.used - r.limit)}` : `剩余 ${fmtNumber(Math.max(0, r.limit - r.used))}`}
                  </Typography.Text>
                </div>
              );
            },
          },
          {
            title: '超额策略',
            dataIndex: 'over_action',
            width: 130,
            render: (v: OverAction, r: Quota) =>
              v === 'degrade' ? (
                <Tag color="orange">
                  降级至 {r.degrade_alias || '-'}
                </Tag>
              ) : (
                <Tag color="red">拒绝请求</Tag>
              ),
          },
          {
            title: '启用',
            dataIndex: 'enabled',
            width: 80,
            render: (_: unknown, record: Quota) => (
              <StatusSwitch checked={record.enabled} onChange={(c) => toggleEnabled(record, c)} />
            ),
          },
          {
            title: '重置时间',
            dataIndex: 'reset_at',
            width: 160,
            render: (v: string) => fmtTime(v),
          },
          {
            title: '操作',
            key: 'action',
            width: 210,
            fixed: 'right',
            render: (_: unknown, record: Quota) => (
              <Space>
                <Popconfirm
                  title="重置配额"
                  description={`确认将使用量归零？`}
                  onConfirm={() => resetMutation.mutate(record.id)}
                >
                  <Button size="small" icon={<RedoOutlined />}>
                    重置
                  </Button>
                </Popconfirm>
                <Button size="small" onClick={() => openEdit(record)}>
                  编辑
                </Button>
                <Popconfirm
                  title="删除配额"
                  description="确认删除该配额规则？"
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

      {/* 新建 / 编辑 */}
      <Modal
        title={editing ? '编辑配额' : '新建配额'}
        open={modalOpen}
        confirmLoading={saveMutation.isPending}
        onOk={handleSave}
        onCancel={() => setModalOpen(false)}
        width={560}
        destroyOnClose
      >
        <Form<QuotaFormValues> form={form} layout="vertical">
          <Form.Item
            name="api_key_id"
            label="API Key"
            rules={[{ required: true, message: '请选择 API Key' }]}
          >
            <Select
              showSearch
              optionFilterProp="label"
              placeholder="选择 API Key"
              options={(apikeysData?.items ?? []).map((k) => ({
                value: k.id,
                label: `${k.name}（${k.key_masked}）`,
              }))}
            />
          </Form.Item>
          <Form.Item
            name="model_alias"
            label="模型别名"
            tooltip="输入 * 表示作用于全部模型"
            rules={[{ required: true, message: '请输入或选择模型别名' }]}
          >
            <AutoComplete options={aliasOptions} placeholder="输入模型别名，或 * 表示全部" filterOption={(v, o) => o.value.toLowerCase().includes(v.toLowerCase())} />
          </Form.Item>
          <Space size={16} align="start" wrap>
            <Form.Item name="quota_type" label="配额类型" rules={[{ required: true }]}>
              <Select style={{ width: 150 }} options={QUOTA_TYPE_OPTIONS} />
            </Form.Item>
            <Form.Item name="period" label="周期类型" rules={[{ required: true }]}>
              <Select style={{ width: 130 }} options={PERIOD_OPTIONS} />
            </Form.Item>
            <Form.Item
              name="limit"
              label="配额上限"
              rules={[{ required: true, message: '请输入配额上限' }]}
            >
              <InputNumber min={1} precision={0} style={{ width: 160 }} placeholder="1000" />
            </Form.Item>
          </Space>
          <Space size={16} align="start" wrap>
            <Form.Item name="over_action" label="超额策略" rules={[{ required: true }]}>
              <Select style={{ width: 180 }} options={OVER_ACTION_OPTIONS} />
            </Form.Item>
            {overAction === 'degrade' ? (
              <Form.Item
                name="degrade_alias"
                label="降级目标模型"
                rules={[{ required: true, message: '超额降级需指定目标模型' }]}
              >
                <AutoComplete options={aliasOptions} style={{ width: 200 }} placeholder="如 gpt-4-mini" />
              </Form.Item>
            ) : null}
            <Form.Item name="enabled" label="启用" valuePropName="checked">
              <Switch />
            </Form.Item>
          </Space>
        </Form>
      </Modal>

      {/* 批量创建 */}
      <Modal
        title="批量创建配额"
        open={batchOpen}
        confirmLoading={batchMutation.isPending}
        onOk={doBatch}
        onCancel={() => setBatchOpen(false)}
        width={680}
      >
        <Typography.Paragraph type="secondary">
          每行一条，逗号分隔：<Typography.Text code>apiKey名称或ID, 模型别名, 配额类型, 周期, 上限[, 超额策略]</Typography.Text>
          <br />
          配额类型：requests / tokens；周期：day / week / month；超额策略：reject（默认）/ degrade。
        </Typography.Paragraph>
        <Input.TextArea
          rows={8}
          placeholder={'例如：\nteam-a, gpt-4, requests, day, 1000\n2, *, tokens, month, 5000000, reject'}
          value={batchText}
          onChange={(e) => {
            setBatchText(e.target.value);
            setBatchError('');
          }}
        />
        {batchError ? (
          <Typography.Paragraph type="danger" style={{ marginTop: 8, whiteSpace: 'pre-wrap' }}>
            {batchError}
          </Typography.Paragraph>
        ) : null}
      </Modal>
    </PageContainer>
  );
}
