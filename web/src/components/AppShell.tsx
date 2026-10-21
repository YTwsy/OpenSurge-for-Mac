import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { isDesktop } from '../desktop'
import { useCompactLayout } from '../hooks/useCompactLayout'
import { t } from '../i18n'
import { StatusDot } from './Common'

type AppShellProps<Page extends string> = {
  page: Page
  nav: ReadonlyArray<{ id: Page; label: string; icon: string }>
  status: string
  statusLabel: string
  onNavigate: (page: Page) => boolean
  footer: ReactNode
  overlays: ReactNode
  children: ReactNode
}

// Drawer state stays here so opening it does not render the active page again.
export function AppShell<Page extends string>({ page, nav, status, statusLabel, onNavigate, footer, overlays, children }: AppShellProps<Page>) {
  const desktop = isDesktop()
  const compact = useCompactLayout() && !desktop
  const [open, setOpen] = useState(false)
  const navigationRef = useRef<HTMLElement>(null)
  const toggleRef = useRef<HTMLButtonElement>(null)
  const restoreFocus = useRef(false)

  useLayoutEffect(() => { setOpen(false) }, [page, compact])

  useLayoutEffect(() => {
    if (!compact || !open) {
      if (restoreFocus.current) toggleRef.current?.focus({ preventScroll: true })
      restoreFocus.current = false
      return
    }
    restoreFocus.current = true
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const navigation = navigationRef.current
    navigation?.querySelector<HTMLButtonElement>('nav button.active')?.focus({ preventScroll: true })
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { event.preventDefault(); setOpen(false); return }
      if (event.key !== 'Tab' || !navigation) return
      const controls = [...navigation.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), [tabindex="0"]')].filter(control => !control.closest('[inert], [hidden]'))
      const first = controls[0], last = controls.at(-1)
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
    }
    navigation?.addEventListener('keydown', onKeyDown)
    return () => {
      document.body.style.overflow = previousOverflow
      navigation?.removeEventListener('keydown', onKeyDown)
    }
  }, [compact, open])

  return <div className={`app-shell${desktop ? ' desktop-shell' : ''}`}>
    {desktop && <div className="desktop-titlebar" aria-hidden="true" />}
    {compact && <header className="mobile-header" inert={open}>
      <div className="mobile-brand"><img src="/opensurge-icon.png" alt="" /><span><strong>OpenSurge</strong><small>{t(nav.find(item => item.id === page)!.label)}</small></span></div>
      <span className="mobile-status"><StatusDot status={status} />{statusLabel}</span>
      <button ref={toggleRef} type="button" className="mobile-nav-toggle" aria-label={t('打开导航')} aria-expanded={open} aria-controls="app-navigation" onClick={() => setOpen(true)}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" aria-hidden="true"><path d="M4 6h16M4 12h16M4 18h16" /></svg></button>
    </header>}
    {compact && open && <div className="mobile-nav-backdrop" aria-hidden="true" onClick={() => setOpen(false)} />}
    <aside className="sidebar" id="app-navigation" ref={navigationRef} hidden={compact && !open} role={compact ? 'dialog' : undefined} aria-modal={compact && open ? true : undefined} aria-label={compact ? t('页面导航') : undefined}>
      {compact && <button type="button" className="mobile-nav-close" aria-label={t('关闭导航')} onClick={() => setOpen(false)}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" aria-hidden="true"><path d="m6 6 12 12M6 18 18 6" /></svg></button>}
      <div className="brand"><img className="brand-mark" src="/opensurge-icon.png" alt="" aria-hidden="true" /><div><strong>OpenSurge</strong><small>for Mac</small></div></div>
      <nav aria-label="OpenSurge sections">
        {nav.map(item => <button key={item.id} className={page === item.id ? 'active' : ''} aria-current={page === item.id ? 'page' : undefined} onClick={() => {
          if (!onNavigate(item.id)) return
          setOpen(false)
          if (compact && item.id !== page && item.id !== 'connections' && item.id !== 'policies') window.scrollTo?.({ top: 0 })
        }}><span aria-hidden="true">{item.icon}</span>{t(item.label)}</button>)}
      </nav>
      {footer}
    </aside>
    <main className="workspace" inert={compact && open}>{children}</main>
    <div style={{ display: 'contents' }} inert={compact && open}>{overlays}</div>
  </div>
}
