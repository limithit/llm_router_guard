// ===== 敏感词管理 (REQ-008) =====
// 列表 + 分类/动作/启用/关键词筛选 + CRUD + 批量导入（粘贴或上传 CSV）+ 导出 CSV + 测试工具
import { useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import {
  App,
  Button,
  Drawer,
  Form,
  Input,
  Modal,
  Popconfirm,
  Radio,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  Upload,
} from 'antd';
import {
  DownloadOutlined,
  ExperimentOutlined,
  PlusOutlined,
  UploadOutlined,
} from '@ant-design/icons';
import type { UploadFile } from 'antd';
import PageContainer from '../../components/PageContainer';
import StatusSwitch from '../../components/StatusSwitch';
import i18n from '../../i18n';
import { keywordApi } from '../../api/endpoints';
import {
  ACTION_META,
  KEYWORD_CATEGORY_META,
  MATCH_MODE_META,
  useDictLabel,
  useDictOptions,
  dictColor,
} from '../../constants/dicts';
import { fmtTime } from '../../utils/format';
import type { GuardHit, Keyword, KeywordCategory, KeywordInput, MatchMode, RuleAction } from '../../api/types';

interface KeywordFormValues {
  word: string;
  category: KeywordCategory;
  match_mode: MatchMode;
  action: RuleAction;
  enabled: boolean;
}

const CSV_HEADER = 'word,category,match_mode,action';

export default function Keywords() {
  const { t } = useTranslation();
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [filters, setFilters] = useState<{
    keyword: string;
    category: KeywordCategory | '';
    enabled: boolean | null;
    action: RuleAction | '';
  }>({ keyword: '', category: '', enabled: null, action: '' });
  const applied = { ...filters };

  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<Keyword | null>(null);
  const [form] = Form.useForm<KeywordFormValues>();

  const [importOpen, setImportOpen] = useState(false);
  const [importMode, setImportMode] = useState<'paste' | 'upload'>('paste');
  const [pasteText, setPasteText] = useState('');
  const [csvFile, setCsvFile] = useState<File | null>(null);

  const [testOpen, setTestOpen] = useState(false);
  const [testText, setTestText] = useState('');
  const [testLoading, setTestLoading] = useState(false);
  const [testResult, setTestResult] = useState<{ blocked: boolean; hits: GuardHit[] } | null>(null);

  // ── i18n dicts ────────────────────────────────────────────────
  const catOpts   = useDictOptions('keywordCategory', KEYWORD_CATEGORY_META);
  const actOpts   = useDictOptions('action', ACTION_META).filter((a) => a.value !== 'mask');
  const modeOpts  = useDictOptions('matchMode', MATCH_MODE_META);
  const catLbl    = useDictLabel('keywordCategory');
  const modeLbl   = useDictLabel('matchMode');
  const actLbl    = useDictLabel('action');

  const queryEnabled = (v: boolean | null) => (v === null ? undefined : v);

  const { data, isLoading } = useQuery({
    queryKey: ['keywords', page, pageSize, applied],
    queryFn: () =>
      keywordApi.list({
        page,
        page_size: pageSize,
        keyword: applied.keyword || undefined,
        category: applied.category || undefined,
        enabled: queryEnabled(applied.enabled),
        action: applied.action || undefined,
      }),
    placeholderData: keepPreviousData,
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['keywords'] });

  const saveMutation = useMutation({
    mutationFn: (body: KeywordInput) =>
      editing ? keywordApi.update(editing.id, body) : keywordApi.create(body),
    onSuccess: () => {
      message.success(editing ? t('keywords.savedEdit') : t('keywords.savedNew'));
      setModalOpen(false);
      invalidate();
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: keywordApi.remove,
    onSuccess: () => {
      message.success(t('keywords.deleted'));
      invalidate();
    },
    onError: () => undefined,
  });

  const importMutation = useMutation({
    mutationFn: keywordApi.importCsv,
    onSuccess: (res) => {
      message.success(t('keywords.importDone', { created: res.created, skipped: res.skipped }));
      setImportOpen(false);
      setPasteText('');
      setCsvFile(null);
      invalidate();
    },
    onError: () => undefined,
  });

  const toggleEnabled = async (record: Keyword, enabled: boolean) => {
    await keywordApi.update(record.id, {
      word: record.word,
      category: record.category,
      match_mode: record.match_mode,
      action: record.action,
      enabled,
    });
    invalidate();
  };

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ category: 'political', match_mode: 'contains', action: 'block', enabled: true });
    setModalOpen(true);
  };

  const openEdit = (record: Keyword) => {
    setEditing(record);
    form.setFieldsValue({
      word: record.word,
      category: record.category,
      match_mode: record.match_mode,
      action: record.action,
      enabled: record.enabled,
    });
    setModalOpen(true);
  };

  const handleSave = async () => {
    const v = await form.validateFields();
    saveMutation.mutate({
      word: v.word.trim(),
      category: v.category,
      match_mode: v.match_mode,
      action: v.action,
      enabled: v.enabled ?? true,
    });
  };

  const readFileAsText = (file: File): Promise<string> =>
    new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result ?? ''));
      reader.onerror = () => reject(new Error(i18n.t('keywords.fileReadFailed')));
      reader.readAsText(file, 'utf-8');
    });

  const doImport = async () => {
    if (importMode === 'paste') {
      if (!pasteText.trim()) {
        message.warning(t('keywords.pasteFirst'));
        return;
      }
      importMutation.mutate(pasteText);
      return;
    }
    if (!csvFile) {
      message.warning(t('keywords.selectFileFirst'));
      return;
    }
    try {
      const text = await readFileAsText(csvFile);
      importMutation.mutate(text);
    } catch (e) {
      message.error(e instanceof Error ? e.message : i18n.t('keywords.fileReadFailed'));
    }
  };

  const runTest = async () => {
    if (!testText.trim()) {
      message.warning(t('keywords.testTextRequired'));
      return;
    }
    setTestLoading(true);
    try {
      const res = await keywordApi.test(testText);
      setTestResult(res);
    } catch {
      // interceptor already showed error
    } finally {
      setTestLoading(false);
    }
  };

  return (
    <PageContainer
      title={t('keywords.title')}
      description={t('keywords.description')}
      extra={
        <>
          <Button icon={<ExperimentOutlined />} onClick={() => setTestOpen(true)}>
            {t('keywords.testTool')}
          </Button>
          <Button icon={<DownloadOutlined />} onClick={() => void keywordApi.exportCsv()}>
            {t('keywords.exportCsv')}
          </Button>
          <Button icon={<UploadOutlined />} onClick={() => setImportOpen(true)}>
            {t('keywords.bulkImport')}
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            {t('keywords.add')}
          </Button>
        </>
      }
    >
      <Space wrap style={{ marginBottom: 12 }}>
        <Input.Search
          allowClear
          placeholder={t('keywords.searchPlaceholder')}
          style={{ width: 200 }}
          onSearch={(v) => {
            setPage(1);
            setFilters((f) => ({ ...f, keyword: v }));
          }}
        />
        <Select
          allowClear
          placeholder={t('keywords.categoryPlaceholder')}
          style={{ width: 130 }}
          options={catOpts}
          value={filters.category || undefined}
          onChange={(v: KeywordCategory) => {
            setPage(1);
            setFilters((f) => ({ ...f, category: v ?? '' }));
          }}
        />
        <Select
          allowClear
          placeholder={t('keywords.actionPlaceholder')}
          style={{ width: 120 }}
          options={actOpts}
          value={filters.action || undefined}
          onChange={(v: RuleAction) => {
            setPage(1);
            setFilters((f) => ({ ...f, action: v ?? '' }));
          }}
        />
        <Select
          allowClear
          placeholder={t('keywords.enabledFilter')}
          style={{ width: 120 }}
          options={[
            { value: true, label: t('keywords.enabledTrue') },
            { value: false, label: t('keywords.enabledFalse') },
          ]}
          value={filters.enabled ?? undefined}
          onChange={(v: boolean) => {
            setPage(1);
            setFilters((f) => ({ ...f, enabled: v ?? null }));
          }}
        />
        <Button
          onClick={() => {
            setPage(1);
            setFilters({ keyword: '', category: '', enabled: null, action: '' });
          }}
        >
          {t('common.reset')}
        </Button>
      </Space>

      <Table<Keyword>
        rowKey="id"
        loading={isLoading || deleteMutation.isPending}
        dataSource={data?.items ?? []}
        scroll={{ x: 860 }}
        columns={[
          { title: t('keywords.colId'), dataIndex: 'id', width: 60, render: (v: number) => <Tag>{v}</Tag> },
          { title: t('keywords.colWord'), dataIndex: 'word', ellipsis: true },
          {
            title: t('keywords.colCategory'),
            dataIndex: 'category',
            width: 110,
            render: (v: KeywordCategory) => (
              <Tag color={dictColor(KEYWORD_CATEGORY_META, v)}>{catLbl(v)}</Tag>
            ),
          },
          {
            title: t('keywords.colMatchMode'),
            dataIndex: 'match_mode',
            width: 120,
            render: (v: MatchMode) => modeLbl(v),
          },
          {
            title: t('keywords.colAction'),
            dataIndex: 'action',
            width: 90,
            render: (v: RuleAction) => (
              <Tag color={dictColor(ACTION_META, v)}>{actLbl(v)}</Tag>
            ),
          },
          {
            title: t('keywords.colEnabled'),
            dataIndex: 'enabled',
            width: 80,
            render: (_: unknown, record: Keyword) => (
              <StatusSwitch checked={record.enabled} onChange={(c) => toggleEnabled(record, c)} />
            ),
          },
          { title: t('keywords.colCreatedAt'), dataIndex: 'created_at', width: 160, render: (v: string) => fmtTime(v) },
          {
            title: t('keywords.colOperation'),
            key: 'action',
            width: 140,
            fixed: 'right',
            render: (_: unknown, record: Keyword) => (
              <Space>
                <Button size="small" onClick={() => openEdit(record)}>
                  {t('keywords.edit')}
                </Button>
                <Popconfirm
                  title={t('keywords.deleteTitle')}
                  description={t('keywords.deleteConfirm', { word: record.word })}
                  okButtonProps={{ danger: true }}
                  onConfirm={() => deleteMutation.mutate(record.id)}
                >
                  <Button size="small" danger>
                    {t('keywords.delete')}
                  </Button>
                </Popconfirm>
              </Space>
            ),
          },
        ]}
        pagination={{
          current: page,
          pageSize,
          total: data?.total ?? 0,
          showSizeChanger: true,
          showTotal: (n) => t('common.total', { total: n }),
        }}
        onChange={(p) => {
          setPage(p.current ?? 1);
          setPageSize(p.pageSize ?? 10);
        }}
      />

      {/* Create / Edit */}
      <Modal
        title={editing ? t('keywords.editTitle', { id: editing.id }) : t('keywords.addTitle')}
        open={modalOpen}
        confirmLoading={saveMutation.isPending}
        onOk={handleSave}
        onCancel={() => setModalOpen(false)}
        width={480}
        destroyOnClose
      >
        <Form<KeywordFormValues> form={form} layout="vertical">
          <Form.Item
            name="word"
            label={t('keywords.wordLabel')}
            rules={[{ required: true, message: t('keywords.wordRequired') }]}
          >
            <Input placeholder={t('keywords.wordPlaceholder')} />
          </Form.Item>
          <Space size={16} align="start" wrap>
            <Form.Item name="category" label={t('keywords.catLabel')} rules={[{ required: true }]}>
              <Select style={{ width: 160 }} options={catOpts} />
            </Form.Item>
            <Form.Item name="match_mode" label={t('keywords.matchModeLabel')} rules={[{ required: true }]}>
              <Select style={{ width: 160 }} options={modeOpts} />
            </Form.Item>
          </Space>
          <Space size={16} align="start" wrap>
            <Form.Item name="action" label={t('keywords.actLabel')} rules={[{ required: true }]}>
              <Select style={{ width: 160 }} options={actOpts} />
            </Form.Item>
            <Form.Item name="enabled" label={t('keywords.enabledLabel')} valuePropName="checked">
              <Switch />
            </Form.Item>
          </Space>
        </Form>
      </Modal>

      {/* Bulk Import */}
      <Modal
        title={t('keywords.importTitle')}
        open={importOpen}
        confirmLoading={importMutation.isPending}
        onOk={doImport}
        okText={t('keywords.importOk')}
        onCancel={() => setImportOpen(false)}
        width={640}
      >
        <Typography.Paragraph type="secondary">
          {t('keywords.importFormat', { header: CSV_HEADER })}
          <br />
          {t('keywords.importEnums')}
        </Typography.Paragraph>
        <Radio.Group
          value={importMode}
          onChange={(e) => setImportMode(e.target.value)}
          style={{ marginBottom: 12 }}
        >
          <Radio.Button value="paste">{t('keywords.importPaste')}</Radio.Button>
          <Radio.Button value="upload">{t('keywords.importUpload')}</Radio.Button>
        </Radio.Group>
        {importMode === 'paste' ? (
          <Input.TextArea
            rows={8}
            placeholder={t('keywords.importPastePlaceholder')}
            value={pasteText}
            onChange={(e) => setPasteText(e.target.value)}
          />
        ) : (
          <Upload
            accept=".csv,text/csv"
            maxCount={1}
            beforeUpload={(file) => {
              setCsvFile(file);
              return false; // prevent auto-upload
            }}
            onRemove={() => setCsvFile(null)}
            fileList={
              csvFile
                ? ([{ uid: '-1', name: csvFile.name, status: 'done' }] as UploadFile[])
                : []
            }
          >
            <Button icon={<UploadOutlined />}>{t('keywords.selectFile')}</Button>
          </Upload>
        )}
      </Modal>

      {/* Test Drawer */}
      <Drawer
        title={t('keywords.testDrawerTitle')}
        open={testOpen}
        onClose={() => setTestOpen(false)}
        width={560}
        extra={
          <Button type="primary" loading={testLoading} onClick={runTest}>
            {t('keywords.testRun')}
          </Button>
        }
      >
        <Typography.Paragraph type="secondary">{t('keywords.testHint')}</Typography.Paragraph>
        <Input.TextArea
          rows={6}
          placeholder={t('keywords.testPlaceholder')}
          value={testText}
          onChange={(e) => setTestText(e.target.value)}
        />
        {testResult ? (
          <div style={{ marginTop: 16 }}>
            <Typography.Text strong>
              {t('keywords.testResultLabel')}
              {testResult.blocked ? (
                <Tag color="error" style={{ marginLeft: 8 }}>
                  {t('keywords.testBlocked')}
                </Tag>
              ) : (
                <Tag color="success" style={{ marginLeft: 8 }}>
                  {t('keywords.testNotBlocked')}
                </Tag>
              )}
            </Typography.Text>
            {testResult.hits.length === 0 ? (
              <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
                {t('keywords.testNoHits')}
              </Typography.Paragraph>
            ) : (
              <Table<GuardHit>
                style={{ marginTop: 12 }}
                rowKey={(r, i) => `${r.type}-${r.value}-${i}`}
                size="small"
                pagination={false}
                dataSource={testResult.hits}
                columns={[
                  { title: t('keywords.colHitType'), dataIndex: 'type', width: 100 },
                  { title: t('keywords.colHitValue'), dataIndex: 'value', ellipsis: true },
                  {
                    title: t('keywords.colCategory'),
                    dataIndex: 'category',
                    width: 120,
                    render: (v: string) => catLbl(v),
                  },
                  {
                    title: t('keywords.colAction'),
                    dataIndex: 'action',
                    width: 100,
                    render: (v: string) => (
                      <Tag color={dictColor(ACTION_META, v)}>{actLbl(v)}</Tag>
                    ),
                  },
                ]}
              />
            )}
          </div>
        ) : null}
      </Drawer>
    </PageContainer>
  );
}
