import { gatewayDisplayState, gatewayIsTransitioning, gatewayStatusDetail, gatewayStatusLabel, takeoverLabel } from '../status'
import type { Overview } from '../types'
import { StatusDot } from './Common'
import { t } from '../i18n'

export function GatewayHealthCard({ overview }: { overview: Overview | null }) {
  const status = overview?.status
  const state = gatewayDisplayState(overview?.presentation, status?.gateway, status?.runtime_state)
  const detail = gatewayStatusDetail(overview?.presentation)
  const transitioning = gatewayIsTransitioning(state)
  const applying = ['starting', 'reloading', 'changing'].includes(state)
  const configState = t(overview?.drift ? applying ? '正在应用…' : state === 'stopped' ? '下次启动应用' : '待应用' : '已同步')
  const takeoverState = (value?: string) => transitioning && value === 'failed' ? 'changing' : value
  const takeoverValue = (value?: string) => transitioning && value === 'failed' ? t('尚未就绪') : takeoverLabel(value)
  return <article className="gateway-health-card" aria-label={t('网关状态')}>
    <div className="gateway-health-main">
      <div className="gateway-health-identity"><div className="orb"><StatusDot status={state} /></div><div><small>GATEWAY</small><h2>{gatewayStatusLabel(state)}</h2><p>{detail || `${status?.interface ?? '—'} · ${status?.lan_ip ?? t('等待状态')}`}</p></div></div>
      <div className="gateway-health-meta">
        <GatewayMeta label={t('接管模式')} value={topologyLabel(overview?.topology)} />
        <GatewayMeta label={t('配置状态')} value={configState} tone={overview?.drift ? 'warn' : 'ok'} />
      </div>
    </div>
    <div className="gateway-service-strip" aria-label={t('核心服务状态')}>
      <ServiceState label={status?.dhcp_enabled === false ? 'DNS' : 'DHCP / DNS'} state={status?.dhcp} />
      <ServiceState label="mihomo" state={status?.mihomo} />
      <ServiceState label={status?.tun_interface ? `TUN · ${status.tun_interface}` : 'TUN'} state={status?.tun} />
      <ServiceState label="PF Anchor" state={status?.pf_anchor} />
      <ServiceState label={t('IPv4 接管')} state={takeoverState(status?.ipv4_takeover)} value={takeoverValue(status?.ipv4_takeover)} />
      <ServiceState label={t('IPv6 接管')} state={takeoverState(status?.ipv6_takeover)} value={takeoverValue(status?.ipv6_takeover)} />
    </div>
  </article>
}

function GatewayMeta({ label, value, tone = '' }: { label: string; value: string; tone?: 'ok' | 'warn' | '' }) {
  return <span className={`gateway-meta ${tone}`.trim()}><small>{t(label)}</small><strong>{t(value)}</strong></span>
}

function ServiceState({ label, state = '—', value }: { label: string; state?: string; value?: string }) {
  return <span className="gateway-service-state"><StatusDot status={state} /><span><strong>{t(label)}</strong><small>{value ?? t(state)}</small></span></span>
}

function topologyLabel(topology?: string) {
  if (topology === 'same_wifi_dhcp') return t('局域网 DHCP 接管')
  if (topology === 'same_lan') return t('旁路由模式')
  if (topology === 'isolated_lan') return t('独立下游 LAN')
  return t('IPv4 网关')
}
