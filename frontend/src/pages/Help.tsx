// ===== 使用帮助：网关对外对接说明 =====
// 服务地址、端点、鉴权（两套 Key 的区分）、模型别名路由、调用示例、对外部署。
import { Trans, useTranslation } from 'react-i18next';
import { Alert, Card, Col, Row, Table, Tag, Typography } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import PageContainer from '../components/PageContainer';

const { Paragraph, Text } = Typography;

function CodeBlock({ children }: { children: string }) {
  const { t } = useTranslation();
  return (
    <pre
      style={{
        background: '#f6f8fa',
        padding: 12,
        borderRadius: 6,
        overflowX: 'auto',
        margin: '8px 0',
        fontSize: 13,
        lineHeight: 1.6,
        position: 'relative',
      }}
    >
      <Typography.Text
        copyable={{ text: children, tooltips: [t('common.copy'), t('common.copied')] }}
        style={{ position: 'absolute', right: 8, top: 8 }}
      />
      {children}
    </pre>
  );
}

export default function Help() {
  const { t } = useTranslation();

  const endpoints = [
    { key: '1', proto: 'OpenAI Chat', method: 'POST', path: '/v1/chat/completions', client: t('help.client1') },
    { key: '2', proto: 'OpenAI Responses', method: 'POST', path: '/v1/responses', client: t('help.client2') },
    { key: '3', proto: 'Anthropic Messages', method: 'POST', path: '/v1/messages', client: t('help.client3') },
    { key: '4', proto: t('help.proto4'), method: 'GET', path: '/v1/models', client: t('help.client4') },
  ];

  return (
    <PageContainer title={t('help.title')} description={t('help.desc')}>
      <Alert
        type="info"
        showIcon
        message={t('help.deploymentTitle')}
        description={
          <div>
            <Trans i18nKey="help.deploymentBody" components={{ c1: <Text code>:8080</Text>, c2: <Text code>PORT</Text>, c3: <Text code>/v1/*</Text> }} />
          </div>
        }
        style={{ marginBottom: 16 }}
      />

      <Card title={t('help.addrTitle')} size="small" style={{ marginBottom: 16 }}>
        <Paragraph>
          <Trans
            i18nKey="help.addrBody"
            components={{
              c1: <Text code copyable>{`http://${t('help.yourHost')}:<port>/v1`}</Text>,
              c2: <Text code>http://localhost:8080/v1</Text>,
              c3: <Text code>{t('help.yourHost')}</Text>,
            }}
          />
        </Paragraph>
        <Table
          size="small"
          pagination={false}
          dataSource={endpoints}
          columns={[
            { title: t('help.colProto'), dataIndex: 'proto', width: 160, render: (v: string) => <Tag color="blue">{v}</Tag> },
            {
              title: t('help.colMethodPath'),
              render: (_: unknown, r: (typeof endpoints)[number]) => (
                <Text code>{r.method} {r.path}</Text>
              ),
            },
            { title: t('help.colClient'), dataIndex: 'client' },
          ]}
        />
      </Card>

      <Card title={t('help.authTitle')} size="small" style={{ marginBottom: 16 }}>
        <Paragraph>
          {t('help.authBody1')}<Text strong>{t('help.authKey')}</Text>{t('help.authBody2')}
        </Paragraph>
        <CodeBlock>{t('help.authCode')}</CodeBlock>
        <Row gutter={16}>
          <Col span={12}>
            <Card size="small" type="inner" title={t('help.gatewayKeyTitle')} style={{ background: '#f6ffed', borderColor: '#b7eb8f' }}>
              {t('help.gatewayKeyBody')}
            </Card>
          </Col>
          <Col span={12}>
            <Card size="small" type="inner" title={t('help.vendorKeyTitle')} style={{ background: '#fff7e6', borderColor: '#ffd591' }}>
              {t('help.vendorKeyBody1')}<Text strong>{t('help.vendorKeyStrong')}</Text>{t('help.vendorKeyBody2')}
            </Card>
          </Col>
        </Row>
      </Card>

      <Card title={t('help.aliasTitle')} size="small" style={{ marginBottom: 16 }}>
        <Paragraph>
          <Trans i18nKey="help.aliasBody" components={{ c1: <Text code>model</Text> }} />
          <Text strong>{t('help.aliasStrong')}</Text>
          <Trans
            i18nKey="help.aliasBody2"
            components={{ c1: <Text code>gpt-4o</Text>, c2: <Text code>404 unknown model</Text> }}
          />
        </Paragraph>
      </Card>

      <Card title={t('help.exampleTitle')} size="small" style={{ marginBottom: 16 }}>
        <Paragraph strong>{t('help.exampleOpenai')}</Paragraph>
        <CodeBlock>{`curl http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer ${t('help.curlKey')}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "messages": [{"role":"user","content":"你好"}],
    "stream": false
  }'`}</CodeBlock>
        <Paragraph strong>{t('help.exampleAnthropic')}</Paragraph>
        <CodeBlock>{`curl http://localhost:8080/v1/messages \\
  -H "x-api-key: ${t('help.curlKey')}" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 256,
    "messages": [{"role":"user","content":"你好"}]
  }'`}</CodeBlock>
      </Card>

      <Card title={t('help.deployTitle')} size="small" style={{ marginBottom: 16 }}>
        <Paragraph>
          <Trans
            i18nKey="help.deployBody"
            components={{
              c1: <Text code>localhost:8080</Text>,
              c2: <Text code>{`http://${t('help.serverAddr')}:8080/v1/...`}</Text>,
              c3: <Text code>:8080</Text>,
            }}
          />
        </Paragraph>
      </Card>

      <Card title={t('help.securityTitle')} size="small" style={{ marginBottom: 16 }}>
        <Paragraph>{t('help.securityIntro')}</Paragraph>
        <ul style={{ margin: 0, paddingLeft: 20 }}>
          {[
            { l: t('help.securityIpLabel'), b: t('help.securityIp') },
            { l: t('help.securityHeadersLabel'), b: t('help.securityHeaders') },
            { l: t('help.securitySecretsLabel'), b: t('help.securitySecrets') },
            { l: t('help.securityJwtLabel'), b: t('help.securityJwt') },
            { l: t('help.securityStorageLabel'), b: t('help.securityStorage') },
            { l: t('help.securityTopologyLabel'), b: t('help.securityTopology') },
            { l: t('help.securityTlsLabel'), b: t('help.securityTls') },
          ].map((it, i) => (
            <li key={i} style={{ marginBottom: 8 }}>
              <Text strong>{it.l}：</Text>
              {it.b}
            </li>
          ))}
        </ul>
      </Card>

      <Card size="small" style={{ background: '#fafafa' }}>
        <Typography>
          <QuestionCircleOutlined style={{ marginRight: 8, color: '#1677ff' }} />
          <Text type="secondary">
            {t('help.tip')}
          </Text>
        </Typography>
      </Card>
    </PageContainer>
  );
}
