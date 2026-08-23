import { useEffect } from 'react'
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
  useEffect(() => {
    const timeout = window.setTimeout(() => onDismiss(notification.id), notification.tone === 'success' ? 6000 : 10000)
    return () => window.clearTimeout(timeout)
  }, [notification.id, notification.tone, onDismiss])

  return <div className={`operation-notification ${notification.tone}`} role={notification.tone === 'error' ? 'alert' : 'status'}>
    <span className="operation-notification-icon" aria-hidden="true">{notification.tone === 'success' ? '✓' : '!'}</span>
    <div><strong>{t(notification.title)}</strong><p>{t(notification.message)}</p></div>
    <button type="button" aria-label={t('关闭通知：{{title}}', { title: t(notification.title) })} onClick={() => onDismiss(notification.id)}>×</button>
  </div>
}
