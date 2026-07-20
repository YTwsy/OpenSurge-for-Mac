import { statusLabel } from '../status'
import type { Overview } from '../types'
import { StatusDot } from './Common'

export function GatewayHealthCard({ overview }: { overview: Overview | null }) {
  const status = overview?.status
  const running = status?.gateway === 'running'
  const configState = overview?.drift ? running ? '待重载' : '下次启动应用' : '已同步'
  return <article className="gateway-health-card" aria-label="网关状态">
    <div className="gateway-health-main">
      <div className="gateway-health-identity"><div className="orb"><span /></div><div><small>GATEWAY</small><h2>{statusLabel(status?.gateway)}</h2><p>{status?.interface ?? '—'} · {status?.lan_ip ?? '等待状态'}</p></div></div>
      <div className="gateway-health-meta">
        <GatewayMeta label="接管模式" value={topologyLabel(overview?.topology)} />
        <GatewayMeta label="配置状态" value={configState} tone={overview?.drift ? 'warn' : 'ok'} />
      </div>
    </div>
    <div className="gateway-service-strip" aria-label="核心服务状态">
      <ServiceState label={status?.dhcp_enabled === false ? 'DNS' : 'DHCP / DNS'} state={status?.dhcp} />
      <ServiceState label="mihomo" state={status?.mihomo} />
      <ServiceState label="PF Anchor" state={status?.pf_anchor} />
      <ServiceState label="IPv4 转发" state={status?.forwarding} />
    </div>
  </article>
}

function GatewayMeta({ label, value, tone = '' }: { label: string; value: string; tone?: 'ok' | 'warn' | '' }) {
  return <span className={`gateway-meta ${tone}`.trim()}><small>{label}</small><strong>{value}</strong></span>
}

function ServiceState({ label, state = '—' }: { label: string; state?: string }) {
  return <span className="gateway-service-state"><StatusDot status={state} /><span><strong>{label}</strong><small>{state}</small></span></span>
}

function topologyLabel(topology?: string) {
  if (topology === 'same_wifi_dhcp') return '局域网 DHCP 接管'
  if (topology === 'same_lan') return '手工网关模式'
  if (topology === 'isolated_lan') return '独立下游 LAN'
  return 'IPv4 网关'
}
