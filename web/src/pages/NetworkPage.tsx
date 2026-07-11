import { useCallback, useEffect, useState } from 'react'
import { api, waitForOperation } from '../api'
import { Mode, PageHeader, SectionTitle } from '../components/Common'
import { recoveryLabel } from '../status'
import type { ControlConfig, GatewayPlan, Overview } from '../types'

const stages = ['prepared', 'mac_static', 'router_dhcp_disabled_confirmed', 'gateway_active', 'client_validated', 'gateway_stopped_waiting_router_dhcp', 'router_dhcp_restored', 'complete'] as const

export function NetworkPage({ overview, onChanged }: { overview: Overview | null; onChanged: () => Promise<void> }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [plan, setPlan] = useState<GatewayPlan | null>(null)
  const [config, setConfig] = useState<ControlConfig | null>(null)
  const [clientIPv4, setClientIPv4] = useState('')
  const [clientConfirmed, setClientConfirmed] = useState(false)
  const [ipv6Acknowledged, setIPv6Acknowledged] = useState(false)
  const current = overview?.recovery.stage ?? 'idle'
  const currentIndex = stages.indexOf(current as typeof stages[number])

  const loadPlan = useCallback(async (next: ControlConfig) => {
    if (next.gateway.mode !== 'same_wifi_dhcp') { setPlan(null); return }
    setPlan(await api.gatewayPlan(false))
  }, [])
  useEffect(() => {
    let active = true
    void api.config().then(value => { if (active) setConfig(value); return active ? loadPlan(value) : undefined }).catch(cause => { if (active) setError(cause instanceof Error ? cause.message : String(cause)) })
    return () => { active = false }
  }, [loadPlan])

  const selectMode = (mode: ControlConfig['gateway']['mode']) => setConfig(currentConfig => {
    if (!currentConfig) return currentConfig
    const sameLAN = mode === 'same_lan' || mode === 'same_wifi_dhcp'
    return { ...currentConfig, gateway: { ...currentConfig.gateway, mode }, dhcp: { ...currentConfig.dhcp, enabled: mode !== 'same_lan' }, transparent: { ...currentConfig.transparent, mode: sameLAN ? 'tun' : currentConfig.transparent.mode } }
  })
  const save = async () => {
    if (!config) return
    setBusy(true); setError('')
    try { const updated = await api.saveConfig(config); setConfig(updated); await onChanged(); await loadPlan(updated) }
    catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }
  const advance = async () => {
    setBusy(true); setError('')
    try {
      switch (current) {
      case 'idle': case 'complete': await api.prepareRecovery(); break
      case 'prepared': await api.applyStatic(); break
      case 'mac_static': await api.probeDHCP(); break
      case 'router_dhcp_disabled_confirmed': await waitForOperation((await api.gateway('start')).id); break
      case 'gateway_active': await api.validateClient(clientIPv4, ipv6Acknowledged); break
      case 'client_validated': await waitForOperation((await api.gateway('stop')).id); break
      case 'gateway_stopped_waiting_router_dhcp': await api.confirmRouterRestored(); break
      case 'router_dhcp_restored': await api.restoreMacDHCP(); break
      }
      await onChanged(); if (config) await loadPlan(config)
    } catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }

  return <>
    <PageHeader eyebrow="NETWORK" title="网络与 DHCP 接管" description="选择 topology 并保存 desired 配置；same-WiFi 还必须完成不可跳过的恢复状态机。" />
    {error && <div className="notice warn" role="alert">{error}</div>}
    {config && <>
      <div className="mode-grid"><Mode title="同一 Wi‑Fi DHCP 接管" badge="重点场景" active={config.gateway.mode === 'same_wifi_dhcp'} onSelect={() => selectMode('same_wifi_dhcp')} description="Mac 与其他设备连接同一 Wi‑Fi；路由器 DHCP 由用户手动关闭。" /><Mode title="同 LAN 手工网关" active={config.gateway.mode === 'same_lan'} onSelect={() => selectMode('same_lan')} description="路由器继续 DHCP；测试设备手工把网关与 DNS 指向 Mac。" /><Mode title="独立下游 LAN" active={config.gateway.mode === 'isolated_lan'} onSelect={() => selectMode('isolated_lan')} description="独立 AP、SSID 或 VLAN；适合要求更强制策略的部署。" /></div>
      <section className="section"><SectionTitle title="Desired 网络配置" subtitle={`revision ${config.revision.slice(0, 12)}；网关运行或恢复未完成时禁止修改`} />
        <div className="rule-form"><input aria-label="LAN interface" value={config.gateway.interface} onChange={event => setConfig({ ...config, gateway: { ...config.gateway, interface: event.target.value } })} /><input aria-label="Upstream interface" value={config.gateway.upstream_interface} onChange={event => setConfig({ ...config, gateway: { ...config.gateway, upstream_interface: event.target.value } })} /><input aria-label="Mac LAN IPv4" value={config.gateway.lan_ip} onChange={event => setConfig({ ...config, gateway: { ...config.gateway, lan_ip: event.target.value }, dns: { ...config.dns, listen: event.target.value } })} /><input aria-label="DHCP range start" value={config.dhcp.range_start} onChange={event => setConfig({ ...config, dhcp: { ...config.dhcp, range_start: event.target.value } })} /><input aria-label="DHCP range end" value={config.dhcp.range_end} onChange={event => setConfig({ ...config, dhcp: { ...config.dhcp, range_end: event.target.value } })} /><input aria-label="DHCP lease time" value={config.dhcp.lease_time} onChange={event => setConfig({ ...config, dhcp: { ...config.dhcp, lease_time: event.target.value } })} /><input aria-label="DNS upstream" placeholder="1.1.1.1 or 127.0.0.1#1053" value={config.dns.upstream} onChange={event => setConfig({ ...config, dns: { ...config.dns, upstream: event.target.value } })} /><select aria-label="Transparent mode" value={config.transparent.mode} disabled={config.gateway.mode !== 'isolated_lan'} onChange={event => setConfig({ ...config, transparent: { ...config.transparent, mode: event.target.value as 'off' | 'tun' } })}><option value="off">off</option><option value="tun">tun</option></select><label><input type="checkbox" checked={config.device_policy.enabled} onChange={event => setConfig({ ...config, device_policy: { ...config.device_policy, enabled: event.target.checked } })} /> 启用每设备策略文件</label><input aria-label="Protected IPv4" placeholder="192.168.1.1, 192.168.1.21" value={config.device_policy.protected_ipv4.join(', ')} onChange={event => setConfig({ ...config, device_policy: { ...config.device_policy, protected_ipv4: event.target.value.split(',').map(item => item.trim()).filter(Boolean) } })} /></div>
        <button className="primary" disabled={busy || overview?.status.gateway === 'running' || overview?.recovery.required} onClick={() => void save()}>{busy ? '正在保存…' : '保存网络配置'}</button>
      </section>
    </>}
    {config?.gateway.mode === 'same_wifi_dhcp' && <>
      {plan && <section className="section"><SectionTitle title="当前网络快照" subtitle={`${plan.snapshot.network_service} · ${plan.snapshot.interface}`} /><div className="inventory"><span>Mac {plan.snapshot.ipv4}</span><span>Router {plan.snapshot.router}</span><span>{plan.snapshot.ipv6_default ? 'IPv6 default active' : 'No IPv6 default'}</span><span>{plan.protected_ipv4.length} protected IPv4</span></div>{plan.blockers.map(item => <div className="notice warn" key={item}>{item}</div>)}{plan.warnings.map(item => <div className="notice" key={item}>{item}</div>)}</section>}
      <section className="section"><SectionTitle title="恢复状态机" subtitle="每一步都有真实系统动作或网络证据，不能通过普通 POST 跳过" /><div className="timeline">{stages.map((stage, index) => <div className={index < currentIndex ? 'done' : index === currentIndex ? 'current' : ''} key={stage}><span>{index < currentIndex ? '✓' : index + 1}</span><p>{recoveryLabel(stage)}</p></div>)}</div><div className="cooperative"><strong>合作式 IPv4 模式</strong><p>同一二层 Wi‑Fi 中，客户端仍可能通过手工路由器网关或 IPv6 绕过 Mac。要求不可绕过时请选择独立 AP/SSID/VLAN。</p></div>{current === 'mac_static' && <div className="notice warn">现在请手动关闭路由器 DHCP。下一步会主动发送 DHCPDISCOVER；只要仍收到任意 OFFER 就拒绝启动。</div>}{current === 'gateway_active' && <div className="form-stack"><input aria-label="验收客户端 IPv4" placeholder="客户端从 OpenSurge 获得的 IPv4" value={clientIPv4} onChange={event => setClientIPv4(event.target.value)} /><label><input type="checkbox" checked={clientConfirmed} onChange={event => setClientConfirmed(event.target.checked)} /> 已在客户端确认默认网关/DNS 为 Mac，且没有显式代理</label>{plan?.snapshot.ipv6_default && <label><input type="checkbox" checked={ipv6Acknowledged} onChange={event => setIPv6Acknowledged(event.target.checked)} /> 已知 IPv6 默认路由可能绕过 IPv4 设备策略</label>}</div>}{current === 'gateway_stopped_waiting_router_dhcp' && <div className="notice warn">现在请先恢复路由器 DHCP。下一步必须探测到 DHCP OFFER，之后才允许把 Mac 恢复为自动获取。</div>}<button className="primary" disabled={busy || (plan?.blockers.length ?? 0) > 0 || (current === 'gateway_active' && (!clientIPv4 || !clientConfirmed || Boolean(plan?.snapshot.ipv6_default && !ipv6Acknowledged)))} onClick={() => void advance()}>{busy ? '正在验证…' : actionLabel(current)}</button></section>
    </>}
  </>
}

function actionLabel(stage: string) {
  switch (stage) {
  case 'idle': case 'complete': return '保存网络快照与离线恢复卡'
  case 'prepared': return '将 Mac 切换为固定 IPv4'
  case 'mac_static': return '已关闭路由器 DHCP，执行 OFFER 探测'
  case 'router_dhcp_disabled_confirmed': return '启动 OpenSurge'
  case 'gateway_active': return '验证客户端 DHCP、DNS 与 TUN 证据'
  case 'client_validated': return '停止 OpenSurge'
  case 'gateway_stopped_waiting_router_dhcp': return '路由器 DHCP 已恢复，执行 OFFER 探测'
  case 'router_dhcp_restored': return '将 Mac 恢复为自动 DHCP'
  default: return recoveryLabel(stage)
  }
}
