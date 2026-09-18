// ===== 首次登录强制改密（SEC-02）=====
// 登录返回 need_change_password 时，前端引导至此页。此页在受限会话（scope=pw）下工作，
// 后端仅放行 /auth/password、/auth/me、/auth/logout；改密成功后旧受限会话被吊销，
// 需回到登录页用新密码重新登录拿全权会话。
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Card, Form, Input, Space, Typography, message } from 'antd';
import { LockOutlined } from '@ant-design/icons';
import { authApi } from '../api/endpoints';
import { useAuthStore } from '../store/auth';

interface ChangePwdValues {
  old_password: string;
  new_password: string;
  confirm_password: string;
}

export default function ChangePassword() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const clearAuth = useAuthStore((s) => s.clearAuth);
  const [submitting, setSubmitting] = useState(false);
  const [form] = Form.useForm<ChangePwdValues>();

  const onFinish = async (v: ChangePwdValues) => {
    if (v.new_password !== v.confirm_password) {
      message.warning(t('login.passwordMismatch'));
      return;
    }
    setSubmitting(true);
    try {
      await authApi.changePassword({ old_password: v.old_password, new_password: v.new_password });
      // 改密成功：后端已吊销当前受限会话，清本地态并要求用新密码重新登录
      clearAuth();
      message.success(t('login.passwordChangedRelogin'));
      navigate('/login', { replace: true });
    } catch {
      // 403/401 已由 http 拦截器统一提示（如「请先修改初始密码」/「会话已被注销」）
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'linear-gradient(135deg, #0f1c2e 0%, #1d3a5f 60%, #2b5876 100%)',
        padding: 16,
      }}
    >
      <Card style={{ width: 420, boxShadow: '0 8px 32px rgba(0,0,0,.25)' }}>
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
          <div style={{ textAlign: 'center' }}>
            <LockOutlined style={{ fontSize: 40, color: '#1677ff' }} />
            <Typography.Title level={3} style={{ marginTop: 8, marginBottom: 0 }}>
              {t('login.changePasswordTitle')}
            </Typography.Title>
          </div>
          <Alert
            type="warning"
            showIcon
            message={t('login.changePasswordHint')}
            description={t('login.changePasswordDesc')}
          />
          <Form<ChangePwdValues> form={form} layout="vertical" onFinish={onFinish} requiredMark={false}>
            <Form.Item
              name="old_password"
              label={t('login.oldPassword')}
              rules={[{ required: true, message: t('login.oldPasswordRequired') }]}
            >
              <Input.Password prefix={<LockOutlined />} autoComplete="current-password" />
            </Form.Item>
            <Form.Item
              name="new_password"
              label={t('login.newPassword')}
              extra={t('login.pwdPolicy')}
              rules={[
                { required: true, message: t('login.newPasswordRequired') },
                { min: 10, max: 72, message: t('login.newPasswordLen') },
              ]}
            >
              <Input.Password prefix={<LockOutlined />} autoComplete="new-password" />
            </Form.Item>
            <Form.Item
              name="confirm_password"
              label={t('login.confirmPassword')}
              rules={[{ required: true, message: t('login.confirmPasswordRequired') }]}
            >
              <Input.Password prefix={<LockOutlined />} autoComplete="new-password" />
            </Form.Item>
            <Button type="primary" htmlType="submit" block loading={submitting}>
              {t('login.changePasswordSubmit')}
            </Button>
          </Form>
        </Space>
      </Card>
    </div>
  );
}
