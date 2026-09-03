// ===== 配额管理 (REQ-012) =====
// 配额列表（进度条展示 used/limit，超额自动变色）+ CRUD（api_key 下拉 / model_alias 输入或下拉）
// + 手动重置 + 批量创建
import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
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
  OVER_ACTION_META,
  PERIOD_META,
  QUOTA_TYPE_META,
  useDictLabel,
  useDictOptions,
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
  const { t } = useTranslation();
  const typeOpts = useDictOptions('quotaType', QUOTA_TYPE_META);
  const periodOpts = useDictOptions('period', PERIOD_META);
  const overActionOpts = useDictOptions('overAction', OVER_ACTION_META);
  const typeLbl = useDictLabel('quotaType');
  const periodLbl = useDictLabel('period');
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
    return ['*', ...Array.from(new Set(aliases))].map((a) => ({ value: a, label: a === '*' ? t('quotas.allModels') : a }));
  }, [modelsData, t]);

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['quotas'] });

  const saveMutation = useMutation({
    mutationFn: (body: QuotaInput) =>
      editing ? quotaApi.update(editing.id, body) : quotaApi.create(body),
    onSuccess: () => {
      message.success(editing ? t('quotas.updated') : t('quotas.created'));
      setModalOpen(false);
      invalidate();
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: quotaApi.remove,
    onSuccess: () => {
      message.success(t('common.deleteSuccess'));
      invalidate();
    },
    onError: () => undefined,
  });

  const resetMutation = useMutation({
    mutationFn: quotaApi.reset,
    onSuccess: () => {
      message.success(t('quotas.resetOk'));
      invalidate();
    },
    onError: () => undefined,
  });

  const batchMutation = useMutation({
    mutationFn: quotaApi.batch,
    onSuccess: () => {
      message.success(t('quotas.batchDone'));
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
      setBatchError(t('quotas.batchErrEmpty'));
      return [];
    }
    const items: QuotaInput[] = [];
    const errors: string[] = [];
    lines.forEach((line, idx) => {
      const parts = line.split(/[,，\t]/).map((s) => s.trim());
      const [keyToken, alias, qtype, period, limitStr, over = 'reject'] = parts;
      const keyId = apiKeyIdOf(keyToken);
      if (keyId == null) {
        errors.push(t('quotas.batchErrKey', { n: idx + 1, key: keyToken ?? '' }));
        return;
      }
      if (!alias || !qtype || !period || !limitStr) {
        errors.push(t('quotas.batchErrFields', { n: idx + 1 }));
        return;
      }
      if ((qtype !== 'requests' && qtype !== 'tokens') || (period !== 'day' && period !== 'week' && period !== 'month')) {
        errors.push(t('quotas.batchErrEnum', { n: idx + 1 }));
        return;
      }
      const limit = Number(limitStr);
      if (!Number.isInteger(limit) || limit <= 0) {
        errors.push(t('quotas.batchErrLimit', { n: idx + 1 }));
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
      title={t('quotas.title')}
      description={t('quotas.desc')}
      extra={
        <>
          <Button icon={<RedoOutlined />} onClick={() => invalidate()}>
            {t('quotas.refresh')}
          </Button>
          <Button onClick={() => setBatchOpen(true)}>{t('quotas.batch')}</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            {t('quotas.add')}
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
            title: t('quotas.colModel'),
            dataIndex: 'model_alias',
            width: 110,
            render: (v: string) => (v === '*' ? <Tag color="blue">{t('quotas.allModelsTag')}</Tag> : v),
          },
          {
            title: t('quotas.colType'),
            dataIndex: 'quota_type',
            width: 100,
            render: (v: QuotaType) => typeLbl(v),
          },
          {
            title: t('quotas.colPeriod'),
            dataIndex: 'period',
            width: 80,
            render: (v: QuotaPeriod) => periodLbl(v),
          },
          {
            title: t('quotas.colUsage'),
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
                    {over ? t('quotas.overBy', { n: fmtNumber(r.used - r.limit) }) : t('quotas.remaining', { n: fmtNumber(Math.max(0, r.limit - r.used)) })}
                  </Typography.Text>
                </div>
              );
            },
          },
          {
            title: t('quotas.colOverAction'),
            dataIndex: 'over_action',
            width: 130,
            render: (v: OverAction, r: Quota) =>
              v === 'degrade' ? (
                <Tag color="orange">
                  {t('quotas.degradeTo', { alias: r.degrade_alias || '-' })}
                </Tag>
              ) : (
                <Tag color="red">{t('quotas.reject')}</Tag>
              ),
          },
          {
            title: t('common.status'),
            dataIndex: 'enabled',
            width: 80,
            render: (_: unknown, record: Quota) => (
              <StatusSwitch checked={record.enabled} onChange={(c) => toggleEnabled(record, c)} />
            ),
          },
          {
            title: t('quotas.colResetAt'),
            dataIndex: 'reset_at',
            width: 160,
            render: (v: string) => fmtTime(v),
          },
          {
            title: t('common.action'),
            key: 'action',
            width: 210,
            fixed: 'right',
            render: (_: unknown, record: Quota) => (
              <Space>
                <Popconfirm
                  title={t('quotas.resetTitle')}
                  description={t('quotas.resetConfirm')}
                  onConfirm={() => resetMutation.mutate(record.id)}
                >
                  <Button size="small" icon={<RedoOutlined />}>
                    {t('quotas.reset')}
                  </Button>
                </Popconfirm>
                <Button size="small" onClick={() => openEdit(record)}>
                  {t('common.edit')}
                </Button>
                <Popconfirm
                  title={t('quotas.deleteTitle')}
                  description={t('quotas.deleteConfirm')}
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

      {/* 新建 / 编辑 */}
      <Modal
        title={editing ? t('quotas.editTitle') : t('quotas.createTitle')}
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
            rules={[{ required: true, message: t('quotas.keyReq') }]}
          >
            <Select
              showSearch
              optionFilterProp="label"
              placeholder={t('quotas.keyPh')}
              options={(apikeysData?.items ?? []).map((k) => ({
                value: k.id,
                label: `${k.name} (${k.key_masked})`,
              }))}
            />
          </Form.Item>
          <Form.Item
            name="model_alias"
            label={t('quotas.colModel')}
            tooltip={t('quotas.modelTooltip')}
            rules={[{ required: true, message: t('quotas.modelReq') }]}
          >
            <AutoComplete options={aliasOptions} placeholder={t('quotas.modelPh')} filterOption={(v, o) => String(o?.value ?? '').toLowerCase().includes(v.toLowerCase())} />
          </Form.Item>
          <Space size={16} align="start" wrap>
            <Form.Item name="quota_type" label={t('quotas.typeLabel')} rules={[{ required: true }]}>
              <Select style={{ width: 150 }} options={typeOpts} />
            </Form.Item>
            <Form.Item name="period" label={t('quotas.periodLabel')} rules={[{ required: true }]}>
              <Select style={{ width: 130 }} options={periodOpts} />
            </Form.Item>
            <Form.Item
              name="limit"
              label={t('quotas.limitLabel')}
              rules={[{ required: true, message: t('quotas.limitReq') }]}
            >
              <InputNumber min={1} precision={0} style={{ width: 160 }} placeholder="1000" />
            </Form.Item>
          </Space>
          <Space size={16} align="start" wrap>
            <Form.Item name="over_action" label={t('quotas.overActionLabel')} rules={[{ required: true }]}>
              <Select style={{ width: 180 }} options={overActionOpts} />
            </Form.Item>
            {overAction === 'degrade' ? (
              <Form.Item
                name="degrade_alias"
                label={t('quotas.degradeLabel')}
                rules={[{ required: true, message: t('quotas.degradeReq') }]}
              >
                <AutoComplete options={aliasOptions} style={{ width: 200 }} placeholder={t('quotas.degradePh')} />
              </Form.Item>
            ) : null}
            <Form.Item name="enabled" label={t('common.status')} valuePropName="checked">
              <Switch />
            </Form.Item>
          </Space>
        </Form>
      </Modal>

      {/* 批量创建 */}
      <Modal
        title={t('quotas.batchTitle')}
        open={batchOpen}
        confirmLoading={batchMutation.isPending}
        onOk={doBatch}
        onCancel={() => setBatchOpen(false)}
        width={680}
      >
        <Typography.Paragraph type="secondary">
          {t('quotas.batchHint')}<Typography.Text code>{t('quotas.batchFormat')}</Typography.Text>
          <br />
          {t('quotas.batchHint2')}
        </Typography.Paragraph>
        <Input.TextArea
          rows={8}
          placeholder={t('quotas.batchPh')}
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
