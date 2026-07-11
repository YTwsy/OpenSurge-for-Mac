import { Empty, Metric, PageHeader, SectionTitle, Service, StatusDot } from '../components/Common'
import { statusLabel } from '../status'
import type { Overview } from '../types'

export function DashboardPage({ overview, busy, onAction }: { overview: Overview | null; busy: boolean; onAction: (action: 'start' | 'stop') => void }) {
  const running = overview?.status.gateway === 'running'
  return <>
    <PageHeader eyebrow="CONTROL CENTER" title="全屋网关，一眼可见" description="OpenSurge 负责网关生命周期；mihomo 是当前代理引擎。" action={<button className={running ? 'danger' : 'primary'} disabled={busy || !overview} onClick={() => onAction(running ? 'stop' : 'start')}>{busy ? '正在提交…' : running ? '停止网关' : '启动网关'}</button>} />
    <section className="hero-grid">
      <article className="gateway-card"><div className="orb"><span /></div><div><small>GATEWAY</small><h2>{statusLabel(overview?.status.gateway)}</h2><p>{overview?.status.interface ?? '—'} · {overview?.status.lan_ip ?? '等待状态'}</p></div></article>
      <Metric label="在线客户端" value={overview?.status.client_count ?? '—'} note="DHCP leases" />
      <Metric label="配置状态" value={overview?.desired_digest && overview.applied_digest && overview.desired_digest !== overview.applied_digest ? '待重启' : '已同步'} note="desired / applied" />
    </section>
    <section className="section"><SectionTitle title="核心服务" subtitle="同一个网关运行期内的关键组件" /><div className="service-grid">
      <Service name="DHCP / DNS" state={overview?.status.dhcp} detail={overview?.status.dhcp_enabled ? 'OpenSurge 分配地址' : '外部 DHCP'} />
      <Service name="mihomo" state={overview?.status.mihomo} detail="TUN 与策略执行" />
      <Service name="PF Anchor" state={overview?.status.pf_anchor} detail="NAT 与转发边界" />
      <Service name="IPv4 Forwarding" state={overview?.status.forwarding} detail="macOS 内核转发" />
    </div></section>
    <section className="section split"><div><SectionTitle title="最近设备" subtitle="来自当前 DHCP 租约" />{overview?.leases?.length ? overview.leases.slice(0, 5).map(lease => <div className="row" key={`${lease.mac}-${lease.ip}`}><StatusDot status={lease.online ? 'running' : 'stopped'} /><div className="grow"><strong>{lease.hostname || '未命名设备'}</strong><small>{lease.mac}</small></div><code>{lease.ip}</code></div>) : <Empty text="暂无租约" />}</div><div><SectionTitle title="注意事项" subtitle="不会被静默忽略" />{overview?.warnings?.length ? overview.warnings.map(item => <div className="notice" key={item}>{item}</div>) : <Empty text="当前没有警告" />}</div></section>
  </>
}
