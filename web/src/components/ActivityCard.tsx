import type { DeviceTraffic } from '../types'

export function ActivityCard({ traffic }: { traffic: DeviceTraffic | null }) {
  const matchedConnections = traffic?.totals.active_connections ?? 0
  const unmatchedConnections = traffic?.unmatched_connections ?? 0
  return <article className="activity-card">
    <header><div><small>ACTIVITY</small><h2>当前活动</h2></div><span className="activity-pulse" aria-hidden="true" /></header>
    <strong className="activity-total">{matchedConnections + unmatchedConnections}</strong>
    <span className="activity-total-label">活跃连接</span>
    <div className="activity-breakdown">
      <span><strong>{matchedConnections}</strong><small>设备连接</small></span>
      <span><strong>{traffic?.totals.devices ?? 0}</strong><small>DHCP 设备</small></span>
      <span><strong>{unmatchedConnections}</strong><small>未归属连接</small></span>
    </div>
  </article>
}
