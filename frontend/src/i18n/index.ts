import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import dayjs from 'dayjs';
import 'dayjs/locale/zh-cn';
import 'dayjs/locale/en';
import zhCommon from './locales/zh-CN/common';
import zhMenu from './locales/zh-CN/menu';
import zhDicts from './locales/zh-CN/dicts';
import enCommon from './locales/en-US/common';
import enMenu from './locales/en-US/menu';
import enDicts from './locales/en-US/dicts';

export const DEFAULT_LANG = 'zh-CN';
export const SUPPORTED_LANGS = ['zh-CN', 'en-US'] as const;
export type SupportedLang = (typeof SUPPORTED_LANGS)[number];
export const LANG_LABELS: Record<SupportedLang, string> = {
  'zh-CN': '中文',
  'en-US': 'English',
};

const STORAGE_KEY = 'llm-router-lang';

function getInitialLang(): SupportedLang {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored && (SUPPORTED_LANGS as readonly string[]).includes(stored)) {
      return stored as SupportedLang;
    }
    if (navigator.language.startsWith('zh')) return 'zh-CN';
  } catch {
    // noop
  }
  return 'en-US';
}

function buildResources() {
  const resources: Record<string, { translation: Record<string, unknown> }> = {
    'zh-CN': { translation: { ...zhCommon, ...zhMenu, ...zhDicts } },
    'en-US': { translation: { ...enCommon, ...enMenu, ...enDicts } },
  };

  // 页面级语言包：src/i18n/locales/<lang>/pages/<name>.ts，
  // 文件内键必须带自身命名空间前缀（如 'keywords.title'），扁平并入词表。
  const modules = import.meta.glob('./locales/*/pages/*.ts', { eager: true });
  for (const [path, mod] of Object.entries(modules)) {
    const normalized = path.replace(/\\/g, '/');
    const match = normalized.match(/locales\/(zh-CN|en-US)\/pages\/.+\.ts$/);
    if (!match) continue;
    const lang = match[1];
    const data = (mod as { default: unknown }).default;
    Object.assign(resources[lang].translation, data);
  }

  return resources;
}

i18n
  .use(initReactI18next)
  .init({
    resources: buildResources(),
    lng: getInitialLang(),
    fallbackLng: DEFAULT_LANG,
    interpolation: { escapeValue: false },
  });

i18n.on('languageChanged', (lang) => {
  const dayjsLocale = lang === 'zh-CN' ? 'zh-cn' : 'en';
  dayjs.locale(dayjsLocale);
  document.documentElement.lang = lang;
  try {
    document.title = i18n.t('menu.brand');
  } catch {
    // noop
  }
});

// 初始状态同步（languageChanged 只在后续切换时触发）
dayjs.locale(i18n.language === 'zh-CN' ? 'zh-cn' : 'en');
document.documentElement.lang = i18n.language;
try {
  document.title = i18n.t('menu.brand');
} catch {
  // noop
}

export function setLanguage(lang: SupportedLang) {
  if (!(SUPPORTED_LANGS as readonly string[]).includes(lang)) return;
  try {
    localStorage.setItem(STORAGE_KEY, lang);
  } catch {
    // noop
  }
  i18n.changeLanguage(lang);
}

export { i18n };
export default i18n;
