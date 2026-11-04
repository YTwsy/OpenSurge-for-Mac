import { useEffect, useRef, useState } from 'react'
import { t } from '../i18n'

export type OperationNotification = {
  tone: 'success' | 'error'
  title: string
  message: string
}

export type OperationNotificationItem = OperationNotification & { id: number }

export function OperationNotifications({ notifications, onDismiss }: { notifications: OperationNotificationItem[]; onDismiss: (id: number) => void }) {
  if (!notifications.length) return null
  return <aside className="operation-notifications" aria-label={t('操作结果通知')}>
    {notifications.map(notification => <OperationNotificationCard key={notification.id} notification={notification} onDismiss={onDismiss} />)}
  </aside>
}

function OperationNotificationCard({ notification, onDismiss }: { notification: OperationNotificationItem; onDismiss: (id: number) => void }) {
  const [hovered, setHovered] = useState(false)
  const [focused, setFocused] = useState(false)
  const [hidden, setHidden] = useState(document.hidden)
  const remaining = useRef(6000)
  const paused = hovered || focused || hidden

  useEffect(() => {
    const visibilityChanged = () => setHidden(document.hidden)
    document.addEventListener('visibilitychange', visibilityChanged)
    return () => document.removeEventListener('visibilitychange', visibilityChanged)
  }, [])

  useEffect(() => {
    // Failures remain available to read and copy. Successes only expire while
    // visible and not being read with the pointer or keyboard.
    if (notification.tone === 'error' || paused) return
    const started = Date.now()
    const timeout = window.setTimeout(() => onDismiss(notification.id), remaining.current)
    return () => {
      window.clearTimeout(timeout)
      remaining.current = Math.max(0, remaining.current - (Date.now() - started))
    }
  }, [notification.id, notification.tone, onDismiss, paused])

  return <div className={`operation-notification ${notification.tone}`} role={notification.tone === 'error' ? 'alert' : 'status'} aria-atomic="true"
    onMouseEnter={() => setHovered(true)} onMouseLeave={() => setHovered(false)}
    onFocus={() => setFocused(true)} onBlur={event => { if (!event.currentTarget.contains(event.relatedTarget)) setFocused(false) }}>
    <span className="operation-notification-icon" aria-hidden="true">{notification.tone === 'success' ? '✓' : '!'}</span>
    <div className="operation-notification-copy"><strong>{t(notification.title)}</strong><p tabIndex={0}>{t(notification.message)}</p></div>
    <button type="button" aria-label={t('关闭通知：{{title}}', { title: t(notification.title) })} onClick={() => onDismiss(notification.id)}>×</button>
  </div>
}
