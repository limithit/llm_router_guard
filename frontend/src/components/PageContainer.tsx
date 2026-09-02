import type { ReactNode } from 'react';
import { Card, Typography } from 'antd';

interface PageContainerProps {
  title: ReactNode;
  /** 页面说明（可选，放在标题下方） */
  description?: ReactNode;
  /** 标题行右侧操作区（如新建按钮） */
  extra?: ReactNode;
  children: ReactNode;
  bodyStyle?: React.CSSProperties;
}

/**
 * 页面通用容器：标题 + 说明 + 右侧操作区 + 内容
 */
export default function PageContainer({
  title,
  description,
  extra,
  children,
}: PageContainerProps) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-start',
          flexWrap: 'wrap',
          gap: 8,
        }}
      >
        <div>
          <Typography.Title level={4} style={{ margin: 0 }}>
            {title}
          </Typography.Title>
          {description ? (
            <Typography.Text type="secondary">{description}</Typography.Text>
          ) : null}
        </div>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>{extra}</div>
      </div>
      <Card size="small" styles={{ body: { paddingTop: 12 } }}>
        {children}
      </Card>
    </div>
  );
}
