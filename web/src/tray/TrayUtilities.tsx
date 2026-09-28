import { useCallback, useEffect, useRef, useState } from 'react'
import { desktopAction } from '../desktop'
import { t } from '../i18n'
import { watchVisibleRefresh } from '../visibility'

type Login = { state: 'enabled' | 'disabled' | 'approval' | 'unavailable', sequence: number, failed: boolean }
type Update = { current: string, version?: string, url?: string, checking: boolean, checked: boolean, failed: boolean, sequence: number }
type Utilities = { login: Login, update: Update, uninstall?: 'available' | 'preview' | 'missing' }

export function TrayUtilities({ canUninstall = false }: { canUninstall?: boolean }) {
 const [state, setState] = useState<Utilities | null>(null)
 const [busy, setBusy] = useState(false)
 const [error, setError] = useState('')
 const active = useRef(true)
 const pending = useRef(false)
 const accept = useCallback((next: Partial<Utilities>) => {
  if (!active.current) return
  setState(current => {
   if (!current) return next.login && next.update ? next as Utilities : null
   return {
    login: next.login && next.login.sequence >= current.login.sequence ? next.login : current.login,
    update: next.update && next.update.sequence >= current.update.sequence ? next.update : current.update,
    uninstall: next.uninstall ?? current.uninstall,
   }
  })
 }, [])
 const check = useCallback(async () => {
  try { accept({ update: await desktopAction<Update>('check-updates') }) }
  catch { if (active.current) setError(t('无法检查更新，请稍后重试。')) }
 }, [accept])
 useEffect(() => {
  active.current = true
  const stop = watchVisibleRefresh(async () => {
   try { accept(await desktopAction<Utilities>('utilities')) } catch { /* Keep OS state; gateway connectivity is independent. */ }
  }, 1000)
  const onCheck = () => { void check() }
  window.addEventListener('opensurge:check-update', onCheck)
  return () => { active.current = false; stop(); window.removeEventListener('opensurge:check-update', onCheck) }
 }, [accept, check])
 const changeLogin = async (enabled: boolean) => {
  if (pending.current) return
  pending.current = true; setBusy(true); setError('')
  try { accept({ login: await desktopAction<Login>('login-item', { enabled }) }) }
  catch (cause) { if (active.current) setError(cause instanceof Error ? cause.message : String(cause)) }
  finally { pending.current = false; if (active.current) setBusy(false) }
 }
 const openLink = (action: string, body: Record<string, unknown> = {}) => {
  void desktopAction(action, body).catch(() => { if (active.current) setError(t('桌面操作未完成，请重试。')) })
 }
 const uninstall = async () => {
  if (pending.current) return
  pending.current = true; setBusy(true); setError('')
  try { await desktopAction('uninstall'); accept(await desktopAction<Utilities>('utilities')) }
  catch (cause) { if (active.current) setError(cause instanceof Error ? cause.message : String(cause)) }
  finally { pending.current = false; if (active.current) setBusy(false) }
 }
 if (!state) return null
 const { login, update } = state
 return <section className="tray-utilities" aria-label={t('App 设置与更新')}>
  <h2>{t('App 设置与更新')}{update.version && <span className="tray-update-dot" aria-label={t('有可用的稳定版更新')} />}</h2>
  <label><span>{t('登录时显示')}</span><input type="checkbox" role="switch" checked={login.state === 'enabled' || login.state === 'approval'} disabled={busy || login.state === 'unavailable'} onChange={event => void changeLogin(event.target.checked)} /></label>
  {login.state === 'approval' && <p className="tray-hint">{t('等待 macOS 批准登录项。')} <button onClick={() => openLink('login-settings')}>{t('打开登录项设置')}</button></p>}
  {login.state === 'unavailable' && <p className="tray-hint">{t('当前 App 无法管理登录项。请从已安装的应用中设置。')}</p>}
  {login.failed && <p role="alert" className="tray-error">{t('登录项未能更新，已保留 macOS 的实际状态。')}</p>}
  <div className="tray-row"><button disabled={update.checking} onClick={() => { setError(''); void check() }}>{t(update.checking ? '正在检查…' : '检查更新')}</button><button onClick={() => openLink('show-main', { page: 'diagnostics' })}>{t('诊断')}</button></div>
  {!update.checking && (update.failed ? <p role="status" className="tray-hint">{t('无法检查更新，请稍后重试。')}</p> : update.version && update.url ? <p className="tray-hint"><button onClick={() => openLink('open-external', { url: update.url })}>{t('打开稳定版 {{version}} 下载页', { version: update.version })}</button></p> : update.checked && <p role="status" className="tray-hint">{t('没有更新的稳定版')}</p>)}
  <button className="tray-uninstall" disabled={busy || !canUninstall || state.uninstall !== 'available'} onClick={() => void uninstall()}>{t('卸载 OpenSurge…')}</button>
  {state.uninstall === 'preview' ? <p className="tray-hint">{t('预览版不会卸载正式安装的 OpenSurge。')}</p> : state.uninstall === 'missing' ? <p className="tray-hint">{t('缺少可信的卸载组件，请重新安装当前版本。')}</p> : !canUninstall && !error && <p className="tray-hint">{t('请先在网络设置中停止网关，再卸载 OpenSurge。')}</p>}
  {error && <p role="alert" className="tray-error">{error}</p>}
 </section>
}
