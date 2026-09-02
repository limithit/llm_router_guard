// ===== 登录页（含 MFA 二次验证流程）(REQ-022) =====
// 密码正确且已绑定 MFA → 响应 mfa_required=true + mfa_token → 切换验证码步骤
// 支持 totp_code 或 recovery_code 完成验证；need_bind_mfa 时引导跳转 /account/security
import { useState } from 'react';
import { Navigate, useNavigate } from 'react-router-dom';
import { Alert, Button, Card, Checkbox, Form, Input, Space, Steps, Typography, message } from 'antd';
import { LockOutlined, SafetyCertificateOutlined, UserOutlined } from '@ant-design/icons';
import { authApi } from '../api/endpoints';
import { isLoggedIn, useAuthStore } from '../store/auth';
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
        setAuth(res.token, res.user);
        if (res.need_bind_mfa) {
          message.warning('检测到您尚未绑定 MFA，请先完成绑定以提升账户安全');
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
      message.error('登录响应异常，请重试');
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
      message.warning('请输入验证码或恢复码');
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
      }}
    >
      <Card style={{ width: 420, boxShadow: '0 8px 32px rgba(0,0,0,.25)' }}>
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
          <div style={{ textAlign: 'center' }}>
            <SafetyCertificateOutlined style={{ fontSize: 40, color: '#1677ff' }} />
            <Typography.Title level={3} style={{ marginTop: 8, marginBottom: 0 }}>
              AI 网关管理平台
            </Typography.Title>
            <Typography.Text type="secondary">AI Gateway & Model Guardrails 管理后台</Typography.Text>
          </div>

          <Steps
            size="small"
            current={step}
            items={[{ title: '账号密码' }, { title: 'MFA 验证' }]}
          />

          {step === 0 ? (
            <Form<LoginFormValues> layout="vertical" onFinish={onCredentialsFinish} requiredMark={false}>
              <Form.Item
                name="username"
                label="用户名"
                rules={[{ required: true, message: '请输入用户名' }]}
              >
                <Input prefix={<UserOutlined />} placeholder="用户名" autoComplete="username" />
              </Form.Item>
              <Form.Item
                name="password"
                label="密码"
                rules={[{ required: true, message: '请输入密码' }]}
              >
                <Input.Password
                  prefix={<LockOutlined />}
                  placeholder="密码"
                  autoComplete="current-password"
                />
              </Form.Item>
              <Button type="primary" htmlType="submit" block loading={submitting}>
                登 录
              </Button>
            </Form>
          ) : (
            <Form<MfaFormValues> layout="vertical" onFinish={onMfaFinish} requiredMark={false}>
              <Alert
                type="info"
                showIcon
                style={{ marginBottom: 16 }}
                message="账户已开启两步验证"
                description="请输入身份验证器中的 6 位动态验证码；如需使用备用恢复码，请勾选下方选项。"
              />
              <Form.Item
                name="code"
                label={useRecovery ? '恢复码' : '动态验证码'}
                rules={[{ required: true, message: useRecovery ? '请输入恢复码' : '请输入动态验证码' }]}
              >
                <Input
                  placeholder={useRecovery ? '例如：xxxx-xxxx-xxxx' : '6 位数字'}
                  maxLength={useRecovery ? 32 : 6}
                  autoFocus
                />
              </Form.Item>
              <Form.Item>
                <Checkbox checked={useRecovery} onChange={(e) => setUseRecovery(e.target.checked)}>
                  使用恢复码登录
                </Checkbox>
              </Form.Item>
              <Button type="primary" htmlType="submit" block loading={submitting}>
                验证并登录
              </Button>
              <Button
                type="link"
                block
                onClick={() => {
                  setStep(0);
                  setUseRecovery(false);
                }}
              >
                返回上一步
              </Button>
            </Form>
          )}
        </Space>
      </Card>
    </div>
  );
}
