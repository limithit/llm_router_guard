// ===== 操作审计日志 (REQ-003) =====
// 记录所有配置变更操作：时间范围 / 操作人 / 模块 / 操作类型筛选 + before/after JSON diff 查看 + CSV 导出
import { useMemo, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
import {
  App,
  Button,
  Col,
  DatePicker,
  Input,
  Modal,
  Row,
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
import { opLogApi } from '../../api/endpoints';
import { OP_ACTION_MAP, OP_MODULE_MAP, OP_ACTION_OPTIONS, OP_MODULE_OPTIONS } from '../../constants/dicts';
import { fmtTime } from '../../utils/format';
import type { OpLogRow } from '../../api/types';

interface FiltersState {
  range: [Dayjs | null, Dayjs | null] | null;
  operator: string;
  module: string;
  action: string;
}

export default function Operations() {
  const { message } = App.useApp();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [filters, setFilters] = useState<FiltersState>({
    range: null,
    operator: '',
    module: '',
    action: '',
  });
  const [applied, setApplied] = useState<FiltersState>(filters);
  const [diffRow, setDiffRow] = useState<OpLogRow | null>(null);

  const params = useMemo(() => {
    const p: Record<string, unknown> = {
      page,
      page_size: pageSize,
      operator: applied.operator || undefined,
      module: applied.module || undefined,
      action: applied.action || undefined,
    };
    if (applied.range?.[0]) p.start = applied.range[0].toISOString();
    if (applied.range?.[1]) p.end = applied.range[1].toISOString();
    return p;
  }, [page, pageSize, applied]);

  const { data, isLoading } = useQuery({
    queryKey: ['op-logs', params],
    queryFn: () => opLogApi.list(params as never),
    placeholderData: keepPreviousData,
  });

  const exportMutation = useMutation({
    mutationFn: () => opLogApi.exportCsv(params as never),
    onSuccess: () => message.success('导出任务已触发，请查看浏览器下载'),
    onError: () => undefined,
  });

  return (
    <PageContainer
      title="操作审计"
      description="所有配置变更操作的全链路追溯（REQ-003）：操作人、类型、模块、变更前后对比、IP、生效状态。"
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
      <Space wrap style={{ marginBottom: 12 }} align="center">
        <DatePicker.RangePicker
          showTime
          value={filters.range}
          onChange={(v) => setFilters((f) => ({ ...f, range: v }))}
        />
        <Input
          allowClear
          placeholder="操作人"
          style={{ width: 150 }}
          value={filters.operator}
          onChange={(e) => setFilters((f) => ({ ...f, operator: e.target.value }))}
        />
        <Select
          allowClear
          placeholder="模块"
          style={{ width: 150 }}
          value={filters.module || undefined}
          onChange={(v) => setFilters((f) => ({ ...f, module: v ?? '' }))}
          options={OP_MODULE_OPTIONS.map((o) => ({ value: o.value, label: o.label }))}
        />
        <Select
          allowClear
          placeholder="操作类型"
          style={{ width: 150 }}
          value={filters.action || undefined}
          onChange={(v) => setFilters((f) => ({ ...f, action: v ?? '' }))}
          options={OP_ACTION_OPTIONS.map((o) => ({ value: o.value, label: o.label }))}
        />
        <Button
          type="primary"
          icon={<SearchOutlined />}
          onClick={() => {
            setApplied(filters);
            setPage(1);
          }}
        >
          查询
        </Button>
      </Space>

      <Table<OpLogRow>
        rowKey="id"
        loading={isLoading}
        dataSource={data?.items ?? []}
        scroll={{ x: 1000 }}
        columns={[
          { title: '时间', dataIndex: 'created_at', width: 170, render: fmtTime },
          { title: '操作人', dataIndex: 'operator', width: 110 },
          {
            title: '操作类型',
            dataIndex: 'action',
            width: 100,
            render: (v: string) => (
              <Tag color={v === 'delete' ? 'red' : v === 'create' ? 'green' : 'blue'}>
                {OP_ACTION_MAP.labels[v] ?? v}
              </Tag>
            ),
          },
          {
            title: '模块',
            dataIndex: 'module',
            width: 110,
            render: (v: string) => <Tag>{OP_MODULE_MAP.labels[v] ?? v}</Tag>,
          },
          { title: '对象', dataIndex: 'target', width: 160, ellipsis: true },
          { title: 'IP', dataIndex: 'ip', width: 130 },
          {
            title: '配置生效',
            dataIndex: 'effective',
            width: 90,
            render: (v: boolean) => (v ? <Tag color="green">已生效</Tag> : <Tag color="orange">待生效</Tag>),
          },
          {
            title: '操作',
            key: 'action',
            width: 100,
            fixed: 'right',
            render: (_: unknown, row: OpLogRow) => (
              <Button size="small" onClick={() => setDiffRow(row)}>
                变更对比
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
          setPageSize(p.pageSize ?? 20);
        }}
      />

      <Modal
        title={`变更对比 — ${diffRow ? `${OP_MODULE_MAP.labels[diffRow.module] ?? diffRow.module} / ${diffRow.target}` : ''}`}
        open={!!diffRow}
        onCancel={() => setDiffRow(null)}
        footer={null}
        width={960}
      >
        <Row gutter={16}>
          <Col span={12}>
            <Typography.Text type="secondary">变更前</Typography.Text>
            <JsonView value={diffRow?.before_json} maxHeight={420} />
          </Col>
          <Col span={12}>
            <Typography.Text type="secondary">变更后</Typography.Text>
            <JsonView value={diffRow?.after_json} maxHeight={420} />
          </Col>
        </Row>
        <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
          操作人：{diffRow?.operator} ｜ 时间：{fmtTime(diffRow?.created_at)} ｜ IP：{diffRow?.ip} ｜{' '}
          {diffRow?.effective ? '已生效' : '待生效'}
        </Typography.Paragraph>
      </Modal>
    </PageContainer>
  );
}
