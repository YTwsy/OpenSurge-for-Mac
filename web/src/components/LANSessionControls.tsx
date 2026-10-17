import { useId, useState } from 'react'
import { authenticationRequiredEvent } from '../api'
import { lanAPI, lanError, validManagementPassword } from '../lanAccess'
import { t } from '../i18n'
import '../lan-access.css'

export function LANSessionControls() {
  const passwordFormID = useId()
  const [editing, setEditing] = useState(false)
  const [oldPassword, setOldPassword] = useState('')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const logout = async () => {
    setBusy(true); setError('')
    try { await lanAPI.logout(); window.dispatchEvent(new Event(authenticationRequiredEvent)) }
    catch (cause) { setError(lanError(cause)) }
    finally { setBusy(false) }
  }
  const change = async (event: React.FormEvent) => {
    event.preventDefault(); setError('')
    if (!validManagementPassword(password)) { setError(lanError('password_invalid')); return }
    if (password !== confirmation) { setError(t('两次输入的密码不一致。')); return }
    setBusy(true)
    try { await lanAPI.changePassword(oldPassword, password); window.dispatchEvent(new Event(authenticationRequiredEvent)) }
    catch (cause) { setError(lanError(cause)) }
    finally { setBusy(false); setOldPassword(''); setPassword(''); setConfirmation('') }
  }
  return <div className="lan-session-controls">
    <button type="button" className="sidebar-control-row" disabled={busy} aria-expanded={editing} aria-controls={passwordFormID} onClick={() => { setEditing(value => !value); setError(''); setOldPassword(''); setPassword(''); setConfirmation('') }}><span className="sidebar-control-copy"><strong>{t('修改管理密码')}</strong></span></button>
    {editing && <form id={passwordFormID} className="lan-password-form" onSubmit={event => void change(event)}>
      <label>{t('当前密码')}<input type="password" autoComplete="current-password" value={oldPassword} onChange={event => setOldPassword(event.target.value)} required disabled={busy} /></label>
      <label>{t('新管理密码')}<input type="password" autoComplete="new-password" value={password} onChange={event => setPassword(event.target.value)} required disabled={busy} maxLength={512} /></label>
      <label>{t('确认新密码')}<input type="password" autoComplete="new-password" value={confirmation} onChange={event => setConfirmation(event.target.value)} required disabled={busy} maxLength={512} /></label>
      <p className="lan-hint">{t('修改后所有局域网浏览器需要重新登录。')}</p>
      <button type="submit" disabled={busy}>{t(busy ? '正在保存…' : '保存密码')}</button>
    </form>}
    <button type="button" className="sidebar-control-row" disabled={busy} onClick={() => void logout()}><span className="sidebar-control-copy"><strong>{t('退出登录')}</strong></span></button>
    {error && <p className="lan-error" role="alert">{error}</p>}
  </div>
}
