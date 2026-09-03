// ===== 输出过滤配置 (REQ-011) =====
// 是否启用输出检测 / 违规响应策略 / 自定义安全提示语模板 / 流式检测阈值
import { useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { App, Button, Card, Form, Input, InputNumber, Select, Switch } from 'antd';
import PageContainer from '../../components/PageContainer';
import { outputApi } from '../../api/endpoints';
import { VIOLATION_STRATEGY_META, useDictOptions } from '../../constants/dicts';
import type { ViolationStrategy } from '../../api/types';

interface OutputFormValues {
  enabled: boolean;
  violation_strategy: ViolationStrategy;
  safe_message: string;
  stream_chunk_threshold: number;
}

export default function OutputFilter() {
  const { t } = useTranslation();
  const strategyOpts = useDictOptions('violationStrategy', VIOLATION_STRATEGY_META);
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [form] = Form.useForm<OutputFormValues>();

  const { data, isLoading } = useQuery({
    queryKey: ['guard-output'],
    queryFn: outputApi.get,
  });

  useEffect(() => {
    if (!data) return;
    form.setFieldsValue({
      enabled: data.enabled,
      violation_strategy: data.violation_strategy,
      safe_message: data.safe_message,
      stream_chunk_threshold: data.stream_chunk_threshold,
    });
  }, [data, form]);

  const saveMutation = useMutation({
    mutationFn: (v: OutputFormValues) =>
      outputApi.save({
        enabled: v.enabled ?? true,
        violation_strategy: v.violation_strategy,
        safe_message: v.safe_message.trim(),
        stream_chunk_threshold: v.stream_chunk_threshold,
      }),
    onSuccess: () => {
      message.success(t('outputFilter.saveOk'));
      queryClient.invalidateQueries({ queryKey: ['guard-output'] });
    },
    onError: () => undefined,
  });

  return (
    <PageContainer
      title={t('outputFilter.title')}
      description={t('outputFilter.desc')}
      extra={
        <Button type="primary" loading={saveMutation.isPending} onClick={() => form.submit()}>
          {t('outputFilter.save')}
        </Button>
      }
    >
      <Card size="small" loading={isLoading} style={{ maxWidth: 720 }}>
        <Form<OutputFormValues>
          form={form}
          layout="vertical"
          onFinish={(v) => saveMutation.mutate(v)}
        >
          <Form.Item name="enabled" label={t('outputFilter.enabled')} valuePropName="checked">
            <Switch checkedChildren={t('outputFilter.on')} unCheckedChildren={t('outputFilter.off')} />
          </Form.Item>
          <Form.Item
            name="violation_strategy"
            label={t('outputFilter.strategy')}
            rules={[{ required: true, message: t('outputFilter.strategyReq') }]}
          >
            <Select style={{ width: 300 }} options={strategyOpts} />
          </Form.Item>
          <Form.Item
            name="safe_message"
            label={t('outputFilter.safeMessage')}
            rules={[{ required: true, message: t('outputFilter.safeMessageReq') }]}
          >
            <Input.TextArea rows={3} placeholder={t('outputFilter.safeMessagePh')} />
          </Form.Item>
          <Form.Item
            name="stream_chunk_threshold"
            label={t('outputFilter.chunkThreshold')}
            tooltip={t('outputFilter.chunkTooltip')}
            rules={[{ required: true, message: t('outputFilter.chunkReq') }]}
          >
            <InputNumber min={16} max={65536} precision={0} style={{ width: 200 }} />
          </Form.Item>
        </Form>
      </Card>
    </PageContainer>
  );
}
