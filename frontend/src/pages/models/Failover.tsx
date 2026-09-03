// ===== 故障转移设置 (REQ-007) =====
// 启用自动故障转移 / 重试次数 / 重试间隔策略 / 触发条件 / 熔断阈值与恢复时间
import { useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { App, Button, Card, Form, InputNumber, Select, Switch } from 'antd';
import PageContainer from '../../components/PageContainer';
import { failoverApi } from '../../api/endpoints';
import { BACKOFF_META, useDictOptions } from '../../constants/dicts';
import type { BackoffStrategy } from '../../api/types';

interface FailoverFormValues {
  enabled: boolean;
  retry_count: number;
  backoff: BackoffStrategy;
  retry_interval_ms: number;
  /** tags 模式下为字符串 */
  trigger_status_codes: string[];
  circuit_failure_threshold: number;
  circuit_reset_seconds: number;
}

const toStr = (v: number) => String(v);

export default function Failover() {
  const { t } = useTranslation();
  const backoffOpts = useDictOptions('backoff', BACKOFF_META);
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [form] = Form.useForm<FailoverFormValues>();

  const { data, isLoading } = useQuery({
    queryKey: ['failover'],
    queryFn: failoverApi.get,
  });

  useEffect(() => {
    if (!data) return;
    form.setFieldsValue({
      enabled: data.enabled,
      retry_count: data.retry_count,
      backoff: data.backoff,
      retry_interval_ms: data.retry_interval_ms,
      trigger_status_codes: data.trigger_status_codes.map(toStr),
      circuit_failure_threshold: data.circuit_failure_threshold,
      circuit_reset_seconds: data.circuit_reset_seconds,
    });
  }, [data, form]);

  const saveMutation = useMutation({
    mutationFn: (v: FailoverFormValues) =>
      failoverApi.save({
        enabled: v.enabled ?? true,
        retry_count: v.retry_count,
        backoff: v.backoff,
        retry_interval_ms: v.retry_interval_ms,
        trigger_status_codes: Array.from(
          new Set(v.trigger_status_codes.map((s) => Number(s)).filter((n) => !Number.isNaN(n)))
        ),
        circuit_failure_threshold: v.circuit_failure_threshold,
        circuit_reset_seconds: v.circuit_reset_seconds,
      }),
    onSuccess: () => {
      message.success(t('failover.saveOk'));
      queryClient.invalidateQueries({ queryKey: ['failover'] });
    },
    onError: () => undefined,
  });

  return (
    <PageContainer
      title={t('failover.title')}
      description={t('failover.desc')}
      extra={
        <Button type="primary" loading={saveMutation.isPending} onClick={() => form.submit()}>
          {t('failover.save')}
        </Button>
      }
    >
      <Card size="small" loading={isLoading}>
        <Form<FailoverFormValues>
          form={form}
          layout="vertical"
          style={{ maxWidth: 640 }}
          onFinish={(v) => saveMutation.mutate(v)}
          initialValues={{ trigger_status_codes: ['429', '500', '502', '503'] }}
        >
          <Form.Item name="enabled" label={t('failover.enabled')} valuePropName="checked">
            <Switch checkedChildren={t('failover.on')} unCheckedChildren={t('failover.off')} />
          </Form.Item>
          <Form.Item
            name="retry_count"
            label={t('failover.retryCount')}
            rules={[{ required: true, message: t('failover.retryCountReq') }]}
          >
            <InputNumber min={0} max={10} precision={0} style={{ width: 200 }} />
          </Form.Item>
          <Form.Item
            name="backoff"
            label={t('failover.backoff')}
            rules={[{ required: true, message: t('failover.backoffReq') }]}
          >
            <Select
              style={{ width: 260 }}
              options={backoffOpts}
            />
          </Form.Item>
          <Form.Item
            name="retry_interval_ms"
            label={t('failover.retryInterval')}
            rules={[{ required: true, message: t('failover.retryIntervalReq') }]}
          >
            <InputNumber min={50} step={50} precision={0} style={{ width: 200 }} />
          </Form.Item>
          <Form.Item
            name="trigger_status_codes"
            label={t('failover.trigger')}
            tooltip={t('failover.triggerTooltip')}
            rules={[{ required: true, message: t('failover.triggerReq') }]}
          >
            <Select
              mode="tags"
              placeholder={t('failover.triggerPh')}
              tokenSeparators={[',', '，', ' ']}
              style={{ width: 400 }}
            />
          </Form.Item>
          <Form.Item
            name="circuit_failure_threshold"
            label={t('failover.circuitThreshold')}
            rules={[{ required: true, message: t('failover.circuitThresholdReq') }]}
          >
            <InputNumber min={1} max={100} precision={0} style={{ width: 200 }} />
          </Form.Item>
          <Form.Item
            name="circuit_reset_seconds"
            label={t('failover.circuitReset')}
            rules={[{ required: true, message: t('failover.circuitResetReq') }]}
          >
            <InputNumber min={5} max={3600} precision={0} style={{ width: 200 }} />
          </Form.Item>
        </Form>
      </Card>
    </PageContainer>
  );
}
