// ===== 输出过滤配置 (REQ-011) =====
// 是否启用输出检测 / 违规响应策略 / 自定义安全提示语模板 / 流式检测阈值
import { useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { App, Button, Card, Form, Input, InputNumber, Select, Switch } from 'antd';
import PageContainer from '../../components/PageContainer';
import { outputApi } from '../../api/endpoints';
import { VIOLATION_STRATEGY_OPTIONS } from '../../constants/dicts';
import type { ViolationStrategy } from '../../api/types';

interface OutputFormValues {
  enabled: boolean;
  violation_strategy: ViolationStrategy;
  safe_message: string;
  stream_chunk_threshold: number;
}

export default function OutputFilter() {
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
      message.success('输出过滤配置已保存，热加载生效');
      queryClient.invalidateQueries({ queryKey: ['guard-output'] });
    },
    onError: () => undefined,
  });

  return (
    <PageContainer
      title="输出过滤配置"
      description="配置模型输出内容的过滤策略（REQ-011）。"
      extra={
        <Button type="primary" loading={saveMutation.isPending} onClick={() => form.submit()}>
          保存配置
        </Button>
      }
    >
      <Card size="small" loading={isLoading} style={{ maxWidth: 720 }}>
        <Form<OutputFormValues>
          form={form}
          layout="vertical"
          onFinish={(v) => saveMutation.mutate(v)}
        >
          <Form.Item name="enabled" label="启用输出检测" valuePropName="checked">
            <Switch checkedChildren="启用" unCheckedChildren="停用" />
          </Form.Item>
          <Form.Item
            name="violation_strategy"
            label="违规响应策略"
            rules={[{ required: true, message: '请选择违规响应策略' }]}
          >
            <Select style={{ width: 300 }} options={VIOLATION_STRATEGY_OPTIONS} />
          </Form.Item>
          <Form.Item
            name="safe_message"
            label="自定义安全提示语模板"
            rules={[{ required: true, message: '请输入安全提示语模板' }]}
          >
            <Input.TextArea rows={3} placeholder="抱歉，该回答包含不当内容。" />
          </Form.Item>
          <Form.Item
            name="stream_chunk_threshold"
            label="流式检测阈值（字符）"
            tooltip="流式输出累积达到该字符数时进行一次检测"
            rules={[{ required: true, message: '请输入流式检测阈值' }]}
          >
            <InputNumber min={16} max={65536} precision={0} style={{ width: 200 }} />
          </Form.Item>
        </Form>
      </Card>
    </PageContainer>
  );
}
