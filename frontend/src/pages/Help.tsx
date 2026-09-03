// ===== 使用帮助：网关对外对接说明 =====
// 服务地址、端点、鉴权（两套 Key 的区分）、模型别名路由、调用示例、对外部署。
import { Alert, Card, Col, Row, Table, Tag, Typography } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import PageContainer from '../components/PageContainer';

const { Paragraph, Text } = Typography;

const ENDPOINTS = [
  { key: '1', proto: 'OpenAI Chat', method: 'POST', path: '/v1/chat/completions', client: 'openai-python、绝大多数 SDK 与工具' },
  { key: '2', proto: 'OpenAI Responses', method: 'POST', path: '/v1/responses', client: 'Responses API 客户端' },
  { key: '3', proto: 'Anthropic Messages', method: 'POST', path: '/v1/messages', client: 'anthropic SDK' },
  { key: '4', proto: '模型目录', method: 'GET', path: '/v1/models', client: '第三方 Agent 工具发现可用模型（OpenAI 兼容，返回已配置别名）' },
];

function CodeBlock({ children }: { children: string }) {
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
        copyable={{ text: children, tooltips: ['复制', '已复制'] }}
        style={{ position: 'absolute', right: 8, top: 8 }}
      />
      {children}
    </pre>
  );
}

export default function Help() {
  return (
    <PageContainer title="使用帮助" description="网关对外服务地址、鉴权方式与调用示例">
      <Alert
        type="info"
        showIcon
        message="单进程部署模型"
        description={
          <div>
            本系统为单进程：一个 Go 后端在同一个端口（默认 <Text code>:8080</Text>，可用环境变量{' '}
            <Text code>PORT</Text> 调整）上同时提供管理 API、网关端点（<Text code>/v1/*</Text>）和前端管理界面。
            前端无需独立服务器，调用方只需对接 <Text code>/v1/*</Text> 网关端点。
          </div>
        }
        style={{ marginBottom: 16 }}
      />

      <Card title="1. 对外服务地址" size="small" style={{ marginBottom: 16 }}>
        <Paragraph>
          对外地址为 <Text code copyable>http://{'<你的主机>:<端口>'}/v1</Text>，按客户端所用协议选择下方端点。
          本机调试即 <Text code>http://localhost:8080/v1</Text>；对外部署时把{' '}
          <Text code>{'<主机>'}</Text> 换成服务器 IP 或域名。
        </Paragraph>
        <Table
          size="small"
          pagination={false}
          dataSource={ENDPOINTS}
          columns={[
            { title: '协议', dataIndex: 'proto', width: 160, render: (v: string) => <Tag color="blue">{v}</Tag> },
            {
              title: '方法 + 路径',
              render: (_: unknown, r: (typeof ENDPOINTS)[number]) => (
                <Text code>{r.method} {r.path}</Text>
              ),
            },
            { title: '适用客户端', dataIndex: 'client' },
          ]}
        />
      </Card>

      <Card title="2. 鉴权方式（注意：两套 API Key）" size="small" style={{ marginBottom: 16 }}>
        <Paragraph>
          调用网关时携带「<Text strong>网关 API Key</Text>」（在「系统设置 → API Key」页面创建），支持两种 header：
        </Paragraph>
        <CodeBlock>{`Authorization: Bearer <网关Key>      # OpenAI 风格
x-api-key: <网关Key>              # Anthropic 风格`}</CodeBlock>
        <Row gutter={16}>
          <Col span={12}>
            <Card size="small" type="inner" title="网关 API Key（发给调用方）" style={{ background: '#f6ffed', borderColor: '#b7eb8f' }}>
              在后台「API Key」页创建，调用方用它鉴权访问网关。SHA256 哈希存储，请求时带原文。
            </Card>
          </Col>
          <Col span={12}>
            <Card size="small" type="inner" title="厂商 Key（配置上游用）" style={{ background: '#fff7e6', borderColor: '#ffd591' }}>
              在「供应商管理」里填的 OpenAI/Anthropic 上游密钥，<Text strong>仅供网关调用上游</Text>，不可发给调用方。
            </Card>
          </Col>
        </Row>
      </Card>

      <Card title="3. 模型别名路由" size="small" style={{ marginBottom: 16 }}>
        <Paragraph>
          请求体里的 <Text code>model</Text> 字段必须填你在「模型别名」页配置的{' '}
          <Text strong>别名（alias）</Text>，由网关映射到上游供应商 + 真实模型名。别名可与上游同名（如{' '}
          <Text code>gpt-4o</Text>），也可自定义。填错会返回 <Text code>404 unknown model</Text>。
        </Paragraph>
      </Card>

      <Card title="4. 调用示例" size="small" style={{ marginBottom: 16 }}>
        <Paragraph strong>OpenAI Chat 协议</Paragraph>
        <CodeBlock>{`curl http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer <网关Key>" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "messages": [{"role":"user","content":"你好"}],
    "stream": false
  }'`}</CodeBlock>
        <Paragraph strong>Anthropic Messages 协议</Paragraph>
        <CodeBlock>{`curl http://localhost:8080/v1/messages \\
  -H "x-api-key: <网关Key>" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 256,
    "messages": [{"role":"user","content":"你好"}]
  }'`}</CodeBlock>
      </Card>

      <Card title="5. 对外部署" size="small" style={{ marginBottom: 16 }}>
        <Paragraph>
          <Text code>localhost:8080</Text> 仅本机可访问。对外服务时，将程序部署到公网/内网服务器，
          调用方使用 <Text code>http://{'<服务器IP或域名>'}:8080/v1/...</Text> 即可；
          需要 HTTPS 或域名时，前置 Nginx 反代到 <Text code>:8080</Text> 做纯转发即可，无需额外前端服务。
        </Paragraph>
      </Card>

      <Card size="small" style={{ background: '#fafafa' }}>
        <Typography>
          <QuestionCircleOutlined style={{ marginRight: 8, color: '#1677ff' }} />
          <Text type="secondary">
            提示：配置流程为「供应商管理」接入厂商并填厂商 Key →「模型别名」建别名映射上游模型 →「API Key」创建网关 Key 发给调用方。
            可在供应商列表点「测试连接」真实验证端点与密钥是否可用。
          </Text>
        </Typography>
      </Card>
    </PageContainer>
  );
}
