// ===== 配额预警配置 (REQ-014) =====
// 预警阈值（80/95% 等）/ 通知方式（页面内通知 / Webhook）/ 通知接收人
import { useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { App, Button, Card, Form, Input, Select, Switch } from 'antd';
import PageContainer from '../../components/PageContainer';
import { quotaAlertApi } from '../../api/endpoints';
import { ALERT_CHANNEL_META, useDictOptions } from '../../constants/dicts';
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
  const { t } = useTranslation();
  const channelOpts = useDictOptions('alertChannel', ALERT_CHANNEL_META);
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
      message.success(t('quotaAlerts.saveOk'));
      queryClient.invalidateQueries({ queryKey: ['quota-alerts'] });
    },
    onError: () => undefined,
  });

  return (
    <PageContainer
      title={t('quotaAlerts.title')}
      description={t('quotaAlerts.desc')}
      extra={
        <Button type="primary" loading={saveMutation.isPending} onClick={() => form.submit()}>
          {t('quotaAlerts.save')}
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
          <Form.Item name="enabled" label={t('quotaAlerts.enabled')} valuePropName="checked">
            <Switch checkedChildren={t('quotaAlerts.on')} unCheckedChildren={t('quotaAlerts.off')} />
          </Form.Item>
          <Form.Item
            name="thresholds"
            label={t('quotaAlerts.thresholds')}
            tooltip={t('quotaAlerts.thresholdsTooltip')}
            rules={[{ required: true, message: t('quotaAlerts.thresholdsReq') }]}
          >
            <Select
              mode="tags"
              placeholder={t('quotaAlerts.thresholdsPh')}
              tokenSeparators={[',', '，', ' ']}
              style={{ width: 300 }}
            />
          </Form.Item>
          <Form.Item name="channel" label={t('quotaAlerts.channel')} rules={[{ required: true }]}>
            <Select style={{ width: 220 }} options={channelOpts} />
          </Form.Item>
          {channel === 'webhook' ? (
            <Form.Item
              name="webhook_url"
              label="Webhook URL"
              rules={[
                { required: true, message: t('quotaAlerts.webhookUrlReq') },
                { type: 'url', message: t('quotaAlerts.urlInvalid') },
              ]}
            >
              <Input placeholder="https://hooks.example.com/xxx" />
            </Form.Item>
          ) : null}
          <Form.Item
            name="receivers"
            label={t('quotaAlerts.receivers')}
            tooltip={t('quotaAlerts.receiversTooltip')}
          >
            <Input placeholder={t('quotaAlerts.receiversPh')} />
          </Form.Item>
        </Form>
      </Card>
    </PageContainer>
  );
}
