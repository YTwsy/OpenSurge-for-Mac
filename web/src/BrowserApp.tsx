import { useEffect, useState } from 'react'
import { App } from './App'
import { authenticationRequiredEvent } from './api'
import { lanAPI, lanError, setBrowserSession, type BrowserSession } from './lanAccess'
import { clearOperations } from './operations'
import { useTheme } from './hooks/useTheme'
import { t } from './i18n'
import './lan-access.css'

export function BrowserApp() {
  useTheme()
  const [session, setSession] = useState<BrowserSession | null>(null)
  const [error, setError] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [attempt, setAttempt] = useState(0)
  const [expired, setExpired] = useState(false)
  useEffect(() => {
    let active = true
    setError('')
    void lanAPI.session().then(next => {
      if (!active) return
      if (next.mode !== 'local' && next.mode !== 'lan') throw new Error('invalid access mode')
      setBrowserSession(next); setSession(next)
    }).catch(cause => { if (active) setError(lanError(cause)) })
    return () => { active = false }
  }, [attempt])
  useEffect(() => {
    if (session?.mode !== 'lan') return
    const expire = () => {
      setSession({ mode: 'lan', authenticated: false }); setExpired(true)
      setPassword(''); clearOperations()
    }
    window.addEventListener(authenticationRequiredEvent, expire)
    return () => window.removeEventListener(authenticationRequiredEvent, expire)
  }, [session?.mode])
  const login = async (event: React.FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    try {
      const next = await lanAPI.login(password)
      setPassword(''); setExpired(false); setBrowserSession(next); setSession(next)
    } catch (cause) { setError(lanError(cause)); setPassword('') }
    finally { setBusy(false) }
  }
  if (session?.mode === 'local' || session?.authenticated) return <App />
  return <main className="lan-login"><section className="lan-login-card">
    <img src="/opensurge-icon.png" alt="" width="64" height="64" />
    <h1>OpenSurge <span>for Mac</span></h1>
    <p>{t('局域网管理')}</p>
    {session ? <form onSubmit={event => void login(event)}>
      {expired && <p role="status">{t('登录已过期或已退出，请重新输入管理密码。')}</p>}
      <label htmlFor="lan-password">{t('管理密码')}</label>
      <input id="lan-password" name="password" type="password" autoComplete="current-password" autoFocus value={password} onChange={event => setPassword(event.target.value)} disabled={busy} required maxLength={512} />
      <button type="submit" className="primary" disabled={busy || !password}>{t(busy ? '正在登录…' : '登录')}</button>
      <p className="lan-hint">{t('忘记密码？请在 Mac 的 OpenSurge 设置中重设。')}</p>
    </form> : !error && <p role="status">{t('正在连接管理服务…')}</p>}
    {error && <p className="lan-error" role="alert">{error}</p>}
    {!session && error && <button onClick={() => setAttempt(value => value + 1)}>{t('重试')}</button>}
  </section></main>
}
