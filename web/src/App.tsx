import { useCallback, useEffect, useState } from 'react'
import { api, RequestError } from './api'
import { RecoveryBanner, StatusDot } from './components/Common'
import { DashboardPage } from './pages/DashboardPage'
import { DevicesPage } from './pages/DevicesPage'
import { DiagnosticsPage } from './pages/DiagnosticsPage'
import { NetworkPage } from './pages/NetworkPage'
import { PoliciesPage } from './pages/PoliciesPage'
import { SourcesPage } from './pages/SourcesPage'
import { statusLabel } from './status'
import type { Overview } from './types'

type Page = 'dashboard' | 'network' | 'sources' | 'devices' | 'policies' | 'diagnostics'

const nav = [
  { id: 'dashboard', label: '总览', icon: '◈' },
  { id: 'network', label: '网络设置', icon: '⌁' },
  { id: 'sources', label: '代理与规则源', icon: '◎' },
  { id: 'devices', label: '设备', icon: '▣' },
  { id: 'policies', label: '策略', icon: '⇄' },
  { id: 'diagnostics', label: '诊断', icon: '⌘' },
] as const satisfies ReadonlyArray<{ id: Page; label: string; icon: string }>

function currentPage(): Page {
  const candidate = window.location.pathname.split('/').filter(Boolean)[0] as Page | undefined
  return nav.some(item => item.id === candidate) ? candidate! : 'dashboard'
}

export function App() {
  const [page, setPage] = useState<Page>(currentPage)
  const [overview, setOverview] = useState<Overview | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const refresh = useCallback(async () => {
    try {
      setOverview(await api.overview())
      setError('')
    } catch (cause) {
      setError(cause instanceof RequestError && cause.status === 401
        ? '此页面需要由 OpenSurge 菜单栏 App 或控制服务生成的安全链接打开。'
        : cause instanceof Error ? cause.message : String(cause))
    }
  }, [])

  useEffect(() => {
    void refresh()
    const timer = window.setInterval(() => void refresh(), 8000)
    const events = typeof EventSource === 'undefined' ? null : new EventSource('/api/v1/events')
    events?.addEventListener('state', () => void refresh())
    const onPop = () => setPage(currentPage())
    window.addEventListener('popstate', onPop)
    return () => {
      window.clearInterval(timer)
      events?.close()
      window.removeEventListener('popstate', onPop)
    }
  }, [refresh])

  const go = (next: Page) => {
    history.pushState({}, '', `/${next}`)
    setPage(next)
  }

  const gatewayAction = async (action: 'start' | 'stop') => {
    setBusy(true)
    try {
      await api.gateway(action)
      window.setTimeout(() => void refresh(), 1000)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setBusy(false)
    }
  }

  return <div className="app-shell">
    <aside className="sidebar">
      <div className="brand"><span className="brand-mark">OS</span><div><strong>OpenSurge</strong><small>for Mac</small></div></div>
      <nav aria-label="OpenSurge sections">
        {nav.map(item => <button key={item.id} className={page === item.id ? 'active' : ''} onClick={() => go(item.id)}><span aria-hidden="true">{item.icon}</span>{item.label}</button>)}
      </nav>
      <div className="sidebar-status"><StatusDot status={overview?.status.gateway ?? 'unreachable'} /><div><strong>{statusLabel(overview?.status.gateway)}</strong><small>{overview?.status.lan_ip || 'Control API'}</small></div></div>
    </aside>
    <main className="workspace">
      {overview?.recovery.required && <RecoveryBanner recovery={overview.recovery.stage} onOpen={() => go('network')} />}
      {error && <div className="error-banner" role="alert"><span>!</span><p>{error}</p><button onClick={() => void refresh()}>重试</button></div>}
      {page === 'dashboard' && <DashboardPage overview={overview} busy={busy} onAction={gatewayAction} />}
      {page === 'network' && <NetworkPage overview={overview} onChanged={refresh} />}
      {page === 'sources' && <SourcesPage />}
      {page === 'devices' && <DevicesPage overview={overview} />}
      {page === 'policies' && <PoliciesPage overview={overview} onChanged={refresh} />}
      {page === 'diagnostics' && <DiagnosticsPage overview={overview} />}
    </main>
  </div>
}
