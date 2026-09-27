import { useCallback, useEffect, useRef, useState } from 'react'
import { copyText, desktopAction } from '../desktop'
import { activateLanguage, cacheRequestedLanguage, initialRequestedLanguage, isRequestedLanguage, prepareLanguage, t } from '../i18n'
import { useTheme } from '../hooks/useTheme'
import { recoveryLabel, statusLabel, takeoverLabel } from '../status'
import type { SleepPreventionStatus } from '../types'
import { watchVisibleRefresh } from '../visibility'
import type { MenuBarStatus, TraySnapshot } from './types'
import './tray.css'

const initial: TraySnapshot = { status: null, indicator: 'connecting', sequence: 0, can_quit: false }
const indicatorLabels = { connecting: '正在连接后台服务…', stopped: '网关已停止', running: '网关正在运行', degraded: '网关运行异常', recovery: '网络恢复尚未完成', unreachable: '无法连接后台服务' }

export function diagnosticSummary(status: MenuBarStatus | null): string {
 if (!status) return 'OpenSurge Control API: unreachable'
 return ['OpenSurge for Mac', `Gateway: ${status.gateway}`, `Topology: ${status.topology}`, `LAN IP: ${status.lan_ip}`, `DHCP/DNS: ${status.dhcp}`, `mihomo: ${status.mihomo}`, `TUN: ${status.tun}${status.tun_interface ? ` [${status.tun_interface}]` : ''}`, `PF: ${status.pf_anchor}`, `Forwarding: ${status.forwarding}`, `IPv4 takeover: ${status.ipv4_takeover}`, `IPv6 takeover: ${status.ipv6_takeover}`, `Clients: ${status.client_count}`, `Drift: ${status.drift}`, `Recovery: ${status.recovery_required ? status.recovery_stage ?? 'required' : 'none'}`, `Error code: ${status.error_code ?? 'none'}`, `Lid-closed sleep prevention: ${status.sleep_prevention.active ? 'active' : 'off'}`].join('\n')
}

export function TrayApp() {
 useTheme()
 const [snapshot, setSnapshot] = useState(initial)
 const [language, setLanguage] = useState(initialRequestedLanguage)
 const [error, setError] = useState('')
 const [copied, setCopied] = useState(false)
 const [sleepBusy, setSleepBusy] = useState(false)
 const [refreshing, setRefreshing] = useState(false)
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
 const show = (page: string) => void run(() => desktopAction('show-main', { page }))
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
  ['DHCP / DNS', statusLabel(status.dhcp)], ['mihomo', statusLabel(status.mihomo)], ['TUN', `${takeoverLabel(status.tun)}${status.tun_interface ? ` · ${status.tun_interface}` : ''}`],
  ['PF', t(status.pf_anchor === 'loaded' ? '已加载' : '未加载')], [t('IPv4 接管'), takeoverLabel(status.ipv4_takeover)], [t('IPv6 接管'), takeoverLabel(status.ipv6_takeover)],
 ] : []
 return <main className="tray-app">
  <header className="tray-header"><img src="/opensurge-icon.png" alt="" /><div><h1>OpenSurge for Mac</h1><p><span className={`tray-dot ${snapshot.indicator}`} />{t(indicatorLabels[snapshot.indicator])}</p></div></header>
  {snapshot.indicator === 'recovery' && <section className="tray-notice warning" role="alert"><strong>{t('网络恢复尚未完成')}</strong><p>{recoveryLabel(status?.recovery_stage ?? '')}</p><p>{t('请在网络设置中完成恢复，再退出 OpenSurge。')}</p></section>}
  {status ? <>
   <dl className="tray-status">{rows.map(([name, value]) => <div key={name}><dt>{name}</dt><dd title={value}>{value}</dd></div>)}</dl>
   {status.recovery_required && status.recovery_stage === 'prepared' && <p className="tray-hint">{t('恢复资料已准备；尚未改动网络')}</p>}
   {status.drift && <p className="tray-notice warning">{t('配置已修改，需要重启网关')}</p>}
  </> : <p className="tray-notice">{t('状态暂不可用。后台连接恢复后会自动更新。')}</p>}
  <section className="tray-actions">
   <button className="primary" onClick={() => show('dashboard')}>{t('打开 OpenSurge 面板')}<span aria-hidden="true">↗</span></button>
   {status?.recovery_required && <button onClick={() => show('network')}>{t(snapshot.indicator === 'recovery' ? '继续恢复' : '查看网络设置')}</button>}
   <div className="tray-row"><button onClick={() => void run(async () => { await copyText(diagnosticSummary(status)); setCopied(true) })}>{t(copied ? '已复制' : '复制诊断摘要')}</button><button disabled={refreshing} aria-label={t('刷新状态')} onClick={() => { setRefreshing(true); void refresh(true).finally(() => setRefreshing(false)) }}>{t(refreshing ? '正在刷新…' : '刷新')}</button></div>
  </section>
  <section className="tray-preferences">
   <label><span>{t('合盖保持运行')}<small>{t('默认关闭 · 本次运行有效')}</small></span><input type="checkbox" checked={status?.sleep_prevention.active ?? false} disabled={!status || sleepBusy} onChange={event => void changeSleep(event.target.checked)} /></label>
   {status?.sleep_prevention.active && <p className="tray-notice warning">{t('合盖后仍会运行，请注意耗电与散热，不要放入不通风的包内。')}</p>}
   {status?.sleep_prevention.error && <p role="alert" className="tray-error">{status.sleep_prevention.error}</p>}
  </section>
  {error && <p role="alert" className="tray-error">{error}</p>}
  <footer><span>{import.meta.env.VITE_OPENSURGE_RELEASE_TAG} · Wind Rose</span><button onClick={() => show('diagnostics')}>{t('诊断')}</button></footer>
 </main>
}
