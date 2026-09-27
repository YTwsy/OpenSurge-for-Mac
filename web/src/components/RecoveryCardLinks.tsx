import { useEffect, useRef, useState } from 'react'
import { api } from '../api'
import { desktopAction, isDesktop } from '../desktop'
import { t } from '../i18n'

export function RecoveryCardLinks() {
  const [text, setText] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const dialog = useRef<HTMLDialogElement>(null)
  useEffect(() => { if (text !== null) dialog.current?.showModal() }, [text])
  if (!isDesktop()) return <div className="recovery-card-actions"><a href="/api/v1/recovery/card" target="_blank" rel="noopener noreferrer">{t('查看恢复卡')}</a><a href="/api/v1/recovery/card?download=1" download="OpenSurge-WiFi-DHCP-Recovery-Card.txt">{t('下载恢复卡')}</a></div>

  const open = async (download: boolean) => {
    setBusy(true)
    setMessage('')
    try {
      if (download) {
        const result = await desktopAction<{ saved: boolean }>('save-recovery-card')
        if (result.saved) setMessage(t('恢复卡已保存。'))
      } else setText(await api.recoveryCard())
    } catch (cause) { setMessage(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }

  return <>
    <div className="recovery-card-actions"><button type="button" disabled={busy} onClick={() => void open(false)}>{t('查看恢复卡')}</button><button type="button" disabled={busy} onClick={() => void open(true)}>{t('下载恢复卡')}</button></div>
    {message && <p role="status">{message}</p>}
    {text !== null && <dialog ref={dialog} className="desktop-recovery-card" aria-labelledby="desktop-recovery-card-title" onClose={() => setText(null)}>
      <header><h2 id="desktop-recovery-card-title">{t('查看恢复卡')}</h2><button type="button" autoFocus onClick={() => dialog.current?.close()}>{t('关闭预览')}</button></header>
      <pre tabIndex={0}>{text}</pre>
    </dialog>}
  </>
}
