import { api } from '../api'
import { PageHeader, SectionTitle, StatusDot } from '../components/Common'
import type { Overview } from '../types'

export function DiagnosticsPage({ overview }: { overview: Overview | null }) {
  return <>
    <PageHeader eyebrow="DIAGNOSTICS" title="诊断与 Provider" description="错误保持结构化，不把 mihomo API 局部不可用误报为整个控制面失效。" />
    <section className="split"><div><SectionTitle title="Doctor" subtitle={overview?.doctor_healthy ? '基础检查通过' : '发现需要处理的问题'} />{overview?.doctor.map(check => <div className="check" key={check.name}><span className={check.ok ? 'ok-mark' : 'bad-mark'}>{check.ok ? '✓' : '!'}</span><div><strong>{check.name}</strong><small>{check.message}</small></div></div>)}</div><div><SectionTitle title="Proxy Providers" subtitle="可从这里观察，不在菜单栏中执行刷新" />{overview?.providers.proxy_providers.map(provider => <div className="row" key={provider.name}><StatusDot status={provider.proxies.some(proxy => proxy.alive) ? 'running' : 'degraded'} /><div className="grow"><strong>{provider.name}</strong><small>{provider.proxy_count} proxies · {provider.vehicle_type}</small></div><button onClick={() => void api.refreshProvider(provider.name)}>刷新</button></div>)}</div></section>
  </>
}
