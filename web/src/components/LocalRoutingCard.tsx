import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import type { LocalRouting, LocalRoutingMode, ProxyHealthEntry } from '../types'
import { OutletSummary } from './OutletSummary'

const modeDetails: Record<LocalRoutingMode, { label: string; description: string }> = {
  rule: { label: '按规则', description: '根据网站和网关规则自动分流' },
  global: { label: '固定出口', description: '本机公网流量统一使用当前全局策略' },
  direct: { label: '本机直连', description: '本机公网流量不使用代理' },
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

  const mode = routing ? modeDetails[routing.mode] : null
  const udpRejected = routing?.mode === 'global' && routing.udp_behavior === 'reject'
  const runtimeWarning = udpRejected ? '' : routing?.warning
  return <article className="this-mac local-routing-card">
    <div className="source-head"><div><small>THIS MAC</small><h3>出口方式</h3></div><span className="effect-badge live">仅影响本机</span></div>
    <p>{interfaceName || 'Mac'} · {lanIP || '本机网络'}</p>
    <div className="local-mode-switch" role="group" aria-label="这台 Mac 的出口方式">
      {(['rule', 'global', 'direct'] as const).map(mode => <button
        key={mode}
        type="button"
        aria-pressed={routing?.mode === mode}
        disabled={!running || !routing || busy || (mode === 'global' && !routing.available_modes.includes('global'))}
        onClick={() => void apply(mode)}
      >{modeDetails[mode].label}</button>)}
    </div>
    {!running && <div className="local-routing-note">启动网关后可以切换本机模式。</div>}
    {running && !routing && !error && <div className="local-routing-note">正在读取本机设置…</div>}
    {mode && <div className="local-routing-state" role="status">
      <strong>{mode.description}</strong>
      <small>局域网访问和下游设备不受影响；切换只影响新连接。</small>
    </div>}
    {routing?.mode === 'global' && routing.global_group && <div className="local-global-policy">
      <OutletSummary
        title="本机全局出口"
        ariaLabel={`本机全局策略组 当前策略 ${routing.global_group.selected}`}
        group={routing.global_group}
        healthByName={healthByName}
        testing={testing}
        onTest={onHealthTest}
        onSelect={policy => apply('global', policy)}
      />
    </div>}
    {udpRejected && <div className="notice warn local-routing-warning" role="alert">当前固定出口不支持 UDP，部分应用可能无法联网。</div>}
    {(runtimeWarning || error) && <div className="notice warn local-routing-warning" role="alert">{error || runtimeWarning}</div>}
    <button className="text-link" type="button" onClick={onPolicies}>前往策略与节点健康 →</button>
  </article>
}
