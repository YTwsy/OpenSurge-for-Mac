import { type ReactNode, useCallback, useEffect, useState } from 'react'
import { api, waitForOperation } from '../api'
import { Mode, PageHeader, SectionTitle } from '../components/Common'
import { recoveryLabel } from '../status'
import type { ControlConfig, GatewayPlan, Overview } from '../types'

const stages = ['prepared', 'mac_static', 'router_dhcp_disabled_confirmed', 'gateway_active', 'client_validated', 'gateway_stopped_waiting_router_dhcp', 'router_dhcp_restored', 'complete'] as const
const ipv4Pattern = /^(?:(?:25[0-5]|2[0-4]\d|1?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|1?\d?\d)$/

export function NetworkPage({ overview, onChanged }: { overview: Overview | null; onChanged: () => Promise<void> }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [plan, setPlan] = useState<GatewayPlan | null>(null)
  const [config, setConfig] = useState<ControlConfig | null>(null)
  const [savedConfig, setSavedConfig] = useState<ControlConfig | null>(null)
  const [clientIPv4, setClientIPv4] = useState('')
  const [clientConfirmed, setClientConfirmed] = useState(false)
  const [ipv6Acknowledged, setIPv6Acknowledged] = useState(false)
  const current = overview?.recovery.stage ?? 'idle'
  const currentIndex = stages.indexOf(current as typeof stages[number])
  const recoveryBlocksConfig = Boolean(overview?.recovery.required && current !== 'prepared')
  const configDirty = Boolean(config && savedConfig && JSON.stringify(config) !== JSON.stringify(savedConfig))
  const configurationEditable = !busy && overview?.status.gateway !== 'running' && !recoveryBlocksConfig
  const planBlockersApply = ['idle', 'complete', 'prepared', 'mac_static', 'router_dhcp_disabled_confirmed'].includes(current)
  const blockedByPlan = planBlockersApply && (plan?.blockers.length ?? 0) > 0
  const recoverySnapshot = overview?.recovery.network_snapshot
  const router = plan?.snapshot.router || recoverySnapshot?.router || ''
  const networkService = plan?.snapshot.network_service || recoverySnapshot?.network_service || 'Wi-Fi'

  const loadPlan = useCallback(async (next: ControlConfig) => {
    if (next.gateway.mode !== 'same_wifi_dhcp') { setPlan(null); return }
    setPlan(await api.gatewayPlan(false))
  }, [])

  useEffect(() => {
    let active = true
    void api.config().then(value => { if (active) { setConfig(value); setSavedConfig(value) }; return active ? loadPlan(value) : undefined }).catch(cause => { if (active) setError(cause instanceof Error ? cause.message : String(cause)) })
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
    try { const updated = await api.saveConfig(config); setConfig(updated); setSavedConfig(updated); await onChanged(); await loadPlan(updated) }
    catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }

  const advance = async () => {
    if (configDirty) {
      setError('网络配置尚未保存。请先保存配置；若恢复资料已准备，保存会清除该预备卡并从第 1 步重新开始。')
      return
    }
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
      await onChanged()
      // `networksetup -setdhcp` returns before macOS necessarily exposes the
      // renewed IPv4/router tuple. Reloading the takeover preflight here turns
      // that normal transition into a false "incomplete IPv4" error after the
      // recovery action itself has succeeded.
      if (config && current !== 'router_dhcp_restored') await loadPlan(config)
    } catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }

  const discardRecovery = async () => {
    if (!window.confirm('确定要放弃这次恢复准备吗？这会永久销毁已保存的网络快照与离线恢复卡，并回到未开始状态。')) return
    setBusy(true); setError('')
    try {
      await api.discardRecovery()
      await onChanged()
      if (config) await loadPlan(config)
    } catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }

  const finishRecoveryManually = async () => {
    if (!window.confirm('仅在你已经确认路由器 DHCP 重新开启时使用。OpenSurge 将跳过 OFFER 证据并立即把 Mac 恢复为自动 DHCP；如果路由器 DHCP 实际未恢复，Mac 可能断网。仍要继续吗？')) return
    setBusy(true); setError('')
    try {
      await api.finishRecoveryManually()
      await onChanged()
    } catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }

  return <>
    <PageHeader eyebrow="NETWORK" title="网络与 DHCP 接管" description="选择 topology 并保存 desired 配置；同一 LAN DHCP 接管必须完成恢复，主动 OFFER 不可用时可人工确认并安全收尾。" />
    {error && <div className="notice warn" role="alert">{error}</div>}
    {config && <>
      <div className="mode-grid">
        <Mode title="同一 LAN DHCP 接管" badge="重点场景" active={config.gateway.mode === 'same_wifi_dhcp'} disabled={!configurationEditable} onSelect={() => selectMode('same_wifi_dhcp')} description="Mac 与其他设备位于同一二层 LAN（Wi‑Fi 或以太网）；路由器 DHCP 由用户手动关闭。" />
        <Mode title="同 LAN 手工网关" active={config.gateway.mode === 'same_lan'} disabled={!configurationEditable} onSelect={() => selectMode('same_lan')} description="路由器继续 DHCP；测试设备手工把网关与 DNS 指向 Mac。" />
        <Mode title="独立下游 LAN" active={config.gateway.mode === 'isolated_lan'} disabled={!configurationEditable} onSelect={() => selectMode('isolated_lan')} description="独立 AP、SSID 或 VLAN；适合要求更强制策略的部署。" />
      </div>
      <section className="section">
        <SectionTitle title="Desired 网络配置" subtitle={`这是下次启动要使用的目标值；保存本身不会切换网络。revision ${config.revision.slice(0, 12)}`} />
        <fieldset disabled={!configurationEditable} style={{ border: 0, margin: 0, minWidth: 0, padding: 0 }}>
          <div className="network-config-guide"><strong>填写顺序</strong><p>先选择上方拓扑，再填写接口与 IPv4。Mac 网关 IPv4 同时也是下游 DNS 的监听地址；保存后的配置会在启动网关时应用。恢复资料已准备但尚未改动网络时仍可修正配置；保存会重新从第 1 步开始。</p></div>
          <div className="config-form">
          <ConfigField label="下游 LAN 接口" setting="gateway.interface" hint="承载客户端流量的 Mac 接口。在同一 LAN DHCP 接管中，它必须和上游接口相同；独立 LAN 通常是 AP、SSID 或 VLAN 的下游接口。">
            <input aria-label="下游 LAN 接口" value={config.gateway.interface} onChange={event => setConfig({ ...config, gateway: { ...config.gateway, interface: event.target.value } })} />
          </ConfigField>
          <ConfigField label="上游网络接口" setting="gateway.upstream_interface" hint="Mac 访问互联网的出口接口。pf 会从这里做 NAT；同一 LAN DHCP 接管通常与下游 LAN 接口相同。">
            <input aria-label="上游网络接口" value={config.gateway.upstream_interface} onChange={event => setConfig({ ...config, gateway: { ...config.gateway, upstream_interface: event.target.value } })} />
          </ConfigField>
          <ConfigField label="Mac 网关 IPv4" setting="gateway.lan_ip / dns.listen" hint="分配给 Mac 的下游网关地址，也是 dnsmasq 的 DNS 监听地址。不能放进 DHCP 地址池；同一 LAN 接管时应使用当前网段的固定且未占用地址。">
            <input aria-label="Mac 网关 IPv4" value={config.gateway.lan_ip} onChange={event => setConfig({ ...config, gateway: { ...config.gateway, lan_ip: event.target.value }, dns: { ...config.dns, listen: event.target.value } })} />
          </ConfigField>
          <ConfigField label="DHCP 地址池起点" setting="dhcp.range_start" hint="dnsmasq 可以动态租给客户端的第一个 IPv4。同 LAN 手工网关不使用 DHCP；同一 LAN DHCP 接管时地址池必须在 Mac 网关的同一个 /24。">
            <input aria-label="DHCP 地址池起点" value={config.dhcp.range_start} onChange={event => setConfig({ ...config, dhcp: { ...config.dhcp, range_start: event.target.value } })} />
          </ConfigField>
          <ConfigField label="DHCP 地址池终点" setting="dhcp.range_end" hint="dnsmasq 可以动态租给客户端的最后一个 IPv4。请避开 Mac、路由器和需要长期保留的静态地址。">
            <input aria-label="DHCP 地址池终点" value={config.dhcp.range_end} onChange={event => setConfig({ ...config, dhcp: { ...config.dhcp, range_end: event.target.value } })} />
          </ConfigField>
          <ConfigField label="DHCP 租约时长" setting="dhcp.lease_time" hint="客户端拿到动态地址后可使用多久，例如 12h。更短会更快回收地址，但会增加续租请求。">
            <input aria-label="DHCP 租约时长" value={config.dhcp.lease_time} onChange={event => setConfig({ ...config, dhcp: { ...config.dhcp, lease_time: event.target.value } })} />
          </ConfigField>
          <ConfigField label="上游 DNS" setting="dns.upstream" hint="dnsmasq 转发客户端 DNS 查询时使用的解析器，可填 IPv4 或 IPv4#port（例如 127.0.0.1#1053）。客户端的 DNS 会指向上面的 Mac 网关 IPv4，而不是此地址。">
            <div className="dns-presets" role="group" aria-label="上游 DNS 预设">
              <button type="button" aria-pressed={config.dns.upstream === '127.0.0.1#1053'} onClick={() => setConfig({ ...config, dns: { ...config.dns, upstream: '127.0.0.1#1053' } })}>mihomo DNS（推荐）</button>
              <button type="button" aria-pressed={config.dns.upstream === '1.1.1.1'} onClick={() => setConfig({ ...config, dns: { ...config.dns, upstream: '1.1.1.1' } })}>公共 DNS（调试）</button>
            </div>
            <input aria-label="上游 DNS" placeholder="1.1.1.1 或 127.0.0.1#1053" value={config.dns.upstream} onChange={event => setConfig({ ...config, dns: { ...config.dns, upstream: event.target.value } })} />
            <small>推荐路径进入 mihomo fake-IP DNS。公共 DNS 仅用于对照；启用 TUN 时仍可能被 dns-hijack 捕获，并不保证绕过代理。</small>
          </ConfigField>
          <ConfigField label="透明代理模式" setting="transparent.mode" hint={config.gateway.mode === 'isolated_lan' ? 'tun 让未设置显式代理的下游流量进入 mihomo TUN；off 不做透明捕获。同 LAN 手工网关与同一 LAN DHCP 接管必须使用 TUN。' : '当前拓扑必须使用 mihomo TUN，因此该选项已锁定。'}>
            <select aria-label="透明代理模式" value={config.transparent.mode} disabled={config.gateway.mode !== 'isolated_lan'} onChange={event => setConfig({ ...config, transparent: { ...config.transparent, mode: event.target.value as 'off' | 'tun' } })}><option value="off">关闭（off）</option><option value="tun">mihomo TUN</option></select>
          </ConfigField>
          <ConfigField label="每设备策略" setting="device_policy.file" hint="启用后可在“设备”页为 MAC 固定租约及独立 mihomo 策略；若尚无策略文件，保存时会创建一个空文件。关闭后不再使用此策略文件。">
            <label className="checkbox-field"><input type="checkbox" checked={config.device_policy.enabled} onChange={event => setConfig({ ...config, device_policy: { ...config.device_policy, enabled: event.target.checked } })} /> 启用每设备策略</label>
          </ConfigField>
          <ConfigField className="wide" label="受保护的 IPv4" setting="device_policy.protected_ipv4" hint="以逗号分隔的路由器、恢复设备或其他静态主机地址。每设备策略的固定租约不得占用这些地址；仅在启用每设备策略时可编辑。">
            <input aria-label="受保护的 IPv4" disabled={!config.device_policy.enabled} placeholder="192.168.1.1, 192.168.1.21" value={config.device_policy.protected_ipv4.join(', ')} onChange={event => setConfig({ ...config, device_policy: { ...config.device_policy, protected_ipv4: event.target.value.split(',').map(item => item.trim()).filter(Boolean) } })} />
          </ConfigField>
          </div>
        </fieldset>
        <button className="primary" disabled={!configurationEditable} onClick={() => void save()}>{busy ? '正在保存…' : '保存网络配置'}</button>
      </section>
    </>}
    {config?.gateway.mode === 'same_wifi_dhcp' && <>
      {plan && <section className="section">
        <SectionTitle title="当前网络快照" subtitle={`${plan.snapshot.network_service} · ${plan.snapshot.interface}`} />
        <div className="inventory"><span>Mac {plan.snapshot.ipv4}</span>{plan.snapshot.router && <span>Router <RouterAddress router={plan.snapshot.router} showHint /></span>}<span>{plan.snapshot.ipv6_default ? 'IPv6 default active' : 'No IPv6 default'}</span><span>{plan.protected_ipv4.length} protected IPv4</span></div>
        {plan.blockers.map(item => <div className="notice warn" key={item}>{item}</div>)}{plan.warnings.map(item => <div className="notice" key={item}>{item}</div>)}
      </section>}
      {recoverySnapshot && <section className="section recovery-card">
        <SectionTitle title="已保存的恢复资料" subtitle="这是切换网络前保存的原始配置；即使当前网络状态随后改变，也以这里的内容作为恢复依据。" />
        <dl className="recovery-card-grid">
          <div><dt>原始 IPv4</dt><dd>{recoverySnapshot.ipv4 || '—'}</dd></div>
          <div><dt>原始路由器</dt><dd>{recoverySnapshot.router ? <RouterAddress router={recoverySnapshot.router} showHint /> : '—'}</dd></div>
          <div><dt>原始 DNS</dt><dd>{recoverySnapshot.dns.length ? recoverySnapshot.dns.join(', ') : '自动 / 未记录'}</dd></div>
          <div><dt>网络服务</dt><dd>{recoverySnapshot.network_service || '—'}</dd></div>
          <div><dt>接口</dt><dd>{recoverySnapshot.interface || '—'}</dd></div>
          <div><dt>子网掩码</dt><dd>{recoverySnapshot.subnet_mask || '—'}</dd></div>
        </dl>
        <div className="recovery-card-actions"><a href="/api/v1/recovery/card" target="_blank" rel="noopener noreferrer">查看恢复卡</a><a href="/api/v1/recovery/card?download=1" download="OpenSurge-WiFi-DHCP-Recovery-Card.txt">下载恢复卡</a></div>
      </section>}
      <section className="section">
        <SectionTitle title="恢复状态机" subtitle="正常路径保留真实系统动作与网络证据；停止后提供显式人工恢复兜底" />
        <div className="timeline">{stages.map((stage, index) => <div className={index < currentIndex ? 'done' : index === currentIndex ? 'current' : ''} key={stage}><span>{index < currentIndex ? '✓' : index + 1}</span><p>{recoveryLabel(stage)}</p></div>)}</div>
        <div className="cooperative"><strong>合作式 IPv4 模式</strong><p>同一二层 LAN 中，客户端仍可能通过手工路由器网关或 IPv6 绕过 Mac。要求不可绕过时请选择独立 AP/SSID/VLAN。</p></div>
        {current === 'prepared' && <div className="notice">恢复资料已经保存，但 Mac、路由器与 DHCP 都尚未改动。此时仍可修正并保存目标配置；保存会清除这张预备恢复卡，并从第 1 步重新开始。</div>}
        {configDirty && <div className="notice warn">网络配置有未保存的修改。先保存配置，再保存恢复资料或继续第 2 步。</div>}
        {current === 'mac_static' && <RouterDHCPGuide action="关闭" router={router} networkService={networkService} />}
        {current === 'gateway_active' && <div className="form-stack"><input aria-label="验收客户端 IPv4" placeholder="客户端从 OpenSurge 获得的 IPv4" value={clientIPv4} onChange={event => setClientIPv4(event.target.value)} /><label><input type="checkbox" checked={clientConfirmed} onChange={event => setClientConfirmed(event.target.checked)} /> 已在客户端确认默认网关/DNS 为 Mac，且没有显式代理</label>{plan?.snapshot.ipv6_default && <label><input type="checkbox" checked={ipv6Acknowledged} onChange={event => setIPv6Acknowledged(event.target.checked)} /> 已知 IPv6 默认路由可能绕过 IPv4 设备策略</label>}</div>}
        {current === 'gateway_stopped_waiting_router_dhcp' && <RouterDHCPGuide action="恢复" router={router} networkService={networkService} />}
        {current === 'gateway_stopped_waiting_router_dhcp' && <div className="notice warn">如果路由器 DHCP 已经恢复，但主动 OFFER 探测不可用，可以人工确认并跳过该证据。兜底动作仍会真实执行 Mac 自动 DHCP 恢复，不会只把状态标成完成。</div>}
        <div className="recovery-actions">
          <button className="primary" disabled={busy || configDirty || blockedByPlan || (current === 'gateway_active' && (!clientIPv4 || !clientConfirmed || Boolean(plan?.snapshot.ipv6_default && !ipv6Acknowledged)))} onClick={() => void advance()}>{busy ? '正在验证…' : actionLabel(current)}</button>
          {current === 'prepared' && <button className="danger" disabled={busy} onClick={() => void discardRecovery()}>放弃恢复并销毁资料</button>}
          {current === 'gateway_stopped_waiting_router_dhcp' && <button className="danger" disabled={busy} onClick={() => void finishRecoveryManually()}>跳过 OFFER 探测并恢复 Mac 自动 DHCP</button>}
        </div>
      </section>
    </>}
  </>
}

function RouterAddress({ router, showHint = false }: { router: string; showHint?: boolean }) {
  if (!isIPv4(router)) return <>{router}</>
  return <><a className="router-link" href={`http://${router}`} target="_blank" rel="noopener noreferrer">{router}</a>{showHint && <small className="router-link-hint">打不开?试试 https 或路由器专属域名</small>}</>
}

function RouterDHCPGuide({ action, router, networkService }: { action: '关闭' | '恢复'; router: string; networkService: string }) {
  const validRouter = isIPv4(router)
  return <div className="notice warn router-guide">
    <strong>{action === '关闭' ? '关闭路由器 DHCP' : '恢复路由器 DHCP'}</strong>
    <p>{validRouter ? <>打开路由器后台 <RouterAddress router={router} showHint /></> : <>请打开路由器管理后台{router ? `（检测值：${router}）` : ''}</>}，按以下通用路径操作：</p>
    <ol>
      <li>登录后台 → LAN / 网络设置 → DHCP 服务器</li>
      <li>{action === '关闭' ? '关闭 DHCP → 保存；保留路由器 LAN IP 不变' : '重新打开 DHCP → 保存；保留路由器 LAN IP 不变'}</li>
      <li>回到 OpenSurge，点击 OFFER 探测按钮</li>
    </ol>
    {!validRouter && <small className="router-fallback">未能自动获取路由器地址，可尝试在浏览器打开 192.168.1.1 / 192.168.0.1，或用 <code>networksetup -getinfo '{networkService}'</code> 自行确认网关</small>}
  </div>
}

function isIPv4(value: string) { return ipv4Pattern.test(value) }

function ConfigField({ label, setting, hint, className = '', children }: { label: string; setting: string; hint: string; className?: string; children: ReactNode }) {
  return <div className={`config-field ${className}`}><div className="config-field-title"><strong>{label}</strong><code>{setting}</code></div>{children}<small>{hint}</small></div>
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
