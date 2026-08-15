import { useId, useMemo, useState } from 'react'
import { testedAgo } from '../proxyHealth'
import type { ProxyGroup, ProxyHealthEntry } from '../types'
import { Empty } from './Common'
import { ProxyHealthBadge } from './ProxyHealthBadge'

type PolicyGroupHealthCardProps = {
  group: ProxyGroup
  search: string
  healthByName: Map<string, ProxyHealthEntry>
  testing: Set<string>
  onTest: (names: string[]) => Promise<void>
  onSelect: (policy: string) => Promise<void>
}

export function PolicyGroupHealthCard({ group, search, healthByName, testing, onTest, onSelect }: PolicyGroupHealthCardProps) {
  const [expanded, setExpanded] = useState(false)
  const [switching, setSwitching] = useState('')
  const [error, setError] = useState('')
  const contentID = useId()
  const options = useMemo(() => group.name.toLowerCase().includes(search.toLowerCase()) ? group.options : group.options.filter(option => option.toLowerCase().includes(search.toLowerCase())), [group.name, group.options, search])
  const probeable = useMemo(() => group.options.filter(option => healthByName.get(option)?.probeable), [group.options, healthByName])
  const reachable = probeable.filter(option => healthByName.get(option)?.status === 'reachable').length
  const manual = ['selector', 'select'].includes(group.type.toLowerCase())

  const select = async (policy: string) => {
    if (!manual || policy === group.selected) return
    setSwitching(policy); setError('')
    try { await onSelect(policy) }
    catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setSwitching('') }
  }

  return <article className={`policy-health-group ${expanded ? 'expanded' : 'collapsed'}`}>
    <header className="policy-group-head"><div><span className="group-kicker"><span>{group.type}</span>{group.name.startsWith('device/') && <span>设备策略</span>}</span><h2>{group.name}</h2><p>当前出口 <strong>{group.selected || '未选择'}</strong>{!manual && ' · 自动策略组'}</p></div><div className="group-health-summary"><span><strong>{reachable}</strong> / {probeable.length} 可达</span><button type="button" disabled={!probeable.length || probeable.some(name => testing.has(name))} onClick={() => void onTest(probeable)}>检测本组</button><button className="policy-group-toggle" type="button" aria-expanded={expanded} aria-controls={contentID} aria-label={`${expanded ? '收起' : '展开'}策略组 ${group.name}`} onClick={() => setExpanded(value => !value)}><svg className="policy-group-chevron" viewBox="0 0 16 16" aria-hidden="true"><path d="m3 6 5 5 5-5" /></svg></button></div></header>
    {expanded && <div id={contentID} className="policy-group-content">
      {error && <div className="notice warn" role="alert">{error}</div>}
      <div className="proxy-node-grid">{options.map(option => {
        const health = healthByName.get(option)
        const selected = option === group.selected
        return <button className={`proxy-node ${selected ? 'selected' : ''}`} type="button" key={option} aria-label={`${group.name} 选择 ${option}`} aria-pressed={selected} disabled={!manual || Boolean(switching)} onClick={() => void select(option)}>
          <span className="node-select-mark" aria-hidden="true">{selected ? '✓' : ''}</span>
          <span className="node-copy"><strong>{option}</strong><small>{health?.selected && health.selected !== option ? `当前链路 → ${health.selected}` : testedAgo(health?.tested_at)}</small></span>
          <span className="node-tags">{health?.type && <span className="protocol-chip">{health.type}</span>}{health?.udp && <span className="protocol-chip">UDP</span>}</span>
          <ProxyHealthBadge health={health} testing={testing.has(option)} />
          {switching === option && <span className="switch-overlay">切换中…</span>}
        </button>
      })}</div>
      {!options.length && <Empty text="这个策略组中没有匹配的节点" />}
    </div>}
  </article>
}
