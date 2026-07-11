import { useEffect, useState } from 'react'
import { api, waitForOperation } from '../api'
import { Mode, PageHeader, SectionTitle } from '../components/Common'
import { recoveryLabel } from '../status'
import type { GatewayPlan, Overview } from '../types'

const stages = ['prepared', 'mac_static', 'router_dhcp_disabled_confirmed', 'gateway_active', 'gateway_stopped_waiting_router_dhcp', 'router_dhcp_restored', 'complete'] as const

export function NetworkPage({ overview, onChanged }: { overview: Overview | null; onChanged: () => Promise<void> }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [plan, setPlan] = useState<GatewayPlan | null>(null)
  const current = overview?.recovery.stage ?? 'idle'
  const currentIndex = stages.indexOf(current as typeof stages[number])

  useEffect(() => {
    void api.gatewayPlan(false).then(setPlan).catch(cause => setError(cause instanceof Error ? cause.message : String(cause)))
  }, [])

  const advance = async () => {
    setBusy(true); setError('')
    try {
      switch (current) {
      case 'idle': case 'complete': await api.prepareRecovery(); break
      case 'prepared': await api.applyStatic(); break
      case 'mac_static': await api.probeDHCP(); break
      case 'router_dhcp_disabled_confirmed': await waitForOperation((await api.gateway('start')).id); break
      case 'gateway_active': await waitForOperation((await api.gateway('stop')).id); break
      case 'gateway_stopped_waiting_router_dhcp': await api.confirmRouterRestored(); break
      case 'router_dhcp_restored': await api.restoreMacDHCP(); break
      }
      await onChanged()
      setPlan(await api.gatewayPlan(false))
    } catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }

  return <>
    <PageHeader eyebrow="NETWORK" title="网络与 DHCP 接管" description="先保存恢复快照，再由窄权限 helper 配置 Mac；路由器 DHCP 仍由用户手动关闭与恢复。" />
    {error && <div className="notice warn" role="alert">{error}</div>}
    <div className="mode-grid"><Mode title="同一 Wi‑Fi DHCP 接管" badge="重点场景" active description="Mac 与其他设备连接同一 Wi‑Fi；路由器 DHCP 由用户手动关闭。" /><Mode title="同 LAN 手工网关" description="路由器继续 DHCP；测试设备手工把网关与 DNS 指向 Mac。" /><Mode title="独立下游 LAN" description="独立 AP、SSID 或 VLAN；适合要求更强制策略的部署。" /></div>
    {plan && <section className="section"><SectionTitle title="当前网络快照" subtitle={`${plan.snapshot.network_service} · ${plan.snapshot.interface}`} /><div className="inventory"><span>Mac {plan.snapshot.ipv4}</span><span>Router {plan.snapshot.router}</span><span>{plan.snapshot.ipv6_default ? 'IPv6 default active' : 'No IPv6 default'}</span><span>{plan.protected_ipv4.length} protected IPv4</span></div>{plan.blockers.map(item => <div className="notice warn" key={item}>{item}</div>)}{plan.warnings.map(item => <div className="notice" key={item}>{item}</div>)}</section>}
    <section className="section"><SectionTitle title="恢复状态机" subtitle="每一步都有真实系统动作或网络证据，不能通过普通 POST 跳过" /><div className="timeline">{stages.map((stage, index) => <div className={index < currentIndex ? 'done' : index === currentIndex ? 'current' : ''} key={stage}><span>{index < currentIndex ? '✓' : index + 1}</span><p>{recoveryLabel(stage)}</p></div>)}</div>
      <div className="cooperative"><strong>合作式 IPv4 模式</strong><p>同一二层 Wi‑Fi 中，客户端仍可能通过手工路由器网关或 IPv6 绕过 Mac。要求不可绕过时请选择独立 AP/SSID/VLAN。</p></div>
      {current === 'mac_static' && <div className="notice warn">现在请手动关闭路由器 DHCP。下一步会主动发送 DHCPDISCOVER；只要仍收到任意 OFFER 就拒绝启动。</div>}
      {current === 'gateway_stopped_waiting_router_dhcp' && <div className="notice warn">现在请先恢复路由器 DHCP。下一步必须探测到 DHCP OFFER，之后才允许把 Mac 恢复为自动获取。</div>}
      <button className="primary" disabled={busy || (plan?.blockers.length ?? 0) > 0} onClick={() => void advance()}>{busy ? '正在验证…' : actionLabel(current)}</button>
    </section>
  </>
}

function actionLabel(stage: string) {
  switch (stage) {
  case 'idle': case 'complete': return '保存网络快照与离线恢复卡'
  case 'prepared': return '将 Mac 切换为固定 IPv4'
  case 'mac_static': return '已关闭路由器 DHCP，执行 OFFER 探测'
  case 'router_dhcp_disabled_confirmed': return '启动 OpenSurge'
  case 'gateway_active': return '停止 OpenSurge'
  case 'gateway_stopped_waiting_router_dhcp': return '路由器 DHCP 已恢复，执行 OFFER 探测'
  case 'router_dhcp_restored': return '将 Mac 恢复为自动 DHCP'
  default: return recoveryLabel(stage)
  }
}
