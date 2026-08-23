import type { ProxyHealthEntry } from './types'
import { t } from './i18n'

export function healthLabel(health: ProxyHealthEntry | undefined, testing = false) {
  if (testing) return t('检测中…')
  if (!health) return t('未检测')
  if (health.status === 'reachable') return health.delay_ms ? `${health.delay_ms} ms` : t('可达')
  if (health.status === 'timeout') return t('超时')
  if (health.status === 'unreachable') return t('不可达')
  if (health.status === 'error') return t('失败')
  if (health.status === 'not_applicable') return t('不适用')
  return t('未检测')
}

export function healthTone(health: ProxyHealthEntry | undefined, testing = false) {
  if (testing) return 'testing'
  if (!health) return 'untested'
  if (health.status !== 'reachable') return health.status
  const delay = health.delay_ms ?? 0
  if (!delay || delay <= 250) return 'excellent'
  if (delay <= 650) return 'good'
  if (delay <= 1500) return 'slow'
  return 'very-slow'
}

export function testedAgo(value?: string) {
  if (!value) return t('尚未检测')
  const elapsed = Date.now() - Date.parse(value)
  if (!Number.isFinite(elapsed) || elapsed < 0) return t('刚刚检测')
  const minutes = Math.floor(elapsed / 60_000)
  if (minutes < 1) return t('刚刚检测')
  if (minutes < 60) return t('{{count}} 分钟前', { count: minutes })
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return t('{{count}} 小时前', { count: hours })
  return t('{{count}} 天前', { count: Math.floor(hours / 24) })
}
