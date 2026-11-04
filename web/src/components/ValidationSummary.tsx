import { t } from '../i18n'

export function ValidationSummary({ title = '配置检查', blockers, warnings }: {
  title?: string
  blockers: string[]
  warnings: string[]
}) {
  if (!blockers.length && !warnings.length) return null

  return <details className={`validation-summary ${blockers.length ? 'has-blockers' : 'has-warnings'}`}>
    <summary>
      <span className="validation-summary-icon" aria-hidden="true">{blockers.length ? '!' : 'i'}</span>
      <strong>{t(title)}</strong>
      <span className="validation-summary-counts">
        {blockers.length > 0 && <span className="validation-count blockers">{t('{{count}} 项需处理', { count: blockers.length })}</span>}
        {warnings.length > 0 && <span className="validation-count warnings">{t('{{count}} 项提醒', { count: warnings.length })}</span>}
      </span>
      <span className="validation-summary-chevron" aria-hidden="true">⌄</span>
    </summary>
    <div className="validation-summary-body">
      {blockers.length > 0 && <div className="validation-summary-group blockers">
        <strong>{t('需要处理')}</strong>
        <ul>{blockers.map((item, index) => <li key={`${index}:${item}`}>{t(item)}</li>)}</ul>
      </div>}
      {warnings.length > 0 && <div className="validation-summary-group warnings">
        <strong>{t('提醒')}</strong>
        <ul>{warnings.map((item, index) => <li key={`${index}:${item}`}>{t(item)}</li>)}</ul>
      </div>}
    </div>
  </details>
}
