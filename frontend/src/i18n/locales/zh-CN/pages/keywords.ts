export default {
  // Page title & description
  'keywords.title': '敏感词管理',
  'keywords.description': '管理敏感词库（REQ-008），支持分类/模式/动作/启用筛选、批量导入导出与命中测试。',

  // Toolbar buttons
  'keywords.testTool': '测试工具',
  'keywords.exportCsv': '导出 CSV',
  'keywords.bulkImport': '批量导入',
  'keywords.add': '添加敏感词',

  // Filter
  'keywords.searchPlaceholder': '搜索关键词',
  'keywords.categoryPlaceholder': '分类',
  'keywords.actionPlaceholder': '动作',
  'keywords.enabledFilter': '启用状态',
  'keywords.enabledTrue': '启用',
  'keywords.enabledFalse': '停用',

  // Table columns
  'keywords.colId': 'ID',
  'keywords.colWord': '关键词',
  'keywords.colCategory': '分类',
  'keywords.colMatchMode': '匹配模式',
  'keywords.colAction': '动作',
  'keywords.colEnabled': '启用',
  'keywords.colCreatedAt': '创建时间',
  'keywords.colOperation': '操作',

  // Actions in table
  'keywords.edit': '编辑',
  'keywords.delete': '删除',
  'keywords.deleteTitle': '删除敏感词',
  'keywords.deleteConfirm': '确认删除「{{word}}」？',

  // Modal: create/edit
  'keywords.addTitle': '添加敏感词',
  'keywords.editTitle': '编辑敏感词 #{{id}}',
  'keywords.wordLabel': '关键词文本',
  'keywords.wordRequired': '请输入关键词',
  'keywords.wordPlaceholder': '要拦截/告警/记录的关键词',
  'keywords.catLabel': '所属类别',
  'keywords.matchModeLabel': '匹配模式',
  'keywords.actLabel': '处理动作',
  'keywords.enabledLabel': '启用',

  // Bulk import modal
  'keywords.importTitle': '批量导入敏感词（CSV）',
  'keywords.importOk': '开始导入',
  'keywords.importFormat': 'CSV 格式（每行一条，第一行可为表头也可省略）：{{header}}',
  'keywords.importEnums': '枚举取值：category 为 political/porn/violence/illegal/discrimination/custom；match_mode 为 contains/exact/regex；action 为 block/warn/log；enabled 可省略（默认 true）。',
  'keywords.importPaste': '粘贴文本',
  'keywords.importUpload': '上传 CSV 文件',
  'keywords.importPastePlaceholder': '例如：\n赌博,political,contains,block\n暴力催收,violence,exact,block',
  'keywords.selectFile': '选择 CSV 文件',
  'keywords.pasteFirst': '请先粘贴 CSV 内容',
  'keywords.selectFileFirst': '请选择 CSV 文件',
  'keywords.fileReadFailed': '文件读取失败',
  'keywords.importDone': '导入完成：新增 {{created}} 条，跳过 {{skipped}} 条',

  // Messages
  'keywords.savedNew': '敏感词已添加',
  'keywords.savedEdit': '敏感词已更新',
  'keywords.deleted': '已删除',

  // Test Drawer
  'keywords.testDrawerTitle': '敏感词命中测试',
  'keywords.testRun': '开始测试',
  'keywords.testHint': '输入一段文本，检测其中命中的敏感词与处理动作。',
  'keywords.testPlaceholder': '输入待检测文本…',
  'keywords.testTextRequired': '请输入要测试的文本',
  'keywords.testResultLabel': '检测结果：',
  'keywords.testBlocked': '判定为需拦截',
  'keywords.testNotBlocked': '未命中拦截动作',
  'keywords.testNoHits': '无命中。',

  // Test results table
  'keywords.colHitType': '类型',
  'keywords.colHitValue': '命中内容',
};
