import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import { Empty, PageHeader } from '../components/Common'
import { OutletSummary } from '../components/OutletSummary'
import { PolicyGroupHealthCard } from '../components/PolicyGroupHealthCard'
import { useProxyHealth } from '../hooks/useProxyHealth'
import type { LocalRouting, Overview, ProxyHealthEntry } from '../types'

type PolicyScope = 'all' | 'global' | 'device'

export function PoliciesPage({ overview, onChanged }: { overview: Overview | null; onChanged: () => Promise<void> }) {
  const [search, setSearch] = useState('')
  const [scope, setScope] = useState<PolicyScope>('global')
  const { byName, testing, error, refresh, test } = useProxyHealth()
  const groups = overview?.policies ?? []
  const filteredGroups = useMemo(() => groups.filter(group => {
    const device = group.name.startsWith('device/')
    if (scope === 'global' && device) return false
    if (scope === 'device' && !device) return false
    const query = search.trim().toLowerCase()
    return !query || group.name.toLowerCase().includes(query) || group.options.some(option => option.toLowerCase().includes(query))
  }), [groups, scope, search])
  const visibleNames = useMemo(() => [...new Set(filteredGroups.flatMap(group => group.options))], [filteredGroups])
  const testableNames = useMemo(() => visibleNames.filter(name => byName.get(name)?.probeable), [visibleNames, byName])
  const reachable = visibleNames.filter(name => byName.get(name)?.status === 'reachable').length
  const tested = visibleNames.filter(name => {
    const status = byName.get(name)?.status
    return status && status !== 'untested' && status !== 'not_applicable'
  }).length

  const select = async (group: string, policy: string) => {
    await api.selectPolicy(group, policy)
    await Promise.all([onChanged(), refresh()])
  }

  return <>
    <PageHeader eyebrow="POLICIES" title="策略与节点健康" description="查看每个策略组的当前出口、节点延迟与可达性；Selector 节点点击后即时生效。" action={<button className="primary" type="button" disabled={!testableNames.length || testableNames.some(name => testing.has(name))} onClick={() => void test(testableNames)}>{testing.size ? `正在检测 ${testing.size} 个节点…` : '检测当前视图'}</button>} />
    <LocalMacGlobalPolicy
      running={overview?.status.gateway === 'running'}
      healthByName={byName}
      testing={testing}
      onTest={test}
      onChanged={async () => { await onChanged(); await refresh() }}
    />
    <section className="policy-health-overview" aria-label="节点健康概览"><div><small>当前视图</small><strong>{filteredGroups.length}</strong><span>个策略组</span></div><div><small>已检测</small><strong>{tested}</strong><span>个出口</span></div><div><small>当前可达</small><strong>{reachable}</strong><span>个出口</span></div><div className="health-legend"><span><i className="legend-dot excellent" />快速</span><span><i className="legend-dot good" />可用</span><span><i className="legend-dot slow" />较慢</span><span><i className="legend-dot unreachable" />不可达</span></div></section>
    <section className="policy-toolbar"><label className="policy-search"><span className="sr-only">搜索策略组或节点</span><input type="search" value={search} placeholder="搜索策略组或节点" onChange={event => setSearch(event.target.value)} /></label><div className="segmented" aria-label="策略组范围">{([['global', '全局策略'], ['device', '设备策略'], ['all', '全部']] as const).map(([value, label]) => <button type="button" key={value} aria-pressed={scope === value} onClick={() => setScope(value)}>{label}</button>)}</div></section>
    {error && <div className="notice warn" role="alert">节点健康暂不可用：{error}</div>}
    <section className="policy-health-list">{filteredGroups.map(group => <PolicyGroupHealthCard key={group.name} group={group} search={search.trim()} healthByName={byName} testing={testing} onTest={test} onSelect={policy => select(group.name, policy)} />)}</section>
    {!filteredGroups.length && <Empty text={groups.length ? '当前筛选没有匹配的策略组或节点' : 'mihomo 未运行或没有可选择的策略组'} />}
    <p className="evidence-note"><strong>检测范围：</strong>延迟由网关 Mac 上的 mihomo 访问探测地址得到；它不代表某台下游设备的 DHCP、DNS 或 TUN 路径已经完成端到端验收。</p>
  </>
}

function LocalMacGlobalPolicy({
  running,
  healthByName,
  testing,
  onTest,
  onChanged,
}: {
  running: boolean
  healthByName: Map<string, ProxyHealthEntry>
  testing: Set<string>
  onTest: (names: string[]) => Promise<void>
  onChanged: () => Promise<void>
}) {
  const [routing, setRouting] = useState<LocalRouting | null>(null)
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

  const select = async (policy: string) => {
    if (!routing) return
    const updated = await api.setLocalRouting(routing.mode, policy)
    setRouting(updated)
    await onChanged()
  }

  return <section className="policy-local-mac" aria-labelledby="policy-local-mac-title">
    <div className="policy-local-mac-copy">
      <div className="source-head"><div><small>THIS MAC</small><h2 id="policy-local-mac-title">本机全局策略组</h2></div><span className="effect-badge live">仅影响本机</span></div>
      <p>设备页选择“固定出口”时使用；更换策略不会改变下游设备。</p>
    </div>
    <div className="policy-local-mac-outlet">
      {routing?.global_group
        ? <OutletSummary
            title="本机全局出口"
            ariaLabel={`本机全局策略组 当前策略 ${routing.global_group.selected}`}
            group={routing.global_group}
            healthByName={healthByName}
            testing={testing}
            onTest={onTest}
            onSelect={select}
          />
        : <button className="outlet-summary unavailable" type="button" disabled><span className="outlet-summary-copy"><small>本机全局出口</small><strong>{running ? '正在读取…' : '启动网关后可用'}</strong></span></button>}
      {error && <div className="notice warn" role="alert">{error}</div>}
    </div>
  </section>
}
