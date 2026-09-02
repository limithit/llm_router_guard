// ===== 配置状态 (REQ-004A) =====
// 热加载状态可视化：当前版本 / 最后加载时间 / 状态 / 已生效模块计数 / 最近加载日志 Timeline / 版本历史
// 操作：手动重新加载 / 导出当前配置 / 回滚到上一版本（5 秒轮询刷新）
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
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

const MODULE_LABELS: Record<string, string> = {
  providers: '供应商',
  model_aliases: '模型别名',
  keywords: '敏感词',
  pii_rules: 'PII规则',
  injection_rules: '注入规则',
  quotas: '配额',
  rate_limits: '限流规则',
  apikeys: 'API Key',
};

export default function ConfigStatus() {
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [historyOpen, setHistoryOpen] = useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ['config-status'],
    queryFn: () => configStatusApi.get(),
    refetchInterval: 5000,
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['config-status'] });

  const reloadMutation = useMutation({
    mutationFn: () => configStatusApi.reload(),
    onSuccess: () => {
      message.success('已触发配置重载');
      setTimeout(invalidate, 500);
    },
    onError: () => undefined,
  });

  const rollbackMutation = useMutation({
    mutationFn: () => configStatusApi.rollback(),
    onSuccess: (res) => {
      Modal.success({
        title: '回滚成功',
        content: `已回滚到版本 ${res.rolled_back_to}，配置已重新加载。`,
      });
      invalidate();
    },
    onError: () => undefined,
  });

  const exportMutation = useMutation({
    mutationFn: () => configStatusApi.exportJson(),
    onSuccess: () => message.success('配置导出已开始下载'),
    onError: () => undefined,
  });

  const statusOk = data?.status === 'ok';

  return (
    <PageContainer
      title="配置状态"
      description="配置热加载状态可视化（REQ-004A）。所有配置变更后 ≤3 秒自动生效，无需重启服务。"
      extra={
        <Space>
          <Button
            icon={<ReloadOutlined />}
            loading={reloadMutation.isPending}
            onClick={() => reloadMutation.mutate()}
          >
            手动重新加载
          </Button>
          <Button
            icon={<DownloadOutlined />}
            loading={exportMutation.isPending}
            onClick={() => exportMutation.mutate()}
          >
            导出当前配置
          </Button>
          <Popconfirm
            title="回滚到上一版本配置"
            description="将恢复上一版本的供应商/模型/护栏/配额等全部配置，确认继续？"
            okButtonProps={{ danger: true }}
            onConfirm={() => rollbackMutation.mutate()}
          >
            <Button danger icon={<RollbackOutlined />} loading={rollbackMutation.isPending}>
              回滚上一版本
            </Button>
          </Popconfirm>
        </Space>
      }
    >
      <Row gutter={16}>
        <Col xs={24} lg={14}>
          <Card size="small" title="加载状态" loading={isLoading && !data}>
            <Descriptions column={1} size="small">
              <Descriptions.Item label="当前配置版本">
                <Typography.Text code>{data?.version ?? '-'}</Typography.Text>
              </Descriptions.Item>
              <Descriptions.Item label="最后加载时间">{fmtTime(data?.last_loaded_at)}</Descriptions.Item>
              <Descriptions.Item label="加载状态">
                {statusOk ? (
                  <Tag icon={<CheckCircleOutlined />} color="success">
                    正常
                  </Tag>
                ) : (
                  <Tag icon={<CloseCircleOutlined />} color="error">
                    异常
                  </Tag>
                )}
              </Descriptions.Item>
              <Descriptions.Item label="已生效配置模块">
                <Space wrap size={4}>
                  {Object.entries(data?.modules ?? {}).map(([k, v]) => (
                    <Tag key={k} color="blue">
                      {MODULE_LABELS[k] ?? k}({v})
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
                message="配置加载异常"
                description={
                  <>
                    {data.error_message}
                    <br />
                    <Typography.Text type="secondary">
                      受影响模块继续使用上一版可用配置；可点击「回滚上一版本」恢复。
                    </Typography.Text>
                  </>
                }
              />
            ) : null}
          </Card>

          <Card
            size="small"
            title="最近加载日志"
            style={{ marginTop: 16 }}
            extra={
              <Button size="small" type="link" onClick={() => setHistoryOpen(true)}>
                版本历史
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
                        {l.status === 'success' ? '成功' : '失败'}
                      </Tag>
                      [{MODULE_LABELS[l.module] ?? l.module}] {l.message}
                    </span>
                  ),
                }))}
              />
            ) : (
              <Empty description="暂无加载日志" />
            )}
          </Card>
        </Col>

        <Col xs={24} lg={10}>
          <Card size="small" title="版本历史（最近 50 个）">
            <Table
              rowKey="version"
              size="small"
              pagination={false}
              scroll={{ y: 480 }}
              dataSource={data?.history ?? []}
              columns={[
                { title: '版本', dataIndex: 'version', render: (v: string) => <Typography.Text code>{v}</Typography.Text> },
                { title: '时间', dataIndex: 'created_at', width: 110, render: (v: string) => fmtTimeShort(v) },
                {
                  title: '状态',
                  dataIndex: 'status',
                  width: 70,
                  render: (v: string) => (
                    <Tag color={v === 'ok' ? 'green' : 'red'}>{v === 'ok' ? '正常' : '异常'}</Tag>
                  ),
                },
              ]}
            />
          </Card>
        </Col>
      </Row>

      <Modal
        title="版本历史详情"
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
            { title: '版本', dataIndex: 'version', width: 180 },
            { title: '时间', dataIndex: 'created_at', width: 170, render: fmtTime },
            { title: '状态', dataIndex: 'status', width: 80 },
            { title: '说明', dataIndex: 'message', ellipsis: true },
          ]}
        />
      </Modal>
    </PageContainer>
  );
}
