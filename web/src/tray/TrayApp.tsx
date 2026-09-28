import { useCallback, useEffect, useRef, useState } from 'react'
import { copyText, desktopAction } from '../desktop'
import { activateLanguage, cacheRequestedLanguage, initialRequestedLanguage, isRequestedLanguage, prepareLanguage, t } from '../i18n'
import { useTheme } from '../hooks/useTheme'
import { recoveryLabel, statusLabel, takeoverLabel } from '../status'
import type { SleepPreventionStatus } from '../types'
import { watchVisibleRefresh } from '../visibility'
import type { MenuBarStatus, TraySnapshot } from './types'
import { TrayUtilities } from './TrayUtilities'
import { TrayActivity } from './TrayActivity'
import './tray.css'

const initial: TraySnapshot = { status: null, indicator: 'connecting', sequence: 0, can_quit: false }
const indicatorLabels = { connecting: '正在连接后台服务…', stopped: '网关已停止', running: '网关正在运行', degraded: '网关运行异常', recovery: '网络恢复尚未完成', unreachable: '无法连接后台服务' }

export function diagnosticSummary(status: MenuBarStatus | null): string {
 if (!status) return 'OpenSurge Control API: unreachable'
 return ['OpenSurge for Mac', `Gateway: ${status.gateway}`, `Topology: ${status.topology}`, `LAN IP: ${status.lan_ip}`, `DHCP/DNS: ${status.dhcp}`, `mihomo: ${status.mihomo}`, `TUN: ${status.tun}${status.tun_interface ? ` [${status.tun_interface}]` : ''}`, `PF: ${status.pf_anchor}`, `Forwarding: ${status.forwarding}`, `IPv4 takeover: ${status.ipv4_takeover}`, `IPv6 takeover: ${status.ipv6_takeover}`, `Clients: ${status.client_count}`, `Drift: ${status.drift}`, `Recovery: ${status.recovery_required ? status.recovery_stage ?? 'required' : 'none'}`, `Error code: ${status.error_code ?? 'none'}`, `Lid-closed sleep prevention: ${status.sleep_prevention.active ? 'active' : 'off'}`].join('\n')
}

export function TrayApp() {
 const [theme] = useTheme()
 useEffect(() => { void desktopAction('tray-appearance', { theme }).catch(() => {}) }, [theme])
 const more = useRef<HTMLDetailsElement>(null)
 useEffect(() => {
  const close = (event: PointerEvent) => { if (more.current && !more.current.contains(event.target as Node)) more.current.open = false }
  const checkUpdate = () => { if (more.current) more.current.open = true }
  document.addEventListener('pointerdown', close)
  window.addEventListener('opensurge:check-update', checkUpdate)
  return () => { document.removeEventListener('pointerdown', close); window.removeEventListener('opensurge:check-update', checkUpdate) }
 }, [])
 const [snapshot, setSnapshot] = useState(initial)
 const [language, setLanguage] = useState(initialRequestedLanguage)
 const [error, setError] = useState('')
 const [copied, setCopied] = useState(false)
 const [sleepBusy, setSleepBusy] = useState(false)
 const [refreshing, setRefreshing] = useState(false)
 const [serviceBusy, setServiceBusy] = useState(false)
 const [networkExpanded, setNetworkExpanded] = useState(false)
 const sequence = useRef(0)
 const active = useRef(true)
 const sleepGeneration = useRef(0)
 const sleepPending = useRef(false)
 const languageRef = useRef(language)
 languageRef.current = language
 const refresh = useCallback(async (force = false) => {
  const generation = sleepGeneration.current
  try {
   const next = await desktopAction<TraySnapshot>('menubar-status', { refresh: force })
   if (!active.current || next.sequence < sequence.current) return
   sequence.current = next.sequence
   const requested = next.status?.ui_preferences.language
   if (isRequestedLanguage(requested) && languageRef.current !== requested) {
    await prepareLanguage(requested)
    if (!active.current || next.sequence < sequence.current) return
    activateLanguage(requested)
    cacheRequestedLanguage(requested)
    languageRef.current = requested
    setLanguage(requested)
   }
   setSnapshot(current => {
    if (next.status && current.status && (sleepPending.current || generation !== sleepGeneration.current)) {
     return { ...next, status: { ...next.status, sleep_prevention: current.status.sleep_prevention } }
    }
    return next
   })
  } catch { if (active.current) setSnapshot(current => ({ ...current, status: null, indicator: 'unreachable', can_quit: false })) }
 }, [])
 useEffect(() => {
  active.current = true
  const stop = watchVisibleRefresh(refresh, 1000)
  return () => { active.current = false; stop() }
 }, [refresh])

 const run = async (action: () => Promise<unknown>) => {
  setError('')
  try { await action() } catch (cause) { if (active.current) setError(cause instanceof Error ? cause.message : String(cause)) }
 }
 const show = (page: string, options?: { owner?: string; section?: 'active-devices' }) => void run(() => desktopAction('show-main', { page, ...options }))
 const serviceAction = async (action: 'reconnect' | 'quit', full = false) => {
  if (serviceBusy) return
  setServiceBusy(true)
  await run(() => desktopAction(action, action === 'quit' ? { full } : {}))
  setServiceBusy(false)
  await refresh(true)
 }
 const changeSleep = async (enabled: boolean) => {
  if (sleepPending.current) return
  sleepPending.current = true
  sleepGeneration.current++
  setSleepBusy(true)
  await run(async () => {
   const sleep = await desktopAction<SleepPreventionStatus>('sleep-prevention', { enabled })
   sleepGeneration.current++
   setSnapshot(current => current.status ? { ...current, status: { ...current.status, sleep_prevention: sleep } } : current)
  })
  sleepGeneration.current++
  sleepPending.current = false
  setSleepBusy(false)
  await refresh(true)
 }
 const status = snapshot.status
 const topology = status ? ({ same_lan: '旁路由模式', same_wifi_dhcp: '局域网 DHCP 接管', isolated_lan: '独立下游 LAN' } as Record<string, string>)[status.topology] ?? status.topology : ''
 const rows = status ? [
  [t('网关'), statusLabel(status.gateway)], [t('拓扑'), t(topology)], ['LAN IP', status.lan_ip], [t('客户端'), String(status.client_count)],
  ['DHCP / DNS', statusLabel(status.dhcp)], ['mihomo', status.mihomo.startsWith('running') ? `${statusLabel('running')}${status.mihomo.slice(7)}` : statusLabel(status.mihomo)], ['TUN', `${takeoverLabel(status.tun)}${status.tun_interface ? ` · ${status.tun_interface}` : ''}`],
  ['PF', t(status.pf_anchor === 'loaded' ? '已加载' : '未加载')], [t('IPv4 接管'), takeoverLabel(status.ipv4_takeover)], [t('IPv6 接管'), takeoverLabel(status.ipv6_takeover)],
 ] : []
 return <main className="tray-app">
  <header className="tray-header">
   <img src="/opensurge-icon.png" alt="" /><h1>OpenSurge</h1>
   <span className={`tray-indicator ${snapshot.indicator}`} title={t(indicatorLabels[snapshot.indicator])}><span className={`tray-dot ${snapshot.indicator}`} />{t(snapshot.indicator === 'running' ? '运行中' : snapshot.indicator === 'stopped' ? '已停止' : snapshot.indicator === 'connecting' ? '正在连接…' : '需要处理')}</span>
   <details className="tray-more" ref={more} onKeyDown={event => { if (event.key === 'Escape' && more.current?.open) { more.current.open = false; event.stopPropagation(); more.current.querySelector('summary')?.focus() } }}>
    <summary aria-label={t('更多操作')} title={t('更多操作')}>···</summary>
    <div className="tray-more-panel">
     <div className="tray-row"><button onClick={() => void run(async () => { await copyText(diagnosticSummary(status)); setCopied(true) })}>{t(copied ? '已复制' : '复制诊断摘要')}</button><button disabled={refreshing} aria-label={t('刷新状态')} onClick={() => { setRefreshing(true); void refresh(true).finally(() => setRefreshing(false)) }}>{t(refreshing ? '正在刷新…' : '刷新')}</button></div>
     <TrayUtilities canUninstall={snapshot.can_uninstall === true} />
     <p className="tray-caption">{import.meta.env.VITE_OPENSURGE_RELEASE_TAG} · Wind Rose</p>
     <section className="tray-exit">
      <button disabled={!snapshot.can_quit || !snapshot.service_actions || serviceBusy} title={!snapshot.can_quit ? t('请先在网络设置中停止网关并完成恢复。') : undefined} onClick={() => void serviceAction('quit', true)}>{t('退出 OpenSurge…')}</button>
      <button disabled={serviceBusy} onClick={() => void serviceAction('quit')}>{t('只退出桌面 App…')}</button>
     </section>
    </div>
   </details>
  </header>
  {status && <p className="tray-subtitle"><span>{t(topology)}</span><span aria-hidden="true"> · </span><span>{status.lan_ip}</span></p>}
  <div className="tray-content">
   {snapshot.indicator === 'recovery' && <section className="tray-notice warning" role="alert"><strong>{t('网络恢复尚未完成')}</strong><p>{recoveryLabel(status?.recovery_stage ?? '')}</p><button onClick={() => show('network')}>{t('继续恢复')}</button></section>}
   {snapshot.indicator === 'degraded' && <section className="tray-notice warning" role="alert"><strong>{t('网关运行异常')}</strong><p>{status?.warnings[0] || t('请在网络设置中查看异常原因。')}</p><button onClick={() => show('network')}>{t('查看网络设置')}</button></section>}
   {!status && <section className="tray-notice" role="status"><p>{t(snapshot.indicator === 'connecting' ? '正在连接后台服务…' : '状态暂不可用。后台连接恢复后会自动更新。')}</p>{snapshot.service_actions && <button disabled={serviceBusy} onClick={() => void serviceAction('reconnect')}>{t('重新连接后台服务')}</button>}</section>}
   {status?.drift && <p className="tray-inline-warning">{t('配置已修改，需要重启网关')}</p>}
   <TrayActivity gateway={status?.gateway} onOpen={show} />
   <section className={`tray-network ${networkExpanded ? 'expanded' : ''}`}>
    <button type="button" className="tray-network-toggle" aria-expanded={networkExpanded} aria-controls="tray-network-details" onClick={() => setNetworkExpanded(value => !value)}><span>{t('网络状态')}</span><span className="tray-caption">{t(networkExpanded ? '收起详情' : '查看详情')} <svg className="tray-disclosure" viewBox="0 0 12 12" aria-hidden="true"><path d="m4 2 4 4-4 4" /></svg></span></button>
    <div className="tray-network-reveal" id="tray-network-details" aria-hidden={!networkExpanded} inert={!networkExpanded}>
     <div className="tray-network-inner">
      {status ? <dl className="tray-status">{rows.map(([name, value]) => <div key={name}><dt>{name}</dt><dd title={value}>{value}</dd></div>)}</dl> : <p className="tray-hint">{t('状态暂不可用。后台连接恢复后会自动更新。')}</p>}
      {status?.recovery_required && status.recovery_stage === 'prepared' && <p className="tray-hint">{t('恢复资料已准备；尚未改动网络')}</p>}
      <button className="tray-text-button" onClick={() => show('network')}>{t('查看网络设置')} ›</button>
     </div>
    </div>
   </section>
   <section className="tray-preferences">
    <label><span>{t('合盖保持运行')}<small>{t('默认关闭 · 本次运行有效')}</small></span><input type="checkbox" role="switch" aria-label={t('合盖保持运行')} checked={status?.sleep_prevention.active ?? false} disabled={!status || sleepBusy} onChange={event => void changeSleep(event.target.checked)} /></label>
    {status?.sleep_prevention.active && <p className="tray-notice warning">{t('合盖后仍会运行，请注意耗电与散热，不要放入不通风的包内。')}</p>}
    {status?.sleep_prevention.error && <p role="alert" className="tray-error">{status.sleep_prevention.error}</p>}
   </section>
   {error && <p role="alert" className="tray-error">{error}</p>}
  </div>
  <footer className="tray-footer"><button className="primary" onClick={() => show('dashboard')}><svg viewBox="0 0 16 16" aria-hidden="true"><path d="M7 3H4a2 2 0 0 0-2 2v7a2 2 0 0 0 2 2h7a2 2 0 0 0 2-2V9M9 2h5v5M7 9l7-7" /></svg>{t('打开 OpenSurge 面板')}</button></footer>
 </main>
}
