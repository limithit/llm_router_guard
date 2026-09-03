// ===== 通用设置 (REQ-001) =====
// 网关全局运行参数：日志级别 / 审计保留天数 / 默认超时 / 最大并发 / 护栏全局开关 / 热加载生效时间
// listen_port 为启动参数，仅回显（修改后需重启，REQ-001 备注）
import { useEffect } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import {
  App,
  Button,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  Switch,
} from 'antd';
import { SaveOutlined } from '@ant-design/icons';
import PageContainer from '../../components/PageContainer';
import { settingsApi } from '../../api/endpoints';
import { LOG_LEVEL_OPTIONS } from '../../constants/dicts';
import type { SystemSettingsInput } from '../../api/types';

export default function GeneralSettings() {
  const { message } = App.useApp();
  const [form] = Form.useForm<SystemSettingsInput>();

  const { data, isLoading } = useQuery({
    queryKey: ['settings-general'],
    queryFn: () => settingsApi.get(),
  });

  useEffect(() => {
    if (data) {
      form.setFieldsValue({
        log_level: data.log_level,
        audit_retention_days: data.audit_retention_days,
        default_timeout_seconds: data.default_timeout_seconds,
        max_connections: data.max_connections,
        guard_enabled: data.guard_enabled,
        hot_reload_seconds: data.hot_reload_seconds,
        call_audit_enabled: data.call_audit_enabled,
        call_audit_only_errors: data.call_audit_only_errors,
        call_audit_sampling: data.call_audit_sampling,
      });
    }
  }, [data, form]);

  const saveMutation = useMutation({
    mutationFn: (v: SystemSettingsInput) => settingsApi.save(v),
    onSuccess: () => message.success('系统参数已保存，热加载生效（≤3 秒）'),
    onError: () => undefined,
  });

  const onSubmit = async () => {
    const v = await form.validateFields();
    saveMutation.mutate(v);
  };

  return (
    <PageContainer
      title="通用设置"
      description="网关全局运行参数（REQ-001）。除监听端口外均支持配置热加载，保存后 ≤3 秒生效。"
      extra={
        <Button
          type="primary"
          icon={<SaveOutlined />}
          loading={saveMutation.isPending}
          onClick={onSubmit}
        >
          保存
        </Button>
      }
    >
      <Form<SystemSettingsInput>
        form={form}
        layout="vertical"
        disabled={isLoading}
        style={{ maxWidth: 560 }}
      >
        <Form.Item name="log_level" label="日志级别" rules={[{ required: true }]}>
          <Select options={LOG_LEVEL_OPTIONS.map((o) => ({ value: o.value, label: o.label }))} />
        </Form.Item>
        <Form.Item
          name="audit_retention_days"
          label="审计日志保留天数"
          tooltip="超过保留期的调用/操作审计日志自动清理"
          rules={[{ required: true, message: '请输入保留天数' }]}
        >
          <InputNumber min={1} max={3650} precision={0} style={{ width: 200 }} addonAfter="天" />
        </Form.Item>
        <Form.Item
          name="default_timeout_seconds"
          label="默认超时时间"
          rules={[{ required: true, message: '请输入超时时间' }]}
        >
          <InputNumber min={1} max={3600} precision={0} style={{ width: 200 }} addonAfter="秒" />
        </Form.Item>
        <Form.Item
          name="max_connections"
          label="最大并发连接数"
          rules={[{ required: true, message: '请输入最大并发数' }]}
        >
          <InputNumber min={1} max={100000} precision={0} style={{ width: 200 }} />
        </Form.Item>
        <Form.Item
          name="guard_enabled"
          label="启用护栏（全局开关）"
          valuePropName="checked"
          tooltip="关闭后所有输入/输出过滤规则失效，仅记录审计"
        >
          <Switch />
        </Form.Item>
        <Form.Item
          name="call_audit_enabled"
          label="调用审计写入"
          valuePropName="checked"
          tooltip="关闭后不再写入调用审计（操作审计仍保留）。大量请求时关闭可降低 DB 写入压力。"
        >
          <Switch />
        </Form.Item>
        <Form.Item
          name="call_audit_only_errors"
          label="仅记录失败/拦截"
          valuePropName="checked"
          tooltip="开启后成功调用不写审计，仅记录错误与被护栏拦截的调用。"
        >
          <Switch />
        </Form.Item>
        <Form.Item
          name="call_audit_sampling"
          label="成功调用采样"
          tooltip="1=全记；N>1=成功调用按 1/N 概率记录（失败/拦截始终全记）。"
          rules={[{ required: true, message: '请输入采样率' }]}
        >
          <InputNumber min={1} max={1000} precision={0} style={{ width: 200 }} addonAfter="1/N" />
        </Form.Item>
        <Form.Item
          name="hot_reload_seconds"
          label="热加载生效时间"
          tooltip="配置变更后最长等待秒数；实时触发通常立即生效，该值为轮询兜底间隔（REQ-004）"
          rules={[{ required: true }]}
        >
          <InputNumber min={1} max={60} precision={0} style={{ width: 200 }} addonAfter="秒" />
        </Form.Item>
        <Form.Item label="网关监听端口" tooltip={data?.listen_port_note || '启动参数，修改需重启服务'}>
          <Space>
            <InputNumber value={data?.listen_port} disabled style={{ width: 140 }} />
            <Input value="仅启动参数生效（PORT 环境变量），修改后需重启服务" disabled variant="borderless" style={{ color: '#999' }} />
          </Space>
        </Form.Item>
      </Form>
    </PageContainer>
  );
}
