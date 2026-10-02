import { Select } from './Select'
import { languageDisplayName, resolveLanguage, t, type RequestedLanguage } from '../i18n'

export function LanguageSelector({ language, changing, onChange }: {
  language: RequestedLanguage
  changing: boolean
  onChange: (language: RequestedLanguage) => void
}) {
  const resolved = resolveLanguage(language)
  const summary = language === 'system'
    ? t('系统语言：{{language}}', { language: languageDisplayName(resolved) })
    : languageDisplayName(resolved)

  return <Select className={`sidebar-control-row language-selector ${changing ? 'changing' : ''}`}
    aria-label={t('选择 OpenSurge Web GUI 和菜单栏使用的语言')}
    value={language} disabled={changing} onChange={value => onChange(value as RequestedLanguage)}
    display={<>
      <span className="sidebar-control-copy"><strong>{t('界面语言')}</strong><small>{changing ? t('正在保存语言…') : summary}</small></span>
      <span className="language-selector-chevron" aria-hidden="true">⌄</span>
    </>}>
    <option value="system">{t('跟随系统')}</option>
    <option value="zh-Hans">简体中文</option>
    <option value="en">English</option>
  </Select>
}
