import { connectivityStatusLabel, connectivityTone, routeLabel } from '../connectivity'
import type { ConnectivityResult, ConnectivityTarget } from '../types'

type ConnectivityTargetCardProps = {
  target: ConnectivityTarget
  result?: ConnectivityResult
  testing: boolean
  enforceBaseline: boolean
}

export function ConnectivityTargetCard({ target, result, testing, enforceBaseline }: ConnectivityTargetCardProps) {
  const tone = connectivityTone(result, testing)
  const expected = enforceBaseline ? target.expected_route : 'any'
  const mismatch = Boolean(result && expected !== 'any' && result.route !== 'unknown' && result.route !== expected)
  const filled = signalStrength(tone)

  return <article className={`connectivity-target ${tone} ${mismatch ? 'route-mismatch' : ''}`}>
    <div className="target-main"><span className="target-symbol" aria-hidden="true">{target.symbol}</span><div><strong>{target.name}</strong><small>{new URL(target.url).hostname}</small></div><span className={`connectivity-value ${tone}`}>{connectivityStatusLabel(result, testing)}</span></div>
    <div className="signal-meter" aria-hidden="true">{Array.from({ length: 8 }, (_, index) => <i className={index < filled ? 'active' : ''} key={index} />)}</div>
    <div className="route-summary"><span><small>预期</small><strong>{expected === 'any' ? '仅观察' : expected === 'direct' ? 'DIRECT' : '代理链路'}</strong></span><span className="route-arrow" aria-hidden="true">→</span><span><small>实际</small><strong>{result?.chain.length ? result.chain.join(' → ') : routeLabel(result?.route)}</strong></span>{mismatch && <span className="mismatch-badge">路径不符</span>}</div>
    {result && <details className="target-details"><summary>查看检测证据</summary><div className="target-evidence"><p><span>命中规则</span><strong>{result.rule || '未采集'}{result.rule_payload ? ` · ${result.rule_payload}` : ''}</strong></p><p><span>HTTP 状态</span><strong>{result.http_status || '—'}</strong></p><p><span>检测时间</span><strong>{new Date(result.tested_at).toLocaleTimeString()}</strong></p><div className="sample-list">{result.samples.map((sample, index) => <span key={`${sample.status}-${index}`} title={sample.error}><b>第 {index + 1} 轮</b>{sample.delay_ms ? `${sample.delay_ms} ms` : connectivitySampleLabel(sample.status)}</span>)}</div></div></details>}
  </article>
}

function signalStrength(tone: string) {
  if (tone === 'excellent') return 8
  if (tone === 'good') return 6
  if (tone === 'slow') return 3
  if (tone === 'very-slow') return 1
  if (tone === 'testing') return 4
  return 0
}

function connectivitySampleLabel(status: string) {
  if (status === 'timeout') return '超时'
  if (status === 'dns_error') return 'DNS 失败'
  if (status === 'tls_error') return 'TLS 失败'
  if (status === 'reachable') return '可达'
  return '失败'
}
