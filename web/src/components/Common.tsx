import type { ReactNode } from 'react'
import { recoveryLabel } from '../status'
import { t } from '../i18n'

export function RecoveryBanner({ recovery, onOpen }: { recovery: string; onOpen: () => void }) {
  return <div className="recovery-banner" role="alert"><span aria-hidden="true">⚠</span><div><strong>{t('网络恢复尚未完成')}</strong><p>{t('{{stage}}。网络已开始变更；请在网络设置中完成状态机，并在路由器 DHCP 恢复已验证前不要把 Mac 切回自动 DHCP。', { stage: recoveryLabel(recovery) })}</p></div><button onClick={onOpen}>{t('继续恢复')}</button></div>
}

export function PageHeader({ eyebrow, title, description, action }: { eyebrow: string; title: string; description: string; action?: ReactNode }) {
  return <header className="page-header"><div><small>{t(eyebrow)}</small><h1>{t(title)}</h1><p>{t(description)}</p></div>{action}</header>
}

export function SectionTitle({ title, subtitle }: { title: string; subtitle: string }) {
  return <div className="section-title"><h2>{t(title)}</h2><p>{t(subtitle)}</p></div>
}

export function Metric({ label, value, note }: { label: string; value: ReactNode; note: string }) {
  return <article className="metric"><small>{t(label)}</small><strong>{value}</strong><span>{t(note)}</span></article>
}

export function Service({ name, state, detail }: { name: string; state?: string; detail: string }) {
  return <article className="service"><StatusDot status={state ?? 'stopped'} /><div><strong>{t(name)}</strong><small>{t(detail)}</small></div><span>{t(state ?? '—')}</span></article>
}

export function Mode({ title, description, badge, active, expanded, controls, disabled, onSelect }: { title: string; description: string; badge?: string; active?: boolean; expanded?: boolean; controls?: string; disabled?: boolean; onSelect?: () => void }) {
  return <button type="button" className={`mode ${active ? 'active' : ''}`} aria-pressed={active} aria-expanded={expanded} aria-controls={controls} disabled={disabled} onClick={onSelect}><span>{badge && <span className="pill ok">{t(badge)}</span>}<h3>{t(title)}</h3><p>{t(description)}</p></span><span className="mode-state" aria-hidden="true"><span className="radio">{active ? '●' : '○'}</span><span className="mode-chevron">⌄</span></span></button>
}

export function Empty({ text }: { text: string }) { return <div className="empty">{t(text)}</div> }

export function StatusDot({ status }: { status: string }) {
  const state = status.includes('running') || status === 'ready'
    ? 'running'
    : status.includes('degraded') || status === 'failed' || status === 'unknown'
      ? 'degraded'
      : 'stopped'
  return <span className={`status-dot ${state}`} aria-label={state} />
}
