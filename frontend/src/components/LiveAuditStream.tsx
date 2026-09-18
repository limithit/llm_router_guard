/**
 * LiveAuditStream — 调用日志页的实时调用流卡片。
 * WebSocket 订阅 /audit/ws，新事件实时追加（最多保留 30 条）；
 * 暂停 = 冻结当前快照（后台继续接收，恢复时展示最新流）；清空 = 丢弃已收列表。
 */
import { useMemo, useState } from 'react';
import { Button, Card, List, Space, Tag, Tooltip, Typography } from 'antd';
import { ClearOutlined, PauseCircleOutlined, PlayCircleOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { auditStreamPhaseText, useAuditStream } from '../hooks/useAuditStream';
import { CALL_STATUS_META, colorOf, useDictLabel } from '../constants/dicts';
import { fmtNumber, fmtTime } from '../utils/format';

export default function LiveAuditStream() {
  const { t } = useTranslation();
  const statusLbl = useDictLabel('callStatus');
  const stream = useAuditStream(true, 30);

  // 暂停语义：frozen 非空 = 冻结快照；totalAtPause 记录暂停时刻，用于「暂停期间新增」角标
  const [frozen, setFrozen] = useState<ReturnType<typeof useAuditStream>['events'] | null>(null);
  const [totalAtPause, setTotalAtPause] = useState(0);

  const shown = frozen ?? stream.events;
  const pending = frozen ? stream.total - totalAtPause : 0;

  const phaseColor = stream.phase === 'live' ? 'green' : stream.phase === 'connecting' ? 'gold' : 'red';

  return (
    <Card
      size="small"
      title={
        <Space size={8}>
          <span>{t('calls.live.title')}</span>
          <Tag color={phaseColor} style={{ marginInlineEnd: 0 }}>
            {auditStreamPhaseText(stream.phase)}
          </Tag>
          {frozen && pending > 0 && <Tag color="orange">+{pending}</Tag>}
        </Space>
      }
      extra={
        <Space size={4}>
          <Tooltip title={frozen ? t('calls.live.resume') : t('calls.live.pause')}>
            <Button
              size="small"
              type="text"
              icon={frozen ? <PlayCircleOutlined /> : <PauseCircleOutlined />}
              onClick={() => {
                if (frozen) {
                  setFrozen(null);
                } else {
                  setFrozen(stream.events);
                  setTotalAtPause(stream.total);
                }
              }}
            />
          </Tooltip>
          <Tooltip title={t('calls.live.clear')}>
            <Button
              size="small"
              type="text"
              icon={<ClearOutlined />}
              onClick={() => {
                stream.reset();
                setFrozen(null);
              }}
            />
          </Tooltip>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {t('calls.live.total', { n: stream.total })}
          </Typography.Text>
        </Space>
      }
    >
      <div style={{ maxHeight: 260, overflow: 'auto' }}>
        <List
          size="small"
          dataSource={shown}
          locale={{ emptyText: t('calls.live.empty') }}
          renderItem={(row) => (
            <List.Item style={{ padding: '4px 0' }}>
              <Space size={8} style={{ width: '100%', justifyContent: 'space-between' }}>
                <Space size={8}>
                  <Tag color={colorOf(CALL_STATUS_META, row.status)} style={{ marginInlineEnd: 0 }}>
                    {statusLbl(row.status)}
                  </Tag>
                  <Typography.Text strong style={{ fontSize: 12 }}>
                    {row.model_alias}
                  </Typography.Text>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }} ellipsis>
                    {row.api_key_label}
                  </Typography.Text>
                </Space>
                <Space size={10}>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {fmtNumber(row.prompt_tokens + row.completion_tokens)} tok · {fmtNumber(row.latency_ms)} ms
                  </Typography.Text>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {fmtTime(row.created_at)}
                  </Typography.Text>
                </Space>
              </Space>
            </List.Item>
          )}
        />
      </div>
    </Card>
  );
}
