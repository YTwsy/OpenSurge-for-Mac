import { ActivityCard } from '../components/ActivityCard'
import { PageHeader } from '../components/Common'
import { DeviceTrafficPanel } from '../components/DeviceTrafficPanel'
import { GatewayHealthCard } from '../components/GatewayHealthCard'
import { LiveRateCard } from '../components/LiveRateCard'
import { TrafficTrendCard } from '../components/TrafficTrendCard'
import { useDeviceTraffic } from '../hooks/useDeviceTraffic'
import type { Overview } from '../types'

export function DashboardPage({ overview, onOpenNetwork }: { overview: Overview | null; onOpenNetwork: () => void }) {
  const running = overview?.status.gateway === 'running' || overview?.status.gateway === 'degraded'
  const stopped = overview?.status.gateway === 'stopped'
  const { traffic, history, error } = useDeviceTraffic(overview?.status.gateway)
  const rates = traffic?.gateway_rates ?? { upload: 0, download: 0 }
  return <>
    <PageHeader eyebrow="CONTROL CENTER" title="全屋网关，一眼可见" description="OpenSurge 负责网关生命周期；mihomo 是当前代理引擎。" action={<button className={running ? 'danger' : 'primary'} disabled={!overview || (!running && !stopped)} onClick={onOpenNetwork}>{running ? '停止网关' : '启动网关'}</button>} />
    {overview?.warnings?.length ? <div className="dashboard-warning-stack" role="status">{overview.warnings.map(item => <div className="notice warn" key={item}>{item}</div>)}</div> : null}
    <section className="dashboard-live-grid">
      <GatewayHealthCard overview={overview} />
      <LiveRateCard direction="upload" value={rates.upload} history={history} />
      <LiveRateCard direction="download" value={rates.download} history={history} />
    </section>
    <section className="dashboard-monitor-grid">
      <ActivityCard traffic={traffic} />
      <TrafficTrendCard title="流量趋势" subtitle="网关全部 mihomo 活跃连接 · 近 60 秒内存采样" history={history} className="gateway-trend-card" />
    </section>
    <DeviceTrafficPanel gateway={overview?.status.gateway} traffic={traffic} history={history} error={error} />
  </>
}
