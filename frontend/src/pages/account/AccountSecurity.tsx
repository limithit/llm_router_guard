// ===== 个人账户 > 安全设置 (REQ-021) =====
// MFA 自助绑定流程：setup 生成二维码 → 输入动态验证码 enable → 展示恢复码（仅一次）
// 已绑定：显示状态 / 剩余恢复码 / 解绑；另含修改密码表单
import { useEffect, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
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
  Popconfirm,
  Row,
  Space,
  Steps,
  Tag,
  Typography,
} from 'antd';
import { CopyOutlined, SafetyOutlined } from '@ant-design/icons';
import PageContainer from '../../components/PageContainer';
import { accountMfaApi, authApi } from '../../api/endpoints';
import { useAuthStore } from '../../store/auth';
import { fmtTime } from '../../utils/format';
import type { MfaSetupResult } from '../../api/types';

interface PasswordFormValues {
  old_password: string;
  new_password: string;
  confirm: string;
}

export default function AccountSecurity() {
  const { t } = useTranslation();
  const { message, modal } = App.useApp();
  const queryClient = useQueryClient();
  const scope = useAuthStore((s) => s.scope);

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
      // 绑定成功后后端签发新的完整会话令牌（旧 scope=mfa 令牌已作废）。
      // 必须先切换令牌，否则后续任何请求都会 401 → 用户被踢回登录页、恢复码没看完。
      if (res.token) {
        const cur = useAuthStore.getState();
        if (cur.user) cur.setAuth(res.token, cur.user, '');
      }
      setRecoveryCodes(res.recovery_codes);
      setStep(2);
    },
    onError: () => undefined,
  });

  const disableMutation = useMutation({
    mutationFn: (c: string) => accountMfaApi.disable({ code: c }),
    onSuccess: () => {
      message.success(t('acctSec.unbindOk'));
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
      message.success(t('acctSec.pwdChanged'));
      pwdForm.resetFields();
    },
    onError: () => undefined,
  });

  const bound = status?.enabled === true;
  const mfaRestricted = scope === 'mfa';

  // 受限会话（scope=mfa）：登录时已被要求绑定，直接进入扫码步骤，省去再点一次「开始绑定」
  const autoStarted = useRef(false);
  useEffect(() => {
    if (!mfaRestricted || bound || autoStarted.current) return;
    if (setupMutation.isPending || setup) return;
    autoStarted.current = true;
    setupMutation.mutate();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mfaRestricted, bound, setup, setupMutation.isPending]);

  return (
    <PageContainer
      title={t('acctSec.title')}
      description={t('acctSec.desc')}
    >
      {mfaRestricted && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message={t('acctSec.forcedTitle')}
          description={t('acctSec.forcedDesc')}
        />
      )}
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={14}>
          <Card
            size="small"
            title={
              <Space>
                <SafetyOutlined />
                {t('acctSec.mfaCard')}
              </Space>
            }
            extra={
              bound ? <Tag color="green">{t('acctSec.bound')}</Tag> : <Tag color="default">{t('acctSec.unbound')}</Tag>
            }
          >
            <Descriptions size="small" column={1}>
              <Descriptions.Item label={t('acctSec.account')}>{me?.username ?? '-'}</Descriptions.Item>
              <Descriptions.Item label={t('acctSec.boundAt')}>{fmtTime(status?.bound_at)}</Descriptions.Item>
              <Descriptions.Item label={t('acctSec.codesLeft')}>
                {t('acctSec.codesLeftUnit', { n: status?.recovery_codes_left ?? '-' })}
              </Descriptions.Item>
            </Descriptions>

            <Divider />

            {!bound && step === 0 && (
              <Space direction="vertical" style={{ width: '100%' }}>
                <Alert
                  type="info"
                  showIcon
                  message={t('acctSec.bindHint')}
                />
                <Button
                  type="primary"
                  loading={setupMutation.isPending}
                  onClick={() => setupMutation.mutate()}
                >
                  {t('acctSec.startBind')}
                </Button>
              </Space>
            )}

            {!bound && step === 1 && setup && (
              <Space direction="vertical" style={{ width: '100%' }}>
                <Steps
                  size="small"
                  current={0}
                  items={[
                    { title: t('acctSec.stepScan') },
                    { title: t('acctSec.stepVerify') },
                    { title: t('acctSec.stepSave') },
                  ]}
                />
                <div style={{ textAlign: 'center', margin: '12px 0' }}>
                  <img
                    alt={t('acctSec.qrAlt')}
                    src={`data:image/png;base64,${setup.qr_png_base64}`}
                    style={{ width: 200, height: 200 }}
                  />
                </div>
                <Typography.Paragraph type="secondary">
                  {t('acctSec.scanHint')}
                </Typography.Paragraph>
                <Typography.Paragraph copyable code>
                  {setup.secret}
                </Typography.Paragraph>
                <Space.Compact style={{ width: '100%' }}>
                  <Input
                    placeholder={t('acctSec.codePh')}
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
                    {t('acctSec.verifyBind')}
                  </Button>
                </Space.Compact>
              </Space>
            )}

            {!bound && step === 2 && (
              <Space direction="vertical" style={{ width: '100%' }}>
                <Alert
                  type="success"
                  showIcon
                  message={t('acctSec.boundOk')}
                />
                <Alert
                  type="error"
                  showIcon
                  message={t('acctSec.recoveryWarnTitle')}
                  description={t('acctSec.recoveryWarnDesc')}
                  style={{ marginBottom: 4 }}
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
                    message.success(t('acctSec.codesCopied'));
                  }}
                >
                  {t('acctSec.copyAll')}
                </Button>
                <Popconfirm
                  title={t('acctSec.doneConfirmTitle')}
                  description={t('acctSec.doneConfirmDesc')}
                  okText={t('acctSec.done')}
                  cancelText={t('common.cancel')}
                  onConfirm={() => setStep(0)}
                >
                  <Button type="primary">
                    {t('acctSec.done')}
                  </Button>
                </Popconfirm>
              </Space>
            )}

            {bound && (
              <Space direction="vertical" style={{ width: '100%' }}>
                <Alert type="success" showIcon message={t('acctSec.active')} />
                <Button danger onClick={() => setDisableOpen(true)}>
                  {t('acctSec.unbind')}
                </Button>
              </Space>
            )}
          </Card>
        </Col>

        <Col xs={24} lg={10}>
          <Card size="small" title={t('acctSec.pwdCard')}>
            <Form<PasswordFormValues>
              form={pwdForm}
              layout="vertical"
              onFinish={async (v) => {
                pwdMutation.mutate({ old_password: v.old_password, new_password: v.new_password });
              }}
            >
              <Form.Item
                name="old_password"
                label={t('acctSec.oldPwd')}
                rules={[{ required: true, message: t('acctSec.oldPwdReq') }]}
              >
                <Input.Password placeholder={t('acctSec.oldPwd')} />
              </Form.Item>
              <Form.Item
                name="new_password"
                label={t('acctSec.newPwd')}
                extra={t('acctSec.pwdPolicy')}
                rules={[
                  { required: true, message: t('acctSec.newPwdReq') },
                  { min: 10, message: t('acctSec.newPwdMin') },
                ]}
              >
                <Input.Password placeholder={t('acctSec.newPwdPh')} />
              </Form.Item>
              <Form.Item
                name="confirm"
                label={t('acctSec.confirmPwd')}
                dependencies={['new_password']}
                rules={[
                  { required: true, message: t('acctSec.confirmReq') },
                  ({ getFieldValue }) => ({
                    validator(_, value) {
                      if (!value || getFieldValue('new_password') === value) return Promise.resolve();
                      return Promise.reject(new Error(t('acctSec.pwdMismatch')));
                    },
                  }),
                ]}
              >
                <Input.Password placeholder={t('acctSec.confirmPh')} />
              </Form.Item>
              <Button type="primary" htmlType="submit" loading={pwdMutation.isPending}>
                {t('acctSec.changePwd')}
              </Button>
            </Form>
          </Card>
        </Col>
      </Row>

      {/* 解绑 MFA：需验证码或恢复码 */}
      <Modal
        title={t('acctSec.unbindModal')}
        open={disableOpen}
        confirmLoading={disableMutation.isPending}
        okButtonProps={{ danger: true }}
        okText={t('acctSec.unbindOkText')}
        onOk={() => disableMutation.mutate(disableCode)}
        onCancel={() => {
          setDisableOpen(false);
          setDisableCode('');
        }}
      >
        <Alert
          type="warning"
          showIcon
          message={t('acctSec.unbindWarn')}
          style={{ marginBottom: 12 }}
        />
        <Input
          placeholder={t('acctSec.unbindPh')}
          value={disableCode}
          onChange={(e) => setDisableCode(e.target.value.trim())}
        />
      </Modal>

      {/* need_bind_mfa 引导提示（登录后由 /login 跳转进来时可见） */}
      {me && !bound && step === 0 && (
        <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
          {t('acctSec.forcedTip')}
        </Typography.Paragraph>
      )}
    </PageContainer>
  );
}
