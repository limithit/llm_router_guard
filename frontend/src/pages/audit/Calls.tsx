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
import dayjs from 'dayjs';
import PageContainer from '../../components/PageContainer';
import JsonView from '../../components/JsonView';
import { apikeyApi, callLogApi } from '../../api/endpoints';
import { CALL_STATUS_MAP, CALL_STATUS_OPTIONS, colorOf, labelOf } from '../../constants/dicts';
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
    onSuccess: () => message.success('导出任务已触发，请查看浏览器下载'),
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
      title="调用日志"
      description="全链路请求/响应审计日志查询（REQ-015）。"
      extra={
        <Button
          icon={<DownloadOutlined />}
          loading={exportMutation.isPending}
          onClick={() => exportMutation.mutate()}
        >
          导出 CSV
        </Button>
      }
    >
      {/* 筛选区 */}
      <Space wrap style={{ marginBottom: 12 }} align="center">
        <DatePicker.RangePicker
          showTime
          value={filters.range}
          onChange={(v) => setFilters((f) => ({ ...f, range: v }))}
          placeholder={['开始时间', '结束时间']}
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
          placeholder="模型别名"
          style={{ width: 140 }}
          value={filters.model}
          onChange={(e) => setFilters((f) => ({ ...f, model: e.target.value }))}
        />
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 130 }}
          options={CALL_STATUS_OPTIONS}
          value={filters.status || undefined}
          onChange={(v: CallStatus) => setFilters((f) => ({ ...f, status: v ?? '' }))}
        />
        <Select
          allowClear
          placeholder="是否拦截"
          style={{ width: 120 }}
          options={[
            { value: true, label: '已拦截' },
            { value: false, label: '未拦截' },
          ]}
          value={filters.blocked}
          onChange={(v: boolean) => setFilters((f) => ({ ...f, blocked: v }))}
        />
        <Input
          allowClear
          placeholder="拦截类别，如 keyword.political"
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
          查询
        </Button>
        <Button
          onClick={() => {
            setPage(1);
            const empty: FiltersState = { range: null, api_key_id: undefined, model: '', status: '', blocked: undefined, category: '' };
            setFilters(empty);
            setApplied(empty);
          }}
        >
          重置
        </Button>
      </Space>

      <Table<CallLogRow>
        rowKey="request_id"
        loading={isLoading}
        dataSource={data?.items ?? []}
        scroll={{ x: 1250 }}
        columns={[
          { title: '时间', dataIndex: 'created_at', width: 165, render: (v: string) => fmtTime(v) },
          {
            title: '请求 ID',
            dataIndex: 'request_id',
            width: 100,
            ellipsis: true,
            render: (v: string) => <Typography.Text code copyable style={{ fontSize: 12 }}>{v.slice(0, 8)}</Typography.Text>,
          },
          { title: 'API Key', dataIndex: 'api_key_label', width: 110 },
          { title: '模型', dataIndex: 'model_alias', width: 110 },
          {
            title: '实际上游',
            dataIndex: 'upstream',
            width: 170,
            ellipsis: true,
            render: (v: string) => v || '-',
          },
          {
            title: '输入预览',
            dataIndex: 'input_preview',
            ellipsis: true,
            render: (v: string) => v || '-',
          },
          {
            title: '输出预览',
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
          { title: '延迟', dataIndex: 'latency_ms', width: 90, render: (v: number) => `${fmtNumber(v)} ms` },
          {
            title: '状态',
            dataIndex: 'status',
            width: 100,
            render: (v: CallStatus) => (
              <Tag color={colorOf(CALL_STATUS_MAP.colors, v)}>{labelOf(CALL_STATUS_MAP.labels, v)}</Tag>
            ),
          },
          {
            title: '拦截',
            key: 'blocked',
            width: 170,
            render: (_: unknown, r: CallLogRow) =>
              r.blocked ? (
                <Space direction="vertical" size={0}>
                  <Tag color="error">已拦截</Tag>
                  <Typography.Text type="danger" style={{ fontSize: 11 }}>
                    {r.block_category || '-'}
                    {r.block_reason ? `：${r.block_reason}` : ''}
                  </Typography.Text>
                </Space>
              ) : (
                <Tag>未拦截</Tag>
              ),
          },
          {
            title: '操作',
            key: 'action',
            width: 90,
            fixed: 'right',
            render: (_: unknown, r: CallLogRow) => (
              <Button size="small" type="link" onClick={() => void showDetail(r)}>
                详情
              </Button>
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

      {/* 详情 Drawer */}
      <Drawer
        title={`调用详情：${detail?.request_id ?? ''}`}
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
                { key: 'time', label: '时间', children: fmtTime(detail.created_at) },
                { key: 'api', label: 'API Key', children: detail.api_key_label },
                { key: 'protocol', label: '协议', children: detail.protocol },
                { key: 'model', label: '模型别名', children: detail.model_alias },
                { key: 'upstream', label: '实际上游', children: detail.upstream || '-' },
                { key: 'status', label: '状态', children: <Tag color={colorOf(CALL_STATUS_MAP.colors, detail.status)}>{labelOf(CALL_STATUS_MAP.labels, detail.status)}</Tag> },
                {
                  key: 'tokens',
                  label: 'Token 用量',
                  children: `prompt ${fmtNumber(detail.prompt_tokens)} / completion ${fmtNumber(detail.completion_tokens)}`,
                },
                { key: 'latency', label: '延迟', children: `${fmtNumber(detail.latency_ms)} ms` },
                {
                  key: 'block',
                  label: '拦截',
                  children: detail.blocked ? (
                    <Typography.Text type="danger">
                      {detail.block_category || '-'} — {detail.block_reason || '-'}
                    </Typography.Text>
                  ) : (
                    '否'
                  ),
                },
                { key: 'error', label: '错误信息', children: detail.error_msg || '-' },
              ]}
            />
            <Typography.Title level={5} style={{ marginTop: 20 }}>
              输入全文
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
              {detail.input_text || '(无)'}
            </Typography.Paragraph>
            <Typography.Title level={5}>输出全文</Typography.Title>
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
              {detail.output_text || '(无)'}
            </Typography.Paragraph>
            <Typography.Title level={5}>护栏命中（guard_findings）</Typography.Title>
            <JsonView value={detail.guard_findings} maxHeight={300} />
          </>
        ) : null}
      </Drawer>
    </PageContainer>
  );
}
