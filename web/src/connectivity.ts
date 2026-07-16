import type { ConnectivityResult } from './types'

export const connectivityCategories = {
  china: { title: '国内直连', description: '用于观察国内站点是否保持低延迟直连', flag: 'CN' },
  global: { title: '全球服务', description: '常用国际站点与基础网络服务', flag: 'GL' },
  ai: { title: 'AI 服务', description: '常用 AI 产品的访问路径与可达性', flag: 'AI' },
  developer: { title: '开发者服务', description: '代码托管、包管理与网络基础设施', flag: 'DEV' },
} as const

export function connectivityStatusLabel(result?: ConnectivityResult, testing = false) {
  if (testing) return '检测中…'
  if (!result) return '未检测'
  if (result.status === 'reachable') return result.median_ms ? `${result.median_ms} ms` : '可达'
  if (result.status === 'degraded') return result.median_ms ? `${result.median_ms} ms · 波动` : '部分可达'
  if (result.status === 'timeout') return '超时'
  if (result.status === 'dns_error') return 'DNS 失败'
  if (result.status === 'tls_error') return 'TLS 失败'
  return '连接失败'
}

export function connectivityTone(result?: ConnectivityResult, testing = false) {
  if (testing) return 'testing'
  if (!result) return 'untested'
  if (result.status !== 'reachable' && result.status !== 'degraded') return 'unreachable'
  return result.grade.replace('_', '-')
}

export function routeLabel(route?: ConnectivityResult['route']) {
  if (route === 'direct') return 'DIRECT'
  if (route === 'proxy') return '代理链路'
  if (route === 'reject') return 'REJECT'
  return '未采集'
}

export function median(values: number[]) {
  if (!values.length) return 0
  const sorted = [...values].sort((left, right) => left - right)
  return sorted[Math.floor(sorted.length / 2)]
}
