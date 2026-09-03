// ===== 配置状态 (REQ-004A) =====
// 热加载状态可视化：当前版本 / 最后加载时间 / 状态 / 已生效模块计数 / 最近加载日志 Timeline / 版本历史
// 操作：手动重新加载 / 导出当前配置 / 回滚到上一版本（5 秒轮询刷新）
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  App,
  Button,
  Card,
  Col,
  Descriptions,
  Empty,
  Modal,
  Popconfirm,
  Row,
  Space,
  Table,
  Tag,
  Timeline,
  Typography,
} from 'antd';
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  DownloadOutlined,
  ReloadOutlined,
  RollbackOutlined,
} from '@ant-design/icons';
import PageContainer from '../../components/PageContainer';
import { configStatusApi } from '../../api/endpoints';
import { fmtTime, fmtTimeShort } from '../../utils/format';

export default function ConfigStatus() {
  const { t } = useTranslation();
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [historyOpen, setHistoryOpen] = useState(false);

  // 模块名展示（i18n）
  const moduleLabel = (k: string) => t(`cfg.mod.${k}`) !== `cfg.mod.${k}` ? t(`cfg.mod.${k}`) : k;

  const { data, isLoading } = useQuery({
    queryKey: ['config-status'],
    queryFn: () => configStatusApi.get(),
    refetchInterval: 5000,
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['config-status'] });

  const reloadMutation = useMutation({
    mutationFn: () => configStatusApi.reload(),
    onSuccess: () => {
      message.success(t('cfg.reloadOk'));
      setTimeout(invalidate, 500);
    },
    onError: () => undefined,
  });

  const rollbackMutation = useMutation({
    mutationFn: () => configStatusApi.rollback(),
    onSuccess: (res) => {
      Modal.success({
        title: t('cfg.rollbackOk'),
        content: t('cfg.rollbackOkContent', { v: res.rolled_back_to }),
      });
      invalidate();
    },
    onError: () => undefined,
  });

  const exportMutation = useMutation({
    mutationFn: () => configStatusApi.exportJson(),
    onSuccess: () => message.success(t('cfg.exportOk')),
    onError: () => undefined,
  });

  const statusOk = data?.status === 'ok';

  return (
    <PageContainer
      title={t('cfg.title')}
      description={t('cfg.desc')}
      extra={
        <Space>
          <Button
            icon={<ReloadOutlined />}
            loading={reloadMutation.isPending}
            onClick={() => reloadMutation.mutate()}
          >
            {t('cfg.reload')}
          </Button>
          <Button
            icon={<DownloadOutlined />}
            loading={exportMutation.isPending}
            onClick={() => exportMutation.mutate()}
          >
            {t('cfg.export')}
          </Button>
          <Popconfirm
            title={t('cfg.rollbackConfirmTitle')}
            description={t('cfg.rollbackConfirmDesc')}
            okButtonProps={{ danger: true }}
            onConfirm={() => rollbackMutation.mutate()}
          >
            <Button danger icon={<RollbackOutlined />} loading={rollbackMutation.isPending}>
              {t('cfg.rollback')}
            </Button>
          </Popconfirm>
        </Space>
      }
    >
      <Row gutter={16}>
        <Col xs={24} lg={14}>
          <Card size="small" title={t('cfg.loadCard')} loading={isLoading && !data}>
            <Descriptions column={1} size="small">
              <Descriptions.Item label={t('cfg.version')}>
                <Typography.Text code>{data?.version ?? '-'}</Typography.Text>
              </Descriptions.Item>
              <Descriptions.Item label={t('cfg.lastLoaded')}>{fmtTime(data?.last_loaded_at)}</Descriptions.Item>
              <Descriptions.Item label={t('cfg.loadStatus')}>
                {statusOk ? (
                  <Tag icon={<CheckCircleOutlined />} color="success">
                    {t('cfg.ok')}
                  </Tag>
                ) : (
                  <Tag icon={<CloseCircleOutlined />} color="error">
                    {t('cfg.err')}
                  </Tag>
                )}
              </Descriptions.Item>
              <Descriptions.Item label={t('cfg.modules')}>
                <Space wrap size={4}>
                  {Object.entries(data?.modules ?? {}).map(([k, v]) => (
                    <Tag key={k} color="blue">
                      {moduleLabel(k)}({v})
                    </Tag>
                  ))}
                </Space>
              </Descriptions.Item>
            </Descriptions>
            {!statusOk && data?.error_message ? (
              <Alert
                type="error"
                showIcon
                style={{ marginTop: 8 }}
                message={t('cfg.loadErr')}
                description={
                  <>
                    {data.error_message}
                    <br />
                    <Typography.Text type="secondary">
                      {t('cfg.loadErrHint')}
                    </Typography.Text>
                  </>
                }
              />
            ) : null}
          </Card>

          <Card
            size="small"
            title={t('cfg.logsCard')}
            style={{ marginTop: 16 }}
            extra={
              <Button size="small" type="link" onClick={() => setHistoryOpen(true)}>
                {t('cfg.history')}
              </Button>
            }
          >
            {data?.logs?.length ? (
              <Timeline
                style={{ marginTop: 8 }}
                items={data.logs.map((l) => ({
                  color: l.status === 'success' ? 'green' : 'red',
                  children: (
                    <span>
                      <Typography.Text type="secondary">{fmtTimeShort(l.time)}</Typography.Text>{' '}
                      <Tag color={l.status === 'success' ? 'green' : 'red'}>
                        {l.status === 'success' ? t('cfg.logSuccess') : t('cfg.logFail')}
                      </Tag>
                      [{moduleLabel(l.module)}] {l.message}
                    </span>
                  ),
                }))}
              />
            ) : (
              <Empty description={t('cfg.noLogs')} />
            )}
          </Card>
        </Col>

        <Col xs={24} lg={10}>
          <Card size="small" title={t('cfg.historyCard')}>
            <Table
              rowKey="version"
              size="small"
              pagination={false}
              scroll={{ y: 480 }}
              dataSource={data?.history ?? []}
              columns={[
                { title: t('cfg.colVersion'), dataIndex: 'version', render: (v: string) => <Typography.Text code>{v}</Typography.Text> },
                { title: t('cfg.colTime'), dataIndex: 'created_at', width: 110, render: (v: string) => fmtTimeShort(v) },
                {
                  title: t('common.status'),
                  dataIndex: 'status',
                  width: 70,
                  render: (v: string) => (
                    <Tag color={v === 'ok' ? 'green' : 'red'}>{v === 'ok' ? t('cfg.ok') : t('cfg.err')}</Tag>
                  ),
                },
              ]}
            />
          </Card>
        </Col>
      </Row>

      <Modal
        title={t('cfg.historyTitle')}
        open={historyOpen}
        onCancel={() => setHistoryOpen(false)}
        footer={null}
        width={720}
      >
        <Table
          rowKey="version"
          size="small"
          dataSource={data?.history ?? []}
          pagination={{ pageSize: 10 }}
          columns={[
            { title: t('cfg.colVersion'), dataIndex: 'version', width: 180 },
            { title: t('cfg.colTime'), dataIndex: 'created_at', width: 170, render: fmtTime },
            { title: t('common.status'), dataIndex: 'status', width: 80 },
            { title: t('cfg.colMessage'), dataIndex: 'message', ellipsis: true },
          ]}
        />
      </Modal>
    </PageContainer>
  );
}
