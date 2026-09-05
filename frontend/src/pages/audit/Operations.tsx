// ===== 操作审计日志 (REQ-003) =====
// 记录所有配置变更操作：时间范围 / 操作人 / 模块 / 操作类型筛选 + before/after JSON diff 查看 + CSV 导出
import { useMemo, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
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
import { tableLoading } from '../../components/TableSkeleton';
import JsonView from '../../components/JsonView';
import { opLogApi } from '../../api/endpoints';
import { OP_ACTION_META, OP_MODULE_META, useDictLabel, useDictOptions } from '../../constants/dicts';
import { fmtTime } from '../../utils/format';
import type { OpLogRow } from '../../api/types';

interface FiltersState {
  range: [Dayjs | null, Dayjs | null] | null;
  operator: string;
  module: string;
  action: string;
}

export default function Operations() {
  const { t } = useTranslation();
  const opActionOpts = useDictOptions('opAction', OP_ACTION_META);
  const opModuleOpts = useDictOptions('opModule', OP_MODULE_META);
  const opActionLbl = useDictLabel('opAction');
  const opModuleLbl = useDictLabel('opModule');
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
    onSuccess: () => message.success(t('ops.exportOk')),
    onError: () => undefined,
  });

  return (
    <PageContainer
      title={t('ops.title')}
      description={t('ops.desc')}
      extra={
        <Button
          icon={<DownloadOutlined />}
          loading={exportMutation.isPending}
          onClick={() => exportMutation.mutate()}
        >
          {t('ops.export')}
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
          placeholder={t('ops.operatorPh')}
          style={{ width: 150 }}
          value={filters.operator}
          onChange={(e) => setFilters((f) => ({ ...f, operator: e.target.value }))}
        />
        <Select
          allowClear
          placeholder={t('ops.modulePh')}
          style={{ width: 150 }}
          value={filters.module || undefined}
          onChange={(v) => setFilters((f) => ({ ...f, module: v ?? '' }))}
          options={opModuleOpts}
        />
        <Select
          allowClear
          placeholder={t('ops.actionPh')}
          style={{ width: 150 }}
          value={filters.action || undefined}
          onChange={(v) => setFilters((f) => ({ ...f, action: v ?? '' }))}
          options={opActionOpts}
        />
        <Button
          type="primary"
          icon={<SearchOutlined />}
          onClick={() => {
            setApplied(filters);
            setPage(1);
          }}
        >
          {t('ops.search')}
        </Button>
      </Space>

      <Table<OpLogRow>
        rowKey="id"
        loading={tableLoading(isLoading, undefined, (data?.items?.length ?? 0) > 0)}
        dataSource={data?.items ?? []}
        scroll={{ x: 1000 }}
        columns={[
          { title: t('ops.colTime'), dataIndex: 'created_at', width: 170, render: fmtTime },
          { title: t('ops.colOperator'), dataIndex: 'operator', width: 110 },
          {
            title: t('ops.colAction'),
            dataIndex: 'action',
            width: 100,
            render: (v: string) => (
              <Tag color={v === 'delete' ? 'red' : v === 'create' ? 'green' : 'blue'}>
                {opActionLbl(v)}
              </Tag>
            ),
          },
          {
            title: t('ops.colModule'),
            dataIndex: 'module',
            width: 110,
            render: (v: string) => <Tag>{opModuleLbl(v)}</Tag>,
          },
          { title: t('ops.colTarget'), dataIndex: 'target', width: 160, ellipsis: true },
          { title: 'IP', dataIndex: 'ip', width: 130 },
          {
            title: t('ops.effective'),
            dataIndex: 'effective',
            width: 90,
            render: (v: boolean) => (v ? <Tag color="green">{t('ops.effectiveYes')}</Tag> : <Tag color="orange">{t('ops.effectiveNo')}</Tag>),
          },
          {
            title: t('common.action'),
            key: 'action',
            width: 100,
            fixed: 'right',
            render: (_: unknown, row: OpLogRow) => (
              <Button size="small" onClick={() => setDiffRow(row)}>
                {t('ops.diff')}
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
          setPageSize(p.pageSize ?? 20);
        }}
      />

      <Modal
        title={t('ops.diffTitle', { detail: diffRow ? `${opModuleLbl(diffRow.module)} / ${diffRow.target}` : '' })}
        open={!!diffRow}
        onCancel={() => setDiffRow(null)}
        footer={null}
        width={960}
      >
        <Row gutter={16}>
          <Col span={12}>
            <Typography.Text type="secondary">{t('ops.before')}</Typography.Text>
            <JsonView value={diffRow?.before_json} maxHeight={420} />
          </Col>
          <Col span={12}>
            <Typography.Text type="secondary">{t('ops.after')}</Typography.Text>
            <JsonView value={diffRow?.after_json} maxHeight={420} />
          </Col>
        </Row>
        <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
          {t('ops.meta', {
            operator: diffRow?.operator ?? '-',
            time: fmtTime(diffRow?.created_at),
            ip: diffRow?.ip ?? '-',
            state: diffRow?.effective ? t('ops.effectiveYes') : t('ops.effectiveNo'),
          })}
        </Typography.Paragraph>
      </Modal>
    </PageContainer>
  );
}
