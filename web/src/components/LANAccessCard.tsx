import { useEffect, useRef, useState } from 'react'
import { QRCodeSVG } from 'qrcode.react'
import { copyText } from '../desktop'
import { lanAPI, lanError, validManagementPassword, type LANAccess, type LANSettings } from '../lanAccess'
import { t } from '../i18n'
import { watchVisibleRefresh } from '../visibility'
import { Select } from './Select'
import '../lan-access.css'

const initial: LANSettings = { enabled: false, interface: '', ipv4: '', prefix_len: 24, port: 61768 }
const addressKey = (value: { interface: string; ipv4: string; prefix_len: number }) => `${value.interface}|${value.ipv4}|${value.prefix_len}`

export function LANAccessCard() {
  const [current, setCurrent] = useState<LANAccess | null>(null)
  const [draft, setDraft] = useState<LANSettings>(initial)
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [resetPassword, setResetPassword] = useState('')
  const [resetConfirmation, setResetConfirmation] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const initialized = useRef(false)
  const generation = useRef(0)
  const saving = useRef(false)
  useEffect(() => {
    let active = true
    const stop = watchVisibleRefresh(async () => {
      if (saving.current) return
      const request = ++generation.current
      try {
        const next = await lanAPI.settings()
        if (!active || request !== generation.current) return
        // Preserve the revision attached to an edited draft. A concurrent save
        // must produce a conflict, never silently authorize overwriting it.
        setCurrent(previous => previous ? { ...next, revision: previous.revision } : next)
        if (!initialized.current) {
          initialized.current = true
          const address = next.addresses.find(item => item.interface === next.recommended_interface) ?? next.addresses[0]
          setDraft({ enabled: next.enabled, interface: next.interface || address?.interface || '', ipv4: next.ipv4 || address?.ipv4 || '', prefix_len: next.ipv4 ? next.prefix_len : address?.prefix_len ?? 24, port: next.port || 61768 })
        }
      } catch (cause) { if (active && request === generation.current) setError(lanError(cause)) }
    }, 3000)
    return () => { active = false; stop() }
  }, [])
  const validatePasswords = (value: string, confirm: string) => {
    if (!validManagementPassword(value)) { setError(lanError('password_invalid')); return false }
    if (value !== confirm) { setError(t('两次输入的密码不一致。')); return false }
    return true
  }
  const save = async (event: React.FormEvent) => {
    event.preventDefault(); setError(''); setNotice('')
    if (!current || (draft.enabled && !current.password_set && !validatePasswords(password, confirmation))) return
    setBusy(true); saving.current = true; generation.current++
    try {
      const next = await lanAPI.save(draft, current.revision, current.password_set ? undefined : password)
      setCurrent(next); setDraft({ enabled: next.enabled, interface: next.interface, ipv4: next.ipv4, prefix_len: next.prefix_len, port: next.port })
      setPassword(''); setConfirmation('')
      setNotice(t(next.state === 'listening' ? '局域网管理已开启。' : next.state === 'disabled' ? '局域网管理已关闭。' : '设置已保存，请处理下方的监听问题。'))
    } catch (cause) { setError(lanError(cause)) }
    finally { saving.current = false; setBusy(false) }
  }
  const reset = async (event: React.FormEvent) => {
    event.preventDefault(); setError(''); setNotice('')
    if (!current || !validatePasswords(resetPassword, resetConfirmation)) return
    setBusy(true); saving.current = true; generation.current++
    try {
      const next = await lanAPI.resetPassword(resetPassword, current.revision)
      setCurrent(next); setResetPassword(''); setResetConfirmation('')
      setNotice(t('管理密码已重设，局域网浏览器需要重新登录。'))
    } catch (cause) { setError(lanError(cause)) }
    finally { saving.current = false; setBusy(false) }
  }
  const reload = async () => {
    setError(''); initialized.current = false
    const request = ++generation.current
    try {
      const next = await lanAPI.settings()
      if (request !== generation.current) return
      setCurrent(next)
      const address = next.addresses.find(item => item.interface === next.recommended_interface) ?? next.addresses[0]
      setDraft({ enabled: next.enabled, interface: next.interface || address?.interface || '', ipv4: next.ipv4 || address?.ipv4 || '', prefix_len: next.ipv4 ? next.prefix_len : address?.prefix_len ?? 24, port: next.port || 61768 })
      initialized.current = true
    } catch (cause) { setError(lanError(cause)) }
  }
  const addressExists = current?.addresses.some(item => addressKey(item) === addressKey(draft))
  return <section className="settings-section lan-settings" aria-label={t('局域网管理')}>
    <div className="lan-settings-heading">
      <h2>{t('局域网管理')}</h2>
      {current && <span className={`lan-state ${current.state}`} role="status">{t(current.state === 'listening' ? '正在监听' : current.state === 'disabled' ? '已关闭' : '暂不可用')}</span>}
    </div>
    <p className="lan-hint">{t('让同一局域网的设备通过浏览器控制 OpenSurge。使用 HTTP，仅建议在受信任的局域网中开启。')}</p>
    {current ? <>
      <form className="lan-settings-form" onSubmit={event => void save(event)}>
        <label className="settings-row"><span>{t('允许局域网管理')}</span><input type="checkbox" role="switch" checked={draft.enabled} disabled={busy} onChange={event => setDraft(value => ({ ...value, enabled: event.target.checked }))} /></label>
        {draft.enabled && <div className="lan-config-fields">
          <label>{t('局域网地址')}<Select aria-label={t('局域网地址')} value={addressKey(draft)} disabled={busy} onChange={key => {
            const value = current.addresses.find(item => addressKey(item) === key)
            if (value) setDraft(previous => ({ ...previous, ...value }))
          }}>
            {!addressExists && <option value={addressKey(draft)}>{draft.ipv4 ? `${draft.interface} · ${draft.ipv4}/${draft.prefix_len} (${t('不可用')})` : t('请选择局域网地址')}</option>}
            {current.addresses.map(address => <option key={addressKey(address)} value={addressKey(address)}>{address.interface} · {address.ipv4}/{address.prefix_len}</option>)}
          </Select></label>
          {current.addresses.length === 0 && <p className="lan-hint">{t('没有可用的局域网 IPv4 地址，请先连接局域网。')}</p>}
          <details className="lan-disclosure">
            <summary><span>{t('高级设置')}</span><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="m9 5 7 7-7 7" /></svg></summary>
            <div className="lan-disclosure-content"><label>{t('管理端口')}<input type="number" min="1024" max="65535" required value={draft.port} disabled={busy} onChange={event => setDraft(value => ({ ...value, port: Number(event.target.value) }))} /></label></div>
          </details>
          {!current.password_set && <>
            <label>{t('管理密码')}<input type="password" autoComplete="new-password" required value={password} disabled={busy} onChange={event => setPassword(event.target.value)} maxLength={512} /></label>
            <label>{t('确认管理密码')}<input type="password" autoComplete="new-password" required value={confirmation} disabled={busy} onChange={event => setConfirmation(event.target.value)} maxLength={512} /></label>
            <p className="lan-hint">{t('密码需要 12–128 个字符，支持空格和长密码。')}</p>
          </>}
        </div>}
        <div className="lan-actions"><button type="submit" disabled={busy || (draft.enabled && !addressExists)}>{t(busy ? '正在保存…' : draft.enabled ? '保存并启用' : '保存设置')}</button><button type="button" disabled={busy} onClick={() => void reload()}>{t('刷新设置')}</button></div>
      </form>
      {current.error && <p className="lan-error" role="status">{lanError(current.error)}</p>}
      {current.state === 'listening' && current.url && <div className="lan-share">
        <QRCodeSVG value={current.url} size={164} marginSize={4} level="M" title={t('扫码打开管理页面')} />
        <div><strong>{t('扫码打开管理页面')}</strong><code>{current.url}</code><button type="button" onClick={() => { void copyText(current.url!).then(() => setNotice(t('访问地址已复制。'))).catch(() => setError(t('无法复制，请选择并复制访问地址。'))) }}>{t('复制访问地址')}</button><p className="lan-hint">{t('二维码只包含地址，打开后仍需输入管理密码。')}</p></div>
      </div>}
      {current.password_set && <details className="lan-reset lan-disclosure"><summary><span>{t('重设管理密码')}</span><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="m9 5 7 7-7 7" /></svg></summary><form className="lan-password-form lan-disclosure-content" onSubmit={event => void reset(event)}>
        <label>{t('新管理密码')}<input type="password" autoComplete="new-password" value={resetPassword} onChange={event => setResetPassword(event.target.value)} required disabled={busy} maxLength={512} /></label>
        <label>{t('确认新密码')}<input type="password" autoComplete="new-password" value={resetConfirmation} onChange={event => setResetConfirmation(event.target.value)} required disabled={busy} maxLength={512} /></label>
        <p className="lan-hint">{t('修改后所有局域网浏览器需要重新登录。')}</p><button type="submit" disabled={busy}>{t('保存密码')}</button>
      </form></details>}
    </> : <p role="status">{t('正在读取局域网管理设置…')}</p>}
    {notice && <p className="lan-hint" role="status">{notice}</p>}
    {error && <p className="lan-error" role="alert">{error}</p>}
    {!current && error && <button onClick={() => void reload()}>{t('重试')}</button>}
  </section>
}
