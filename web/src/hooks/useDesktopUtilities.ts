import { useCallback, useEffect, useRef, useState } from 'react'
import { desktopAction } from '../desktop'
import { t } from '../i18n'
import { watchVisibleRefresh } from '../visibility'

export type Login = { state: 'enabled' | 'disabled' | 'approval' | 'not_found' | 'unavailable', sequence: number, failed: boolean, error?: string }
type Update = { current: string, version?: string, url?: string, checking: boolean, checked: boolean, failed: boolean, sequence: number }
type Utilities = { login: Login, update: Update, uninstall?: 'available' | 'preview' | 'missing' }

export function useDesktopUtilities() {
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
 return { state, busy, error, setError, check, changeLogin, openLink, uninstall }
}
