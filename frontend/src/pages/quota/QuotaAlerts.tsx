// ===== 配额预警配置 (REQ-014) =====
// 预警阈值（80/95% 等）/ 通知方式（页面内通知 / Webhook）/ 通知接收人
import { useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { App, Button, Card, Form, Input, Select, Switch } from 'antd';
import PageContainer from '../../components/PageContainer';
import { quotaAlertApi } from '../../api/endpoints';
import { ALERT_CHANNEL_OPTIONS } from '../../constants/dicts';
import type { AlertChannel } from '../../api/types';

interface AlertFormValues {
  enabled: boolean;
  /** Select tags → 字符串 */
  thresholds: string[];
  channel: AlertChannel;
  webhook_url?: string;
  receivers?: string;
}

const toStr = (v: number) => String(v);

export default function QuotaAlerts() {
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [form] = Form.useForm<AlertFormValues>();
  const channel = Form.useWatch('channel', form);

  const { data, isLoading } = useQuery({
    queryKey: ['quota-alerts'],
    queryFn: quotaAlertApi.get,
  });

  useEffect(() => {
    if (!data) return;
    form.setFieldsValue({
      enabled: data.enabled,
      thresholds: (data.thresholds ?? []).map(toStr),
      channel: data.channel,
      webhook_url: data.webhook_url || undefined,
      receivers: data.receivers || undefined,
    });
  }, [data, form]);

  const saveMutation = useMutation({
    mutationFn: (v: AlertFormValues) =>
      quotaAlertApi.save({
        enabled: v.enabled ?? false,
        thresholds: Array.from(
          new Set(v.thresholds.map((s) => Number(s)).filter((n) => !Number.isNaN(n) && n > 0 && n <= 100))
        ).sort((a, b) => a - b),
        channel: v.channel,
        webhook_url: (v.channel === 'webhook' ? v.webhook_url?.trim() : '') || '',
        receivers: v.receivers?.trim() || '',
      }),
    onSuccess: () => {
      message.success('配额预警配置已保存，热加载生效');
      queryClient.invalidateQueries({ queryKey: ['quota-alerts'] });
    },
    onError: () => undefined,
  });

  return (
    <PageContainer
      title="配额预警配置"
      description="配额使用率达到阈值时触发通知（REQ-014）。"
      extra={
        <Button type="primary" loading={saveMutation.isPending} onClick={() => form.submit()}>
          保存配置
        </Button>
      }
    >
      <Card size="small" loading={isLoading} style={{ maxWidth: 640 }}>
        <Form<AlertFormValues>
          form={form}
          layout="vertical"
          onFinish={(v) => saveMutation.mutate(v)}
          initialValues={{ thresholds: ['80', '95'], channel: 'ui' }}
        >
          <Form.Item name="enabled" label="启用配额预警" valuePropName="checked">
            <Switch checkedChildren="启用" unCheckedChildren="停用" />
          </Form.Item>
          <Form.Item
            name="thresholds"
            label="预警阈值（百分比，回车添加多个）"
            tooltip="如 80、95，使用率首次越过阈值时触发一次通知"
            rules={[{ required: true, message: '请至少配置一个阈值' }]}
          >
            <Select
              mode="tags"
              placeholder="输入百分比后回车，例如 80"
              tokenSeparators={[',', '，', ' ']}
              style={{ width: 300 }}
            />
          </Form.Item>
          <Form.Item name="channel" label="通知方式" rules={[{ required: true }]}>
            <Select style={{ width: 220 }} options={ALERT_CHANNEL_OPTIONS} />
          </Form.Item>
          {channel === 'webhook' ? (
            <Form.Item
              name="webhook_url"
              label="Webhook URL"
              rules={[
                { required: true, message: '选择 Webhook 通知时必须填写 URL' },
                { type: 'url', message: '请输入合法的 URL' },
              ]}
            >
              <Input placeholder="https://hooks.example.com/xxx" />
            </Form.Item>
          ) : null}
          <Form.Item
            name="receivers"
            label="通知接收人"
            tooltip="接收人标识，多个以逗号分隔"
          >
            <Input placeholder="例如 admin@example.com, ops@example.com" />
          </Form.Item>
        </Form>
      </Card>
    </PageContainer>
  );
}
