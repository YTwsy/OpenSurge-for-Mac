import { useState } from 'react'
import type { ConnectionRefreshResult } from '../types'

const refreshExplanation = '手动关闭当前仍使用旧链路的连接；客户端产生新连接后，将按当前选择的新节点建立。'

export function ConnectionRefreshControl({
  ariaLabel,
  disabled = false,
  disabledReason,
  refresh,
  onRefreshed,
}: {
  ariaLabel: string
  disabled?: boolean
  disabledReason?: string
  refresh: () => Promise<ConnectionRefreshResult>
  onRefreshed?: () => Promise<void>
}) {
  const [busy, setBusy] = useState(false)
  const [feedback, setFeedback] = useState<{ tone: 'success' | 'error'; message: string } | null>(null)

  const run = async () => {
    setBusy(true)
    setFeedback(null)
    try {
      const result = await refresh()
      setFeedback({
        tone: 'success',
        message: result.closed_connections > 0
          ? `已关闭 ${result.closed_connections} 个连接，等待客户端建立新连接。`
          : '当前没有需要刷新的连接。',
      })
      try { await onRefreshed?.() } catch { /* Connection refresh already succeeded; status refresh is best-effort. */ }
    } catch (cause) {
      setFeedback({ tone: 'error', message: cause instanceof Error ? cause.message : String(cause) })
    } finally {
      setBusy(false)
    }
  }

  return <div className="connection-refresh-control">
    <div className="connection-refresh-copy">
      <strong>刷新连接</strong>
      <small>{disabled && disabledReason ? disabledReason : refreshExplanation}</small>
    </div>
    <button className="connection-refresh-button" type="button" aria-label={ariaLabel} disabled={disabled || busy} onClick={() => void run()}>{busy ? '正在刷新…' : '刷新连接'}</button>
    {feedback ? <small className={`connection-refresh-feedback ${feedback.tone}`} role={feedback.tone === 'error' ? 'alert' : 'status'}>{feedback.message}</small> : null}
  </div>
}
