// ===== 通用设置 (REQ-001) =====
// 网关全局运行参数：日志级别 / 审计保留天数 / 默认超时 / 最大并发 / 护栏全局开关 / 热加载生效时间
// listen_port 为启动参数，仅回显（修改后需重启，REQ-001 备注）
import { useEffect } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import {
  App,
  Button,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  Switch,
  Typography,
} from 'antd';
import { SaveOutlined } from '@ant-design/icons';
import PageContainer from '../../components/PageContainer';
import { settingsApi } from '../../api/endpoints';
import { LOG_LEVEL_META, useDictOptions } from '../../constants/dicts';
import type { SystemSettingsInput } from '../../api/types';

export default function GeneralSettings() {
  const { t } = useTranslation();
  const logLevelOpts = useDictOptions('logLevel', LOG_LEVEL_META);
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
    onSuccess: () => message.success(t('general.saveOk')),
    onError: () => undefined,
  });

  const onSubmit = async () => {
    const v = await form.validateFields();
    saveMutation.mutate(v);
  };

  return (
    <PageContainer
      title={t('general.title')}
      description={t('general.desc')}
      extra={
        <Button
          type="primary"
          icon={<SaveOutlined />}
          loading={saveMutation.isPending}
          onClick={onSubmit}
        >
          {t('general.save')}
        </Button>
      }
    >
      <Form<SystemSettingsInput>
        form={form}
        layout="vertical"
        disabled={isLoading}
        style={{ maxWidth: 560 }}
      >
        <Form.Item name="log_level" label={t('general.logLevel')} rules={[{ required: true }]}>
          <Select options={logLevelOpts} />
        </Form.Item>
        <Form.Item
          name="audit_retention_days"
          label={t('general.auditRetention')}
          tooltip={t('general.auditRetentionTooltip')}
          rules={[{ required: true, message: t('general.auditRetentionReq') }]}
        >
          <InputNumber min={1} max={3650} precision={0} style={{ width: 200 }} addonAfter={t('general.auditRetentionUnit')} />
        </Form.Item>
        <Form.Item
          name="default_timeout_seconds"
          label={t('general.timeout')}
          rules={[{ required: true, message: t('general.timeoutReq') }]}
        >
          <InputNumber min={1} max={3600} precision={0} style={{ width: 200 }} addonAfter={t('general.timeoutUnit')} />
        </Form.Item>
        <Form.Item
          name="max_connections"
          label={t('general.maxConn')}
          rules={[{ required: true, message: t('general.maxConnReq') }]}
        >
          <InputNumber min={1} max={100000} precision={0} style={{ width: 200 }} />
        </Form.Item>
        <Form.Item
          name="guard_enabled"
          label={t('general.guardEnabled')}
          valuePropName="checked"
          tooltip={t('general.guardTooltip')}
        >
          <Switch />
        </Form.Item>
        <Form.Item
          name="call_audit_enabled"
          label={t('general.auditWrite')}
          valuePropName="checked"
          tooltip={t('general.auditWriteTooltip')}
        >
          <Switch />
        </Form.Item>
        <Form.Item
          name="call_audit_only_errors"
          label={t('general.auditOnlyErrors')}
          valuePropName="checked"
          tooltip={t('general.auditOnlyErrorsTooltip')}
          extra={
            <Typography.Text type="warning" style={{ fontSize: 12 }}>
              {t('general.auditOnlyErrorsWarn')}
            </Typography.Text>
          }
        >
          <Switch />
        </Form.Item>
        <Form.Item
          name="call_audit_sampling"
          label={t('general.auditSampling')}
          tooltip={t('general.auditSamplingTooltip')}
          rules={[{ required: true, message: t('general.auditSamplingReq') }]}
        >
          <InputNumber min={1} max={1000} precision={0} style={{ width: 200 }} addonAfter="1/N" />
        </Form.Item>
        <Form.Item
          name="hot_reload_seconds"
          label={t('general.hotReload')}
          tooltip={t('general.hotReloadTooltip')}
          rules={[{ required: true }]}
        >
          <InputNumber min={1} max={60} precision={0} style={{ width: 200 }} addonAfter={t('general.timeoutUnit')} />
        </Form.Item>
        <Form.Item label={t('general.listenPort')} tooltip={data?.listen_port_note || t('general.listenPortNote')}>
          <Space>
            <InputNumber value={data?.listen_port} disabled style={{ width: 140 }} />
            <Input value={t('general.listenPortInput')} disabled variant="borderless" style={{ color: '#999' }} />
          </Space>
        </Form.Item>
      </Form>
    </PageContainer>
  );
}
