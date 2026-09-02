// ===== 状态监控 (REQ-018) =====
// 服务运行状态 / 内存 CPU / 连接数 / 上游健康 / 模型 QPS / 最近错误 / 配置加载状态（5 秒轮询）
import { useQuery } from '@tanstack/react-query';
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
  const { data, isLoading } = useQuery({
    queryKey: ['runtime-status'],
    queryFn: () => runtimeStatusApi.get(),
    refetchInterval: 5000,
  });

  const memMb = data ? Math.round(data.memory_alloc_kb / 1024) : 0;

  return (
    <PageContainer
      title="状态监控"
      description="网关运行状态与性能指标（REQ-018），每 5 秒自动刷新。"
      extra={
        <Badge
          status={data?.running ? 'processing' : 'error'}
          text={data?.running ? '服务运行中' : '服务异常'}
        />
      }
    >
      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}>
          <Card size="small" loading={isLoading && !data}>
            <Statistic
              title="运行时长"
              value={fmtDuration(data?.uptime_seconds)}
              prefix={<ClockCircleOutlined />}
            />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card size="small" loading={isLoading && !data}>
            <Statistic title="内存占用" value={memMb} suffix="MB" prefix={<DashboardOutlined />} />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card size="small" loading={isLoading && !data}>
            <Statistic
              title="当前连接数"
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
          <Card size="small" title="上游健康状态">
            <Table<UpstreamRuntime>
              rowKey="provider"
              size="small"
              pagination={false}
              dataSource={data?.upstreams ?? []}
              columns={[
                { title: '供应商', dataIndex: 'provider' },
                {
                  title: '健康',
                  dataIndex: 'healthy',
                  width: 90,
                  render: (v: boolean) =>
                    v ? <Tag color="green">健康</Tag> : <Tag color="red">熔断中</Tag>,
                },
                { title: '连续失败', dataIndex: 'fail_count', width: 90 },
                { title: '最近错误', dataIndex: 'last_error', ellipsis: true, render: (v: string) => v || '-' },
              ]}
            />
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card size="small" title="模型 QPS（近 60 秒）">
            <Table<QpsByModel>
              rowKey="model"
              size="small"
              pagination={false}
              dataSource={data?.qps_by_model ?? []}
              columns={[
                { title: '模型别名', dataIndex: 'model' },
                {
                  title: 'QPS',
                  dataIndex: 'qps',
                  width: 100,
                  render: (v: number) => <Tag color="blue">{v.toFixed(2)}</Tag>,
                },
                { title: '近 1 分钟错误', dataIndex: 'errors_1m', width: 130 },
              ]}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={12}>
          <Card size="small" title="最近错误日志">
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
              <Empty description="暂无错误" />
            )}
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card size="small" title="系统与配置状态">
            <Row gutter={16}>
              <Col span={12}>
                <Statistic title="版本" value={data?.version ?? '-'} />
                <Typography.Text type="secondary">
                  配置版本：{data?.config_status?.version ?? '-'}（
                  {fmtTime(data?.config_status?.last_loaded_at)}）
                </Typography.Text>
                <div style={{ marginTop: 8 }}>
                  {data?.config_status?.status === 'ok' ? (
                    <Tag color="green">配置正常</Tag>
                  ) : (
                    <Tag color="red">配置异常</Tag>
                  )}
                </div>
              </Col>
              <Col span={12}>
                <Typography.Text type="secondary">CPU 占用（进程估算）</Typography.Text>
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
