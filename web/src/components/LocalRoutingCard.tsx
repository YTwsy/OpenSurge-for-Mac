import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import type { LocalRouting, LocalRoutingMode, ProxyHealthEntry } from '../types'
import { OutletSummary } from './OutletSummary'

const modeLabels: Record<LocalRoutingMode, string> = {
  rule: '规则',
  global: '全局',
  direct: '直连',
}

export function LocalRoutingCard({
  running,
  interfaceName,
  lanIP,
  healthByName,
  testing,
  onHealthTest,
  onChanged,
  onPolicies,
}: {
  running: boolean
  interfaceName?: string
  lanIP?: string
  healthByName: Map<string, ProxyHealthEntry>
  testing: Set<string>
  onHealthTest: (names: string[]) => Promise<void>
  onChanged: () => Promise<void>
  onPolicies: () => void
}) {
  const [routing, setRouting] = useState<LocalRouting | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    if (!running) {
      setRouting(null)
      setError('')
      return
    }
    try {
      setRouting(await api.localRouting())
      setError('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    }
  }, [running])

  useEffect(() => { void refresh() }, [refresh])

  const apply = async (mode: LocalRoutingMode, globalPolicy?: string) => {
    setBusy(true)
    setError('')
    try {
      const updated = await api.setLocalRouting(mode, globalPolicy)
      setRouting(updated)
      await onChanged()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
      await refresh()
    } finally {
      setBusy(false)
    }
  }

  const transports = routing?.transports.map(value => value === 'tun' ? 'TUN' : '本机回环显式代理').join(' + ')
  return <article className="this-mac local-routing-card">
    <div className="source-head"><div><small>THIS MAC</small><h3>Mac 本机流量模式</h3></div><span className="effect-badge live">即时生效</span></div>
    <p>{interfaceName || 'Mac'} · {lanIP || '本机网络'}</p>
    <small className="card-help">只影响此 Mac 通过 TUN 或本机回环代理进入 mihomo 的新连接；下游设备继续使用网关规则或各自的设备策略。</small>
    <div className="local-mode-switch" role="group" aria-label="Mac 本机流量模式">
      {(['rule', 'global', 'direct'] as const).map(mode => <button
        key={mode}
        type="button"
        aria-pressed={routing?.mode === mode}
        disabled={!running || busy || (mode === 'global' && !routing?.available_modes.includes('global'))}
        onClick={() => void apply(mode)}
      >{modeLabels[mode]}</button>)}
    </div>
    {!running && <div className="local-routing-note">启动网关后可以切换本机模式。</div>}
    {routing?.global_group && <div className="local-global-policy">
      <OutletSummary
        title="本机全局出口"
        ariaLabel={`本机全局出口 当前出口 ${routing.global_group.selected}`}
        group={routing.global_group}
        healthByName={healthByName}
        testing={testing}
        onTest={onHealthTest}
        onSelect={policy => apply(routing.mode, policy)}
      />
    </div>}
    {routing && <div className="local-routing-note">
      <span>作用入口：{transports}</span>
      <span>{routing.new_connections_only ? '仅影响新连接' : '影响现有与新连接'}</span>
      {routing.mode === 'global' && routing.udp_behavior === 'reject' && <span className="attention">当前出口不支持 UDP，本机 UDP 已安全拒绝。</span>}
    </div>}
    {(routing?.warning || error) && <div className="notice warn local-routing-warning" role="alert">{error || routing?.warning}</div>}
    <button className="text-link" type="button" onClick={onPolicies}>完整节点健康见策略页 →</button>
  </article>
}
