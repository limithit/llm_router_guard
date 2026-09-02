// ===== 个人账户 > 安全设置 (REQ-021) =====
// MFA 自助绑定流程：setup 生成二维码 → 输入动态验证码 enable → 展示恢复码（仅一次）
// 已绑定：显示状态 / 剩余恢复码 / 解绑；另含修改密码表单
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  App,
  Button,
  Card,
  Col,
  Descriptions,
  Divider,
  Form,
  Input,
  Modal,
  Row,
  Space,
  Steps,
  Tag,
  Typography,
} from 'antd';
import { CopyOutlined, SafetyOutlined } from '@ant-design/icons';
import PageContainer from '../../components/PageContainer';
import { accountMfaApi, authApi } from '../../api/endpoints';
import { fmtTime } from '../../utils/format';
import type { MfaSetupResult } from '../../api/types';

interface PasswordFormValues {
  old_password: string;
  new_password: string;
  confirm: string;
}

export default function AccountSecurity() {
  const { message, modal } = App.useApp();
  const queryClient = useQueryClient();

  // ---- MFA 绑定流程状态 ----
  const [step, setStep] = useState(0); // 0 未开始 | 1 扫码 | 2 完成展示恢复码
  const [setup, setSetup] = useState<MfaSetupResult | null>(null);
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [code, setCode] = useState('');
  const [disableOpen, setDisableOpen] = useState(false);
  const [disableCode, setDisableCode] = useState('');

  const { data: status } = useQuery({
    queryKey: ['account-mfa-status'],
    queryFn: () => accountMfaApi.status(),
  });

  const { data: me } = useQuery({
    queryKey: ['auth-me'],
    queryFn: () => authApi.me(),
  });

  const invalidateStatus = () =>
    queryClient.invalidateQueries({ queryKey: ['account-mfa-status'] });

  const setupMutation = useMutation({
    mutationFn: () => accountMfaApi.setup(),
    onSuccess: (res) => {
      setSetup(res);
      setStep(1);
    },
    onError: () => undefined,
  });

  const enableMutation = useMutation({
    mutationFn: (c: string) => accountMfaApi.enable({ code: c }),
    onSuccess: (res) => {
      setRecoveryCodes(res.recovery_codes);
      setStep(2);
      invalidateStatus();
    },
    onError: () => undefined,
  });

  const disableMutation = useMutation({
    mutationFn: (c: string) => accountMfaApi.disable({ code: c }),
    onSuccess: () => {
      message.success('MFA 已解绑');
      setDisableOpen(false);
      setDisableCode('');
      invalidateStatus();
    },
    onError: () => undefined,
  });

  // ---- 修改密码 ----
  const [pwdForm] = Form.useForm<PasswordFormValues>();
  const pwdMutation = useMutation({
    mutationFn: authApi.changePassword,
    onSuccess: () => {
      message.success('密码已修改');
      pwdForm.resetFields();
    },
    onError: () => undefined,
  });

  const bound = status?.enabled === true;

  return (
    <PageContainer
      title="个人账户 — 安全设置"
      description="MFA（TOTP）绑定/解绑、恢复码管理与密码修改（REQ-021）。"
    >
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={14}>
          <Card
            size="small"
            title={
              <Space>
                <SafetyOutlined />
                多因素认证（MFA）
              </Space>
            }
            extra={
              bound ? <Tag color="green">已绑定</Tag> : <Tag color="default">未绑定</Tag>
            }
          >
            <Descriptions size="small" column={1}>
              <Descriptions.Item label="账户">{me?.username ?? '-'}</Descriptions.Item>
              <Descriptions.Item label="绑定时间">{fmtTime(status?.bound_at)}</Descriptions.Item>
              <Descriptions.Item label="剩余恢复码">
                {status?.recovery_codes_left ?? '-'} 个
              </Descriptions.Item>
            </Descriptions>

            <Divider />

            {!bound && step === 0 && (
              <Space direction="vertical" style={{ width: '100%' }}>
                <Alert
                  type="info"
                  showIcon
                  message="绑定后登录需额外输入动态验证码，可显著提升管理后台安全性。"
                />
                <Button
                  type="primary"
                  loading={setupMutation.isPending}
                  onClick={() => setupMutation.mutate()}
                >
                  开始绑定 MFA
                </Button>
              </Space>
            )}

            {!bound && step === 1 && setup && (
              <Space direction="vertical" style={{ width: '100%' }}>
                <Steps
                  size="small"
                  current={0}
                  items={[{ title: '扫描二维码' }, { title: '验证动态码' }, { title: '保存恢复码' }]}
                />
                <div style={{ textAlign: 'center', margin: '12px 0' }}>
                  <img
                    alt="TOTP 绑定二维码"
                    src={`data:image/png;base64,${setup.qr_png_base64}`}
                    style={{ width: 200, height: 200 }}
                  />
                </div>
                <Typography.Paragraph type="secondary">
                  使用 Google Authenticator / Microsoft Authenticator 等应用扫描二维码，或手动输入密钥：
                </Typography.Paragraph>
                <Typography.Paragraph copyable code>
                  {setup.secret}
                </Typography.Paragraph>
                <Space.Compact style={{ width: '100%' }}>
                  <Input
                    placeholder="输入 6 位动态验证码完成绑定"
                    maxLength={6}
                    value={code}
                    onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))}
                    onPressEnter={() => code.length === 6 && enableMutation.mutate(code)}
                  />
                  <Button
                    type="primary"
                    loading={enableMutation.isPending}
                    disabled={code.length !== 6}
                    onClick={() => enableMutation.mutate(code)}
                  >
                    验证并绑定
                  </Button>
                </Space.Compact>
              </Space>
            )}

            {!bound && step === 2 && (
              <Space direction="vertical" style={{ width: '100%' }}>
                <Alert
                  type="success"
                  showIcon
                  message="MFA 绑定成功！请立即保存以下备用恢复码（仅展示这一次）。"
                />
                <Card size="small" style={{ background: '#fafafa' }}>
                  <Space wrap>
                    {recoveryCodes.map((c) => (
                      <Typography.Text key={c} code copyable>
                        {c}
                      </Typography.Text>
                    ))}
                  </Space>
                </Card>
                <Button
                  icon={<CopyOutlined />}
                  onClick={async () => {
                    await navigator.clipboard.writeText(recoveryCodes.join('\n'));
                    message.success('恢复码已复制');
                  }}
                >
                  复制全部恢复码
                </Button>
                <Button type="primary" onClick={() => setStep(0)}>
                  完成
                </Button>
              </Space>
            )}

            {bound && (
              <Space direction="vertical" style={{ width: '100%' }}>
                <Alert type="success" showIcon message="MFA 已启用，登录时将要求输入动态验证码。" />
                <Button danger onClick={() => setDisableOpen(true)}>
                  解绑 MFA
                </Button>
              </Space>
            )}
          </Card>
        </Col>

        <Col xs={24} lg={10}>
          <Card size="small" title="修改密码">
            <Form<PasswordFormValues>
              form={pwdForm}
              layout="vertical"
              onFinish={async (v) => {
                pwdMutation.mutate({ old_password: v.old_password, new_password: v.new_password });
              }}
            >
              <Form.Item
                name="old_password"
                label="当前密码"
                rules={[{ required: true, message: '请输入当前密码' }]}
              >
                <Input.Password placeholder="当前密码" />
              </Form.Item>
              <Form.Item
                name="new_password"
                label="新密码"
                rules={[
                  { required: true, message: '请输入新密码' },
                  { min: 8, message: '密码至少 8 位' },
                ]}
              >
                <Input.Password placeholder="至少 8 位" />
              </Form.Item>
              <Form.Item
                name="confirm"
                label="确认新密码"
                dependencies={['new_password']}
                rules={[
                  { required: true, message: '请再次输入新密码' },
                  ({ getFieldValue }) => ({
                    validator(_, value) {
                      if (!value || getFieldValue('new_password') === value) return Promise.resolve();
                      return Promise.reject(new Error('两次输入的密码不一致'));
                    },
                  }),
                ]}
              >
                <Input.Password placeholder="再次输入新密码" />
              </Form.Item>
              <Button type="primary" htmlType="submit" loading={pwdMutation.isPending}>
                修改密码
              </Button>
            </Form>
          </Card>
        </Col>
      </Row>

      {/* 解绑 MFA：需验证码或恢复码 */}
      <Modal
        title="解绑 MFA"
        open={disableOpen}
        confirmLoading={disableMutation.isPending}
        okButtonProps={{ danger: true }}
        okText="确认解绑"
        onOk={() => disableMutation.mutate(disableCode)}
        onCancel={() => {
          setDisableOpen(false);
          setDisableCode('');
        }}
      >
        <Alert
          type="warning"
          showIcon
          message="解绑后登录将不再要求动态验证码（该操作会记录审计）。"
          style={{ marginBottom: 12 }}
        />
        <Input
          placeholder="输入当前 6 位动态验证码或任一恢复码"
          value={disableCode}
          onChange={(e) => setDisableCode(e.target.value.trim())}
        />
      </Modal>

      {/* need_bind_mfa 引导提示（登录后由 /login 跳转进来时可见） */}
      {me && !bound && step === 0 && (
        <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
          提示：若管理员开启了「强制所有用户启用 MFA」，未绑定用户登录后会被引导至本页。
        </Typography.Paragraph>
      )}
    </PageContainer>
  );
}
