import { useEffect, useState } from 'react'
import { api } from '../api'
import { PageHeader, SectionTitle, StatusDot } from '../components/Common'
import type { Diagnostics, Overview } from '../types'

export function DiagnosticsPage({ overview }: { overview: Overview | null }) {
  const [details, setDetails] = useState<Diagnostics | null>(null)
  useEffect(() => {
    let active = true
    void api.diagnostics().then(value => { if (active) setDetails(value) }).catch(() => { if (active) setDetails(null) })
    return () => { active = false }
  }, [overview?.revision, overview?.status.gateway])
  return <>
    <PageHeader eyebrow="DIAGNOSTICS" title="诊断、连接与 Provider" description="错误保持结构化；日志经过已知凭据脱敏，菜单栏只复制压缩摘要。" />
    <section className="split"><div><SectionTitle title="Doctor" subtitle={overview?.doctor_healthy ? '基础检查通过' : '发现需要处理的问题'} />{overview?.doctor.map(check => <div className="check" key={check.name}><span className={check.ok ? 'ok-mark' : 'bad-mark'}>{check.ok ? '✓' : '!'}</span><div><strong>{check.name}</strong><small>{check.message}</small></div></div>)}</div><div><SectionTitle title="Proxy Providers" subtitle="可从这里观察和刷新，不在菜单栏中执行" />{overview?.providers.proxy_providers.map(provider => <div className="row" key={provider.name}><StatusDot status={provider.proxies.some(proxy => proxy.alive) ? 'running' : 'degraded'} /><div className="grow"><strong>{provider.name}</strong><small>{provider.proxy_count} proxies · {provider.vehicle_type}</small></div><button onClick={() => void api.refreshProvider(provider.name)}>刷新</button></div>)}</div></section>
    <section className="section"><SectionTitle title="Live Connections" subtitle={details?.connection_error || `${details?.connections.connections.length ?? 0} active connections`} /><div className="inventory"><span>↑ {details?.connections.upload_total ?? 0} bytes</span><span>↓ {details?.connections.download_total ?? 0} bytes</span>{details?.connections.connections.slice(0, 12).map(connection => <span key={connection.id}>{connection.rule || 'MATCH'} · {(connection.chains ?? []).join(' → ') || connection.id.slice(0, 8)}</span>)}</div></section>
    <section className="section"><SectionTitle title="Recent logs" subtitle="每个进程最多 80 行；API 会遮蔽 mihomo secret 与 upstream credentials" />{Object.entries(details?.logs ?? {}).map(([name, lines]) => <div key={name}><h3>{name}</h3><pre>{lines.join('\n') || 'No log output'}</pre></div>)}</section>
  </>
}
