// ===== 调用日志查询 (REQ-015) =====
// 时间范围 / API Key / 模型 / 状态 / 是否拦截 筛选 + 详情 Drawer（含 input/output 全文与 guard_findings）+ CSV 导出
import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
import {
  App,
  Button,
  DatePicker,
  Descriptions,
  Drawer,
  Input,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd';
import { DownloadOutlined, SearchOutlined } from '@ant-design/icons';
import type { Dayjs } from 'dayjs';
import PageContainer from '../../components/PageContainer';
import JsonView from '../../components/JsonView';
import LiveAuditStream from '../../components/LiveAuditStream';
import { apikeyApi, callLogApi } from '../../api/endpoints';
import { CALL_STATUS_META, colorOf, useDictLabel, useDictOptions } from '../../constants/dicts';
import { useTranslation } from 'react-i18next';
import { fmtNumber, fmtTime } from '../../utils/format';
import type { CallLogDetail, CallLogRow, CallStatus } from '../../api/types';

interface FiltersState {
  range: [Dayjs | null, Dayjs | null] | null;
  api_key_id: number | undefined;
  model: string;
  status: CallStatus | '';
  blocked: boolean | undefined;
  category: string;
}

export default function Calls() {
  const { t } = useTranslation();
  const statusOpts = useDictOptions('callStatus', CALL_STATUS_META);
  const statusLbl = useDictLabel('callStatus');
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [filters, setFilters] = useState<FiltersState>({
    range: null,
    api_key_id: undefined,
    model: '',
    status: '',
    blocked: undefined,
    category: '',
  });
  const [applied, setApplied] = useState<FiltersState>(filters);

  const [detail, setDetail] = useState<CallLogDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const params = useMemo(() => {
    const p: Record<string, unknown> = {
      page,
      page_size: pageSize,
      model: applied.model || undefined,
      status: applied.status || undefined,
      category: applied.category || undefined,
    };
    if (applied.range?.[0]) p.start = applied.range[0].toISOString();
    if (applied.range?.[1]) p.end = applied.range[1].toISOString();
    if (applied.api_key_id != null) p.api_key_id = applied.api_key_id;
    if (applied.blocked != null) p.blocked = applied.blocked;
    return p;
  }, [page, pageSize, applied]);

  const { data, isLoading } = useQuery({
    queryKey: ['call-logs', params],
    queryFn: () => callLogApi.list(params as never),
    placeholderData: keepPreviousData,
  });

  const { data: apikeysData } = useQuery({
    queryKey: ['apikeys-all'],
    queryFn: () => apikeyApi.list({ page: 1, page_size: 500 }),
  });

  const exportMutation = useMutation({
    mutationFn: () => callLogApi.exportCsv(params as never),
    onSuccess: () => message.success(t('calls.exportOk')),
    onError: () => undefined,
  });

  const showDetail = async (row: CallLogRow) => {
    setDetailLoading(true);
    setDetail(null);
    try {
      const d = await callLogApi.detail(row.request_id);
      setDetail(d);
    } catch {
      // 拦截器已提示
    } finally {
      setDetailLoading(false);
    }
  };

  return (
    <PageContainer
      title={t('calls.title')}
      description={t('calls.desc')}
      extra={
        <Button
          icon={<DownloadOutlined />}
          loading={exportMutation.isPending}
          onClick={() => exportMutation.mutate()}
        >
          {t('calls.export')}
        </Button>
      }
    >
      {/* 筛选区 */}
      <Space wrap style={{ marginBottom: 12 }} align="center">
        <DatePicker.RangePicker
          showTime
          value={filters.range}
          onChange={(v) => setFilters((f) => ({ ...f, range: v }))}
          placeholder={[t('calls.start'), t('calls.end')]}
        />
        <Select
          allowClear
          showSearch
          optionFilterProp="label"
          placeholder="API Key"
          style={{ width: 180 }}
          options={(apikeysData?.items ?? []).map((k) => ({
            value: k.id,
            label: k.name,
          }))}
          value={filters.api_key_id}
          onChange={(v: number) => setFilters((f) => ({ ...f, api_key_id: v }))}
        />
        <Input
          allowClear
          placeholder={t('calls.modelPh')}
          style={{ width: 140 }}
          value={filters.model}
          onChange={(e) => setFilters((f) => ({ ...f, model: e.target.value }))}
        />
        <Select
          allowClear
          placeholder={t('calls.statusPh')}
          style={{ width: 130 }}
          options={statusOpts}
          value={filters.status || undefined}
          onChange={(v: CallStatus) => setFilters((f) => ({ ...f, status: v ?? '' }))}
        />
        <Select
          allowClear
          placeholder={t('calls.blockedPh')}
          style={{ width: 120 }}
          options={[
            { value: true, label: t('calls.blockedYes') },
            { value: false, label: t('calls.blockedNo') },
          ]}
          value={filters.blocked}
          onChange={(v: boolean) => setFilters((f) => ({ ...f, blocked: v }))}
        />
        <Input
          allowClear
          placeholder={t('calls.categoryPh')}
          style={{ width: 200 }}
          value={filters.category}
          onChange={(e) => setFilters((f) => ({ ...f, category: e.target.value }))}
        />
        <Button
          type="primary"
          icon={<SearchOutlined />}
          onClick={() => {
            setPage(1);
            setApplied(filters);
          }}
        >
          {t('calls.search')}
        </Button>
        <Button
          onClick={() => {
            setPage(1);
            const empty: FiltersState = { range: null, api_key_id: undefined, model: '', status: '', blocked: undefined, category: '' };
            setFilters(empty);
            setApplied(empty);
          }}
        >
          {t('common.reset')}
        </Button>
      </Space>

      {/* 实时调用流（P2 #5：WebSocket 推送，暂停/清空） */}
      <div style={{ marginBottom: 12 }}>
        <LiveAuditStream />
      </div>

      <Table<CallLogRow>
        rowKey="request_id"
        loading={isLoading}
        dataSource={data?.items ?? []}
        scroll={{ x: 1250 }}
        columns={[
          { title: t('calls.colTime'), dataIndex: 'created_at', width: 165, render: (v: string) => fmtTime(v) },
          {
            title: t('calls.colReqId'),
            dataIndex: 'request_id',
            width: 100,
            ellipsis: true,
            render: (v: string) => <Typography.Text code copyable style={{ fontSize: 12 }}>{v.slice(0, 8)}</Typography.Text>,
          },
          { title: 'API Key', dataIndex: 'api_key_label', width: 110 },
          { title: t('calls.colModel'), dataIndex: 'model_alias', width: 110 },
          {
            title: t('calls.colUpstream'),
            dataIndex: 'upstream',
            width: 170,
            ellipsis: true,
            render: (v: string) => v || '-',
          },
          {
            title: t('calls.colInput'),
            dataIndex: 'input_preview',
            ellipsis: true,
            render: (v: string) => v || '-',
          },
          {
            title: t('calls.colOutput'),
            dataIndex: 'output_preview',
            ellipsis: true,
            render: (v: string) => v || '-',
          },
          {
            title: 'Tokens',
            key: 'tokens',
            width: 110,
            render: (_: unknown, r: CallLogRow) => `${fmtNumber(r.prompt_tokens)}/${fmtNumber(r.completion_tokens)}`,
          },
          { title: t('calls.colLatency'), dataIndex: 'latency_ms', width: 90, render: (v: number) => `${fmtNumber(v)} ms` },
          {
            title: t('common.status'),
            dataIndex: 'status',
            width: 100,
            render: (v: CallStatus) => (
              <Tag color={colorOf(CALL_STATUS_META, v)}>{statusLbl(v)}</Tag>
            ),
          },
          {
            title: t('calls.colBlocked'),
            key: 'blocked',
            width: 170,
            render: (_: unknown, r: CallLogRow) =>
              r.blocked ? (
                <Space direction="vertical" size={0}>
                  <Tag color="error">{t('calls.blockedYes')}</Tag>
                  <Typography.Text type="danger" style={{ fontSize: 11 }}>
                    {r.block_category || '-'}
                    {r.block_reason ? `: ${r.block_reason}` : ''}
                  </Typography.Text>
                </Space>
              ) : (
                <Tag>{t('calls.blockedNo')}</Tag>
              ),
          },
          {
            title: t('common.action'),
            key: 'action',
            width: 90,
            fixed: 'right',
            render: (_: unknown, r: CallLogRow) => (
              <Button size="small" type="link" onClick={() => void showDetail(r)}>
                {t('calls.detail')}
              </Button>
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

      {/* 详情 Drawer */}
      <Drawer
        title={t('calls.detailTitle', { id: detail?.request_id ?? '' })}
        open={Boolean(detail) || detailLoading}
        onClose={() => setDetail(null)}
        width={760}
        loading={detailLoading}
      >
        {detail ? (
          <>
            <Descriptions
              size="small"
              bordered
              column={2}
              items={[
                { key: 'time', label: t('calls.colTime'), children: fmtTime(detail.created_at) },
                { key: 'api', label: 'API Key', children: detail.api_key_label },
                { key: 'protocol', label: t('calls.protocol'), children: detail.protocol },
                { key: 'model', label: t('calls.modelPh'), children: detail.model_alias },
                { key: 'upstream', label: t('calls.colUpstream'), children: detail.upstream || '-' },
                { key: 'status', label: t('common.status'), children: <Tag color={colorOf(CALL_STATUS_META, detail.status)}>{statusLbl(detail.status)}</Tag> },
                {
                  key: 'tokens',
                  label: t('calls.tokenUsage'),
                  children: `prompt ${fmtNumber(detail.prompt_tokens)} / completion ${fmtNumber(detail.completion_tokens)}`,
                },
                { key: 'latency', label: t('calls.colLatency'), children: `${fmtNumber(detail.latency_ms)} ms` },
                {
                  key: 'block',
                  label: t('calls.colBlocked'),
                  children: detail.blocked ? (
                    <Typography.Text type="danger">
                      {detail.block_category || '-'} — {detail.block_reason || '-'}
                    </Typography.Text>
                  ) : (
                    t('calls.blockNone')
                  ),
                },
                { key: 'error', label: t('calls.errorLabel'), children: detail.error_msg || '-' },
              ]}
            />
            <Typography.Title level={5} style={{ marginTop: 20 }}>
              {t('calls.inputFull')}
            </Typography.Title>
            <Typography.Paragraph
              style={{
                padding: 10,
                background: '#fafafa',
                borderRadius: 6,
                maxHeight: 260,
                overflow: 'auto',
                whiteSpace: 'pre-wrap',
              }}
              copyable
            >
              {detail.input_text || t('calls.none')}
            </Typography.Paragraph>
            <Typography.Title level={5}>{t('calls.outputFull')}</Typography.Title>
            <Typography.Paragraph
              style={{
                padding: 10,
                background: '#fafafa',
                borderRadius: 6,
                maxHeight: 260,
                overflow: 'auto',
                whiteSpace: 'pre-wrap',
              }}
              copyable
            >
              {detail.output_text || t('calls.none')}
            </Typography.Paragraph>
            <Typography.Title level={5}>{t('calls.findings')}</Typography.Title>
            <JsonView value={detail.guard_findings} maxHeight={300} />
          </>
        ) : null}
      </Drawer>
    </PageContainer>
  );
}
