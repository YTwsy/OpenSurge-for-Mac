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

  return <label className={`language-selector ${changing ? 'changing' : ''}`}>
    <span className="language-selector-icon" aria-hidden="true">文</span>
    <span className="language-selector-copy">
      <small>{t('界面语言')}</small>
      <strong>{changing ? t('正在保存语言…') : summary}</strong>
    </span>
    <span className="language-selector-chevron" aria-hidden="true">⌄</span>
    <select
      aria-label={t('选择 OpenSurge Web GUI 和菜单栏使用的语言')}
      value={language}
      disabled={changing}
      onChange={event => onChange(event.target.value as RequestedLanguage)}
    >
      <option value="system">{t('跟随系统')}</option>
      <option value="zh-Hans">简体中文</option>
      <option value="en">English</option>
    </select>
  </label>
}
