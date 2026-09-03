// ===== Token 用量统计看板（API Key 维度） =====
// 按时间范围统计每个 API Key / 模型消耗的 token：汇总卡片 + 历史趋势 + 模型分布 + Key 排行表 + CSV 导出。
import { useMemo, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import {
  App,
  Button,
  Card,
  Col,
  DatePicker,
  Input,
  Progress,
  Row,
  Segmented,
  Select,
  Space,
  Statistic,
  Table,
  Typography,
} from 'antd';
import { DownloadOutlined, SearchOutlined } from '@ant-design/icons';
import dayjs, { type Dayjs } from 'dayjs';
import { DonutChart, LineChart } from '../../components/Charts';
import { apikeyApi, tokenStatsApi } from '../../api/endpoints';
import { fmtNumber } from '../../utils/format';
import type { TokenStatsFilter, TokenUsageByKey } from '../../api/types';

type Granularity = 'day' | 'hour';

interface FiltersState {
  range: [Dayjs | null, Dayjs | null] | null;
  api_key_id: number | undefined;
  model: string;
  granularity: Granularity;
}

/** 构建预设时间范围（label 在组件内用 i18n 注入） */
const buildPresets = (
  t: (k: string) => string,
): {
  key: string;
  label: string;
  range: () => [Dayjs, Dayjs];
  granularity: Granularity;
}[] => [
  { key: 'today', label: t('tokenStats.presetToday'), range: () => [dayjs().startOf('day'), dayjs()], granularity: 'hour' },
  { key: '7d', label: t('tokenStats.preset7d'), range: () => [dayjs().subtract(6, 'day').startOf('day'), dayjs()], granularity: 'day' },
  { key: '30d', label: t('tokenStats.preset30d'), range: () => [dayjs().subtract(29, 'day').startOf('day'), dayjs()], granularity: 'day' },
  { key: 'month', label: t('tokenStats.presetMonth'), range: () => [dayjs().startOf('month'), dayjs()], granularity: 'day' },
];

export default function TokenStats() {
  const { t } = useTranslation();
  const { message } = App.useApp();

  const PRESETS = useMemo(() => buildPresets(t), [t]);
  const defaultFilters: FiltersState = {
    range: PRESETS[2].range(), // 默认近 30 天
    api_key_id: undefined,
    model: '',
    granularity: 'day',
  };

  const [filters, setFilters] = useState<FiltersState>(defaultFilters);
  const [applied, setApplied] = useState<FiltersState>(defaultFilters);
  const [activePreset, setActivePreset] = useState<string>('30d');

  const params = useMemo<TokenStatsFilter>(() => {
    const p: TokenStatsFilter = {
      granularity: applied.granularity,
      model: applied.model || undefined,
    };
    if (applied.range?.[0]) p.start = applied.range[0].toISOString();
    if (applied.range?.[1]) p.end = applied.range[1].toISOString();
    if (applied.api_key_id != null) p.api_key_id = applied.api_key_id;
    return p;
  }, [applied]);

  const { data, isLoading } = useQuery({
    queryKey: ['token-stats', params],
    queryFn: () => tokenStatsApi.get(params),
  });

  const { data: apikeysData } = useQuery({
    queryKey: ['apikeys-all'],
    queryFn: () => apikeyApi.list({ page: 1, page_size: 500 }),
  });

  const exportMutation = useMutation({
    mutationFn: () =>
      tokenStatsApi.exportCsv({
        start: params.start,
        end: params.end,
        api_key_id: params.api_key_id,
        model: params.model,
      }),
    onSuccess: () => message.success(t('tokenStats.exportSuccess')),
  });

  const applyPreset = (key: string) => {
    const preset = PRESETS.find((x) => x.key === key);
    if (!preset) return;
    const next: FiltersState = { ...filters, range: preset.range(), granularity: preset.granularity };
    setActivePreset(key);
    setFilters(next);
    setApplied(next); // 预设立即生效
  };

  const summary = data?.summary;
  const maxKeyTokens = Math.max(1, ...(data?.by_key ?? []).map((k) => k.total_tokens));

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {/* 标题 + 导出 */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', flexWrap: 'wrap', gap: 8 }}>
        <div>
          <Typography.Title level={4} style={{ margin: 0 }}>
            {t('tokenStats.title')}
          </Typography.Title>
          <Typography.Text type="secondary">
            {t('tokenStats.desc')}
          </Typography.Text>
        </div>
        <Button
          icon={<DownloadOutlined />}
          loading={exportMutation.isPending}
          onClick={() => exportMutation.mutate()}
        >
          {t('common.export')} CSV
        </Button>
      </div>

      {/* 筛选区 */}
      <Card size="small" styles={{ body: { paddingTop: 12 } }}>
        <Space wrap align="center">
          <Segmented
            value={activePreset}
            onChange={(v) => {
              if (v === 'custom') {
                setActivePreset('custom');
                return;
              }
              applyPreset(String(v));
            }}
            options={[...PRESETS.map((p) => ({ value: p.key, label: p.label })), { value: 'custom', label: t('tokenStats.presetCustom') }]}
          />
          <DatePicker.RangePicker
            showTime
            value={filters.range}
            onChange={(v) => {
              setActivePreset('custom');
              setFilters((f) => ({ ...f, range: v }));
            }}
            placeholder={[t('tokenStats.placeholderStart'), t('tokenStats.placeholderEnd')]}
          />
          <Select
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder={t('tokenStats.placeholderApikey')}
            style={{ width: 180 }}
            options={(apikeysData?.items ?? []).map((k) => ({ value: k.id, label: k.name }))}
            value={filters.api_key_id}
            onChange={(v: number | undefined) => setFilters((f) => ({ ...f, api_key_id: v }))}
          />
          <Input
            allowClear
            placeholder={t('tokenStats.placeholderModelAlias')}
            style={{ width: 140 }}
            value={filters.model}
            onChange={(e) => setFilters((f) => ({ ...f, model: e.target.value }))}
          />
          <Segmented
            value={filters.granularity}
            onChange={(v) => setFilters((f) => ({ ...f, granularity: v as Granularity }))}
            options={[
              { value: 'day', label: t('tokenStats.granDay') },
              { value: 'hour', label: t('tokenStats.granHour') },
            ]}
          />
          <Button
            type="primary"
            icon={<SearchOutlined />}
            onClick={() => setApplied(filters)}
          >
            {t('common.search')}
          </Button>
          <Button
            onClick={() => {
              setActivePreset('30d');
              setFilters(defaultFilters);
              setApplied(defaultFilters);
            }}
          >
            {t('common.reset')}
          </Button>
        </Space>
      </Card>

      {/* 汇总卡片 */}
      <Row gutter={[12, 12]}>
        <Col xs={12} md={4} flex="1 1 0">
          <Card size="small">
            <Statistic title={t('tokenStats.totalTokensTitle')} value={fmtNumber(summary?.total_tokens)} loading={isLoading} />
          </Card>
        </Col>
        <Col xs={12} md={4} flex="1 1 0">
          <Card size="small">
            <Statistic
              title={t('tokenStats.colPromptTokens')}
              value={fmtNumber(summary?.prompt_tokens)}
              valueStyle={{ color: '#1677ff' }}
              loading={isLoading}
            />
          </Card>
        </Col>
        <Col xs={12} md={4} flex="1 1 0">
          <Card size="small">
            <Statistic
              title={t('tokenStats.colCompletionTokens')}
              value={fmtNumber(summary?.completion_tokens)}
              valueStyle={{ color: '#52c41a' }}
              loading={isLoading}
            />
          </Card>
        </Col>
        <Col xs={12} md={4} flex="1 1 0">
          <Card size="small">
            <Statistic title={t('tokenStats.colCallsCount')} value={fmtNumber(summary?.calls)} loading={isLoading} />
          </Card>
        </Col>
        <Col xs={12} md={4} flex="1 1 0">
          <Card size="small">
            <Statistic
              title={t('tokenStats.totalKeys')}
              value={summary?.keys_with_usage ?? 0}
              suffix={`/ ${summary?.total_keys ?? 0}`}
              loading={isLoading}
            />
          </Card>
        </Col>
      </Row>

      {/* 趋势 + 模型分布 */}
      <Row gutter={[12, 12]}>
        <Col xs={24} lg={15}>
          <Card
            size="small"
            title={
              applied.granularity === 'hour'
                ? t('tokenStats.trendByHour')
                : t('tokenStats.trendByDay')
            }
            loading={isLoading}
          >
            <LineChart
              data={(data?.trend ?? []).map((item) => ({
                label: item.bucket.slice(5),
                value: item.prompt_tokens,
                value2: item.completion_tokens,
              }))}
              seriesName={[t('tokenStats.seriesPrompt'), t('tokenStats.seriesCompletion')]}
            />
          </Card>
        </Col>
        <Col xs={24} lg={9}>
          <Card size="small" title={t('tokenStats.byModelTitle')} loading={isLoading}>
            <DonutChart
              data={(data?.by_model ?? []).map((m) => ({ label: m.model, value: m.total_tokens }))}
              centerText={t('tokenStats.byModelCenter')}
            />
          </Card>
        </Col>
      </Row>

      {/* 按 Key 用量排行表 */}
      <Card size="small" title={t('tokenStats.byKeyTitle')} loading={isLoading} styles={{ body: { paddingTop: 8 } }}>
        <Table<TokenUsageByKey>
          rowKey="api_key_id"
          size="small"
          loading={isLoading}
          dataSource={data?.by_key ?? []}
          pagination={{ pageSize: 10, hideOnSinglePage: true }}
          locale={{ emptyText: t('tokenStats.byKeyEmpty') }}
          columns={[
            {
              title: t('tokenStats.colApiKey'),
              dataIndex: 'api_key_label',
              render: (v: string, r) => (
                <Space size={4}>
                  <Typography.Text strong>{v}</Typography.Text>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    #{r.api_key_id}
                  </Typography.Text>
                </Space>
              ),
            },
            {
              title: t('tokenStats.colCallsCount'),
              dataIndex: 'calls',
              width: 110,
              align: 'right',
              sorter: (a, b) => a.calls - b.calls,
              render: (v: number) => fmtNumber(v),
            },
            {
              title: t('tokenStats.colPromptTokens'),
              dataIndex: 'prompt_tokens',
              width: 140,
              align: 'right',
              sorter: (a, b) => a.prompt_tokens - b.prompt_tokens,
              render: (v: number) => fmtNumber(v),
            },
            {
              title: t('tokenStats.colCompletionTokens'),
              dataIndex: 'completion_tokens',
              width: 150,
              align: 'right',
              sorter: (a, b) => a.completion_tokens - b.completion_tokens,
              render: (v: number) => fmtNumber(v),
            },
            {
              title: t('tokenStats.colTotalTokens'),
              dataIndex: 'total_tokens',
              width: 140,
              align: 'right',
              sorter: (a, b) => a.total_tokens - b.total_tokens,
              defaultSortOrder: 'descend',
              render: (v: number) => (
                <Typography.Text strong style={{ color: '#1677ff' }}>
                  {fmtNumber(v)}
                </Typography.Text>
              ),
            },
            {
              title: t('tokenStats.colShare'),
              key: 'share',
              width: 180,
              render: (_, r) => (
                <Progress
                  percent={Math.round((r.total_tokens / maxKeyTokens) * 100)}
                  size="small"
                  format={() => fmtNumber(r.total_tokens)}
                />
              ),
            },
          ]}
        />
      </Card>
    </div>
  );
}
