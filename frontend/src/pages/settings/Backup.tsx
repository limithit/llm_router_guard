// ===== 数据备份 (REQ-019) =====
// 手动备份（全配置表 JSON）/ 下载 / 从备份或粘贴 JSON 恢复 / 查看历史 / 删除
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
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
  const { t } = useTranslation();
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
      message.success(t('backup.created'));
      invalidate();
    },
    onError: () => undefined,
  });

  const restoreMutation = useMutation({
    mutationFn: backupApi.restore,
    onSuccess: () => {
      modal.success({
        title: t('backup.restoreOk'),
        content: t('backup.restoreOkContent'),
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
      message.success(t('common.deleteSuccess'));
      invalidate();
    },
    onError: () => undefined,
  });

  const confirmRestore = (id?: number, content?: string) => {
    modal.confirm({
      title: t('backup.confirmTitle'),
      content: t('backup.confirmContent'),
      okText: t('backup.confirmOk'),
      okButtonProps: { danger: true },
      onOk: () => restoreMutation.mutateAsync(id != null ? { id } : { content }),
    });
  };

  return (
    <PageContainer
      title={t('backup.title')}
      description={t('backup.desc')}
      extra={
        <Space>
          <Button icon={<CloudUploadOutlined />} onClick={() => setPasteOpen(true)}>
            {t('backup.restoreJson')}
          </Button>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            loading={createMutation.isPending}
            onClick={() => createMutation.mutate()}
          >
            {t('backup.create')}
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
          { title: t('backup.colFilename'), dataIndex: 'filename' },
          { title: t('backup.colSize'), dataIndex: 'size_bytes', width: 110, render: fmtBytes },
          { title: t('common.createdAt'), dataIndex: 'created_at', width: 180, render: fmtTime },
          {
            title: t('common.action'),
            key: 'action',
            width: 260,
            fixed: 'right',
            render: (_: unknown, row: BackupItem) => (
              <Space>
                <Button size="small" icon={<DownloadOutlined />} onClick={() => backupApi.download(row.id)}>
                  {t('backup.download')}
                </Button>
                <Button size="small" onClick={() => confirmRestore(row.id)}>
                  {t('backup.restore')}
                </Button>
                <Popconfirm
                  title={t('backup.deleteTitle')}
                  description={t('backup.deleteConfirm', { filename: row.filename })}
                  okButtonProps={{ danger: true }}
                  onConfirm={() => deleteMutation.mutate(row.id)}
                >
                  <Button size="small" danger>
                    {t('common.delete')}
                  </Button>
                </Popconfirm>
              </Space>
            ),
          },
        ]}
        pagination={{ pageSize: 10, showTotal: (total) => t('common.total', { total }) }}
      />

      {/* 从 JSON 文本/文件恢复 */}
      <Modal
        title={t('backup.pasteTitle')}
        open={pasteOpen}
        width={640}
        onCancel={() => {
          setPasteOpen(false);
          setFileContent(null);
        }}
        footer={[
          <Button key="cancel" onClick={() => setPasteOpen(false)}>
            {t('common.cancel')}
          </Button>,
          <Button
            key="ok"
            danger
            type="primary"
            loading={restoreMutation.isPending}
            disabled={!fileContent && pasteContent.trim() === ''}
            onClick={() => confirmRestore(undefined, fileContent?.text ?? pasteContent)}
          >
            {t('backup.start')}
          </Button>,
        ]}
      >
        <Alert
          type="warning"
          showIcon
          message={t('backup.pasteWarn')}
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
          <p style={{ margin: '8px 0' }}>{t('backup.dropHint')}</p>
        </Upload.Dragger>
        {fileContent ? (
          <Tag color="blue" style={{ marginTop: 8 }}>
            {t('backup.selected', { name: fileContent.name })}
          </Tag>
        ) : (
          <Input.TextArea
            rows={8}
            style={{ marginTop: 12 }}
            placeholder={t('backup.pastePh')}
            value={pasteContent}
            onChange={(e) => setPasteContent(e.target.value)}
          />
        )}
      </Modal>
    </PageContainer>
  );
}
