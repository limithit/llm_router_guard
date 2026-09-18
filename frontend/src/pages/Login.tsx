// ===== 登录页（含 MFA 二次验证流程）(REQ-022) =====
// 密码正确且已绑定 MFA → 响应 mfa_required=true + mfa_token → 切换验证码步骤
// 支持 totp_code 或 recovery_code 完成验证；need_bind_mfa 时引导跳转 /account/security
import { useState } from 'react';
import { Navigate, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Card, Checkbox, Form, Input, Space, Steps, Typography, message } from 'antd';
import { LockOutlined, SafetyCertificateOutlined, UserOutlined } from '@ant-design/icons';
import { authApi } from '../api/endpoints';
import { isLoggedIn, useAuthStore } from '../store/auth';
import LanguageSwitch from '../components/LanguageSwitch';
import type { LoginBody } from '../api/types';

interface LoginFormValues {
  username: string;
  password: string;
}

interface MfaFormValues {
  code: string;
  use_recovery: boolean;
}

export default function Login() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const setAuth = useAuthStore((s) => s.setAuth);

  const [step, setStep] = useState<0 | 1>(0);
  const [mfaToken, setMfaToken] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [useRecovery, setUseRecovery] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  if (isLoggedIn()) {
    return <Navigate to="/" replace />;
  }

  const doLogin = async (body: LoginBody) => {
    setSubmitting(true);
    try {
      const res = await authApi.login(body);
      if ('token' in res && res.token) {
        // 记录会话 scope：受限会话必须把用户按在引导页上（刷新后依然生效）
        const scope = res.need_change_password ? 'pw' : res.need_bind_mfa ? 'mfa' : '';
        setAuth(res.token, res.user, scope);
        if (res.need_change_password) {
          // SEC-02：受限会话（scope=pw），引导强制改密
          navigate('/change-password', { replace: true });
        } else if (res.need_bind_mfa) {
          message.warning(t('login.needBindMfa'));
          navigate('/account/security', { replace: true });
        } else {
          navigate('/', { replace: true });
        }
        return;
      }
      // mfa_required
      if ('mfa_required' in res && res.mfa_required) {
        setMfaToken(res.mfa_token);
        setStep(1);
        return;
      }
      message.error(t('login.loginResponseInvalid'));
    } catch (e) {
      // 401：全局拦截器刻意静默（它服务于「过期跳登录」），这里给出统一通用提示（防枚举）。
      // 403（口令正确但账户锁定）/429（IP 限频）：拦截器已展示安全文案，不重复弹。
      const status = (e as { response?: { status?: number } }).response?.status;
      const code = (e as { code?: number }).code;
      if (status === 401 || code === 40101 || code === 40102 || code === 40103) {
        message.error(t('login.invalidCredentials'));
      } else if (!status) {
        // 网络错误 / 超时等，非鉴权失败，不伪装成账密错误
        message.error(t('common.networkError'));
      }
    } finally {
      setSubmitting(false);
    }
  };

  const onCredentialsFinish = (values: LoginFormValues) => {
    setUsername(values.username);
    setPassword(values.password);
    void doLogin({ username: values.username, password: values.password });
  };

  const onMfaFinish = async (values: MfaFormValues) => {
    const code = values.code.trim();
    if (!code) {
      message.warning(t('login.mfaCodeMissing'));
      return;
    }
    await doLogin({
      username,
      password,
      mfa_token: mfaToken,
      ...(useRecovery ? { recovery_code: code } : { totp_code: code }),
    });
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
        position: 'relative',
      }}
    >
      <div style={{ position: 'absolute', top: 16, right: 24 }}>
        <LanguageSwitch />
      </div>
      <Card style={{ width: 420, boxShadow: '0 8px 32px rgba(0,0,0,.25)' }}>
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
          <div style={{ textAlign: 'center' }}>
            <SafetyCertificateOutlined style={{ fontSize: 40, color: '#1677ff' }} />
            <Typography.Title level={3} style={{ marginTop: 8, marginBottom: 0 }}>
              {t('login.title')}
            </Typography.Title>
            <Typography.Text type="secondary">{t('login.subtitle')}</Typography.Text>
          </div>

          <Steps
            size="small"
            current={step}
            items={[{ title: t('login.stepCredentials') }, { title: t('login.stepMfa') }]}
          />

          {step === 0 ? (
            <Form<LoginFormValues> layout="vertical" onFinish={onCredentialsFinish} requiredMark={false}>
              <Form.Item
                name="username"
                label={t('login.username')}
                rules={[{ required: true, message: t('login.usernameRequired') }]}
              >
                <Input prefix={<UserOutlined />} placeholder={t('login.username')} autoComplete="username" />
              </Form.Item>
              <Form.Item
                name="password"
                label={t('login.password')}
                rules={[{ required: true, message: t('login.passwordRequired') }]}
              >
                <Input.Password
                  prefix={<LockOutlined />}
                  placeholder={t('login.password')}
                  autoComplete="current-password"
                />
              </Form.Item>
              <Button type="primary" htmlType="submit" block loading={submitting}>
                {t('login.submit')}
              </Button>
            </Form>
          ) : (
            <Form<MfaFormValues> layout="vertical" onFinish={onMfaFinish} requiredMark={false}>
              <Alert
                type="info"
                showIcon
                style={{ marginBottom: 16 }}
                message={t('login.mfaBannerTitle')}
                description={t('login.mfaBannerDesc')}
              />
              <Form.Item
                name="code"
                label={useRecovery ? t('login.recoveryCodeLabel') : t('login.mfaCodeLabel')}
                rules={[{ required: true, message: useRecovery ? t('login.recoveryCodeRequired') : t('login.mfaCodeRequired') }]}
              >
                <Input
                  placeholder={useRecovery ? t('login.recoveryCodePlaceholder') : t('login.mfaCodePlaceholder')}
                  maxLength={useRecovery ? 32 : 6}
                  autoFocus
                />
              </Form.Item>
              <Form.Item>
                <Checkbox checked={useRecovery} onChange={(e) => setUseRecovery(e.target.checked)}>
                  {t('login.useRecovery')}
                </Checkbox>
              </Form.Item>
              <Button type="primary" htmlType="submit" block loading={submitting}>
                {t('login.verifyAndLogin')}
              </Button>
              <Button
                type="link"
                block
                onClick={() => {
                  setStep(0);
                  setUseRecovery(false);
                }}
              >
                {t('login.backToPrevStep')}
              </Button>
            </Form>
          )}
        </Space>
      </Card>
    </div>
  );
}
