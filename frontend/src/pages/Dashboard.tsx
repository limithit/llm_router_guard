// ===== 首页 / Dashboard (REQ-016) =====
// 今日请求指标卡片 + 近 7 天趋势图 + 模型排行 + 拦截类别分布 + 上游健康 + 配额使用率 + 系统状态
import { useQuery } from '@tanstack/react-query';
import { Alert, Card, Col, Progress, Row, Statistic, Table, Tag, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { dashboardApi } from '../api/endpoints';
import { DonutChart, LineChart } from '../components/Charts';
import { fmtDuration, fmtNumber } from '../utils/format';
import { KEYWORD_CATEGORY_META, PROTOCOL_META, useDictLabel } from '../constants/dicts';
import { useNavigate } from 'react-router-dom';
import type { UpstreamHealth } from '../api/types';

export default function Dashboard() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const catLbl = useDictLabel('keywordCategory');
  const protoLbl = useDictLabel('protocol');
  const { data, isLoading } = useQuery({
    queryKey: ['dashboard'],
    queryFn: dashboardApi.get,
    refetchInterval: 30_000,
  });

  const today = data?.today;
  const system = data?.system;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {/* 关键指标卡片 */}
      <Row gutter={[12, 12]}>
        <Col xs={12} sm={8} md={4}>
          <Card size="small">
            <Statistic title={t('dashboard.totalCalls')} value={fmtNumber(today?.calls)} loading={isLoading} />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card size="small">
            <Statistic
              title={t('dashboard.successCount')}
              value={fmtNumber(today?.success)}
              valueStyle={{ color: '#52c41a' }}
              loading={isLoading}
            />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card size="small">
            <Statistic
              title={t('dashboard.blockedCount')}
              value={fmtNumber(today?.blocked)}
              valueStyle={{ color: '#ff4d4f' }}
              loading={isLoading}
            />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card size="small">
            <Statistic
              title={t('dashboard.errorCount')}
              value={fmtNumber(today?.errors)}
              valueStyle={{ color: '#faad14' }}
              loading={isLoading}
            />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card size="small">
            <Statistic
              title={t('dashboard.avgLatency')}
              value={today?.avg_latency_ms}
              suffix="ms"
              loading={isLoading}
            />
          </Card>
        </Col>
      </Row>

      {/* 趋势图 + 拦截类别分布 */}
      <Row gutter={[12, 12]}>
        <Col xs={24} lg={15}>
          <Card
            size="small"
            title={t('dashboard.trend7d')}
            loading={isLoading}
          >
            <LineChart
              data={(data?.trend_7d ?? []).map((d) => ({
                label: d.date.slice(5),
                value: d.calls,
                value2: d.blocked,
              }))}
              seriesName={[t('dashboard.trendCalls'), t('dashboard.trendBlocked')]}
            />
          </Card>
        </Col>
        <Col xs={24} lg={9}>
          <Card size="small" title={t('dashboard.blockCategories')} loading={isLoading}>
            <DonutChart
              data={(data?.block_categories ?? []).map((c) => ({
                label: catLbl(c.category),
                value: c.count,
              }))}
              centerText={t('dashboard.blockTotal')}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[12, 12]}>
        {/* 模型调用量排行 */}
        <Col xs={24} lg={8}>
          <Card size="small" title={t('dashboard.topModels')} loading={isLoading} styles={{ body: { paddingTop: 8 } }}>
            {(data?.top_models ?? []).length === 0 ? (
              <Typography.Text type="secondary">{t('dashboard.noData')}</Typography.Text>
            ) : (
              (data?.top_models ?? []).map((m, i) => {
                const max = Math.max(1, ...(data?.top_models ?? []).map((x) => x.calls));
                return (
                  <div key={m.model} style={{ marginBottom: 8 }}>
                    <div
                      style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        fontSize: 12,
                        marginBottom: 2,
                      }}
                    >
                      <span>
                        {i + 1}. {m.model}
                      </span>
                      <span>{fmtNumber(m.calls)}</span>
                    </div>
                    <Progress
                      percent={Math.round((m.calls / max) * 100)}
                      size="small"
                      showInfo={false}
                      strokeColor={i === 0 ? '#1677ff' : undefined}
                    />
                  </div>
                );
              })
            )}
          </Card>
        </Col>

        {/* 上游健康状态 */}
        <Col xs={24} lg={8}>
          <Card size="small" title={t('dashboard.upstreamHealth')} loading={isLoading}>
            <Table<UpstreamHealth>
              rowKey="provider"
              size="small"
              pagination={false}
              dataSource={data?.upstream_health ?? []}
              locale={{ emptyText: t('dashboard.noUpstreams') }}
              columns={[
                { title: t('dashboard.colProvider'), dataIndex: 'provider' },
                {
                  title: t('dashboard.colProtocol'),
                  dataIndex: 'protocol',
                  width: 150,
                  render: (v: string) => protoLbl(v),
                },
                {
                  title: t('dashboard.colStatus'),
                  dataIndex: 'healthy',
                  width: 90,
                  render: (v: boolean) =>
                    v ? <Tag color="success">{t('common.enabled')}</Tag> : <Tag color="error">{t('dashboard.abnormal')}</Tag>,
                },
                {
                  title: t('dashboard.colFailCount'),
                  dataIndex: 'fail_count',
                  width: 90,
                  render: (v: number) => v || '-',
                },
              ]}
            />
          </Card>
        </Col>

        {/* 配额使用率总览 */}
        <Col xs={24} lg={8}>
          <Card size="small" title={t('dashboard.quotaUsage')} loading={isLoading} styles={{ body: { paddingTop: 8 } }}>
            {(data?.quota_usage ?? []).length === 0 ? (
              <Typography.Text type="secondary">{t('dashboard.noQuotaData')}</Typography.Text>
            ) : (
              (data?.quota_usage ?? []).map((q) => {
                const pct = q.limit ? Math.min(200, Math.round((q.used / q.limit) * 100)) : 0;
                const over = q.used >= q.limit;
                return (
                  <div key={q.name} style={{ marginBottom: 8 }}>
                    <div
                      style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        fontSize: 12,
                        marginBottom: 2,
                      }}
                    >
                      <Typography.Text ellipsis style={{ maxWidth: 220 }} title={q.name}>
                        {q.name}
                      </Typography.Text>
                      <span style={{ color: over ? '#ff4d4f' : undefined }}>
                        {fmtNumber(q.used)}/{fmtNumber(q.limit)}
                        {over ? t('dashboard.overused') : ''}
                      </span>
                    </div>
                    <Progress
                      percent={pct}
                      size="small"
                      status={pct >= 90 || over ? 'exception' : 'active'}
                    />
                  </div>
                );
              })
            )}
          </Card>
        </Col>
      </Row>

      {/* 系统运行状态 */}
      <Card size="small" title={t('dashboard.systemStatus')} loading={isLoading}>
        {data && (
          <Row gutter={[16, 8]}>
            <Col span={12} md={4}>
              <Typography.Text type="secondary">{t('dashboard.version')}</Typography.Text>
              <Tag color="blue">{system?.version}</Tag>
            </Col>
            <Col span={12} md={4}>
              <Typography.Text type="secondary">{t('dashboard.uptime')}</Typography.Text>
              {fmtDuration(system?.uptime_seconds)}
            </Col>
            <Col span={12} md={4}>
              <Typography.Text type="secondary">{t('dashboard.configLoad')}</Typography.Text>
              {system?.config_status === 'ok' ? (
                <Tag color="success">{t('dashboard.configOk')}</Tag>
              ) : (
                <Tag color="error">{system?.config_status ?? t('common.unknown')}</Tag>
              )}
            </Col>
            <Col span={12} md={4}>
              <Typography.Text type="secondary">{t('dashboard.goVersion')}</Typography.Text>
              {system?.go_version ?? '-'}
            </Col>
            {today && today.blocked > 0 ? (
              <Col span={24} md={8}>
                <Alert
                  type={today.blocked > today.errors ? 'warning' : 'info'}
                  showIcon
                  message={t('dashboard.todayIntercepted', { blocked: fmtNumber(today.blocked), errors: fmtNumber(today.errors) })}
                  action={
                    <Typography.Link onClick={() => navigate('/audit/calls')}>
                      {t('dashboard.viewLogs')}
                    </Typography.Link>
                  }
                />
              </Col>
            ) : null}
          </Row>
        )}
      </Card>
    </div>
  );
}
