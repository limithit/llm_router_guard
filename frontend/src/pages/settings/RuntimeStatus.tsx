// ===== 状态监控 (REQ-018) =====
// 服务运行状态 / 内存 CPU / 连接数 / 上游健康 / 模型 QPS / 最近错误 / 配置加载状态（5 秒轮询）
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import {
  Badge,
  Card,
  Col,
  Empty,
  List,
  Progress,
  Row,
  Statistic,
  Table,
  Tag,
  Typography,
} from 'antd';
import {
  ApiOutlined,
  ClockCircleOutlined,
  DashboardOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import PageContainer from '../../components/PageContainer';
import { runtimeStatusApi } from '../../api/endpoints';
import { fmtDuration, fmtNumber, fmtTime } from '../../utils/format';
import type { QpsByModel, RecentError, UpstreamRuntime } from '../../api/types';

export default function RuntimeStatus() {
  const { t } = useTranslation();
  const { data, isLoading } = useQuery({
    queryKey: ['runtime-status'],
    queryFn: () => runtimeStatusApi.get(),
    refetchInterval: 5000,
  });

  const memMb = data ? Math.round(data.memory_alloc_kb / 1024) : 0;

  return (
    <PageContainer
      title={t('rt.title')}
      description={t('rt.desc')}
      extra={
        <Badge
          status={data?.running ? 'processing' : 'error'}
          text={data?.running ? t('rt.running') : t('rt.error')}
        />
      }
    >
      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}>
          <Card size="small" loading={isLoading && !data}>
            <Statistic
              title={t('rt.uptime')}
              value={fmtDuration(data?.uptime_seconds)}
              prefix={<ClockCircleOutlined />}
            />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card size="small" loading={isLoading && !data}>
            <Statistic title={t('rt.memory')} value={memMb} suffix="MB" prefix={<DashboardOutlined />} />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card size="small" loading={isLoading && !data}>
            <Statistic
              title={t('rt.connections')}
              value={fmtNumber(data?.open_connections)}
              prefix={<ApiOutlined />}
            />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card size="small" loading={isLoading && !data}>
            <Statistic
              title="Goroutines"
              value={fmtNumber(data?.goroutines)}
              prefix={<ThunderboltOutlined />}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={12}>
          <Card size="small" title={t('rt.upstreamCard')}>
            <Table<UpstreamRuntime>
              rowKey="provider"
              size="small"
              pagination={false}
              dataSource={data?.upstreams ?? []}
              columns={[
                { title: t('rt.colProvider'), dataIndex: 'provider' },
                {
                  title: t('rt.colHealth'),
                  dataIndex: 'healthy',
                  width: 90,
                  render: (v: boolean) =>
                    v ? <Tag color="green">{t('rt.healthy')}</Tag> : <Tag color="red">{t('rt.breaking')}</Tag>,
                },
                { title: t('rt.colFailCount'), dataIndex: 'fail_count', width: 90 },
                { title: t('rt.colLastError'), dataIndex: 'last_error', ellipsis: true, render: (v: string) => v || '-' },
              ]}
            />
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card size="small" title={t('rt.qpsCard')}>
            <Table<QpsByModel>
              rowKey="model"
              size="small"
              pagination={false}
              dataSource={data?.qps_by_model ?? []}
              columns={[
                { title: t('rt.colModel'), dataIndex: 'model' },
                {
                  title: 'QPS',
                  dataIndex: 'qps',
                  width: 100,
                  render: (v: number) => <Tag color="blue">{v.toFixed(2)}</Tag>,
                },
                { title: t('rt.colErrors1m'), dataIndex: 'errors_1m', width: 130 },
              ]}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={12}>
          <Card size="small" title={t('rt.errorsCard')}>
            {data?.recent_errors?.length ? (
              <List<RecentError>
                size="small"
                dataSource={data.recent_errors}
                renderItem={(item) => (
                  <List.Item>
                    <Typography.Text type="secondary" style={{ marginRight: 8 }}>
                      {fmtTime(item.time)}
                    </Typography.Text>
                    <Typography.Text code style={{ marginRight: 8 }}>
                      {item.request_id.slice(0, 8)}
                    </Typography.Text>
                    <Typography.Text type="danger" ellipsis>
                      {item.message}
                    </Typography.Text>
                  </List.Item>
                )}
              />
            ) : (
              <Empty description={t('rt.noErrors')} />
            )}
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card size="small" title={t('rt.sysCard')}>
            <Row gutter={16}>
              <Col span={12}>
                <Statistic title={t('rt.version')} value={data?.version ?? '-'} />
                <Typography.Text type="secondary">
                  {t('rt.configVersion', {
                    v: data?.config_status?.version ?? '-',
                    time: fmtTime(data?.config_status?.last_loaded_at),
                  })}
                </Typography.Text>
                <div style={{ marginTop: 8 }}>
                  {data?.config_status?.status === 'ok' ? (
                    <Tag color="green">{t('rt.cfgOk')}</Tag>
                  ) : (
                    <Tag color="red">{t('rt.cfgErr')}</Tag>
                  )}
                </div>
              </Col>
              <Col span={12}>
                <Typography.Text type="secondary">{t('rt.cpu')}</Typography.Text>
                <Progress
                  percent={Math.min(100, Math.round((data?.cpu_percent ?? 0) * 10) / 10)}
                  steps={10}
                  size="small"
                  format={() => `${(data?.cpu_percent ?? 0).toFixed(1)}%`}
                />
              </Col>
            </Row>
          </Card>
        </Col>
      </Row>
    </PageContainer>
  );
}
