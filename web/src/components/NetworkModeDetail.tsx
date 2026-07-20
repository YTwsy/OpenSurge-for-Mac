import type { ControlConfig } from '../types'

type NetworkMode = ControlConfig['gateway']['mode']

type ModeDetail = {
  title: string
  subtitle: string
  badge: string
  dhcp: string
  client: string
  change: string
  scope: string
  guidance: string
  accessibleDescription: string
}

const modeDetails = {
  same_wifi_dhcp: {
    title: '局域网 DHCP 接管',
    subtitle: '让现有局域网设备自动接入 OpenSurge',
    badge: '自动接管',
    dhcp: 'OpenSurge',
    client: '保持自动获取',
    change: '关闭主路由 DHCP',
    scope: '局域网中使用自动网络设置的设备',
    guidance: 'OpenSurge 会通过引导流程，协助你逐步完成网络设置、启动确认和停止后的网络恢复。',
    accessibleDescription: '主路由关闭 DHCP，OpenSurge 为现有局域网中的设备提供 DHCP、DNS 和默认网关。',
  },
  same_lan: {
    title: '手工网关模式',
    subtitle: '仅让局域网内的部分设备使用 OpenSurge',
    badge: '部分设备',
    dhcp: '主路由',
    client: '在部分设备上手工设置网关与 DNS',
    change: '只修改需要接入的设备',
    scope: '手工设置为使用 OpenSurge 的设备',
    guidance: '主路由和其他设备保持原有网络设置；没有手工设置网关的设备不受影响。',
    accessibleDescription: '主路由保持 DHCP，只在部分设备上手工把固定 IPv4、默认网关和 DNS 指向 OpenSurge。',
  },
  isolated_lan: {
    title: '独立下游 LAN',
    subtitle: '通过独立 AP、SSID 或 VLAN 接入 OpenSurge',
    badge: '独立网络',
    dhcp: 'OpenSurge（仅下游）',
    client: '连接下游网络后自动获取',
    change: '准备独立下游网络和接口',
    scope: '连接到下游网络的设备',
    guidance: '上游路由器通常无需改变；OpenSurge 只接管独立下游网络中的设备。',
    accessibleDescription: 'Mac 使用不同接口连接上游路由器和独立下游网络，并为全部下游设备提供 DHCP、DNS 和默认网关。',
  },
} satisfies Record<NetworkMode, ModeDetail>

export function NetworkModeDetail({ mode }: { mode: NetworkMode }) {
  const detail = modeDetails[mode]

  return <section className="section mode-detail">
    <div className="mode-detail-heading">
      <div><h2>{detail.title}</h2><p>{detail.subtitle}</p></div>
      <span className="pill ok">{detail.badge}</span>
    </div>
    <div className="mode-detail-layout">
      <div
        className={`mode-topology-image mode-topology-image-${mode}`}
        role="img"
        aria-label={`${detail.title}：${detail.accessibleDescription}`}
      />
      <div className="mode-detail-copy">
        <dl className="mode-facts">
          <div><dt>DHCP 来源</dt><dd>{detail.dhcp}</dd></div>
          <div><dt>设备设置</dt><dd>{detail.client}</dd></div>
          <div><dt>需要操作</dt><dd>{detail.change}</dd></div>
          <div><dt>接入范围</dt><dd>{detail.scope}</dd></div>
        </dl>
        <div className="notice mode-guidance">{detail.guidance}</div>
      </div>
    </div>
  </section>
}
