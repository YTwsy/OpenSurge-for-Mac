import { useState } from 'react'
import { api } from '../api'
import { Mode, PageHeader, SectionTitle } from '../components/Common'
import { recoveryLabel } from '../status'
import type { Overview } from '../types'

const stages = ['prepared', 'mac_static', 'router_dhcp_disabled_confirmed', 'gateway_active', 'gateway_stopped_waiting_router_dhcp', 'router_dhcp_restored', 'complete'] as const

export function NetworkPage({ overview, onChanged }: { overview: Overview | null; onChanged: () => Promise<void> }) {
  const [busy, setBusy] = useState(false)
  const current = overview?.recovery.stage ?? 'idle'
  const currentIndex = stages.indexOf(current as typeof stages[number])
  const next = stages[currentIndex + 1] ?? 'prepared'
  const advance = async () => {
    setBusy(true)
    try { await api.recovery(next); await onChanged() } finally { setBusy(false) }
  }
  return <><PageHeader eyebrow="NETWORK" title="网络与 DHCP 接管" description="先准备恢复路径，再让 OpenSurge 接管同一 Wi‑Fi 上的 IPv4 DHCP/DNS。" />
    <div className="mode-grid"><Mode title="同一 Wi‑Fi DHCP 接管" badge="重点场景" active description="Mac 与其他设备连接同一 Wi‑Fi；路由器 DHCP 由用户手动关闭。" /><Mode title="同 LAN 手工网关" description="路由器继续 DHCP；测试设备手工把网关与 DNS 指向 Mac。" /><Mode title="独立下游 LAN" description="独立 AP、SSID 或 VLAN；适合要求更强制策略的部署。" /></div>
    <section className="section"><SectionTitle title="恢复状态机" subtitle="危险阶段会持久化，关闭浏览器也不会丢失" /><div className="timeline">{stages.map((stage, index) => <div className={index < currentIndex ? 'done' : index === currentIndex ? 'current' : ''} key={stage}><span>{index < currentIndex ? '✓' : index + 1}</span><p>{recoveryLabel(stage)}</p></div>)}</div>
      <div className="cooperative"><strong>合作式 IPv4 模式</strong><p>同一二层 Wi‑Fi 中，客户端仍可能通过手工路由器网关或 IPv6 绕过 Mac。要求不可绕过时请选择独立 AP/SSID/VLAN。</p></div>
      <button className="primary" disabled={busy || current === 'gateway_active'} onClick={() => void advance()}>{current === 'idle' || current === 'complete' ? '开始准备恢复资料' : `确认：${recoveryLabel(next)}`}</button>
    </section>
  </>
}
