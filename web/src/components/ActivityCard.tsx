import type { DeviceTraffic } from '../types'

export function ActivityCard({ traffic }: { traffic: DeviceTraffic | null }) {
  const localConnections = traffic?.gateway_local.active_connections ?? 0
  const unidentifiedConnections = traffic?.unidentified_device_connections ?? 0
  const attributedConnections = Math.max(0, (traffic?.totals.active_connections ?? 0) - unidentifiedConnections)
  const unclassifiedConnections = traffic?.unclassified_connections ?? 0
  return <article className="activity-card">
    <header><div><small>ACTIVITY</small><h2>当前活动</h2></div><span className="activity-pulse" aria-hidden="true" /></header>
    <strong className="activity-total">{localConnections + attributedConnections + unidentifiedConnections + unclassifiedConnections}</strong>
    <span className="activity-total-label">活跃连接</span>
    <div className="activity-breakdown">
      <span><strong>{localConnections}</strong><small>本机连接</small></span>
      <span><strong>{attributedConnections}</strong><small>已归属设备连接</small></span>
      <span><strong>{unidentifiedConnections}</strong><small>待识别设备连接</small></span>
    </div>
  </article>
}
