import { useCallback, useEffect, useRef, useState } from 'react'
import { api, authenticationRequiredEvent, RequestError } from './api'
import { PageErrorBoundary } from './components/PageErrorBoundary'
import { OperationNotifications, type OperationNotification, type OperationNotificationItem } from './components/OperationNotifications'
import { OperationProgress } from './components/OperationProgress'
import { LanguageSelector } from './components/LanguageSelector'
import { RecoveryBanner, StatusDot } from './components/Common'
import { DashboardPage } from './pages/DashboardPage'
import { ConnectivityPage } from './pages/ConnectivityPage'
import { DevicesPage } from './pages/DevicesPage'
import { DiagnosticsPage } from './pages/DiagnosticsPage'
import { NetworkPage } from './pages/NetworkPage'
import { PoliciesPage, type PoliciesViewState } from './pages/PoliciesPage'
import { SourcesPage } from './pages/SourcesPage'
import { needsNetworkRecoveryWarning, statusLabel } from './status'
import { operationStatusUnknownMessage } from './operations'
import type { Overview } from './types'
import { activateLanguage, cacheRequestedLanguage, initialRequestedLanguage, isRequestedLanguage, prepareLanguage, t, type RequestedLanguage } from './i18n'

type Page = 'dashboard' | 'network' | 'sources' | 'devices' | 'policies' | 'connectivity' | 'diagnostics'
type Theme = 'dark' | 'light'
type NetworkNavigationTarget = 'none' | 'control' | 'bottom'

const nav = [
  { id: 'dashboard', label: '总览', icon: '◈' },
  { id: 'network', label: '网络设置', icon: '⌁' },
  { id: 'sources', label: '代理与规则源', icon: '◎' },
  { id: 'devices', label: '设备', icon: '▣' },
  { id: 'policies', label: '策略', icon: '⇄' },
  { id: 'connectivity', label: '连通性', icon: '◌' },
  { id: 'diagnostics', label: '诊断', icon: '⌘' },
] as const satisfies ReadonlyArray<{ id: Page; label: string; icon: string }>

function currentPage(): Page {
  const candidate = window.location.pathname.split('/').filter(Boolean)[0] as Page | undefined
  return nav.some(item => item.id === candidate) ? candidate! : 'dashboard'
}

function initialTheme(): Theme {
  const stored = window.localStorage.getItem('opensurge-theme')
  if (stored === 'dark' || stored === 'light') return stored
  return typeof window.matchMedia === 'function' && window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

function focusGatewayControl(target: Exclude<NetworkNavigationTarget, 'none'>) {
  const control = document.getElementById('gateway-control')
  if (!(control instanceof HTMLButtonElement)) return
  const reducedMotion = typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
  if (target === 'bottom') {
    window.scrollTo?.({ top: document.documentElement.scrollHeight, behavior: reducedMotion ? 'auto' : 'smooth' })
  } else {
    control.scrollIntoView?.({ behavior: reducedMotion ? 'auto' : 'smooth', block: 'center' })
  }
  if (!control.disabled) control.focus({ preventScroll: true })
}

function networkNavigationHash(target: NetworkNavigationTarget) {
  if (target === 'control') return '#gateway-control'
  if (target === 'bottom') return '#gateway-control-bottom'
  return ''
}

export function App() {
  const [page, setPage] = useState<Page>(currentPage)
  const [overview, setOverview] = useState<Overview | null>(null)
  const [error, setError] = useState('')
  const [authenticationRequired, setAuthenticationRequired] = useState(false)
  const [theme, setTheme] = useState<Theme>(initialTheme)
  const [language, setLanguage] = useState<RequestedLanguage>(initialRequestedLanguage)
  const [languageChanging, setLanguageChanging] = useState(false)
  const [devicesDirty, setDevicesDirty] = useState(false)
  const [policiesViewState, setPoliciesViewState] = useState<PoliciesViewState>({ search: '', scope: 'global', activeGroup: null })
  const [sleepPreventionChanging, setSleepPreventionChanging] = useState(false)
  const [notifications, setNotifications] = useState<OperationNotificationItem[]>([])
  const notificationID = useRef(0)
  const sleepPreventionGeneration = useRef(0)
  const languageGeneration = useRef(0)
  const policiesScrollPosition = useRef<number | null>(null)
  const pageRef = useRef(page)
  const devicesDirtyRef = useRef(devicesDirty)
  pageRef.current = page
  devicesDirtyRef.current = devicesDirty

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    window.localStorage.setItem('opensurge-theme', theme)
  }, [theme])

  useEffect(() => {
    activateLanguage(language)
    cacheRequestedLanguage(language)
  }, [language])

  const commitLanguage = useCallback(async (nextLanguage: RequestedLanguage) => {
    await prepareLanguage(nextLanguage)
    activateLanguage(nextLanguage)
    setLanguage(nextLanguage)
  }, [])

  const refresh = useCallback(async () => {
    const sleepGeneration = sleepPreventionGeneration.current
    const requestedLanguageGeneration = languageGeneration.current
    try {
      const nextOverview = await api.overview()
      setOverview(current => sleepGeneration === sleepPreventionGeneration.current || !current
        ? nextOverview
        : { ...nextOverview, sleep_prevention: current.sleep_prevention })
      setError('')
      if (requestedLanguageGeneration === languageGeneration.current && isRequestedLanguage(nextOverview.ui_preferences?.language)) {
        await commitLanguage(nextOverview.ui_preferences.language)
      }
    } catch (cause) {
      if (cause instanceof RequestError && cause.status === 401) {
        setAuthenticationRequired(true)
        setError('')
        return
      }
      setError(cause instanceof Error ? cause.message : String(cause))
    }
  }, [commitLanguage])

  const changeLanguage = async (nextLanguage: RequestedLanguage) => {
    if (languageChanging || nextLanguage === language) return
    const previousLanguage = language
    languageGeneration.current += 1
    setLanguageChanging(true)
    try {
      await commitLanguage(nextLanguage)
      const preferences = await api.setUIPreferences({ language: nextLanguage })
      languageGeneration.current += 1
      await commitLanguage(preferences.language)
      await refresh()
    } catch (cause) {
      languageGeneration.current += 1
      await commitLanguage(previousLanguage)
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setLanguageChanging(false)
    }
  }

  useEffect(() => {
    const requireAuthentication = () => {
      setAuthenticationRequired(true)
      setError('')
    }
    window.addEventListener(authenticationRequiredEvent, requireAuthentication)
    return () => window.removeEventListener(authenticationRequiredEvent, requireAuthentication)
  }, [])

  useEffect(() => {
    if (authenticationRequired) return
    void refresh()
    const timer = window.setInterval(() => void refresh(), 8000)
    const events = typeof EventSource === 'undefined' ? null : new EventSource('/api/v1/events')
    events?.addEventListener('state', () => void refresh())
    const onPop = () => {
      const next = currentPage()
      if (pageRef.current === 'devices' && next !== 'devices' && devicesDirtyRef.current && !window.confirm(t('设备页还有尚未保存的修改，确定离开并放弃这些修改吗？'))) {
        history.pushState({}, '', '/devices')
        return
      }
      if (pageRef.current === 'devices' && next !== 'devices') setDevicesDirty(false)
      if (pageRef.current === 'policies' && next !== 'policies') policiesScrollPosition.current = window.scrollY
      setPage(next)
    }
    window.addEventListener('popstate', onPop)
    return () => {
      window.clearInterval(timer)
      events?.close()
      window.removeEventListener('popstate', onPop)
    }
  }, [authenticationRequired, refresh])

  const go = (next: Page, networkTarget: NetworkNavigationTarget = 'none') => {
    if (next === page) {
      if (networkTarget !== 'none') {
        history.replaceState({}, '', `/${next}${networkNavigationHash(networkTarget)}`)
        focusGatewayControl(networkTarget)
      }
      return
    }
    if (page === 'devices' && next !== 'devices' && devicesDirty && !window.confirm(t('设备页还有尚未保存的修改，确定离开并放弃这些修改吗？'))) return
    if (page === 'devices' && next !== 'devices') setDevicesDirty(false)
    if (page === 'policies' && next !== 'policies') policiesScrollPosition.current = window.scrollY
    history.pushState({}, '', `/${next}${networkNavigationHash(networkTarget)}`)
    setPage(next)
  }

  const setSleepPrevention = async (enabled: boolean) => {
    if (sleepPreventionChanging) return
    sleepPreventionGeneration.current += 1
    setSleepPreventionChanging(true)
    try {
      const sleepPrevention = await api.setSleepPrevention(enabled)
      sleepPreventionGeneration.current += 1
      setOverview(current => current ? { ...current, sleep_prevention: sleepPrevention } : current)
      await refresh()
    } catch (cause) {
      sleepPreventionGeneration.current += 1
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setSleepPreventionChanging(false)
    }
  }

  const notify = useCallback((notification: OperationNotification) => {
    const uncertain = notification.message.includes(operationStatusUnknownMessage) || notification.message.includes(t(operationStatusUnknownMessage))
    const item = { ...notification, title: uncertain ? t('结果尚未确认') : notification.title, id: ++notificationID.current }
    setNotifications(current => [...current, item].slice(-3))
  }, [])

  const dismissNotification = useCallback((id: number) => {
    setNotifications(current => current.filter(notification => notification.id !== id))
  }, [])

  const updatePoliciesViewState = useCallback((patch: Partial<PoliciesViewState>) => {
    setPoliciesViewState(current => {
      const next = { ...current, ...patch }
      return next.search === current.search && next.scope === current.scope && next.activeGroup === current.activeGroup ? current : next
    })
  }, [])

  const updatePoliciesScrollPosition = useCallback((scrollY: number) => {
    policiesScrollPosition.current = scrollY
  }, [])

  return <div className="app-shell">
    <aside className="sidebar">
      <div className="brand"><img className="brand-mark" src="/opensurge-icon.png" alt="" aria-hidden="true" /><div><strong>OpenSurge</strong><small>for Mac</small></div></div>
      <nav aria-label="OpenSurge sections">
        {nav.map(item => <button key={item.id} className={page === item.id ? 'active' : ''} onClick={() => go(item.id)}><span aria-hidden="true">{item.icon}</span>{t(item.label)}</button>)}
      </nav>
      <div className="sidebar-controls">
        <label className={`sidebar-switch ${overview?.sleep_prevention?.active ? 'active' : ''}`} title={t('阻止空闲睡眠和合盖睡眠。合盖运行可能明显增加耗电与发热，请勿放入不通风的包内。')}><input type="checkbox" checked={overview?.sleep_prevention?.active ?? false} disabled={!overview || sleepPreventionChanging} onChange={event => void setSleepPrevention(event.target.checked)} /><span><strong>{t(sleepPreventionChanging ? '正在切换…' : '合盖保持运行')}</strong><small>{t(overview?.sleep_prevention?.active ? '系统睡眠已临时禁用' : '默认关闭 · 本次运行有效')}</small></span></label>
        {overview?.sleep_prevention?.error && <small className="sidebar-control-error" role="status">{overview.sleep_prevention.error}</small>}
        <LanguageSelector language={language} changing={languageChanging} onChange={next => void changeLanguage(next)} />
        <button type="button" className="theme-toggle" aria-pressed={theme === 'light'} aria-label={t(theme === 'dark' ? '切换为浅色模式' : '切换为深色模式')} onClick={() => setTheme(current => current === 'dark' ? 'light' : 'dark')}><span aria-hidden="true">{theme === 'dark' ? '☀' : '◐'}</span>{t(theme === 'dark' ? '浅色模式' : '深色模式')}</button>
      </div>
      <div className="sidebar-status"><StatusDot status={overview?.status.gateway ?? 'unreachable'} /><div><strong>{statusLabel(overview?.status.gateway, overview?.status.runtime_state)}</strong><small>{import.meta.env.VITE_OPENSURGE_RELEASE_TAG} Wind Rose</small></div></div>
    </aside>
    <main className="workspace">
      {authenticationRequired ? <section className="session-expired" role="alert"><span aria-hidden="true">!</span><div><h1>{t('Web GUI 与 OpenSurge 的安全连接已过期')}</h1><p>{t('请点击 macOS 菜单栏中的 OpenSurge 图标，然后选择“打开 OpenSurge 面板”。')}</p></div></section> : <>
        {overview?.recovery.required && needsNetworkRecoveryWarning(overview.recovery.stage) && <RecoveryBanner recovery={overview.recovery.stage} onOpen={() => go('network', 'control')} />}
        {error && <div className="error-banner" role="alert"><span>!</span><p>{error}</p><button onClick={() => void refresh()}>{t('重试')}</button></div>}
        <PageErrorBoundary key={page}>
          {page === 'dashboard' && <DashboardPage overview={overview} onOpenNetwork={action => go('network', action === 'cleanup' ? 'control' : action === 'stop' ? 'bottom' : 'none')} />}
          {page === 'network' && <NetworkPage overview={overview} onChanged={refresh} onNavigate={() => go('devices')} onNotify={notify} />}
          {page === 'sources' && <SourcesPage overview={overview} onChanged={refresh} onNotify={notify} />}
          {page === 'devices' && <DevicesPage overview={overview} onChanged={refresh} onNavigate={go} onDirtyChange={setDevicesDirty} onNotify={notify} />}
          {page === 'policies' && <PoliciesPage overview={overview} onChanged={refresh} viewState={policiesViewState} onViewStateChange={updatePoliciesViewState} restoreScrollY={policiesScrollPosition.current} onScrollPositionChange={updatePoliciesScrollPosition} />}
          {page === 'connectivity' && <ConnectivityPage overview={overview} onChanged={refresh} />}
          {page === 'diagnostics' && <DiagnosticsPage overview={overview} />}
        </PageErrorBoundary>
      </>}
    </main>
    {!authenticationRequired && <OperationProgress onOpenDiagnostics={() => go('diagnostics')} />}
    <OperationNotifications notifications={notifications} onDismiss={dismissNotification} />
  </div>
}
