import { useEffect } from 'react'

export type OperationNotification = {
  tone: 'success' | 'error'
  title: string
  message: string
}

export type OperationNotificationItem = OperationNotification & { id: number }

export function OperationNotifications({ notifications, onDismiss }: { notifications: OperationNotificationItem[]; onDismiss: (id: number) => void }) {
  if (!notifications.length) return null
  return <aside className="operation-notifications" aria-label="操作结果通知">
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
    <div><strong>{notification.title}</strong><p>{notification.message}</p></div>
    <button type="button" aria-label={`关闭通知：${notification.title}`} onClick={() => onDismiss(notification.id)}>×</button>
  </div>
}
