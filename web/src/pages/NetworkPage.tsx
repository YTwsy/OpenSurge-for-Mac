import { type ReactNode, useCallback, useEffect, useRef, useState } from 'react'
import { api, waitForOperation } from '../api'
import { Mode, PageHeader, SectionTitle } from '../components/Common'
import { NetworkModeDetail } from '../components/NetworkModeDetail'
import { recoveryLabel } from '../status'
import type { ControlConfig, GatewayPlan, NetworkInterfaceOption, Overview } from '../types'

const ipv4Pattern = /^(?:(?:25[0-5]|2[0-4]\d|1?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|1?\d?\d)$/
type NetworkMode = ControlConfig['gateway']['mode']

export function NetworkPage({ overview, onChanged }: { overview: Overview | null; onChanged: () => Promise<void> }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [plan, setPlan] = useState<GatewayPlan | null>(null)
  const [config, setConfig] = useState<ControlConfig | null>(null)
  const [savedConfig, setSavedConfig] = useState<ControlConfig | null>(null)
  const [expandedMode, setExpandedMode] = useState<NetworkMode | null>('same_wifi_dhcp')
  const [detailMode, setDetailMode] = useState<NetworkMode>('same_wifi_dhcp')
  const gatewayControlRef = useRef<HTMLButtonElement>(null)
  const gatewayControlFocused = useRef(false)
  const [interfaceOptions, setInterfaceOptions] = useState<NetworkInterfaceOption[]>([])
  const [interfaceDiscoveryError, setInterfaceDiscoveryError] = useState(false)
  const [clientIPv4, setClientIPv4] = useState('')
  const [clientConfirmed, setClientConfirmed] = useState(false)
  const [ipv6Acknowledged, setIPv6Acknowledged] = useState(false)
  const current = overview?.recovery.stage ?? 'idle'
  const clientCheckpoint = overview?.recovery.client_validation_skipped ? 'client_validation_skipped' : 'client_validated'
  const completion = current === 'complete_static' ? 'complete_static' : 'complete'
  const stages = ['prepared', 'mac_static', 'router_dhcp_disabled_confirmed', 'gateway_active', clientCheckpoint, 'gateway_stopped_waiting_router_dhcp', 'router_dhcp_restored', completion]
  const currentIndex = stages.indexOf(current)
  const recoveryBlocksConfig = Boolean(overview?.recovery.required && current !== 'prepared')
  const configDirty = Boolean(config && savedConfig && JSON.stringify(config) !== JSON.stringify(savedConfig))
  const gatewayActive = overview?.status.gateway === 'running' || overview?.status.gateway === 'degraded'
  const gatewayStopped = overview?.status.gateway === 'stopped'
  const dhcpRuntimeDisabled = config?.gateway.mode === 'same_lan'
  const configurationEditable = !busy && gatewayStopped && !recoveryBlocksConfig
  const planBlockersApply = ['idle', 'complete', 'complete_static', 'prepared', 'mac_static', 'router_dhcp_disabled_confirmed'].includes(current)
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
    void api.config().then(value => { if (active) { setConfig(value); setSavedConfig(value); setExpandedMode(value.gateway.mode); setDetailMode(value.gateway.mode) }; return active ? loadPlan(value) : undefined }).catch(cause => { if (active) setError(cause instanceof Error ? cause.message : String(cause)) })
    void api.networkInterfaces().then(value => { if (active) setInterfaceOptions(value.interfaces) }).catch(() => { if (active) setInterfaceDiscoveryError(true) })
    return () => { active = false }
  }, [loadPlan])

  useEffect(() => {
    if (window.location.hash !== '#gateway-control') {
      gatewayControlFocused.current = false
      return
    }
    if (!config || gatewayControlFocused.current || !gatewayControlRef.current) return
    gatewayControlFocused.current = true
    const control = gatewayControlRef.current
    const reducedMotion = typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
    control.scrollIntoView?.({ behavior: reducedMotion ? 'auto' : 'smooth', block: 'center' })
    if (!control.disabled) control.focus({ preventScroll: true })
  }, [config])

  const selectMode = (mode: ControlConfig['gateway']['mode']) => setConfig(currentConfig => {
    if (!currentConfig) return currentConfig
    const sameLAN = mode === 'same_lan' || mode === 'same_wifi_dhcp'
    return { ...currentConfig, gateway: { ...currentConfig.gateway, mode }, dhcp: { ...currentConfig.dhcp, enabled: mode !== 'same_lan' }, transparent: { ...currentConfig.transparent, mode: sameLAN ? 'tun' : currentConfig.transparent.mode } }
  })

  const toggleMode = (mode: NetworkMode) => {
    setDetailMode(mode)
    setExpandedMode(currentMode => currentMode === mode ? null : mode)
    if (config?.gateway.mode !== mode) selectMode(mode)
  }

  const save = async () => {
    if (!config) return
    setBusy(true); setError(''); setMessage('')
    try { const updated = await api.saveConfig(config); setConfig(updated); setSavedConfig(updated); await onChanged(); await loadPlan(updated) }
    catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }

  const controlGateway = async (action: 'start' | 'stop') => {
    if (!config || config.gateway.mode === 'same_wifi_dhcp') return
    if (action === 'start' && configDirty) {
      setError('网络配置尚未保存。请先保存配置，再启动网关。')
      return
    }
    if (!window.confirm(gatewayConfirmation(config.gateway.mode, action))) return
    setBusy(true); setError(''); setMessage('')
    try {
      const operation = await api.gateway(action)
      await waitForOperation(operation.id)
      await onChanged()
      setMessage(action === 'start' ? `${gatewayModeLabel(config.gateway.mode)}已启动。` : `${gatewayModeLabel(config.gateway.mode)}已停止。`)
    } catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
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
      case 'idle': case 'complete': case 'complete_static': await api.prepareRecovery(); break
      case 'prepared': await api.applyStatic(); break
      case 'mac_static': await api.probeDHCP(); break
      case 'router_dhcp_disabled_confirmed': await waitForOperation((await api.gateway('start')).id); break
      case 'gateway_active': await api.validateClient(clientIPv4, ipv6Acknowledged); break
      case 'client_validated': case 'client_validation_skipped': await waitForOperation((await api.gateway('stop')).id); break
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

  const skipClientValidation = async () => {
    if (!window.confirm('跳过后不会检查客户端租约、DHCPACK、DNS 查询或 mihomo TUN 日志，也不能把本次运行称为已验收。OpenSurge 会记录这次跳过，并允许继续停止网关。仍要跳过吗？')) return
    setBusy(true); setError('')
    try {
      await api.skipClientValidation()
      await onChanged()
    } catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }

  const finishKeepingStatic = async () => {
    if (!window.confirm('OpenSurge 将结束恢复流程，但不会探测路由器 DHCP，也不会把 Mac 切回自动 DHCP。请确认当前静态 IPv4、路由器和 DNS 可长期使用；其他客户端需要有效静态配置或另一个 DHCP 服务器。仍要保留静态 IP 并结束吗？')) return
    setBusy(true); setError('')
    try {
      await api.finishRecoveryKeepingStatic()
      await onChanged()
    } catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }

  return <>
    <PageHeader eyebrow="NETWORK" title="网络与 DHCP 接管" description="选择设备如何接入 OpenSurge，并保存下次启动时使用的网络配置。使用 DHCP 接管时，OpenSurge 会引导你完成设置、确认和恢复。" />
    {error && <div className="notice warn" role="alert">{error}</div>}
    {message && <div className="notice ok-notice" role="status">{message}</div>}
    {config && <>
      <div className="mode-grid">
        <Mode title="局域网 DHCP 接管" badge="自动接管" active={config.gateway.mode === 'same_wifi_dhcp'} expanded={expandedMode === 'same_wifi_dhcp'} controls="network-mode-detail" disabled={!configurationEditable} onSelect={() => toggleMode('same_wifi_dhcp')} description="现有局域网 · 设备自动接入" />
        <Mode title="手工网关模式" badge="部分设备" active={config.gateway.mode === 'same_lan'} expanded={expandedMode === 'same_lan'} controls="network-mode-detail" disabled={!configurationEditable} onSelect={() => toggleMode('same_lan')} description="现有局域网 · 部分设备接入" />
        <Mode title="独立下游 LAN" badge="独立网络" active={config.gateway.mode === 'isolated_lan'} expanded={expandedMode === 'isolated_lan'} controls="network-mode-detail" disabled={!configurationEditable} onSelect={() => toggleMode('isolated_lan')} description="独立网络 · 下游设备自动接入" />
      </div>
      <div className={`mode-detail-shell ${expandedMode ? 'open' : ''}`} id="network-mode-detail" aria-hidden={!expandedMode}>
        <div className="mode-detail-clip"><NetworkModeDetail key={detailMode} mode={detailMode} /></div>
      </div>
      <section className="section">
        <SectionTitle title="Desired 网络配置" subtitle={`这是下次启动要使用的目标值；保存本身不会切换网络。revision ${config.revision.slice(0, 12)}`} />
        <fieldset disabled={!configurationEditable} style={{ border: 0, margin: 0, minWidth: 0, padding: 0 }}>
          <div className="network-config-guide"><strong>填写顺序</strong><p>先选择上方网络模式，再填写接口与 IPv4。Mac 网关 IPv4 同时也是下游 DNS 的监听地址。保存不会立即改动网络；保存后的配置会在启动网关时应用。恢复资料已准备但网络尚未改动时仍可修正配置，保存后会从第 1 步重新开始。</p></div>
          <datalist id="network-interface-options">
            {interfaceOptions.map(option => <option key={`${option.interface}:${option.network_service}`} value={option.interface} label={`${option.network_service} · ${option.interface}`} />)}
          </datalist>
          {interfaceDiscoveryError && <div className="notice">无法读取当前 Mac 的网络接口清单；仍可手工填写接口名称。</div>}
          <div className="config-form">
          <ConfigField label="下游 LAN 接口" setting="gateway.interface" hint="可从当前 Mac 网络服务中选择，也可手工输入接口名。在局域网 DHCP 接管模式中，它必须和上游接口相同；独立 LAN 通常是 AP、SSID 或 VLAN 的下游接口。">
            <input aria-label="下游 LAN 接口" list="network-interface-options" value={config.gateway.interface} onChange={event => setConfig({ ...config, gateway: { ...config.gateway, interface: event.target.value } })} />
          </ConfigField>
          <ConfigField label="上游网络接口" setting="gateway.upstream_interface" hint="可从当前 Mac 网络服务中选择，也可手工输入接口名。pf 会从这里做 NAT；局域网 DHCP 接管模式通常与下游 LAN 接口相同。">
            <input aria-label="上游网络接口" list="network-interface-options" value={config.gateway.upstream_interface} onChange={event => setConfig({ ...config, gateway: { ...config.gateway, upstream_interface: event.target.value } })} />
          </ConfigField>
          <ConfigField label="Mac 网关 IPv4" setting="gateway.lan_ip / dns.listen" hint="分配给 Mac 的下游网关地址，也是 dnsmasq 的 DNS 监听地址。不能放进 DHCP 地址池；局域网 DHCP 接管时应使用当前网段的固定且未占用地址。">
            <input aria-label="Mac 网关 IPv4" value={config.gateway.lan_ip} onChange={event => setConfig({ ...config, gateway: { ...config.gateway, lan_ip: event.target.value }, dns: { ...config.dns, listen: event.target.value } })} />
          </ConfigField>
          <fieldset className={`dhcp-config-group ${dhcpRuntimeDisabled ? 'runtime-inactive' : ''}`} disabled={dhcpRuntimeDisabled}>
            <legend><strong>DHCP 地址池</strong><small>{dhcpRuntimeDisabled ? '手工网关模式运行时不使用；当前值仅保留供切换网络模式后复用' : 'dnsmasq 为下游客户端分配 IPv4 时使用'}</small></legend>
            <div className="dhcp-config-grid">
              <ConfigField label="地址池起点" setting="dhcp.range_start" hint="dnsmasq 可以动态租给客户端的第一个 IPv4；应与 Mac 网关位于同一 /24。">
                <input aria-label="DHCP 地址池起点" value={config.dhcp.range_start} onChange={event => setConfig({ ...config, dhcp: { ...config.dhcp, range_start: event.target.value } })} />
              </ConfigField>
              <ConfigField label="地址池终点" setting="dhcp.range_end" hint="dnsmasq 可以动态租给客户端的最后一个 IPv4；请避开 Mac、路由器和静态地址。">
                <input aria-label="DHCP 地址池终点" value={config.dhcp.range_end} onChange={event => setConfig({ ...config, dhcp: { ...config.dhcp, range_end: event.target.value } })} />
              </ConfigField>
              <ConfigField label="租约时长" setting="dhcp.lease_time" hint="客户端取得动态地址后可使用多久，例如 12h；更短会增加续租请求。">
                <input aria-label="DHCP 租约时长" value={config.dhcp.lease_time} onChange={event => setConfig({ ...config, dhcp: { ...config.dhcp, lease_time: event.target.value } })} />
              </ConfigField>
            </div>
          </fieldset>
          <ConfigField label="上游 DNS" setting="dns.upstream" hint="dnsmasq 转发客户端 DNS 查询时使用的解析器，可填 IPv4 或 IPv4#port（例如 127.0.0.1#1053）。客户端的 DNS 会指向上面的 Mac 网关 IPv4，而不是此地址。">
            <div className="dns-presets" role="group" aria-label="上游 DNS 预设">
              <button type="button" aria-pressed={config.dns.upstream === '127.0.0.1#1053'} onClick={() => setConfig({ ...config, dns: { ...config.dns, upstream: '127.0.0.1#1053' } })}>mihomo DNS（推荐）</button>
              <button type="button" aria-pressed={config.dns.upstream === '1.1.1.1'} onClick={() => setConfig({ ...config, dns: { ...config.dns, upstream: '1.1.1.1' } })}>公共 DNS（调试）</button>
            </div>
            <input aria-label="上游 DNS" placeholder="1.1.1.1 或 127.0.0.1#1053" value={config.dns.upstream} onChange={event => setConfig({ ...config, dns: { ...config.dns, upstream: event.target.value } })} />
            <small>推荐路径进入 mihomo fake-IP DNS。公共 DNS 仅用于对照；启用 TUN 时仍可能被 dns-hijack 捕获，并不保证绕过代理。</small>
          </ConfigField>
          <ConfigField label="透明代理模式" setting="transparent.mode" hint={config.gateway.mode === 'isolated_lan' ? 'tun 让未设置显式代理的下游流量进入 mihomo TUN；off 不做透明捕获。手工网关模式与局域网 DHCP 接管模式必须使用 TUN。' : '当前拓扑必须使用 mihomo TUN，因此该选项已锁定。'}>
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
        <div className="network-save-bar" aria-live="polite"><span className={configDirty ? 'dirty' : ''}><i aria-hidden="true">{configDirty ? '•' : '✓'}</i>{configDirty ? '有未保存的修改' : '当前配置已保存'}</span><button className="primary" disabled={!configurationEditable} onClick={() => void save()}>{busy ? <><span className="button-spinner" aria-hidden="true" />正在保存…</> : '保存网络配置'}</button></div>
      </section>
    </>}
    {config && config.gateway.mode !== 'same_wifi_dhcp' && <section className="section gateway-lifecycle-control">
      <SectionTitle title="网关运行控制" subtitle="使用已保存的网络配置启动或停止；总览页的按钮只负责导航到这里" />
      <div className="gateway-lifecycle-row">
        <div>
          <span className={`pill ${gatewayActive ? 'ok' : ''}`}>{gatewayActive ? '运行中' : gatewayStopped ? '已停止' : '状态未知'}</span>
          <strong>{gatewayModeLabel(config.gateway.mode)}</strong>
          <p>{gatewayModeDescription(config)}</p>
        </div>
        <button ref={gatewayControlRef} id="gateway-control" className={gatewayActive ? 'danger' : 'primary'} type="button" disabled={busy || !overview || (!gatewayActive && !gatewayStopped) || (!gatewayActive && configDirty)} onClick={() => void controlGateway(gatewayActive ? 'stop' : 'start')}>{busy ? '正在执行…' : gatewayActive ? `停止${gatewayModeLabel(config.gateway.mode)}` : `启动${gatewayModeLabel(config.gateway.mode)}`}</button>
      </div>
      {!gatewayActive && configDirty && <div className="notice warn">网络配置有未保存的修改。保存后才能启动网关。</div>}
      {!gatewayActive && !gatewayStopped && <div className="notice warn">当前网关状态无法确认；为避免重复启动，运行控制暂时不可用。</div>}
    </section>}
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
        <SectionTitle title="恢复状态机" subtitle="推荐路径保留真实系统动作与网络证据；可跳过节点会明确记录为未验证" />
        <div className="timeline">{stages.map((stage, index) => <div className={index < currentIndex ? 'done' : index === currentIndex ? 'current' : ''} key={stage}><span>{index < currentIndex ? '✓' : index + 1}</span><p>{recoveryLabel(stage)}</p></div>)}</div>
        <div className="cooperative"><strong>合作式 IPv4 模式</strong><p>同一二层 LAN 中，客户端仍可能通过手工路由器网关或 IPv6 绕过 Mac。要求不可绕过时请选择独立 AP/SSID/VLAN。</p></div>
        {current === 'prepared' && <div className="notice">恢复资料已经保存，但 Mac、路由器与 DHCP 都尚未改动。此时仍可修正并保存目标配置；保存会清除这张预备恢复卡，并从第 1 步重新开始。</div>}
        {configDirty && <div className="notice warn">网络配置有未保存的修改。先保存配置，再保存恢复资料或继续第 2 步。</div>}
        {current === 'mac_static' && <RouterDHCPGuide action="关闭" router={router} networkService={networkService} />}
        {current === 'gateway_active' && <div className="form-stack"><input aria-label="验收客户端 IPv4" placeholder="客户端从 OpenSurge 获得的 IPv4" value={clientIPv4} onChange={event => setClientIPv4(event.target.value)} /><label><input type="checkbox" checked={clientConfirmed} onChange={event => setClientConfirmed(event.target.checked)} /> 已在客户端确认默认网关/DNS 为 Mac，且没有显式代理</label>{plan?.snapshot.ipv6_default && <label><input type="checkbox" checked={ipv6Acknowledged} onChange={event => setIPv6Acknowledged(event.target.checked)} /> 已知 IPv6 默认路由可能绕过 IPv4 设备策略</label>}</div>}
        {current === 'gateway_active' && <div className="notice">推荐完成客户端验收。若当前没有合适客户端，可跳过；跳过只解除 GUI 流程阻塞，不会产生 DHCP、DNS 或 TUN 验收证据。</div>}
        {current === 'client_validation_skipped' && <div className="notice warn">客户端验收已由用户跳过，本次运行没有客户端 DHCP、DNS 与 TUN 数据面验收结论。</div>}
        {current === 'gateway_stopped_waiting_router_dhcp' && <RouterDHCPGuide action="恢复" router={router} networkService={networkService} />}
        {current === 'gateway_stopped_waiting_router_dhcp' && <div className="notice warn">可以恢复路由器 DHCP 并执行 OFFER 探测，也可以人工确认后跳过 OFFER 证据并恢复 Mac 自动 DHCP。若要长期保持静态 IPv4，可直接结束；这不会恢复其他客户端的自动获取能力。</div>}
        {current === 'router_dhcp_restored' && <div className="notice">已经检测到 DHCP OFFER。你可以把 Mac 恢复为自动 DHCP，也可以保留当前静态 IPv4 后结束流程。</div>}
        {current === 'complete_static' && <div className="notice">恢复流程已结束，Mac 仍使用静态 IPv4；路由器 DHCP 与其他客户端的自动获取能力没有在这条路径中验证。</div>}
        <div className="recovery-actions">
          <button ref={gatewayControlRef} id="gateway-control" className="primary" disabled={busy || configDirty || blockedByPlan || (current === 'gateway_active' && (!clientIPv4 || !clientConfirmed || Boolean(plan?.snapshot.ipv6_default && !ipv6Acknowledged)))} onClick={() => void advance()}>{busy ? '正在验证…' : actionLabel(current)}</button>
          {current === 'prepared' && <button className="danger" disabled={busy} onClick={() => void discardRecovery()}>放弃恢复并销毁资料</button>}
          {current === 'gateway_active' && <button className="danger" disabled={busy} onClick={() => void skipClientValidation()}>跳过客户端验收</button>}
          {current === 'gateway_stopped_waiting_router_dhcp' && <button className="danger" disabled={busy} onClick={() => void finishRecoveryManually()}>跳过 OFFER 探测并恢复 Mac 自动 DHCP</button>}
          {(current === 'gateway_stopped_waiting_router_dhcp' || current === 'router_dhcp_restored') && <button className="danger" disabled={busy} onClick={() => void finishKeepingStatic()}>保留静态 IP 并结束</button>}
        </div>
      </section>
    </>}
  </>
}

function gatewayModeLabel(mode: ControlConfig['gateway']['mode']) {
  return mode === 'same_lan' ? '手工网关模式' : '独立下游 LAN'
}

function gatewayModeDescription(config: ControlConfig) {
  if (config.gateway.mode === 'same_lan') return '启动 DNS、mihomo TUN、PF/NAT 与 IPv4 forwarding；路由器 DHCP 保持开启，部分设备需手工把网关和 DNS 指向 Mac。'
  const proxyMode = config.transparent.mode === 'tun' ? 'mihomo TUN 透明代理' : '不启用透明代理'
  return `启动 DHCP/DNS、PF/NAT 与 IPv4 forwarding；当前配置为${proxyMode}。`
}

function gatewayConfirmation(mode: ControlConfig['gateway']['mode'], action: 'start' | 'stop') {
  if (mode === 'same_lan') {
    return action === 'start'
      ? '将按已保存配置启动手工网关模式。路由器 DHCP 不会被关闭；部分设备需要自行把网关和 DNS 指向 Mac。继续吗？'
      : '停止后，仍把网关或 DNS 指向 Mac 的设备可能立即断网。确定停止手工网关模式吗？'
  }
  return action === 'start'
    ? '将按已保存配置启动独立下游 LAN 的 DHCP/DNS、PF/NAT 与 IPv4 forwarding。继续吗？'
    : '停止后，独立下游 LAN 客户端将失去 OpenSurge 提供的 DHCP/DNS 和网关连接。确定停止吗？'
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
  case 'idle': case 'complete': case 'complete_static': return '保存网络快照与离线恢复卡'
  case 'prepared': return '将 Mac 切换为固定 IPv4'
  case 'mac_static': return '已关闭路由器 DHCP，执行 OFFER 探测'
  case 'router_dhcp_disabled_confirmed': return '启动 OpenSurge'
  case 'gateway_active': return '验证客户端 DHCP、DNS 与 TUN 证据'
  case 'client_validated': return '停止 OpenSurge'
  case 'client_validation_skipped': return '停止 OpenSurge'
  case 'gateway_stopped_waiting_router_dhcp': return '路由器 DHCP 已恢复，执行 OFFER 探测'
  case 'router_dhcp_restored': return '将 Mac 恢复为自动 DHCP'
  default: return recoveryLabel(stage)
  }
}
