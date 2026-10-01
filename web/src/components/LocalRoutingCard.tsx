import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import type { LocalRouting, LocalRoutingMode, ProxyHealthEntry } from '../types'
import { policyDisplayName } from '../policyDisplay'
import { ConnectionRefreshControl } from './ConnectionRefreshControl'
import type { ConnectionRefreshSuggestion } from './ConnectionRefreshPrompts'
import { OutletSummary } from './OutletSummary'
import { t } from '../i18n'

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
  onOpenConnections,
  onSuggestConnectionRefresh,
}: {
  running: boolean
  interfaceName?: string
  lanIP?: string
  healthByName: Map<string, ProxyHealthEntry>
  testing: Set<string>
  onHealthTest: (names: string[]) => Promise<void>
  onChanged: () => Promise<void>
  onPolicies: () => void
  onOpenConnections?: () => void
  onSuggestConnectionRefresh?: (suggestion: ConnectionRefreshSuggestion) => void
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
      const previous = routing
      const updated = await api.setLocalRouting(mode, globalPolicy)
      setRouting(updated)
      const changed = previous?.mode !== updated.mode || (updated.mode === 'global' && previous?.global_group?.selected !== updated.global_group?.selected)
      if (changed) {
        const selection = updated.mode === 'global' && updated.global_group
          ? policyDisplayName(updated.global_group.selected, healthByName.get(updated.global_group.selected))
          : t(modeDetails[updated.mode].label)
        onSuggestConnectionRefresh?.({ key: 'gateway_local', scope: 'gateway_local', subject: t('Mac 本机'), selection })
      }
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
    <div className="local-mac-heading"><span className="local-mac-symbol" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><rect x="3" y="4" width="18" height="13" rx="2" /><path d="M8 21h8m-4-4v4" /></svg></span><div><h2>{t('当前 Mac 的设备设置')}</h2><p>{interfaceName || 'Mac'} · {lanIP || t('本机网络')}</p></div><span className="effect-badge live">{t('仅影响本机')}</span></div>
    <div className="local-mode-switch" role="group" aria-label={t('这台 Mac 的出口方式')}>
      {(['rule', 'global', 'direct'] as const).map(mode => <button
        key={mode}
        type="button"
        aria-pressed={routing?.mode === mode}
        disabled={!running || !routing || busy || (mode === 'global' && !routing.available_modes.includes('global'))}
        onClick={() => void apply(mode)}
      >{t(modeDetails[mode].label)}</button>)}
    </div>
    {!running && <div className="local-routing-note">{t('启动网关后可以切换本机模式。')}</div>}
    {running && !routing && !error && <div className="local-routing-note">{t('正在读取本机设置…')}</div>}
    {mode && <div className="local-routing-state" role="status">
      <strong>{t(mode.description)}</strong>
      <small>{t('即时生效，仅影响本机的新连接；局域网访问和下游设备不受影响。')}</small>
    </div>}
    {routing?.mode === 'global' && routing.global_group && <div className="local-global-policy">
      <OutletSummary
        inline
        title={t('本机全局出口')}
        ariaLabel={t('本机全局策略组 当前策略 {{selected}}', { selected: routing.global_group.selected })}
        group={routing.global_group}
        healthByName={healthByName}
        testing={testing}
        onTest={onHealthTest}
        onSelect={policy => apply('global', policy)}
      />
    </div>}
    {udpRejected && <div className="notice warn local-routing-warning" role="alert">{t('当前固定出口不支持 UDP，部分应用可能无法联网。')}</div>}
    {(runtimeWarning || error) && <div className="notice warn local-routing-warning" role="alert">{error || runtimeWarning}</div>}
    <details className="local-routing-tools"><summary>{t('连接与详情')}</summary>
    {onOpenConnections && <button className="text-link" type="button" onClick={onOpenConnections}>{t('查看本机连接')}</button>}
    <ConnectionRefreshControl ariaLabel={t('刷新 Mac 本机连接')} disabled={!running || !routing} disabledReason={t('启动网关并读取本机设置后可以刷新连接。')} refresh={api.refreshLocalConnections} onRefreshed={onChanged} />
    <button className="text-link" type="button" onClick={onPolicies}>{t('前往策略与节点健康')} →</button>
    </details>
  </article>
}
