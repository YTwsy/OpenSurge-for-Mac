import { type ReactNode, useCallback, useEffect, useRef, useState } from 'react'
import { api, waitForOperation } from '../api'
import { Mode, PageHeader, SectionTitle } from '../components/Common'
import { NetworkModeDetail } from '../components/NetworkModeDetail'
import type { OperationNotification } from '../components/OperationNotifications'
import { recoveryLabel } from '../status'
import type { ControlConfig, DevicePolicyDocument, GatewayPlan, NetworkDefaults, NetworkInterfaceOption, Overview, PolicyDevice, PolicySet } from '../types'
import { t } from '../i18n'

const ipv4Pattern = /^(?:(?:25[0-5]|2[0-4]\d|1?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|1?\d?\d)$/
type NetworkMode = ControlConfig['gateway']['mode']

// The gateway rejects prefixes outside /8-/30: wider has no private use here and
// narrower leaves no host addresses beside the Mac itself.
const supportedPrefixLengths = Array.from({ length: 23 }, (_, index) => index + 8)

function netmaskForPrefixLength(prefixLength: number): string {
  const mask = prefixLength === 0 ? 0 : (0xffffffff << (32 - prefixLength)) >>> 0
  return [24, 16, 8, 0].map(shift => (mask >>> shift) & 0xff).join('.')
}

function isInstallerNetworkSeed(config: ControlConfig): boolean {
  return config.gateway.mode === 'isolated_lan'
    && config.gateway.interface === 'en0'
    && config.gateway.upstream_interface === 'en0'
    && config.gateway.lan_ip === '192.168.50.1'
    && config.dhcp.enabled
    && config.dhcp.range_start === '192.168.50.100'
    && config.dhcp.range_end === '192.168.50.200'
    && config.dns.listen === '192.168.50.1'
}

type PolicyMigrationDevice = Pick<PolicyDevice, 'id' | 'name' | 'ipv4'> & { mac?: string }
type PolicyMigration = {
  target: ControlConfig
  document: DevicePolicyDocument
  policy: PolicySet
  resolved: PolicyMigrationDevice[]
  unresolved: PolicyMigrationDevice[]
}

export function NetworkPage({ overview, onChanged, onNavigate, onNotify }: { overview: Overview | null; onChanged: () => Promise<void>; onNavigate: (page: 'devices') => void; onNotify: (notification: OperationNotification) => void }) {
  const [busy, setBusy] = useState(false)
  const [configSaving, setConfigSaving] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [plan, setPlan] = useState<GatewayPlan | null>(null)
  const [planSettled, setPlanSettled] = useState(false)
  const [config, setConfig] = useState<ControlConfig | null>(null)
  const [savedConfig, setSavedConfig] = useState<ControlConfig | null>(null)
  const [expandedMode, setExpandedMode] = useState<NetworkMode | null>('same_wifi_dhcp')
  const [detailMode, setDetailMode] = useState<NetworkMode>('same_wifi_dhcp')
  const gatewayControlRef = useRef<HTMLButtonElement>(null)
  const gatewayControlFocused = useRef(false)
  const [interfaceOptions, setInterfaceOptions] = useState<NetworkInterfaceOption[]>([])
  const [interfaceDiscoveryError, setInterfaceDiscoveryError] = useState(false)
  const [initialNetworkSetup, setInitialNetworkSetup] = useState(false)
  const [networkDefaultsBusy, setNetworkDefaultsBusy] = useState(false)
  const [networkDefaultsMessage, setNetworkDefaultsMessage] = useState('')
  const [networkDefaultsError, setNetworkDefaultsError] = useState('')
  const [clientIPv4, setClientIPv4] = useState('')
  const [clientConfirmed, setClientConfirmed] = useState(false)
  const [ipv6Acknowledged, setIPv6Acknowledged] = useState(false)
  const [policyMigration, setPolicyMigration] = useState<PolicyMigration | null>(null)
  const current = overview?.recovery.stage ?? 'idle'
  const clientCheckpoint = overview?.recovery.client_validation_skipped ? 'client_validation_skipped' : 'client_validated'
  const completion = current === 'complete_static' ? 'complete_static' : 'complete'
  const stages = ['prepared', 'mac_static', 'router_dhcp_disabled_confirmed', 'gateway_active', clientCheckpoint, 'gateway_stopped_waiting_router_dhcp', 'router_dhcp_restored', completion]
  const currentIndex = stages.indexOf(current)
  const recoveryBlocksConfig = Boolean(overview?.recovery.required && current !== 'prepared')
  const configDirty = Boolean(config && savedConfig && JSON.stringify(config) !== JSON.stringify(savedConfig))
  const gatewayActive = overview?.status.gateway === 'running' || overview?.status.gateway === 'degraded'
  const gatewayStopped = overview?.status.gateway === 'stopped'
  const gatewayInterrupted = overview?.status.runtime_state === 'interrupted'
  const dhcpRuntimeDisabled = config?.gateway.mode === 'same_lan'
  const configurationEditable = !busy && !networkDefaultsBusy && gatewayStopped && !recoveryBlocksConfig
  const planBlockersApply = ['idle', 'complete', 'complete_static', 'prepared', 'mac_static', 'router_dhcp_disabled_confirmed'].includes(current)
  const blockedByPlan = planBlockersApply && (plan?.blockers.length ?? 0) > 0
  const recoverySnapshot = overview?.recovery.network_snapshot
  const router = plan?.snapshot.router || recoverySnapshot?.router || ''
  const networkService = plan?.snapshot.network_service || recoverySnapshot?.network_service || 'Wi-Fi'

  const loadPlan = useCallback(async (next: ControlConfig) => {
    setPlanSettled(false)
    try {
      if (next.gateway.mode !== 'same_wifi_dhcp') { setPlan(null); return }
      setPlan(await api.gatewayPlan(false))
    } finally {
      setPlanSettled(true)
    }
  }, [])

  useEffect(() => {
    let active = true
    void api.config().then(value => { if (active) { setConfig(value); setSavedConfig(value); setInitialNetworkSetup(isInstallerNetworkSeed(value)); setExpandedMode(value.gateway.mode); setDetailMode(value.gateway.mode) }; return active ? loadPlan(value) : undefined }).catch(cause => { if (active) setError(cause instanceof Error ? cause.message : String(cause)) })
    void api.networkInterfaces().then(value => { if (active) setInterfaceOptions(value.interfaces) }).catch(() => { if (active) setInterfaceDiscoveryError(true) })
    return () => { active = false }
  }, [loadPlan])

  useEffect(() => {
    const navigationTarget = window.location.hash
    if (navigationTarget !== '#gateway-control' && navigationTarget !== '#gateway-control-bottom') {
      gatewayControlFocused.current = false
      return
    }
    if (!config || !planSettled || gatewayControlFocused.current || !gatewayControlRef.current) return
    gatewayControlFocused.current = true
    const control = gatewayControlRef.current
    const reducedMotion = typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
    if (navigationTarget === '#gateway-control-bottom') {
      window.scrollTo?.({ top: document.documentElement.scrollHeight, behavior: reducedMotion ? 'auto' : 'smooth' })
    } else {
      control.scrollIntoView?.({ behavior: reducedMotion ? 'auto' : 'smooth', block: 'center' })
    }
    if (!control.disabled) control.focus({ preventScroll: true })
  }, [config, planSettled])

  const selectMode = (mode: ControlConfig['gateway']['mode']) => setConfig(currentConfig => {
    if (!currentConfig) return currentConfig
    const sameLAN = mode === 'same_lan' || mode === 'same_wifi_dhcp'
    const topologyChanged = currentConfig.gateway.mode !== mode
    return {
      ...currentConfig,
      gateway: { ...currentConfig.gateway, mode },
      dhcp: { ...currentConfig.dhcp, enabled: mode !== 'same_lan' },
      transparent: {
        ...currentConfig.transparent,
        mode: sameLAN ? 'tun' : currentConfig.transparent.mode,
        ipv6_shared_l2_ready: topologyChanged ? false : currentConfig.transparent.ipv6_shared_l2_ready,
      },
    }
  })

  const applyNetworkDefaults = async (mode: NetworkDefaults['mode']) => {
    setNetworkDefaultsBusy(true); setNetworkDefaultsError(''); setNetworkDefaultsMessage('')
    try {
      const defaults = await api.networkDefaults(mode)
      if (defaults.blockers.length) throw new Error(defaults.blockers.join('；'))
      setConfig(currentConfig => {
        if (!currentConfig || currentConfig.gateway.mode !== mode) return currentConfig
        const dhcp = mode === 'same_wifi_dhcp'
          ? {
              ...currentConfig.dhcp,
              range_start: defaults.dhcp_range_start!,
              range_end: defaults.dhcp_range_end!,
              bypass_gateway: defaults.bypass_gateway || defaults.snapshot.router || '',
              bypass_dns: defaults.bypass_dns.length ? defaults.bypass_dns : defaults.snapshot.router ? [defaults.snapshot.router] : [],
            }
          : currentConfig.dhcp
        return {
          ...currentConfig,
          gateway: {
            ...currentConfig.gateway,
            interface: defaults.snapshot.interface,
            upstream_interface: defaults.snapshot.interface,
            lan_ip: defaults.gateway_ipv4,
            lan_prefix_len: defaults.lan_prefix_len ?? currentConfig.gateway.lan_prefix_len,
          },
          dhcp,
          dns: { ...currentConfig.dns, listen: defaults.gateway_ipv4 },
        }
      })
      const warning = defaults.warnings.length ? ` ${defaults.warnings.join('；')}。` : ''
      const fields = t(mode === 'same_wifi_dhcp' ? 'IPv4、子网前缀、地址池和主路由建议值' : 'IPv4 与子网前缀建议值')
      setNetworkDefaultsMessage(t('已根据当前 {{service}}（{{interface}}）填入 {{fields}}，尚未保存。{{warning}}', { service: defaults.snapshot.network_service, interface: defaults.snapshot.interface, fields, warning }))
    } catch (cause) {
      const fields = t(mode === 'same_lan' ? '接口和 IPv4' : '接口、IPv4、地址池、主路由网关和 DNS')
      setNetworkDefaultsError(t('无法自动填写当前网络：{{error}}。请手工确认{{fields}}。', { error: cause instanceof Error ? cause.message : String(cause), fields }))
    } finally {
      setNetworkDefaultsBusy(false)
    }
  }

  const toggleMode = (mode: NetworkMode) => {
    setDetailMode(mode)
    setExpandedMode(currentMode => currentMode === mode ? null : mode)
    if (mode === 'isolated_lan') { setNetworkDefaultsMessage(''); setNetworkDefaultsError('') }
    if (config?.gateway.mode !== mode) {
      selectMode(mode)
      if (initialNetworkSetup && mode !== 'isolated_lan') void applyNetworkDefaults(mode)
    }
  }

  const persistConfig = async (target: ControlConfig, migration?: PolicyMigration) => {
    setBusy(true); setConfigSaving(true); setError(''); setMessage('')
    let policySaved = false
    try {
      if (migration?.resolved.length) {
        await api.saveDevicePolicy(migration.policy, migration.document.revision)
        policySaved = true
      }
      const updated = await api.saveConfig(target)
      setConfig(updated); setSavedConfig(updated); setPolicyMigration(null)
      setInitialNetworkSetup(false); setNetworkDefaultsMessage(''); setNetworkDefaultsError('')
      await onChanged(); await loadPlan(updated)
      if (migration) {
        const messages = []
        if (migration.resolved.length) messages.push(t('已为 {{count}} 台设备补充 MAC', { count: migration.resolved.length }))
        if (migration.unresolved.length) messages.push(t('{{count}} 台设备策略已暂停', { count: migration.unresolved.length }))
        setMessage(`${messages.join('；')}。`)
      }
    } catch (cause) {
      const failure = cause instanceof Error ? cause.message : String(cause)
      setError(policySaved ? t('设备 MAC 已保存，但网络配置切换失败：{{error}}', { error: failure }) : failure)
    }
    finally { setBusy(false); setConfigSaving(false) }
  }

  const save = async () => {
    if (!config || !savedConfig) return
    const leavingTakeover = savedConfig.gateway.mode === 'same_wifi_dhcp' && config.gateway.mode !== 'same_wifi_dhcp' && config.device_policy.enabled
    if (leavingTakeover) {
      try {
        const document = await api.devicePolicy()
        const bypassDevices = document.policy.devices.filter(device => device.gateway_target === 'upstream_router')
        if (bypassDevices.length) {
          const names = bypassDevices.map(device => device.name || device.id).join('、')
          setError(t('{{names}} 正在使用“直连主路由”；该方式仅适用于局域网 DHCP 接管。请先在设备页将这些设备切回 OpenSurge。', { names }))
          return
        }
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : String(cause))
        return
      }
    }
    const leavingSameLAN = savedConfig.gateway.mode === 'same_lan' && config.gateway.mode !== 'same_lan' && config.device_policy.enabled
    if (!leavingSameLAN) {
      await persistConfig(config)
      return
    }
    setBusy(true); setError(''); setMessage('')
    try {
      const document = await api.devicePolicy()
      const missing = document.policy.devices.filter(device => !device.mac.trim())
      if (!missing.length) {
        await persistConfig(config)
        return
      }
      const devices = await api.devices()
      const usedMACs = new Set(document.policy.devices.map(device => device.mac.trim().toLowerCase()).filter(Boolean))
      const resolved: PolicyMigrationDevice[] = []
      const unresolved: PolicyMigrationDevice[] = []
      const nextPolicy = structuredClone(document.policy)
      for (const device of missing) {
        const observations = (devices.observed_devices ?? []).filter(observed => observed.ip === device.ipv4 && validMAC(observed.mac ?? ''))
        const mac = observations.length === 1 ? observations[0].mac!.trim().toLowerCase() : ''
        if (!mac || usedMACs.has(mac)) {
          unresolved.push({ id: device.id, name: device.name, ipv4: device.ipv4 })
          continue
        }
        usedMACs.add(mac)
        nextPolicy.devices = nextPolicy.devices.map(candidate => candidate.id === device.id ? { ...candidate, mac } : candidate)
        resolved.push({ id: device.id, name: device.name, ipv4: device.ipv4, mac })
      }
      setPolicyMigration({ target: structuredClone(config), document, policy: nextPolicy, resolved, unresolved })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally { setBusy(false) }
  }

  const controlGateway = async (action: 'start' | 'stop') => {
    if (!config || config.gateway.mode === 'same_wifi_dhcp') return
    if (action === 'start' && configDirty) {
      setError(t('网络配置尚未保存。请先保存配置，再启动网关。'))
      return
    }
    if (!window.confirm(gatewayConfirmation(config.gateway.mode, action))) return
    setBusy(true); setError(''); setMessage('')
    try {
      const operation = await api.gateway(action)
      await waitForOperation(operation.id)
      await onChanged()
      const result = action === 'start' ? t('{{mode}}已启动。', { mode: gatewayModeLabel(config.gateway.mode) }) : t('{{mode}}已停止。', { mode: gatewayModeLabel(config.gateway.mode) })
      setMessage(result)
      onNotify({ tone: 'success', title: t(action === 'start' ? '启动网关成功' : '停止网关成功'), message: action === 'start' ? t('OpenSurge 已完成{{mode}}启动。', { mode: gatewayModeLabel(config.gateway.mode) }) : t('OpenSurge 已完成{{mode}}停止。', { mode: gatewayModeLabel(config.gateway.mode) }) })
    } catch (cause) {
      const failure = cause instanceof Error ? cause.message : String(cause)
      setError(failure)
      onNotify({ tone: 'error', title: t(action === 'start' ? '启动网关失败' : '停止网关失败'), message: failure })
    }
    finally { setBusy(false) }
  }

  const cleanupInterruptedRuntime = async () => {
    if (!config || !gatewayInterrupted) return
    if (!window.confirm(t('确认安全清理上次运行留下的状态吗？OpenSurge 不会向旧 PID 发送信号，也不会更改本次开机的 PF 或 IPv4 forwarding。如果上次运行启用了系统代理协同，将恢复为 OpenSurge 启动前保存的状态。'))) return
    setBusy(true); setError(''); setMessage('')
    try {
      const operation = await api.gateway('stop')
      await waitForOperation(operation.id)
      await onChanged()
      const result = config.gateway.mode === 'same_wifi_dhcp'
        ? t('旧状态已安全清理。请继续完成路由器 DHCP 与 Mac 网络恢复。')
        : t('旧状态已安全清理。现在可以重新启动{{mode}}。', { mode: gatewayModeLabel(config.gateway.mode) })
      setMessage(result)
      onNotify({ tone: 'success', title: t('旧网关状态清理成功'), message: t('上次运行遗留的 runtime 已完成安全清理。') })
    } catch (cause) {
      const failure = cause instanceof Error ? cause.message : String(cause)
      setError(failure)
      onNotify({ tone: 'error', title: t('旧网关状态清理失败'), message: failure })
    }
    finally { setBusy(false) }
  }

  const advance = async () => {
    if (configDirty) {
      setError(t('网络配置尚未保存。请先保存配置；若恢复资料已准备，保存会清除该预备卡并从第 1 步重新开始。'))
      return
    }
    const lifecycleAction = current === 'router_dhcp_disabled_confirmed'
      ? 'start'
      : current === 'client_validated' || current === 'client_validation_skipped'
        ? 'stop'
        : null
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
      if (lifecycleAction === 'start') onNotify({ tone: 'success', title: t('启动网关成功'), message: t('局域网 DHCP 接管网关已启动，请继续验证客户端接入。') })
      if (lifecycleAction === 'stop') onNotify({ tone: 'success', title: t('停止网关成功'), message: t('网关已停止，请继续恢复路由器 DHCP 与 Mac 网络。') })
    } catch (cause) {
      const failure = cause instanceof Error ? cause.message : String(cause)
      setError(failure)
      if (lifecycleAction) onNotify({ tone: 'error', title: t(lifecycleAction === 'start' ? '启动网关失败' : '停止网关失败'), message: failure })
    }
    finally { setBusy(false) }
  }

  const discardRecovery = async () => {
    if (!window.confirm(t('确定要放弃这次恢复准备吗？这会永久销毁已保存的网络快照与离线恢复卡，并回到未开始状态。'))) return
    setBusy(true); setError('')
    try {
      await api.discardRecovery()
      await onChanged()
      if (config) await loadPlan(config)
    } catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }

  const abandonTakeover = async () => {
    if (!window.confirm(t('放弃本次局域网 DHCP 接管？OpenSurge 会先探测可用 DHCP：若有 OFFER，就把 Mac 恢复为自动 DHCP；若没有，就保留当前固定 IPv4 并结束流程。后一种情况不会确认路由器 DHCP 或其他设备的自动获取能力。'))) return
    setBusy(true); setError(''); setMessage('')
    try {
      await api.abandonTakeover()
      await onChanged()
      setMessage(t('已放弃 DHCP 接管；网关停止后，菜单栏中的“退出 OpenSurge”可用。'))
    } catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }

  const finishRecoveryManually = async () => {
    if (!window.confirm(t('仅在你已经确认路由器 DHCP 重新开启时使用。OpenSurge 将跳过 OFFER 证据并立即把 Mac 恢复为自动 DHCP；如果路由器 DHCP 实际未恢复，Mac 可能断网。仍要继续吗？'))) return
    setBusy(true); setError('')
    try {
      await api.finishRecoveryManually()
      await onChanged()
    } catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }

  const skipClientValidation = async () => {
    if (!window.confirm(t('跳过后不会检查客户端租约、DHCPACK、DNS 查询或 mihomo TUN 日志，也不能把本次运行称为已验收。OpenSurge 会记录这次跳过，并允许继续停止网关。仍要跳过吗？'))) return
    setBusy(true); setError('')
    try {
      await api.skipClientValidation()
      await onChanged()
    } catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }

  const finishKeepingStatic = async () => {
    if (!window.confirm(t('OpenSurge 将结束恢复流程，但不会探测路由器 DHCP，也不会把 Mac 切回自动 DHCP。请确认当前静态 IPv4、路由器和 DNS 可长期使用；其他客户端需要有效静态配置或另一个 DHCP 服务器。仍要保留静态 IP 并结束吗？'))) return
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
        <Mode title="旁路由模式" badge="部分设备" active={config.gateway.mode === 'same_lan'} expanded={expandedMode === 'same_lan'} controls="network-mode-detail" disabled={!configurationEditable} onSelect={() => toggleMode('same_lan')} description="现有局域网 · 部分设备手动接入" />
        <Mode title="独立下游 LAN" badge="独立网络" active={config.gateway.mode === 'isolated_lan'} expanded={expandedMode === 'isolated_lan'} controls="network-mode-detail" disabled={!configurationEditable} onSelect={() => toggleMode('isolated_lan')} description="独立网络 · 下游设备自动接入" />
      </div>
      <div className={`mode-detail-shell ${expandedMode ? 'open' : ''}`} id="network-mode-detail" aria-hidden={!expandedMode}>
        <div className="mode-detail-clip"><NetworkModeDetail key={detailMode} mode={detailMode} /></div>
      </div>
      <section className="section">
        <SectionTitle title="Desired 网络配置" subtitle={t('这是下次启动要使用的目标值；保存本身不会切换网络。revision {{revision}}', { revision: config.revision.slice(0, 12) })} />
        <fieldset disabled={!configurationEditable} style={{ border: 0, margin: 0, minWidth: 0, padding: 0 }}>
          <div className="network-config-guide"><strong>{t('填写顺序')}</strong><p>{t('先选择上方网络模式，再填写接口与 IPv4。Mac 网关 IPv4 同时也是下游 DNS 的监听地址。保存不会立即改动网络；保存后的配置会在启动网关时应用。恢复资料已准备但网络尚未改动时仍可修正配置，保存后会从第 1 步重新开始。')}</p></div>
          {initialNetworkSetup && <div className="notice">{t('首次设置：选择旁路由模式或局域网 DHCP 接管后，OpenSurge 会读取当前主网络并填入未保存的建议值；独立下游 LAN 保持手工配置。')}</div>}
          {networkDefaultsBusy && <div className="notice" role="status">{t('正在读取当前主网络…')}</div>}
          {networkDefaultsMessage && <div className="notice ok-notice" role="status">{networkDefaultsMessage}</div>}
          {networkDefaultsError && <div className="notice warn" role="alert">{networkDefaultsError}</div>}
          {config.gateway.mode !== 'isolated_lan' && <div className="network-defaults-refill">
            <button type="button" className="text-link" onClick={() => void applyNetworkDefaults(config.gateway.mode as NetworkDefaults['mode'])}>{t('根据当前网络重新填入')}</button>
            <small>{config.gateway.mode === 'same_wifi_dhcp'
              ? t('重新读取 Mac 当前主网络，覆盖下面尚未保存的接口、Mac 网关 IPv4、子网前缀、DHCP 地址池和主路由建议值。换网络或换网段后用它对齐。')
              : t('重新读取 Mac 当前主网络，覆盖下面尚未保存的接口、Mac 网关 IPv4 和子网前缀。换网络或换网段后用它对齐。')}</small>
          </div>}
          <datalist id="network-interface-options">
            {interfaceOptions.map(option => <option key={`${option.interface}:${option.network_service}`} value={option.interface} label={`${option.network_service} · ${option.interface}`} />)}
          </datalist>
          {interfaceDiscoveryError && <div className="notice">{t('无法读取当前 Mac 的网络接口清单；仍可手工填写接口名称。')}</div>}
          <div className="config-form">
          <ConfigField label="下游 LAN 接口" setting="gateway.interface" hint="可从当前 Mac 网络服务中选择，也可手工输入接口名。在局域网 DHCP 接管模式中，它必须和上游接口相同；独立 LAN 通常是 AP、SSID 或 VLAN 的下游接口。">
            <input aria-label={t('下游 LAN 接口')} list="network-interface-options" value={config.gateway.interface} onChange={event => setConfig({ ...config, gateway: { ...config.gateway, interface: event.target.value } })} />
          </ConfigField>
          <ConfigField label="上游网络接口" setting="gateway.upstream_interface" hint="可从当前 Mac 网络服务中选择，也可手工输入接口名。pf 会从这里做 NAT；局域网 DHCP 接管模式通常与下游 LAN 接口相同。">
            <input aria-label={t('上游网络接口')} list="network-interface-options" value={config.gateway.upstream_interface} onChange={event => setConfig({ ...config, gateway: { ...config.gateway, upstream_interface: event.target.value } })} />
          </ConfigField>
          <ConfigField label="Mac 网关 IPv4" setting="gateway.lan_ip / dns.listen" hint="分配给 Mac 的下游网关地址，也是 dnsmasq 的 DNS 监听地址。不能放进 DHCP 地址池；局域网 DHCP 接管时应使用当前网段的固定且未占用地址。">
            <input aria-label={t('Mac 网关 IPv4')} value={config.gateway.lan_ip} onChange={event => setConfig({ ...config, gateway: { ...config.gateway, lan_ip: event.target.value }, dns: { ...config.dns, listen: event.target.value } })} />
          </ConfigField>
          <ConfigField label="下游 LAN 子网前缀" setting="gateway.lan_prefix_len" hint="下游网段的真实子网掩码。pf NAT、TUN 路由排除、DHCP 地址池校验和设备地址归属都由它推导，填错会让同网段设备被当成外部流量。接入现有局域网时请与主路由保持一致。">
            <select aria-label={t('下游 LAN 子网前缀')} value={config.gateway.lan_prefix_len || 24} onChange={event => setConfig({ ...config, gateway: { ...config.gateway, lan_prefix_len: Number(event.target.value) } })}>
              {supportedPrefixLengths.map(prefixLength => <option key={prefixLength} value={prefixLength}>{`/${prefixLength}（${netmaskForPrefixLength(prefixLength)}）`}</option>)}
            </select>
          </ConfigField>
          <fieldset className={`dhcp-config-group ${dhcpRuntimeDisabled ? 'runtime-inactive' : ''}`} disabled={dhcpRuntimeDisabled}>
            <legend><strong>{t('DHCP 地址池')}</strong><small>{t(dhcpRuntimeDisabled ? '旁路由模式运行时不使用；当前值仅保留供切换网络模式后复用' : 'dnsmasq 为下游客户端分配 IPv4 时使用')}</small></legend>
            <div className="dhcp-config-grid">
              <ConfigField label="地址池起点" setting="dhcp.range_start" hint="dnsmasq 可以动态租给客户端的第一个 IPv4；必须与 Mac 网关位于同一子网，范围由上面的子网前缀决定。">
                <input aria-label={t('DHCP 地址池起点')} value={config.dhcp.range_start} onChange={event => setConfig({ ...config, dhcp: { ...config.dhcp, range_start: event.target.value } })} />
              </ConfigField>
              <ConfigField label="地址池终点" setting="dhcp.range_end" hint="dnsmasq 可以动态租给客户端的最后一个 IPv4；请避开 Mac、路由器和静态地址。">
                <input aria-label={t('DHCP 地址池终点')} value={config.dhcp.range_end} onChange={event => setConfig({ ...config, dhcp: { ...config.dhcp, range_end: event.target.value } })} />
              </ConfigField>
              <ConfigField label="租约时长" setting="dhcp.lease_time" hint="客户端取得动态地址后可使用多久，例如 12h；更短会增加续租请求。">
                <input aria-label={t('DHCP 租约时长')} value={config.dhcp.lease_time} onChange={event => setConfig({ ...config, dhcp: { ...config.dhcp, lease_time: event.target.value } })} />
              </ConfigField>
            </div>
          </fieldset>
          {config.gateway.mode === 'same_wifi_dhcp' && <fieldset className="dhcp-config-group">
            <legend><strong>{t('直连主路由设备')}</strong><small>{t('仅供设备页“直连主路由”使用；普通接管设备仍获得 Mac 网关和 DNS')}</small></legend>
            <div className="dhcp-config-grid">
              <ConfigField label="主路由网关" setting="dhcp.bypass_gateway" hint="向取消 OpenSurge IPv4 网关接管的设备下发。必须与 Mac 网关处于同一子网，且不能位于 DHCP 地址池中。">
                <input aria-label={t('直连主路由网关')} placeholder="192.168.1.1" value={config.dhcp.bypass_gateway} onChange={event => setConfig({ ...config, dhcp: { ...config.dhcp, bypass_gateway: event.target.value } })} />
              </ConfigField>
              <ConfigField label="主路由 DNS" setting="dhcp.bypass_dns" hint="向直连主路由设备下发，可填写原路由器或公共 DNS；多个地址用逗号分隔。">
                <input aria-label={t('直连主路由 DNS')} placeholder="192.168.1.1" value={config.dhcp.bypass_dns.join(', ')} onChange={event => setConfig({ ...config, dhcp: { ...config.dhcp, bypass_dns: event.target.value.split(',').map(item => item.trim()).filter(Boolean) } })} />
              </ConfigField>
            </div>
            <button type="button" className="text-link" disabled={!plan?.snapshot.router && !recoverySnapshot?.router} onClick={() => {
              const snapshot = plan?.snapshot || recoverySnapshot
              if (!snapshot) return
              setConfig({ ...config, dhcp: { ...config.dhcp, bypass_gateway: snapshot.router || '', bypass_dns: snapshot.dns.length ? snapshot.dns : snapshot.router ? [snapshot.router] : [] } })
            }}>{t('使用当前网络快照中的路由器与 DNS')}</button>
          </fieldset>}
          <details className="mihomo-dns-advanced">
            <summary aria-label={t('高级 Mihomo / DNS 设置')}>
              <span><strong>{t('高级 Mihomo / DNS 设置')}</strong><small>{t('fake-IP 映射、dnsmasq 上游与透明代理入口')}</small></span>
              <span className="mihomo-dns-advanced-status">{t(config.mihomo.store_fake_ip ? '映射已持久化' : '映射不持久化')} · {config.transparent.mode === 'tun' ? 'TUN' : t('透明代理关闭')}</span>
            </summary>
            <div className="mihomo-dns-advanced-grid">
              <ConfigField className="wide" label="上游 DNS" setting="dns.upstream" hint="dnsmasq 转发客户端 DNS 查询时使用的解析器，可填 IPv4 或 IPv4#port（例如 127.0.0.1#1053）。客户端的 DNS 会指向上面的 Mac 网关 IPv4，而不是此地址。">
                <div className="dns-presets" role="group" aria-label={t('上游 DNS 预设')}>
                  <button type="button" aria-pressed={config.dns.upstream === '127.0.0.1#1053'} onClick={() => setConfig({ ...config, dns: { ...config.dns, upstream: '127.0.0.1#1053' } })}>{t('mihomo DNS（推荐）')}</button>
                  <button type="button" aria-pressed={config.dns.upstream === '1.1.1.1'} onClick={() => setConfig({ ...config, dns: { ...config.dns, upstream: '1.1.1.1' } })}>{t('公共 DNS（调试）')}</button>
                </div>
                <input aria-label={t('上游 DNS')} placeholder={t('1.1.1.1 或 127.0.0.1#1053')} value={config.dns.upstream} onChange={event => setConfig({ ...config, dns: { ...config.dns, upstream: event.target.value } })} />
                <small>{t('推荐路径进入 mihomo fake-IP DNS。公共 DNS 仅用于对照；启用 TUN 时仍可能被 dns-hijack 捕获，并不保证绕过代理。')}</small>
              </ConfigField>
              <ConfigField label="透明代理模式" setting="transparent.mode" hint={config.gateway.mode === 'isolated_lan' ? 'tun 让未设置显式代理的下游流量进入 mihomo TUN；off 不做透明捕获。旁路由模式与局域网 DHCP 接管模式必须使用 TUN。' : '当前拓扑必须使用 mihomo TUN，因此该选项已锁定。'}>
                <select aria-label={t('透明代理模式')} value={config.transparent.mode} disabled={config.gateway.mode !== 'isolated_lan'} onChange={event => {
                  const mode = event.target.value as 'off' | 'tun'
                  setConfig({ ...config, dns: { ...config.dns, ipv6: mode === 'tun' && config.dns.ipv6 }, transparent: { ...config.transparent, mode, tun_ipv6: mode === 'off' ? 'off' : config.transparent.tun_ipv6 }, local_system_proxy: { ...config.local_system_proxy, enabled: mode === 'tun' && config.local_system_proxy.enabled } })
                }}><option value="off">{t('关闭（off）')}</option><option value="tun">mihomo TUN</option></select>
              </ConfigField>
              <ConfigField label="Fake-IP 映射持久化" setting="mihomo.store_fake_ip" hint="生成 profile.store-fake-ip。开启后 mihomo 会在重启时恢复域名与 fake-IP 的映射，避免 cloudflared 等长驻进程继续使用已经失效的 198.18.x.x；不会保留既有 TCP/QUIC 连接。修改 fake-ip-filter 后，旧映射仍可能需要单独清理缓存。">
                <ConfigSwitch
                  label="重启后保留 fake-IP 映射"
                  checked={config.mihomo.store_fake_ip}
                  onChange={store_fake_ip => setConfig({ ...config, mihomo: { ...config.mihomo, store_fake_ip } })}
                />
              </ConfigField>
              <div className="notice mihomo-dns-import-boundary" role="note" aria-label={t('导入 mihomo profile 设置边界')}>
                <p><strong>{t('OpenSurge 会保留并合并：')}</strong>{t('导入 profile 中的 DNS 解析器与过滤设置，例如')} <code>nameserver</code>、<code>nameserver-policy</code>、<code>proxy-server-nameserver</code>、<code>direct-nameserver</code>、<code>respect-rules</code>、<code>fake-ip-filter</code> {t('和')} <code>fallback</code>。</p>
                <p><strong>{t('OpenSurge 自主管理，不保留导入值：')}</strong>{t('DNS 监听、IPv6、fake-IP 模式与网段，以及')} <code>profile.store-fake-ip</code>。</p>
              </div>
            </div>
          </details>
          <DownstreamIPv6Card
            config={config}
            editable={configurationEditable}
            gatewayActive={gatewayActive}
            runtimeStatus={overview?.status ?? null}
            ipv6LinkLocalGateway={interfaceOptions.find(option => option.interface === config.gateway.interface)?.ipv6_link_local ?? ''}
            onDNSChange={ipv6 => setConfig({ ...config, dns: { ...config.dns, ipv6 } })}
            onTakeoverChange={tun_ipv6 => setConfig({ ...config, transparent: { ...config.transparent, tun_ipv6 } })}
            onSharedL2ReadyChange={ipv6_shared_l2_ready => setConfig({ ...config, transparent: { ...config.transparent, ipv6_shared_l2_ready } })}
          />
          {config.gateway.mode === 'same_lan' && <BypassRouterClientGuide
            config={config}
            ipv6LinkLocalGateway={interfaceOptions.find(option => option.interface === config.gateway.interface)?.ipv6_link_local ?? ''}
          />}
          <ConfigField label="Mac 本机系统代理协同" setting="local_system_proxy.enabled" hint="启动时把上游网络服务的 macOS Web Proxy（HTTP）和 Secure Web Proxy（HTTPS）指向 127.0.0.1:mihomo.mixed_port，停止、回滚或 mihomo 重启失败时恢复原状态。可用于兼容 SafeDNS、DNS Proxy、内容过滤或其他 Network Extension 干扰仅 TUN 本机 DNS 的问题；只覆盖遵循系统代理的 Mac 应用，不替代 TUN，也不影响下游设备。已有系统代理、PAC 或自动发现时会拒绝启动，避免覆盖用户配置。">
            <ConfigSwitch
              label="启用 macOS HTTP/HTTPS 系统代理"
              accessibleLabel="同时启用 macOS HTTP/HTTPS 系统代理"
              checked={config.local_system_proxy.enabled}
              disabled={config.transparent.mode !== 'tun'}
              disabledText="需要 TUN"
              onChange={enabled => setConfig({ ...config, local_system_proxy: { ...config.local_system_proxy, enabled } })}
            />
          </ConfigField>
          <ConfigField label="每设备策略" setting="device_policy.file" hint="启用后可在“设备”页为 MAC 固定租约及独立 mihomo 策略；若尚无策略文件，保存时会创建一个空文件。关闭后不再使用此策略文件。">
            <ConfigSwitch
              label="启用每设备策略"
              checked={config.device_policy.enabled}
              onChange={enabled => setConfig({ ...config, device_policy: { ...config.device_policy, enabled } })}
            />
          </ConfigField>
          <ConfigField className="wide" label="受保护的 IPv4" setting="device_policy.protected_ipv4" hint="以逗号分隔的路由器、恢复设备或其他静态主机地址。每设备策略的固定租约不得占用这些地址；仅在启用每设备策略时可编辑。">
            <input aria-label={t('受保护的 IPv4')} disabled={!config.device_policy.enabled} placeholder="192.168.1.1, 192.168.1.21" value={config.device_policy.protected_ipv4.join(', ')} onChange={event => setConfig({ ...config, device_policy: { ...config.device_policy, protected_ipv4: event.target.value.split(',').map(item => item.trim()).filter(Boolean) } })} />
          </ConfigField>
          </div>
        </fieldset>
        <div className="network-save-bar" aria-live="polite"><span className={configDirty ? 'dirty' : ''}><i aria-hidden="true">{configDirty ? '•' : '✓'}</i>{t(configDirty ? '有未保存的修改' : '当前配置已保存')}</span><button className="primary" disabled={!configurationEditable} onClick={() => void save()}>{configSaving ? <><span className="button-spinner" aria-hidden="true" />{t('正在保存…')}</> : t('保存网络配置')}</button></div>
      </section>
    </>}
    {config && gatewayInterrupted && <section className="section gateway-lifecycle-control interrupted-runtime-control">
      <SectionTitle title="安全清理旧状态" subtitle="Mac 重启已经结束上一次网关运行；清理磁盘上的旧运行记录后，才能继续恢复或重新启动。" />
      <div className="gateway-lifecycle-row">
        <div>
          <span className="pill">{t('重启后待清理')}</span>
          <strong>{t('上一次网关运行已中断')}</strong>
          <p>{t('此操作只清理上次开机留下的状态，不会停止本次开机的其他进程，也不会更改本次开机的 PF 或 IPv4 forwarding。')}</p>
        </div>
        <button ref={gatewayControlRef} id="gateway-control" className="primary" type="button" disabled={busy || !overview} onClick={() => void cleanupInterruptedRuntime()}>{t(busy ? '正在安全清理…' : '安全清理旧状态')}</button>
      </div>
      <div className="notice">{t('如果上次运行启用了系统代理协同，HTTP/HTTPS 会恢复为 OpenSurge 启动前保存的状态；接管期间的手动修改也会被该快照替换。')}</div>
    </section>}
    {config && !gatewayInterrupted && config.gateway.mode !== 'same_wifi_dhcp' && <section className="section gateway-lifecycle-control">
      <SectionTitle title="网关运行控制" subtitle="使用已保存的网络配置启动或停止；总览页的按钮只负责导航到这里" />
      <div className="gateway-lifecycle-row">
        <div>
          <span className={`pill ${gatewayActive ? 'ok' : ''}`}>{t(gatewayActive ? '运行中' : gatewayStopped ? '已停止' : '状态未知')}</span>
          <strong>{gatewayModeLabel(config.gateway.mode)}</strong>
          <p>{gatewayModeDescription(config)}</p>
        </div>
        <button ref={gatewayControlRef} id="gateway-control" className={gatewayActive ? 'danger' : 'primary'} type="button" disabled={busy || !overview || (!gatewayActive && !gatewayStopped) || (!gatewayActive && configDirty)} onClick={() => void controlGateway(gatewayActive ? 'stop' : 'start')}>{busy ? t('正在执行…') : gatewayActive ? t('停止{{mode}}', { mode: gatewayModeLabel(config.gateway.mode) }) : t('启动{{mode}}', { mode: gatewayModeLabel(config.gateway.mode) })}</button>
      </div>
      {!gatewayActive && configDirty && <div className="notice warn">{t('网络配置有未保存的修改。保存后才能启动网关。')}</div>}
      {!gatewayActive && !gatewayStopped && <div className="notice warn">{t('当前网关状态无法确认；为避免重复启动，运行控制暂时不可用。')}</div>}
    </section>}
    {config?.gateway.mode === 'same_wifi_dhcp' && !gatewayInterrupted && <>
      {plan && <section className="section">
        <SectionTitle title="当前网络快照" subtitle={`${plan.snapshot.network_service} · ${plan.snapshot.interface}`} />
        <div className="inventory"><span>Mac {plan.snapshot.ipv4}</span>{plan.snapshot.router && <span>Router <RouterAddress router={plan.snapshot.router} showHint /></span>}<span>{plan.snapshot.ipv6_default ? 'IPv6 default active' : 'No IPv6 default'}</span><span>{plan.protected_ipv4.length} protected IPv4</span></div>
        {plan.blockers.map(item => <div className="notice warn" key={item}>{item}</div>)}{plan.warnings.map(item => <div className="notice" key={item}>{item}</div>)}
      </section>}
      {recoverySnapshot && <section className="section recovery-card">
        <SectionTitle title="已保存的恢复资料" subtitle="这是切换网络前保存的原始配置；即使当前网络状态随后改变，也以这里的内容作为恢复依据。" />
        <dl className="recovery-card-grid">
          <div><dt>{t('原始 IPv4')}</dt><dd>{recoverySnapshot.ipv4 || '—'}</dd></div>
          <div><dt>{t('原始路由器')}</dt><dd>{recoverySnapshot.router ? <RouterAddress router={recoverySnapshot.router} showHint /> : '—'}</dd></div>
          <div><dt>{t('原始 DNS')}</dt><dd>{recoverySnapshot.dns.length ? recoverySnapshot.dns.join(', ') : t('自动 / 未记录')}</dd></div>
          <div><dt>{t('网络服务')}</dt><dd>{recoverySnapshot.network_service || '—'}</dd></div>
          <div><dt>{t('接口')}</dt><dd>{recoverySnapshot.interface || '—'}</dd></div>
          <div><dt>{t('子网掩码')}</dt><dd>{recoverySnapshot.subnet_mask || '—'}</dd></div>
        </dl>
        <div className="recovery-card-actions"><a href="/api/v1/recovery/card" target="_blank" rel="noopener noreferrer">{t('查看恢复卡')}</a><a href="/api/v1/recovery/card?download=1" download="OpenSurge-WiFi-DHCP-Recovery-Card.txt">{t('下载恢复卡')}</a></div>
      </section>}
      <section className="section">
        <SectionTitle title="恢复状态机" subtitle="推荐路径保留真实系统动作与网络证据；可跳过节点会明确记录为未验证" />
        <div className="timeline">{stages.map((stage, index) => <div className={index < currentIndex ? 'done' : index === currentIndex ? 'current' : ''} key={stage}><span>{index < currentIndex ? '✓' : index + 1}</span><p>{recoveryLabel(stage)}</p></div>)}</div>
        <div className="cooperative"><strong>{t(config.transparent.tun_ipv6 !== 'off' ? 'IPv4 与 IPv6 全 LAN 接管' : '合作式 IPv4 模式')}</strong><p>{t(config.transparent.tun_ipv6 !== 'off' ? 'OpenSurge 将同时提供 DHCP 与 IPv6 RA。继续前必须关闭主路由 IPv4 DHCP 和 IPv6 RA/DHCPv6，或用 RA Guard 保证客户端只收到 OpenSurge 默认路由。' : '同一二层 LAN 中，客户端仍可能通过手工路由器网关或 IPv6 绕过 Mac。要求不可绕过时请选择独立 AP/SSID/VLAN。')}</p></div>
        {current === 'prepared' && <div className="notice">{t('恢复资料已经保存，但 Mac、路由器与 DHCP 都尚未改动。此时仍可修正并保存目标配置；保存会清除这张预备恢复卡，并从第 1 步重新开始。')}</div>}
        {configDirty && <div className="notice warn">{t('网络配置有未保存的修改。先保存配置，再保存恢复资料或继续第 2 步。')}</div>}
        {current === 'mac_static' && <RouterDHCPGuide action="关闭" router={router} networkService={networkService} />}
        {current === 'mac_static' && config.transparent.tun_ipv6 !== 'off' && <div className="notice warn">{t('还需要关闭主路由的 IPv6 RA、SLAAC 或 DHCPv6 路由发布。仅关闭 IPv4 DHCP 不足以阻止 IPv6 绕过 Mac。')}</div>}
        {current === 'gateway_active' && <div className="form-stack"><input aria-label={t('验收客户端 IPv4')} placeholder={t('客户端从 OpenSurge 获得的 IPv4')} value={clientIPv4} onChange={event => setClientIPv4(event.target.value)} /><label><input type="checkbox" checked={clientConfirmed} onChange={event => setClientConfirmed(event.target.checked)} /> {t('已在客户端确认默认网关/DNS 为 Mac，且没有显式代理')}</label>{plan?.snapshot.ipv6_default && <label><input type="checkbox" checked={ipv6Acknowledged} onChange={event => setIPv6Acknowledged(event.target.checked)} /> {t('已知 IPv6 默认路由可能绕过 IPv4 设备策略')}</label>}</div>}
        {current === 'gateway_active' && <div className="notice">{t('推荐完成客户端验收。若当前没有合适客户端，可跳过；跳过只解除 GUI 流程阻塞，不会产生 DHCP、DNS 或 TUN 验收证据。')}</div>}
        {current === 'client_validation_skipped' && <div className="notice warn">{t('客户端验收已由用户跳过，本次运行没有客户端 DHCP、DNS 与 TUN 数据面验收结论。')}</div>}
        {current === 'gateway_stopped_waiting_router_dhcp' && <RouterDHCPGuide action="恢复" router={router} networkService={networkService} />}
        {current === 'gateway_stopped_waiting_router_dhcp' && <div className="notice warn">{t('可以恢复路由器 DHCP 并执行 OFFER 探测，也可以人工确认后跳过 OFFER 证据并恢复 Mac 自动 DHCP。若要长期保持静态 IPv4，可直接结束；这不会恢复其他客户端的自动获取能力。')}</div>}
        {current === 'router_dhcp_restored' && <div className="notice">{t('已经检测到 DHCP OFFER。你可以把 Mac 恢复为自动 DHCP，也可以保留当前静态 IPv4 后结束流程。')}</div>}
        {current === 'complete_static' && <div className="notice">{t('恢复流程已结束，Mac 仍使用静态 IPv4；路由器 DHCP 与其他客户端的自动获取能力没有在这条路径中验证。')}</div>}
        <div className="recovery-actions">
          <button ref={gatewayControlRef} id="gateway-control" className="primary" disabled={busy || configDirty || blockedByPlan || (current === 'gateway_active' && (!clientIPv4 || !clientConfirmed || Boolean(plan?.snapshot.ipv6_default && !ipv6Acknowledged)))} onClick={() => void advance()}>{busy ? t('正在验证…') : actionLabel(current)}</button>
          {current === 'prepared' && <button className="danger" disabled={busy} onClick={() => void discardRecovery()}>{t('放弃恢复并销毁资料')}</button>}
          {(current === 'mac_static' || current === 'router_dhcp_disabled_confirmed') && <button className="danger" disabled={busy} onClick={() => void abandonTakeover()}>{t('放弃 DHCP 接管')}</button>}
          {current === 'gateway_active' && <button className="danger" disabled={busy} onClick={() => void skipClientValidation()}>{t('跳过客户端验收')}</button>}
          {current === 'gateway_stopped_waiting_router_dhcp' && <button className="danger" disabled={busy} onClick={() => void finishRecoveryManually()}>{t('跳过 OFFER 探测并恢复 Mac 自动 DHCP')}</button>}
          {(current === 'gateway_stopped_waiting_router_dhcp' || current === 'router_dhcp_restored') && <button className="danger" disabled={busy} onClick={() => void finishKeepingStatic()}>{t('保留静态 IP 并结束')}</button>}
        </div>
      </section>
    </>}
    {policyMigration && <PolicyMigrationDialog migration={policyMigration} busy={busy} onInspect={() => { setPolicyMigration(null); onNavigate('devices') }} onCancel={() => setPolicyMigration(null)} onConfirm={() => void persistConfig(policyMigration.target, policyMigration)} />}
  </>
}

function DownstreamIPv6Card({ config, editable, gatewayActive, runtimeStatus, ipv6LinkLocalGateway, onDNSChange, onTakeoverChange, onSharedL2ReadyChange }: {
  config: ControlConfig
  editable: boolean
  gatewayActive: boolean
  runtimeStatus: Overview['status'] | null
  ipv6LinkLocalGateway: string
  onDNSChange: (enabled: boolean) => void
  onTakeoverChange: (mode: ControlConfig['transparent']['tun_ipv6']) => void
  onSharedL2ReadyChange: (ready: boolean) => void
}) {
  const isolated = config.gateway.mode === 'isolated_lan'
  const bypassRouter = config.gateway.mode === 'same_lan'
  const sharedL2 = !isolated
  const tunReady = config.transparent.mode === 'tun'
  const available = tunReady
  const enabled = available && config.transparent.tun_ipv6 !== 'off'
  const sharedL2Ready = Boolean(config.transparent.ipv6_shared_l2_ready)
  const interfacesSeparated = config.gateway.interface.trim() !== '' && config.gateway.upstream_interface.trim() !== '' && config.gateway.interface !== config.gateway.upstream_interface
  const interfacesShared = config.gateway.interface.trim() !== '' && config.gateway.interface === config.gateway.upstream_interface
  const status = ipv6CardStatus(available, gatewayActive, runtimeStatus, config.transparent.tun_ipv6)
  const automaticAddressing = !bypassRouter
  const manualGateway = ipv6LinkLocalGateway ? `${ipv6LinkLocalGateway}%${config.gateway.interface}` : ''

  return <article className={`downstream-ipv6-card ${available ? 'is-available' : 'is-safe-closed'}`} aria-labelledby="downstream-ipv6-title">
    <header className="ipv6-card-header">
      <div className="ipv6-card-identity"><span aria-hidden="true">6</span><div><small>IPV6 GATEWAY</small><h3 id="downstream-ipv6-title">{t('下游 IPv6')}</h3><p>{t('复用 OpenSurge 用户态数据面接管 TCP、UDP 与 QUIC；独立 LAN 和 DHCP 接管可自动下发，旁路由仅接入手工配置的设备。')}</p></div></div>
      <span className={`pill ${status.tone}`.trim()}>{status.label}</span>
    </header>

    {!available ? <div className="ipv6-safe-state" role="status">
      <span aria-hidden="true">✓</span>
      <div><strong>{t('先启用 mihomo TUN')}</strong><p>{t('透明代理关闭时不会建立下游 IPv6 数据面。先在上方选择 mihomo TUN，再配置此卡片。')}</p></div>
    </div> : <>
      {sharedL2 && <div className="ipv6-card-step ipv6-shared-l2-step">
        <div className="ipv6-step-copy"><span>1</span><div><small>{t('共享局域网前置条件')}</small><strong>{t(bypassRouter ? '只接入手工配置的设备' : '确认 OpenSurge 是唯一 IPv6 路由提供者')}</strong><p>{t(bypassRouter ? 'OpenSurge 不广播 RA；选定设备必须手工使用 OpenSurge ULA、默认路由和 DNS，并且不能保留主路由 IPv6 默认路由。' : '此模式会向整个 LAN 广播 RA。请先关闭主路由 IPv6 RA/DHCPv6，或使用 RA Guard 确保客户端只能收到 OpenSurge。')}</p></div></div>
        <label className={`ipv6-readiness-ack ${sharedL2Ready ? 'is-acknowledged' : ''} ${!editable ? 'is-disabled' : ''}`}>
          <input
            aria-label={t('我已知晓共享局域网 IPv6 前置条件')}
            type="checkbox"
            checked={sharedL2Ready}
            disabled={!editable}
            onChange={event => onSharedL2ReadyChange(event.target.checked)}
          />
          <span className="ipv6-readiness-check" aria-hidden="true">{sharedL2Ready ? '✓' : ''}</span>
          <span><strong>{t('我已知晓上述前置条件')}</strong><small>{t(sharedL2Ready ? '已确认' : '勾选后才可启用共享 LAN IPv6')}</small></span>
        </label>
      </div>}

      <div className="ipv6-card-step">
        <div className="ipv6-step-copy"><span>{sharedL2 ? '2' : '1'}</span><div><small>{t('接管策略')}</small><strong>{t('何时为下游启用 IPv6')}</strong><p>{t('推荐自动检测；需要依赖代理节点承载 IPv6 时再使用“总是开启”。')}</p></div></div>
        <fieldset className="ipv6-mode-options" disabled={!editable} aria-label={t('下游 IPv6 接管策略')}>
          <legend className="sr-only">{t('下游 IPv6 接管策略')}</legend>
          <IPv6ModeOption mode="off" current={config.transparent.tun_ipv6} title="关闭" description="不建立下游 IPv6 数据面" onSelect={onTakeoverChange} />
          <IPv6ModeOption mode="auto" current={config.transparent.tun_ipv6} title="自动" badge="推荐" description="上游原生 IPv6 可用时启用" onSelect={onTakeoverChange} />
          <IPv6ModeOption mode="always" current={config.transparent.tun_ipv6} title="总是开启" description="无论上游状态都建立接管路径" onSelect={onTakeoverChange} />
        </fieldset>
      </div>

      <div className="ipv6-card-step ipv6-dns-step">
        <div className="ipv6-step-copy"><span>{sharedL2 ? '3' : '2'}</span><div><small>{t('域名解析')}</small><strong>{t('解析 IPv6 域名')}</strong><p>{t('开启后 OpenSurge DNS 回答 AAAA 并可生成 fake IPv6；它不会单独建立下游路由。')}</p></div></div>
        <ConfigSwitch label="允许 AAAA 查询" accessibleLabel="解析 IPv6 域名" checked={config.dns.ipv6} disabled={!editable} onChange={onDNSChange} />
      </div>

      <div className={`ipv6-result ${enabled ? 'is-enabled' : ''}`}>
        <div className="ipv6-result-heading"><div><small>{t('设备最终获得')}</small><strong>{t(enabled ? (bypassRouter ? '一条手工接入 OpenSurge 的 IPv6 路径' : '一条由 OpenSurge 完整提供的 IPv6 路径') : '不使用 OpenSurge 下游 IPv6')}</strong></div><span>{enabled ? ipv6ModeLabel(config.transparent.tun_ipv6) : t('已关闭')}</span></div>
        <dl>
          <div><dt>{t('IPv6 地址')}</dt><dd>{enabled ? t(automaticAddressing ? 'OpenSurge ULA · 自动' : 'OpenSurge ULA · 手工') : t('不提供')}</dd></div>
          <div><dt>{t('默认路由')}</dt><dd>{enabled ? t(automaticAddressing ? '经此 Mac · 自动' : '经此 Mac · 手工') : t('不提供')}</dd></div>
          <div><dt>DNS</dt><dd>{enabled ? `${t(bypassRouter ? '手工指向 OpenSurge' : 'OpenSurge 自动下发')} · ${t(config.dns.ipv6 ? 'AAAA 开启' : '仅 IPv4')}` : t('不提供')}</dd></div>
        </dl>
        {enabled && <p>{t('下游设备不需要运营商 Prefix Delegation；公网出口由 Mac 的原生 IPv6 或所选代理节点完成。')}</p>}
      </div>

      {enabled && <div className="ipv6-readiness">
        <strong>{t('启动前确认')}</strong>
        {isolated ? <ul>
          <li className={interfacesSeparated ? 'ready' : 'warn'}><span>{interfacesSeparated ? '✓' : '!'}</span><div><b>{t('上游与下游使用不同接口')}</b><small>{interfacesSeparated ? `${config.gateway.upstream_interface} → ${config.gateway.interface}` : t('当前接口相同，请先修正接口配置')}</small></div></li>
          <li className="ready"><span>✓</span><div><b>{t('自动发布 RA/SLAAC/RDNSS')}</b><small>{t('连接独立下游网络的设备自动获得完整 IPv6 配置')}</small></div></li>
          <li><span>•</span><div><b>{t('下游没有其他 IPv6 路由器')}</b><small>{t('请确认 AP、VLAN 或独立 LAN 不会发布竞争默认路由')}</small></div></li>
        </ul> : bypassRouter ? <ul>
          <li className={interfacesShared ? 'ready' : 'warn'}><span>{interfacesShared ? '✓' : '!'}</span><div><b>{t('共享同一 LAN 接口')}</b><small>{interfacesShared ? config.gateway.interface : t('旁路由模式要求上下游接口相同')}</small></div></li>
          <li className="ready"><span>✓</span><div><b>{t('OpenSurge 不会广播 RA')}</b><small>{t('未选中的局域网设备不会被自动迁移到 Mac')}</small></div></li>
          <li className={sharedL2Ready && Boolean(manualGateway) ? 'ready' : 'warn'}><span>{sharedL2Ready && manualGateway ? '✓' : '!'}</span><div><b>{t('逐台手工设置 IPv6')}</b><small>{t('地址 fdfe:dcba:9878::/64；默认网关与 DNS 均使用')} {manualGateway || t('尚未检测到的 Mac 接口链路本地 IPv6')}</small></div></li>
        </ul> : <ul>
          <li className={interfacesShared ? 'ready' : 'warn'}><span>{interfacesShared ? '✓' : '!'}</span><div><b>{t('共享同一 LAN 接口')}</b><small>{interfacesShared ? config.gateway.interface : t('DHCP 接管模式要求上下游接口相同')}</small></div></li>
          <li className={sharedL2Ready ? 'ready' : 'warn'}><span>{sharedL2Ready ? '✓' : '!'}</span><div><b>{t('主路由 IPv6 RA 已停止')}</b><small>{t('OpenSurge 将向整个 LAN 自动发布 RA/SLAAC/RDNSS')}</small></div></li>
          <li><span>•</span><div><b>{t('全 LAN IPv6 都会经 Mac')}</b><small>{t('这不是只影响 DHCP 租约设备的局部设置')}</small></div></li>
        </ul>}
      </div>}

      {enabled && sharedL2 && !sharedL2Ready && <div className="notice warn">{t('保存会被后端拒绝：请先完成共享局域网 IPv6 前置条件并勾选确认。')}</div>}
      {enabled && bypassRouter && <div className="notice">{t('旁路由 IPv6 不发送 RA。只接入手工配置')} <code>fdfe:dcba:9878::/64</code> {t('地址、默认网关与 DNS')} <code>{manualGateway || t('Mac 的 fe80:: 链路本地地址%{{interface}}', { interface: config.gateway.interface })}</code>，{t('且已移除主路由 IPv6 默认路由的设备。')}</div>}
      {enabled && config.dns.ipv6 === false && <div className="notice">{t('接管路径已开启，但普通域名不会获得 AAAA；IPv6 字面地址仍可进入 OpenSurge。')}</div>}
      {config.transparent.tun_ipv6 === 'always' && <div className="notice warn">{t('“总是开启”只保证下游接管路径存在。若上游没有原生 IPv6，DIRECT IPv6 不可用，UDP/QUIC 还需要代理节点支持相应流量。')}</div>}
      {gatewayActive && runtimeStatus && <div className="ipv6-runtime" role="status">
        <div><small>{t('当前运行')}</small><strong>{status.label}</strong>{runtimeStatus.ipv6_reason && <p>{ipv6ReasonLabel(runtimeStatus.ipv6_reason)}</p>}</div>
        <dl><div><dt>AAAA</dt><dd>{t(runtimeStatus.dns_ipv6 ? '开启' : '关闭')}</dd></div><div><dt>{t('接管请求')}</dt><dd>{ipv6ModeLabel(runtimeStatus.tun_ipv6_requested)}</dd></div><div><dt>{t('数据面')}</dt><dd>{ipv6PacketLabel(runtimeStatus.ipv6_packet)}</dd></div><div><dt>{t('上游原生 IPv6')}</dt><dd>{ipv6NativeLabel(runtimeStatus.native_ipv6_available, runtimeStatus.ipv6_reason)}</dd></div></dl>
      </div>}
    </>}
  </article>
}

function BypassRouterClientGuide({ config, ipv6LinkLocalGateway }: { config: ControlConfig; ipv6LinkLocalGateway: string }) {
  const ipv4Hint = ipv4ClientAddressHint(config.gateway.lan_ip)
  const ipv6Gateway = ipv6LinkLocalGateway ? `${ipv6LinkLocalGateway}%${config.gateway.interface}` : t('等待 {{interface}} 获得 fe80:: 地址', { interface: config.gateway.interface })
  const ipv6Enabled = config.transparent.tun_ipv6 !== 'off'

  return <aside className="bypass-client-guide" aria-label={t('旁路由设备填写速查')}>
    <header><div><small>CLIENT SETUP</small><strong>{t('旁路由设备填写速查')}</strong><p>{t('只修改需要接入 OpenSurge 的设备。每台设备的 IPv4 与 IPv6 地址必须唯一；网关和 DNS 可直接照填。')}</p></div><span className="pill ok">{t('手工接入')}</span></header>
    <div className="bypass-client-guide-grid">
      <section aria-label={t('IPv4 填写提示')}><h4>IPv4</h4><dl>
        <div><dt>{t('设备地址')}</dt><dd>{ipv4Hint}</dd></div>
        <div><dt>{t('子网掩码')}</dt><dd><code>255.255.255.0</code></dd></div>
        <div><dt>{t('默认网关')}</dt><dd><code>{config.gateway.lan_ip}</code></dd></div>
        <div><dt>DNS</dt><dd><code>{config.gateway.lan_ip}</code></dd></div>
      </dl></section>
      <section aria-label={t('IPv6 填写提示')} className={ipv6Enabled ? '' : 'inactive'}><h4>IPv6 <span>{t(ipv6Enabled ? '已启用' : '需先开启上方接管')}</span></h4><dl>
        <div><dt>{t('设备地址')}</dt><dd><code>fdfe:dcba:9878::{t('设备编号')}/64</code><small>{t('例如 fdfe:dcba:9878::21/64')}</small></dd></div>
        <div><dt>{t('默认网关')}</dt><dd><code>{ipv6Gateway}</code><small>{t('必须使用 Mac 的链路本地地址')}</small></dd></div>
        <div><dt>DNS</dt><dd><code>{ipv6Gateway}</code><small>{t('与 IPv6 默认网关使用同一 Mac 链路本地地址')}</small></dd></div>
        <div><dt>{t('主路由默认路由')}</dt><dd>{t('在这台设备上移除，避免 IPv6 绕过 Mac')}</dd></div>
      </dl></section>
    </div>
  </aside>
}

function ipv4ClientAddressHint(gateway: string) {
  if (!ipv4Pattern.test(gateway)) return t('保留设备当前的稳定局域网 IPv4')
  const parts = gateway.split('.')
  return t('保留设备当前稳定地址（{{prefix}}.x/24，每台唯一）', { prefix: parts.slice(0, 3).join('.') })
}

function IPv6ModeOption({ mode, current, title, badge, description, onSelect }: {
  mode: ControlConfig['transparent']['tun_ipv6']
  current: ControlConfig['transparent']['tun_ipv6']
  title: string
  badge?: string
  description: string
  onSelect: (mode: ControlConfig['transparent']['tun_ipv6']) => void
}) {
  const active = current === mode
  return <button type="button" className={active ? 'active' : ''} aria-pressed={active} onClick={() => onSelect(mode)}><span><strong>{t(title)}</strong>{badge && <em>{t(badge)}</em>}</span><small>{t(description)}</small></button>
}

function PolicyMigrationDialog({ migration, busy, onInspect, onCancel, onConfirm }: { migration: PolicyMigration; busy: boolean; onInspect: () => void; onCancel: () => void; onConfirm: () => void }) {
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === 'Escape' && !busy) onCancel() }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [busy, onCancel])
  return <dialog className="reload-dialog" open aria-modal="true" aria-labelledby="policy-migration-title">
    <h2 id="policy-migration-title">{t('确认设备身份后切换 DHCP 模式')}</h2>
    {migration.resolved.length > 0 && <><p>{t('以下设备此前只登记了固定 IPv4；OpenSurge 现在已观察到 MAC，确认后会补充到设备资料：')}</p><ul>{migration.resolved.map(device => <li key={device.id}><strong>{device.name || device.id}</strong> · <code>{device.ipv4}</code> · <code>{device.mac}</code></li>)}</ul></>}
    {migration.unresolved.length > 0 && <><div className="notice warn"><strong>{t('这些设备的策略将在 DHCP 模式下暂停，补充 MAC 后恢复。')}</strong></div><ul>{migration.unresolved.map(device => <li key={device.id}><strong>{device.name || device.id}</strong> · <code>{device.ipv4}</code> · {t('MAC 尚未观察到')}</li>)}</ul><p>{t('设备登记、Profile 和规则都会保留；DHCP 模式不会继续按旧 IP 匹配它们。')}</p></>}
    <div className="dialog-actions">{migration.unresolved.length > 0 && <button type="button" disabled={busy} onClick={onInspect}>{t('检查设备')}</button>}<button type="button" disabled={busy} onClick={onCancel}>{t('取消')}</button><button className="primary" type="button" autoFocus disabled={busy} onClick={onConfirm}>{t(busy ? '正在保存…' : migration.unresolved.length > 0 ? '仍然切换并暂停这些策略' : '确认 MAC 并切换')}</button></div>
  </dialog>
}

function gatewayModeLabel(mode: ControlConfig['gateway']['mode']) {
  if (mode === 'same_lan') return t('旁路由模式')
  if (mode === 'same_wifi_dhcp') return t('局域网 DHCP 接管')
  return t('独立下游 LAN')
}

function ipv6CardStatus(available: boolean, gatewayActive: boolean, runtimeStatus: Overview['status'] | null, desired: ControlConfig['transparent']['tun_ipv6']) {
  if (!available) return { label: t('安全关闭'), tone: '' }
  if (!gatewayActive || !runtimeStatus) return desired === 'off'
    ? { label: t('已关闭'), tone: '' }
    : { label: `${ipv6ModeLabel(desired)} · ${t('待启动')}`, tone: 'ok' }
  if (runtimeStatus.ipv6_packet === 'ready') return { label: t('正在接管'), tone: 'ok' }
  if (runtimeStatus.ipv6_packet === 'failed') return { label: t('运行异常'), tone: 'bad' }
  if (runtimeStatus.ipv6_reason === 'native_ipv6_unavailable') return { label: t('等待上游 IPv6'), tone: '' }
  if (runtimeStatus.tun_ipv6_requested === 'off' || runtimeStatus.ipv6_packet === 'disabled') return { label: t('已关闭'), tone: '' }
  return { label: t('尚未建立'), tone: '' }
}

function ipv6ModeLabel(mode: ControlConfig['transparent']['tun_ipv6']) {
  return t(mode === 'auto' ? '自动' : mode === 'always' ? '总是启用' : '关闭')
}

function ipv6PacketLabel(status: Overview['status']['ipv6_packet']) {
  return t(status === 'ready' ? '已接管' : status === 'failed' ? '异常' : status === 'stopped' ? '未运行' : '已关闭')
}

function ipv6NativeLabel(available: boolean, reason?: string) {
  if (reason?.includes('native_detection_failed:')) return t('检测失败')
  return t(available ? '可用' : '不可用')
}

function ipv6ReasonLabel(reason: string) {
  if (reason === 'native_ipv6_unavailable') return t('自动模式未启用：上游没有可用原生 IPv6')
  if (reason === 'native_ipv6_available') return t('已检测到上游原生 IPv6')
  if (reason === 'forced_userspace_packet_path') return t('已按“总是启用”建立用户态数据面')
  if (reason === 'disabled') return t('已关闭')
  if (reason === 'stopped') return t('网关未运行')
  if (reason.includes('native_detection_failed:')) return t('上游 IPv6 探测失败：{{error}}', { error: reason.slice(reason.indexOf('native_detection_failed:') + 'native_detection_failed:'.length).trim() })
  return reason
}

function gatewayModeDescription(config: ControlConfig) {
  if (config.gateway.mode === 'same_lan') return config.transparent.tun_ipv6 === 'off'
    ? t('启动 DNS、mihomo TUN、PF/NAT 与 IPv4 forwarding；路由器 DHCP 保持开启，部分设备需手工把网关和 DNS 指向 Mac。')
    : t('启动选择性 IPv6 packet 数据面但不广播 RA；指定设备需手工设置 OpenSurge ULA、默认网关和 DNS，其他局域网设备不变。')
  const proxyMode = t(config.transparent.mode === 'tun' ? 'mihomo TUN 透明代理' : '不启用透明代理')
  return t('启动 DHCP/DNS、PF/NAT 与 IPv4 forwarding；当前配置为{{mode}}。', { mode: proxyMode })
}

function gatewayConfirmation(mode: ControlConfig['gateway']['mode'], action: 'start' | 'stop') {
  if (mode === 'same_lan') {
    return t(action === 'start'
      ? '将按已保存配置启动旁路由模式。路由器 DHCP 不会被关闭；部分设备需要自行把网关和 DNS 指向 Mac。继续吗？'
      : '停止后，仍把网关或 DNS 指向 Mac 的设备可能立即断网。确定停止旁路由模式吗？')
  }
  if (mode === 'same_wifi_dhcp') {
    return t(action === 'start'
      ? '将按已保存配置启动局域网 DHCP/DNS 接管。请确认主路由器的 DHCP 与 IPv6 RA 已关闭。继续吗？'
      : '停止后，已使用 OpenSurge 作为 DHCP、DNS 或默认网关的局域网设备可能立即断网。确定停止吗？')
  }
  return t(action === 'start'
    ? '将按已保存配置启动独立下游 LAN 的 DHCP/DNS、PF/NAT 与 IPv4 forwarding。继续吗？'
    : '停止后，独立下游 LAN 客户端将失去 OpenSurge 提供的 DHCP/DNS 和网关连接。确定停止吗？')
}

function RouterAddress({ router, showHint = false }: { router: string; showHint?: boolean }) {
  if (!isIPv4(router)) return <>{router}</>
  return <><a className="router-link" href={`http://${router}`} target="_blank" rel="noopener noreferrer">{router}</a>{showHint && <small className="router-link-hint">{t('打不开?试试 https 或路由器专属域名')}</small>}</>
}

function RouterDHCPGuide({ action, router, networkService }: { action: '关闭' | '恢复'; router: string; networkService: string }) {
  const validRouter = isIPv4(router)
  return <div className="notice warn router-guide">
    <strong>{t(action === '关闭' ? '关闭路由器 DHCP' : '恢复路由器 DHCP')}</strong>
    <p>{validRouter ? <>{t('打开路由器后台')} <RouterAddress router={router} showHint /></> : <>{t('请打开路由器管理后台')}{router ? t('（检测值：{{router}}）', { router }) : ''}</>}，{t('按以下通用路径操作：')}</p>
    <ol>
      <li>{t('登录后台 → LAN / 网络设置 → DHCP 服务器')}</li>
      <li>{t(action === '关闭' ? '关闭 DHCP → 保存；保留路由器 LAN IP 不变' : '重新打开 DHCP → 保存；保留路由器 LAN IP 不变')}</li>
      <li>{t('回到 OpenSurge，点击 OFFER 探测按钮')}</li>
    </ol>
    {!validRouter && <small className="router-fallback">{t('未能自动获取路由器地址，可尝试在浏览器打开 192.168.1.1 / 192.168.0.1，或用')} <code>networksetup -getinfo '{networkService}'</code> {t('自行确认网关')}</small>}
  </div>
}

function isIPv4(value: string) { return ipv4Pattern.test(value) }
function validMAC(value: string) { return /^(?:[0-9a-f]{2}:){5}[0-9a-f]{2}$/i.test(value.trim()) }

function ConfigField({ label, setting, hint, className = '', children }: { label: string; setting: string; hint: string; className?: string; children: ReactNode }) {
  return <div className={`config-field ${className}`}><div className="config-field-title"><strong>{t(label)}</strong><code>{setting}</code></div>{children}<small>{t(hint)}</small></div>
}

function ConfigSwitch({ label, accessibleLabel = label, checked, disabled = false, disabledText, onChange }: { label: string; accessibleLabel?: string; checked: boolean; disabled?: boolean; disabledText?: string; onChange: (checked: boolean) => void }) {
  const status = t(disabled && disabledText ? disabledText : checked ? '已开启' : '已关闭')
  return <label className={`config-switch ${checked ? 'is-on' : ''} ${disabled ? 'is-disabled' : ''}`}>
    <input className="config-switch-input" aria-label={t(accessibleLabel)} type="checkbox" disabled={disabled} checked={checked} onChange={event => onChange(event.target.checked)} />
    <span className="config-switch-copy"><strong>{t(label)}</strong><small>{status}</small></span>
    <span className="config-switch-toggle" aria-hidden="true"><i /></span>
  </label>
}

function actionLabel(stage: string) {
  switch (stage) {
  case 'idle': case 'complete': case 'complete_static': return t('保存网络快照与离线恢复卡')
  case 'prepared': return t('将 Mac 切换为固定 IPv4')
  case 'mac_static': return t('已关闭路由器 DHCP，执行 OFFER 探测')
  case 'router_dhcp_disabled_confirmed': return t('启动 OpenSurge')
  case 'gateway_active': return t('验证客户端 DHCP、DNS 与 TUN 证据')
  case 'client_validated': return t('停止 OpenSurge')
  case 'client_validation_skipped': return t('停止 OpenSurge')
  case 'gateway_stopped_waiting_router_dhcp': return t('路由器 DHCP 已恢复，执行 OFFER 探测')
  case 'router_dhcp_restored': return t('将 Mac 恢复为自动 DHCP')
  default: return recoveryLabel(stage)
  }
}
