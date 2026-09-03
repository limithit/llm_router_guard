import { Typography } from 'antd';
import i18n from '../i18n';

interface JsonViewProps {
  /** JSON 字符串或对象 */
  value: string | Record<string, unknown> | unknown[] | null | undefined;
  maxHeight?: number;
}

/**
 * 将任意 JSON 字符串/对象格式化为可读 <pre> 展示。
 * 解析失败时原样展示字符串。
 */
export default function JsonView({ value, maxHeight = 320 }: JsonViewProps) {
  let text = '-';
  if (value != null) {
    if (typeof value === 'string') {
      if (value.trim() === '') {
        text = i18n.t('common.empty');
      } else {
        try {
          text = JSON.stringify(JSON.parse(value), null, 2);
        } catch {
          text = value;
        }
      }
    } else {
      text = JSON.stringify(value, null, 2);
    }
  }
  return (
    <Typography.Paragraph
      style={{
        margin: 0,
        padding: 8,
        background: '#f6f6f6',
        borderRadius: 6,
        maxHeight,
        overflow: 'auto',
        fontFamily: 'SFMono-Regular, Consolas, "Liberation Mono", Menlo, monospace',
        fontSize: 12,
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-all',
      }}
    >
      {text}
    </Typography.Paragraph>
  );
}
