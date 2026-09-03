import { useTranslation } from 'react-i18next';
import { Dropdown, Typography } from 'antd';
import { GlobalOutlined } from '@ant-design/icons';
import { LANG_LABELS, SUPPORTED_LANGS, setLanguage, type SupportedLang } from '../i18n';

/**
 * 顶栏语言切换：图标 + 当前语言，下拉选择中文/English。
 * 选择持久化到 localStorage，即时切换无需刷新。
 * @param textColor 文字颜色（默认白色，适配深色顶栏；浅色背景页可传入深色）
 */
export default function LanguageSwitch({ textColor = '#fff' }: { textColor?: string }) {
  const { i18n } = useTranslation();
  const current = (SUPPORTED_LANGS as readonly string[]).includes(i18n.language)
    ? (i18n.language as SupportedLang)
    : 'zh-CN';

  return (
    <Dropdown
      menu={{
        selectedKeys: [current],
        items: SUPPORTED_LANGS.map((lang) => ({
          key: lang,
          label: LANG_LABELS[lang],
        })),
        onClick: ({ key }) => setLanguage(key as SupportedLang),
      }}
    >
      <Typography.Text style={{ color: textColor, cursor: 'pointer' }}>
        <GlobalOutlined style={{ marginRight: 4 }} />
        {LANG_LABELS[current]}
      </Typography.Text>
    </Dropdown>
  );
}
