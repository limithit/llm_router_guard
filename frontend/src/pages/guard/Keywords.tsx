// ===== 敏感词管理 (REQ-008) =====
// 列表 + 分类/动作/启用/关键词筛选 + CRUD + 批量导入（粘贴或上传 CSV）+ 导出 CSV + 测试工具
import { useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keepPreviousData } from '@tanstack/react-query';
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
import { keywordApi } from '../../api/endpoints';
import {
  ACTION_MAP,
  ACTION_OPTIONS,
  CATEGORY_MAP,
  KEYWORD_CATEGORY_OPTIONS,
  MATCH_MODE_MAP,
  MATCH_MODE_OPTIONS,
  colorOf,
  labelOf,
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
      message.success(editing ? '敏感词已更新' : '敏感词已添加');
      setModalOpen(false);
      invalidate();
    },
    onError: () => undefined,
  });

  const deleteMutation = useMutation({
    mutationFn: keywordApi.remove,
    onSuccess: () => {
      message.success('已删除');
      invalidate();
    },
    onError: () => undefined,
  });

  const importMutation = useMutation({
    mutationFn: keywordApi.importCsv,
    onSuccess: (res) => {
      message.success(`导入完成：新增 ${res.created} 条，跳过 ${res.skipped} 条`);
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
      reader.onerror = () => reject(new Error('文件读取失败'));
      reader.readAsText(file, 'utf-8');
    });

  const doImport = async () => {
    if (importMode === 'paste') {
      if (!pasteText.trim()) {
        message.warning('请先粘贴 CSV 内容');
        return;
      }
      importMutation.mutate(pasteText);
      return;
    }
    if (!csvFile) {
      message.warning('请选择 CSV 文件');
      return;
    }
    try {
      const text = await readFileAsText(csvFile);
      importMutation.mutate(text);
    } catch (e) {
      message.error(e instanceof Error ? e.message : '文件读取失败');
    }
  };

  const runTest = async () => {
    if (!testText.trim()) {
      message.warning('请输入要测试的文本');
      return;
    }
    setTestLoading(true);
    try {
      const res = await keywordApi.test(testText);
      setTestResult(res);
    } catch {
      // 拦截器已提示
    } finally {
      setTestLoading(false);
    }
  };

  return (
    <PageContainer
      title="敏感词管理"
      description="管理敏感词库（REQ-008），支持分类/模式/动作/启用筛选、批量导入导出与命中测试。"
      extra={
        <>
          <Button icon={<ExperimentOutlined />} onClick={() => setTestOpen(true)}>
            测试工具
          </Button>
          <Button icon={<DownloadOutlined />} onClick={() => void keywordApi.exportCsv()}>
            导出 CSV
          </Button>
          <Button icon={<UploadOutlined />} onClick={() => setImportOpen(true)}>
            批量导入
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            添加敏感词
          </Button>
        </>
      }
    >
      <Space wrap style={{ marginBottom: 12 }}>
        <Input.Search
          allowClear
          placeholder="搜索关键词"
          style={{ width: 200 }}
          onSearch={(v) => {
            setPage(1);
            setFilters((f) => ({ ...f, keyword: v }));
          }}
        />
        <Select
          allowClear
          placeholder="分类"
          style={{ width: 130 }}
          options={KEYWORD_CATEGORY_OPTIONS}
          value={filters.category || undefined}
          onChange={(v: KeywordCategory) => {
            setPage(1);
            setFilters((f) => ({ ...f, category: v ?? '' }));
          }}
        />
        <Select
          allowClear
          placeholder="动作"
          style={{ width: 120 }}
          options={ACTION_OPTIONS.filter((a) => a.value !== 'mask')}
          value={filters.action || undefined}
          onChange={(v: RuleAction) => {
            setPage(1);
            setFilters((f) => ({ ...f, action: v ?? '' }));
          }}
        />
        <Select
          allowClear
          placeholder="启用状态"
          style={{ width: 120 }}
          options={[
            { value: true, label: '启用' },
            { value: false, label: '停用' },
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
          重置
        </Button>
      </Space>

      <Table<Keyword>
        rowKey="id"
        loading={isLoading || deleteMutation.isPending}
        dataSource={data?.items ?? []}
        scroll={{ x: 860 }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 60, render: (v: number) => <Tag>{v}</Tag> },
          { title: '关键词', dataIndex: 'word', ellipsis: true },
          {
            title: '分类',
            dataIndex: 'category',
            width: 110,
            render: (v: KeywordCategory) => (
              <Tag color={colorOf(CATEGORY_MAP.colors, v)}>{labelOf(CATEGORY_MAP.labels, v)}</Tag>
            ),
          },
          {
            title: '匹配模式',
            dataIndex: 'match_mode',
            width: 120,
            render: (v: MatchMode) => labelOf(MATCH_MODE_MAP.labels, v),
          },
          {
            title: '动作',
            dataIndex: 'action',
            width: 90,
            render: (v: RuleAction) => (
              <Tag color={colorOf(ACTION_MAP.colors, v)}>{labelOf(ACTION_MAP.labels, v)}</Tag>
            ),
          },
          {
            title: '启用',
            dataIndex: 'enabled',
            width: 80,
            render: (_: unknown, record: Keyword) => (
              <StatusSwitch checked={record.enabled} onChange={(c) => toggleEnabled(record, c)} />
            ),
          },
          { title: '创建时间', dataIndex: 'created_at', width: 160, render: (v: string) => fmtTime(v) },
          {
            title: '操作',
            key: 'action',
            width: 140,
            fixed: 'right',
            render: (_: unknown, record: Keyword) => (
              <Space>
                <Button size="small" onClick={() => openEdit(record)}>
                  编辑
                </Button>
                <Popconfirm
                  title="删除敏感词"
                  description={`确认删除「${record.word}」？`}
                  okButtonProps={{ danger: true }}
                  onConfirm={() => deleteMutation.mutate(record.id)}
                >
                  <Button size="small" danger>
                    删除
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
          showTotal: (t) => `共 ${t} 条`,
        }}
        onChange={(p) => {
          setPage(p.current ?? 1);
          setPageSize(p.pageSize ?? 10);
        }}
      />

      {/* 新增/编辑 */}
      <Modal
        title={editing ? `编辑敏感词 #${editing.id}` : '添加敏感词'}
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
            label="关键词文本"
            rules={[{ required: true, message: '请输入关键词' }]}
          >
            <Input placeholder="要拦截/告警/记录的关键词" />
          </Form.Item>
          <Space size={16} align="start" wrap>
            <Form.Item name="category" label="所属类别" rules={[{ required: true }]}>
              <Select style={{ width: 160 }} options={KEYWORD_CATEGORY_OPTIONS} />
            </Form.Item>
            <Form.Item name="match_mode" label="匹配模式" rules={[{ required: true }]}>
              <Select style={{ width: 160 }} options={MATCH_MODE_OPTIONS} />
            </Form.Item>
          </Space>
          <Space size={16} align="start" wrap>
            <Form.Item name="action" label="处理动作" rules={[{ required: true }]}>
              <Select style={{ width: 160 }} options={ACTION_OPTIONS.filter((a) => a.value !== 'mask')} />
            </Form.Item>
            <Form.Item name="enabled" label="启用" valuePropName="checked">
              <Switch />
            </Form.Item>
          </Space>
        </Form>
      </Modal>

      {/* 批量导入 */}
      <Modal
        title="批量导入敏感词（CSV）"
        open={importOpen}
        confirmLoading={importMutation.isPending}
        onOk={doImport}
        okText="开始导入"
        onCancel={() => setImportOpen(false)}
        width={640}
      >
        <Typography.Paragraph type="secondary">
          CSV 格式（每行一条，第一行可为表头也可省略）：{CSV_HEADER}
          <br />
          枚举取值：category 为 political/porn/violence/illegal/discrimination/custom；match_mode 为
          contains/exact/regex；action 为 block/warn/log；enabled 可省略（默认 true）。
        </Typography.Paragraph>
        <Radio.Group
          value={importMode}
          onChange={(e) => setImportMode(e.target.value)}
          style={{ marginBottom: 12 }}
        >
          <Radio.Button value="paste">粘贴文本</Radio.Button>
          <Radio.Button value="upload">上传 CSV 文件</Radio.Button>
        </Radio.Group>
        {importMode === 'paste' ? (
          <Input.TextArea
            rows={8}
            placeholder={'例如：\n赌博,political,contains,block\n暴力催收,violence,exact,block'}
            value={pasteText}
            onChange={(e) => setPasteText(e.target.value)}
          />
        ) : (
          <Upload
            accept=".csv,text/csv"
            maxCount={1}
            beforeUpload={(file) => {
              setCsvFile(file);
              return false; // 阻止自动上传
            }}
            onRemove={() => setCsvFile(null)}
            fileList={
              csvFile
                ? ([{ uid: '-1', name: csvFile.name, status: 'done' }] as UploadFile[])
                : []
            }
          >
            <Button icon={<UploadOutlined />}>选择 CSV 文件</Button>
          </Upload>
        )}
      </Modal>

      {/* 测试工具 */}
      <Drawer
        title="敏感词命中测试"
        open={testOpen}
        onClose={() => setTestOpen(false)}
        width={560}
        extra={
          <Button type="primary" loading={testLoading} onClick={runTest}>
            开始测试
          </Button>
        }
      >
        <Typography.Paragraph type="secondary">输入一段文本，检测其中命中的敏感词与处理动作。</Typography.Paragraph>
        <Input.TextArea
          rows={6}
          placeholder="输入待检测文本…"
          value={testText}
          onChange={(e) => setTestText(e.target.value)}
        />
        {testResult ? (
          <div style={{ marginTop: 16 }}>
            <Typography.Text strong>
              检测结果：
              {testResult.blocked ? (
                <Tag color="error" style={{ marginLeft: 8 }}>
                  判定为需拦截
                </Tag>
              ) : (
                <Tag color="success" style={{ marginLeft: 8 }}>
                  未命中拦截动作
                </Tag>
              )}
            </Typography.Text>
            {testResult.hits.length === 0 ? (
              <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
                无命中。
              </Typography.Paragraph>
            ) : (
              <Table<GuardHit>
                style={{ marginTop: 12 }}
                rowKey={(r, i) => `${r.type}-${r.value}-${i}`}
                size="small"
                pagination={false}
                dataSource={testResult.hits}
                columns={[
                  { title: '类型', dataIndex: 'type', width: 100 },
                  { title: '命中内容', dataIndex: 'value', ellipsis: true },
                  {
                    title: '分类',
                    dataIndex: 'category',
                    width: 120,
                    render: (v: string) => labelOf(CATEGORY_MAP.labels, v),
                  },
                  {
                    title: '动作',
                    dataIndex: 'action',
                    width: 100,
                    render: (v: string) => (
                      <Tag color={colorOf(ACTION_MAP.colors, v)}>{labelOf(ACTION_MAP.labels, v)}</Tag>
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
