// ===== 故障转移设置 (REQ-007) =====
// 启用自动故障转移 / 重试次数 / 重试间隔策略 / 触发条件 / 熔断阈值与恢复时间
import { useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { App, Button, Card, Form, InputNumber, Select, Switch } from 'antd';
import PageContainer from '../../components/PageContainer';
import { failoverApi } from '../../api/endpoints';
import { BACKOFF_OPTIONS } from '../../constants/dicts';
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
      message.success('故障转移策略已保存，热加载生效');
      queryClient.invalidateQueries({ queryKey: ['failover'] });
    },
    onError: () => undefined,
  });

  return (
    <PageContainer
      title="故障转移设置"
      description="当一个上游供应商失败 / 触发指定状态码时自动切换到其他上游（REQ-007）。"
      extra={
        <Button type="primary" loading={saveMutation.isPending} onClick={() => form.submit()}>
          保存配置
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
          <Form.Item name="enabled" label="启用自动故障转移" valuePropName="checked">
            <Switch checkedChildren="启用" unCheckedChildren="停用" />
          </Form.Item>
          <Form.Item
            name="retry_count"
            label="重试次数"
            rules={[{ required: true, message: '请输入重试次数' }]}
          >
            <InputNumber min={0} max={10} precision={0} style={{ width: 200 }} />
          </Form.Item>
          <Form.Item
            name="backoff"
            label="重试间隔策略"
            rules={[{ required: true, message: '请选择策略' }]}
          >
            <Select
              style={{ width: 260 }}
              options={BACKOFF_OPTIONS.map((o) => ({ value: o.value, label: o.label }))}
            />
          </Form.Item>
          <Form.Item
            name="retry_interval_ms"
            label="重试间隔（毫秒）"
            rules={[{ required: true, message: '请输入重试间隔' }]}
          >
            <InputNumber min={50} step={50} precision={0} style={{ width: 200 }} />
          </Form.Item>
          <Form.Item
            name="trigger_status_codes"
            label="触发条件（HTTP 状态码，回车添加）"
            tooltip="上游返回这些状态码时触发故障转移，如 429/500/502/503"
            rules={[{ required: true, message: '至少配置一个触发状态码' }]}
          >
            <Select
              mode="tags"
              placeholder="输入状态码后回车，例如 429"
              tokenSeparators={[',', '，', ' ']}
              style={{ width: 400 }}
            />
          </Form.Item>
          <Form.Item
            name="circuit_failure_threshold"
            label="熔断阈值（连续失败次数）"
            rules={[{ required: true, message: '请输入熔断阈值' }]}
          >
            <InputNumber min={1} max={100} precision={0} style={{ width: 200 }} />
          </Form.Item>
          <Form.Item
            name="circuit_reset_seconds"
            label="熔断恢复时间（秒）"
            rules={[{ required: true, message: '请输入恢复时间' }]}
          >
            <InputNumber min={5} max={3600} precision={0} style={{ width: 200 }} />
          </Form.Item>
        </Form>
      </Card>
    </PageContainer>
  );
}
