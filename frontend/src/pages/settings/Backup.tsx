// ===== 数据备份 (REQ-019) =====
// 手动备份（全配置表 JSON）/ 下载 / 从备份或粘贴 JSON 恢复 / 查看历史 / 删除
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  App,
  Button,
  Input,
  Modal,
  Popconfirm,
  Space,
  Table,
  Tag,
  Upload,
} from 'antd';
import { CloudUploadOutlined, DownloadOutlined, PlusOutlined } from '@ant-design/icons';
import PageContainer from '../../components/PageContainer';
import { backupApi } from '../../api/endpoints';
import { fmtBytes, fmtTime } from '../../utils/format';
import type { BackupItem } from '../../api/types';

export default function Backup() {
  const { message, modal } = App.useApp();
  const queryClient = useQueryClient();
  const [pasteOpen, setPasteOpen] = useState(false);
  const [pasteContent, setPasteContent] = useState('');
  const [fileContent, setFileContent] = useState<{ name: string; text: string } | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ['backups'],
    queryFn: () => backupApi.list(),
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['backups'] });

  const createMutation = useMutation({
    mutationFn: () => backupApi.create(),
    onSuccess: () => {
      message.success('备份完成');
      invalidate();
    },
    onError: () => undefined,
  });

  const restoreMutation = useMutation({
    mutationFn: backupApi.restore,
    onSuccess: () => {
      modal.success({
        title: '恢复完成',
        content: '配置已从备份恢复并触发热加载，≤3 秒内生效。',
      });
      setPasteOpen(false);
      setPasteContent('');
      setFileContent(null);
      invalidate();
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: backupApi.remove,
    onSuccess: () => {
      message.success('已删除');
      invalidate();
    },
    onError: () => undefined,
  });

  const confirmRestore = (id?: number, content?: string) => {
    modal.confirm({
      title: '确认恢复配置？',
      content: '恢复将覆盖当前全部配置（供应商/模型别名/护栏/配额/限流/API Key/系统设置），且立即生效。',
      okText: '确认恢复',
      okButtonProps: { danger: true },
      onOk: () => restoreMutation.mutateAsync(id != null ? { id } : { content }),
    });
  };

  return (
    <PageContainer
      title="数据备份"
      description="备份并恢复全部配置数据（REQ-019）。备份为 JSON 文件，包含所有配置表。"
      extra={
        <Space>
          <Button icon={<CloudUploadOutlined />} onClick={() => setPasteOpen(true)}>
            从 JSON 恢复
          </Button>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            loading={createMutation.isPending}
            onClick={() => createMutation.mutate()}
          >
            立即备份
          </Button>
        </Space>
      }
    >
      <Table<BackupItem>
        rowKey="id"
        loading={isLoading}
        dataSource={data?.items ?? []}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 60, render: (v: number) => <Tag>{v}</Tag> },
          { title: '文件名', dataIndex: 'filename' },
          { title: '大小', dataIndex: 'size_bytes', width: 110, render: fmtBytes },
          { title: '创建时间', dataIndex: 'created_at', width: 180, render: fmtTime },
          {
            title: '操作',
            key: 'action',
            width: 260,
            fixed: 'right',
            render: (_: unknown, row: BackupItem) => (
              <Space>
                <Button size="small" icon={<DownloadOutlined />} onClick={() => backupApi.download(row.id)}>
                  下载
                </Button>
                <Button size="small" onClick={() => confirmRestore(row.id)}>
                  恢复
                </Button>
                <Popconfirm
                  title="删除备份"
                  description={`确认删除「${row.filename}」？`}
                  okButtonProps={{ danger: true }}
                  onConfirm={() => deleteMutation.mutate(row.id)}
                >
                  <Button size="small" danger>
                    删除
                  </Button>
                </Popconfirm>
              </Space>
            ),
          },
        ]}
        pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 条` }}
      />

      {/* 从 JSON 文本/文件恢复 */}
      <Modal
        title="从备份 JSON 恢复"
        open={pasteOpen}
        width={640}
        onCancel={() => {
          setPasteOpen(false);
          setFileContent(null);
        }}
        footer={[
          <Button key="cancel" onClick={() => setPasteOpen(false)}>
            取消
          </Button>,
          <Button
            key="ok"
            danger
            type="primary"
            loading={restoreMutation.isPending}
            disabled={!fileContent && pasteContent.trim() === ''}
            onClick={() => confirmRestore(undefined, fileContent?.text ?? pasteContent)}
          >
            开始恢复
          </Button>,
        ]}
      >
        <Alert
          type="warning"
          showIcon
          message="恢复将覆盖当前全部配置并立即生效，请谨慎操作。"
          style={{ marginBottom: 12 }}
        />
        <Upload.Dragger
          accept=".json,application/json"
          maxCount={1}
          beforeUpload={(file) => {
            const reader = new FileReader();
            reader.onload = (e) =>
              setFileContent({ name: file.name, text: String(e.target?.result ?? '') });
            reader.readAsText(file);
            return false; // 阻止自动上传，本地解析
          }}
          onRemove={() => setFileContent(null)}
        >
          <p style={{ margin: '8px 0' }}>点击或拖拽备份 JSON 文件到此处</p>
        </Upload.Dragger>
        {fileContent ? (
          <Tag color="blue" style={{ marginTop: 8 }}>
            已选择：{fileContent.name}
          </Tag>
        ) : (
          <Input.TextArea
            rows={8}
            style={{ marginTop: 12 }}
            placeholder="或直接粘贴备份 JSON 内容"
            value={pasteContent}
            onChange={(e) => setPasteContent(e.target.value)}
          />
        )}
      </Modal>
    </PageContainer>
  );
}
