> 🌐 **English** (frontend i18n guide — no Chinese counterpart; the guide is English-only)

# Frontend i18n Guide (EN/CN)

## Overview

The project uses **i18next + react-i18next** for frontend-only internationalization. Language switching only affects UI strings — backend responses remain unchanged. Users toggle between Chinese (中文) and English (English).

## Installation (already done)

Dependencies: `i18next ^26`, `react-i18next ^17`

## File Structure

```
src/
  i18n/
    index.ts                       ← initialization, lazy-loads page locales via import.meta.glob
    locales/
      zh-CN/
        common.ts                  ← shared phrases (buttons, messages, units)
        dicts.ts                   ← dictionary enum labels
        menu.ts                    ← sidebar menu / breadcrumb names
        pages/                     ← auto-discovered per-component files
          keywords.ts              ← one .ts per React component (name matches basename camelCase)
      en-US/                       ← same structure as zh-CN
```

## Key Conventions

### 1. Namespace prefix

Each language pack file's keys use a flat prefix matching the source filename:

| Source file                      | Locale file                 | Prefix used inside     |
| -------------------------------- | --------------------------- | ---------------------- |
| `src/pages/guard/Keywords.tsx`   | `.../pages/keywords.ts`     | `'keywords.xxx'`       |
| `src/pages/Dashboard.tsx`        | `.../pages/dashboard.ts`    | `'dashboard.xxx'`      |
| `src/components/Charts.tsx`      | `.../pages/charts.ts`       | `'charts.xxx'`         |
| `src/layouts/MainLayout.tsx`     | `.../pages/mainLayout.ts`   | `'mainLayout.xxx'`     |

Prefixes must be consistent across all locale files. Do NOT add nesting layers.

### 2. Accessing translations

Every React component that renders user-visible text must call `useTranslation()`:

```tsx
import { useTranslation } from 'react-i18next';

function MyComponent() {
  const { t } = useTranslation();
  return <Button>{t('keywords.add')}</Button>;
}
```

Non-react modules (e.g., `utils/format.ts`) access the global instance directly:

```tsx
import i18n from '../i18n';
const msg = i18n.t('common.networkError');
```

### 3. Interpolation

Use i18next interpolation syntax `{{var}}`:

```ts
// keyword.ts (zh or en)
'keywords.deleteConfirm': '确认删除「{{word}}」？',
'keywords.importDone': '导入完成：新增 {{created}} 条，跳过 {{skipped}} 条',
```

```tsx
// Component usage
t('keywords.deleteConfirm', { word: record.word })
t('keywords.importDone', { created: res.created, skipped: res.skipped })
```

### 4. Dictionary enum options

Old pattern: `KEYWORD_CATEGORY_OPTIONS`, `labelOf(CATEGORY_MAP.labels, v)`

New pattern:
```tsx
import { KEYWORD_CATEGORY_META, ACTION_META, useDictOptions, useDictLabel, dictColor } from '../../constants/dicts';

// Select options
const catOpts = useDictOptions('keywordCategory', KEYWORD_CATEGORY_META);
const actOpts = useDictOptions('action', ACTION_META).filter(a => a.value !== 'mask');

// Label lookup function
const catLbl = useDictLabel('keywordCategory');
catLbl(v) → localized string or raw value as fallback

// Color lookup (no localization needed)
dictColor(KEYWORD_CATEGORY_META, v)
```

Import mapping:
| Old name               | New replacement                                              |
| ---------------------- | ------------------------------------------------------------ |
| `XXX_OPTIONS`          | `useDictOptions('xxxNs', XXX_META)`                          |
| `LABELS_MAP.labels`    | `useDictLabel('xxxNs')`                                      |
| `LABELS_MAP.colors`    | `dictColor(XXX_META, v)`                                     |

### 5. Shared keys (don't redefine locally)

Common phrases already exist under the following namespaces. Reference these instead of adding duplicates:

| Namespace            | Example keys                              |
| -------------------- | ----------------------------------------- |
| `common.*`           | save, cancel, delete, confirm, reset, export, search, total, noData, operationFailed, etc. |
| `menu.*`             | Home, tokenUsage, models, providers, guard, quota, audit, settings, logout… |
| `pageName.*`         | breadcrumbs titles                        |
| `dicts.*`            | enum labels                               |

If a needed key doesn't exist in any shared file, define it locally with your page namespace. This avoids cross-file conflicts during parallel subagent work.

### 6. What NOT to translate

- Backend API messages (they come from server envelope responses and stay in whatever language the backend sends)
- Code comments (can remain in Chinese)
- Variable names, types, URLs, enum values
- User-generated content (model names, descriptions, tags, etc.)

### 7. dayjs locale

DayJS locale switches automatically when the app language changes (handled in `i18n/index.ts`). Components don't need to worry about this.

### 8. antd locale

ConfigProvider locale swaps dynamically based on app language (set in App.tsx). antd components like DatePicker, Modal, Select will follow.

## Steps to convert a page/component

1. Read the existing `.tsx` file.
2. Create two locale files:
   - `src/i18n/locales/zh-CN/pages/<camelCaseBasename>.ts`
   - `src/i18n/locales/en-US/pages/<camelCaseBasename>.ts`
3. Add all user-visible Chinese strings as `'<prefix>.key': '<translation>'` entries. Keep alphabetical order.
4. Rewrite the component:
   - Import `useTranslation` from 'react-i18next'.
   - Replace all hardcoded Chinese strings with `t('prefix.key')` calls.
   - Update dicts imports (old `*_OPTIONS`/`*_MAP`/`labelOf`/`colorOf` → new hook-based API).
   - For non-react modules: import `i18n` and use `i18n.t('key')`.
5. Return nothing — the files you've written are enough. Do NOT run `npm run build` or check TypeScript errors globally (other half-converted pages may fail).
