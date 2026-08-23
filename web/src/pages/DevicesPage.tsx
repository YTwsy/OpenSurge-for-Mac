import { useCallback, useEffect, useId, useMemo, useRef, useState, type RefObject } from 'react'
import { api, RequestError, waitForOperation } from '../api'
import { Empty, PageHeader, SectionTitle } from '../components/Common'
import { DeviceOutletSummary } from '../components/DeviceOutletSummary'
import { ConnectionRefreshControl } from '../components/ConnectionRefreshControl'
import { LocalRoutingCard } from '../components/LocalRoutingCard'
import { CLAUDE_CODE_RULE_SET_NAMES, CLAUDE_CODE_RULE_SETS, CLAUDE_CODE_SOURCE, CLAUDE_CODE_TEMPLATE } from '../data/builtinRuleLibrary'
import type { OperationNotification } from '../components/OperationNotifications'
import { useProxyHealth } from '../hooks/useProxyHealth'
import { policyDisplayName, TAILSCALE_POLICY } from '../policyDisplay'
import type { AppliedDeviceEgressMode, CompiledDevice, ControlConfig, DeviceEgressMode, DeviceGatewayTarget, DevicePolicyDocument, DevicesResponse, Lease, ObservedDevice, Overview, PolicyDevice, PolicyProfile, PolicyRule, PolicyRuleSet, PolicySet, PolicyTemplate, ProxyGroup, ProxyHealthEntry } from '../types'

const emptyPolicy = (): PolicySet => ({ devices: [], profiles: [], templates: [], rule_sets: [] })
const normalizePolicy = (value: PolicySet): PolicySet => ({ devices: value.devices ?? [], profiles: value.profiles ?? [], templates: value.templates ?? [], rule_sets: value.rule_sets ?? [] })
const copyPolicy = (value: PolicySet) => normalizePolicy(structuredClone(value))

type DevicesPageProps = {
  overview: Overview | null
  onChanged: () => Promise<void>
  onNavigate: (page: 'dashboard' | 'network' | 'policies') => void
  onDirtyChange: (dirty: boolean) => void
  onNotify: (notification: OperationNotification) => void
}

type DeviceRebindRequest = { deviceID: string; name: string; fromIPv4: string; toIPv4: string }
type RuleLibraryTab = 'rule_sets' | 'templates' | 'device_routes'

export function DevicesPage({ overview, onChanged, onNavigate, onDirtyChange, onNotify }: DevicesPageProps) {
  const proxyHealth = useProxyHealth()
  const [data, setData] = useState<DevicesResponse | null>(null)
  const [controlConfig, setControlConfig] = useState<ControlConfig | null>(null)
  const [document, setDocument] = useState<DevicePolicyDocument | null>(null)
  const [policy, setPolicy] = useState<PolicySet>(emptyPolicy)
  const [importedCandidates, setImportedCandidates] = useState<string[]>([])
  const [tailscaleExitAvailable, setTailscaleExitAvailable] = useState(false)
  const [selectedDeviceID, setSelectedDeviceID] = useState('')
  const [ruleLibraryTab, setRuleLibraryTab] = useState<RuleLibraryTab>('rule_sets')
  const ruleLibraryRef = useRef<HTMLElement | null>(null)
  const [registrationOpen, setRegistrationOpen] = useState(false)
  const [registrationSeed, setRegistrationSeed] = useState<{ token: number; draft: RegistrationDraft } | null>(null)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [reloadOpen, setReloadOpen] = useState(false)
  const [reloading, setReloading] = useState(false)
  const [revisionConflict, setRevisionConflict] = useState(false)
  const [rebindRequest, setRebindRequest] = useState<DeviceRebindRequest | null>(null)
  const [rebinding, setRebinding] = useState(false)
  const dirty = Boolean(document && JSON.stringify(policy) !== JSON.stringify(normalizePolicy(document.policy)))
  const dirtyRef = useRef(dirty)
  dirtyRef.current = dirty

  const groups = overview?.policies ?? []
  const globalGroups = useMemo(() => groups.filter(group => !group.name.startsWith('device/')), [groups])
  const candidates = useMemo(() => [...new Set(['DIRECT', 'REJECT', ...globalGroups.map(group => group.name), ...importedCandidates, ...(tailscaleExitAvailable ? [TAILSCALE_POLICY] : [])])], [globalGroups, importedCandidates, tailscaleExitAvailable])
  const routerBypass = {
    gateway: controlConfig?.dhcp.bypass_gateway.trim() ?? '',
    dns: controlConfig?.dhcp.bypass_dns ?? [],
  }
  const routerBypassReady = Boolean(routerBypass.gateway && routerBypass.dns.length)
  const appliedByID = new Map((data?.applied_devices ?? []).map(device => [device.id, device]))
  const routerBypassRenewalNames = policy.devices.filter(device => desiredGatewayTarget(device) === 'upstream_router' && appliedGatewayTarget(appliedByID.get(device.id)) !== 'upstream_router').map(displayDeviceName)
  const openSurgeRenewalNames = policy.devices.filter(device => desiredGatewayTarget(device) === 'opensurge' && appliedGatewayTarget(appliedByID.get(device.id)) === 'upstream_router').map(displayDeviceName)

  const refresh = useCallback(async (discardDraft = false) => {
    try {
      const [devices, config, sources, tailscale] = await Promise.all([api.devices(), api.config(), api.sources().catch(() => ({ revision: '', sources: [] })), api.tailscale().catch(() => null)])
      const nextDocument = config.device_policy.enabled ? await api.devicePolicy() : null
      const imported = sources.sources.filter(source => source.applied && source.valid).flatMap(source => [...source.inventory.proxies, ...source.inventory.proxy_groups])
      setData(devices)
      setControlConfig(config)
      setImportedCandidates(imported)
      setTailscaleExitAvailable(Boolean(tailscale?.selectable_exit))
      setDocument(nextDocument)
      if (nextDocument && (!dirtyRef.current || discardDraft)) {
        const nextPolicy = copyPolicy(nextDocument.policy)
        setPolicy(nextPolicy)
        setSelectedDeviceID(current => nextPolicy.devices.some(device => device.id === current) ? current : nextPolicy.devices[0]?.id ?? '')
        setRegistrationOpen(current => current || nextPolicy.devices.length === 0)
        setRevisionConflict(false)
      }
      if (!nextDocument && (!dirtyRef.current || discardDraft)) setPolicy(emptyPolicy())
      setError('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    }
  }, [])

  const refreshDeviceObservation = useCallback(async () => {
    try {
      setData(await api.devices())
      setError('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    }
  }, [])

  useEffect(() => { void refresh() }, [refresh])
  useEffect(() => { onDirtyChange(dirty) }, [dirty, onDirtyChange])
  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => {
      if (!dirty) return
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [dirty])

  const save = async () => {
    if (!document || !dirty) return
    setSaving(true); setMessage(''); setError(''); setRevisionConflict(false)
    try {
      const updated = await api.saveDevicePolicy(policy, document.revision)
      setDocument(updated)
      setPolicy(copyPolicy(updated.policy))
      await refresh()
      await onChanged()
    } catch (cause) {
      if (cause instanceof RequestError && cause.code === 'revision_conflict') {
        setRevisionConflict(true)
        setError('配置已被其他操作更新。你的本地修改仍保留；如需继续，请先放弃本地修改并加载最新版本。')
      } else {
        setError(cause instanceof Error ? cause.message : String(cause))
      }
    } finally { setSaving(false) }
  }

  const reload = async () => {
    setReloading(true); setError(''); setMessage('')
    try {
      const operation = await api.gateway('reload')
      await waitForOperation(operation.id)
      setReloadOpen(false)
      await refresh(true)
      await onChanged()
      const result = '网关已使用最新设备配置重新启动。改变固定 IPv4 的设备可能需要重新连接以获取新租约。'
      setMessage(result)
      onNotify({ tone: 'success', title: '应用并重载网关成功', message: result })
    } catch (cause) {
      const failure = cause instanceof Error ? cause.message : String(cause)
      setReloadOpen(false)
      await refresh()
      await onChanged()
      setError(failure)
      onNotify({ tone: 'error', title: '应用并重载网关失败', message: failure })
    } finally { setReloading(false) }
  }

  const applyObservedIPv4 = async () => {
    if (!document || !rebindRequest) return
    const request = rebindRequest
    const owner = policy.devices.find(device => device.id !== request.deviceID && device.ipv4 === request.toIPv4)
    if (owner) {
      setRebindRequest(null)
      setError(`当前 IPv4 ${request.toIPv4} 已登记给 ${displayDeviceName(owner)}，请先解决设备身份冲突。`)
      return
    }
    const target = policy.devices.find(device => device.id === request.deviceID)
    if (!target) {
      setRebindRequest(null)
      setError('设备已不在当前配置中，请刷新后重试。')
      return
    }

    const next = copyPolicy(policy)
    next.devices = next.devices.map(device => device.id === request.deviceID ? { ...device, ipv4: request.toIPv4 } : device)
    const running = overview?.status.gateway === 'running'
    let saved = false
    let reloadCompleted = !running
    dirtyRef.current = true
    setPolicy(next)
    setRebinding(true); setMessage(''); setError(''); setRevisionConflict(false)
    try {
      const updated = await api.saveDevicePolicy(next, document.revision)
      saved = true
      dirtyRef.current = false
      setDocument(updated)
      setPolicy(copyPolicy(updated.policy))
      if (running) {
        const operation = await api.gateway('reload')
        await waitForOperation(operation.id)
        reloadCompleted = true
      }
      setRebindRequest(null)
      await refresh(true)
      try { await onChanged() } catch { /* Status refresh is best-effort after the completed operation. */ }
      setMessage(running
        ? `${request.name} 已改用 ${request.toIPv4}，网关已安全重载；设备 ID、Profile、规则和出口选择均已保留。`
        : `${request.name} 已改用 ${request.toIPv4}；设备 ID、Profile、规则和出口选择均已保留，下次启动网关时生效。`)
    } catch (cause) {
      const failure = cause instanceof Error ? cause.message : String(cause)
      setRebindRequest(null)
      if (saved) {
        dirtyRef.current = false
        await refresh(true)
      } else {
        await refresh()
      }
      try { await onChanged() } catch { /* Keep the original operation failure visible. */ }
      if (!saved && cause instanceof RequestError && cause.code === 'revision_conflict') {
        setRevisionConflict(true)
        setError('配置已被其他操作更新。你的本地修改仍保留；如需继续，请先放弃本地修改并加载最新版本。')
      } else {
        setError(saved
          ? reloadCompleted
            ? `设备 IP 已更新${running ? '且网关已重载' : ''}，但页面状态刷新失败：${failure}`
            : `设备 IP 已保存，但网关重载失败：${failure}`
          : failure)
      }
    } finally { setRebinding(false) }
  }

  // Re-registering an existing device reuses the registration form so identity
  // and routing stay a single editing surface. Device rules keep their own
  // panel below the device stack.
  const editDeviceIdentity = (deviceID: string) => {
    const target = policy.devices.find(item => item.id === deviceID)
    if (!target) return
    setRegistrationSeed(current => ({
      token: (current?.token ?? 0) + 1,
      draft: { id: target.id, name: displayDeviceName(target), mac: target.mac, ipv4: target.ipv4, profile: target.profile, gateway_target: desiredGatewayTarget(target), egress_mode: target.egress_mode ?? '' },
    }))
    setRegistrationOpen(true)
    setSelectedDeviceID(deviceID)
    setMessage(''); setError('')
  }

  const removeDevice = (deviceID: string) => {
    const target = policy.devices.find(item => item.id === deviceID)
    if (!target) return
    const name = displayDeviceName(target)
    if (!window.confirm(`删除设备“${name}”吗？它的设备规则和出口选择会一并移除；保存并重载后生效。`)) return
    const next = copyPolicy(policy)
    next.devices = next.devices.filter(item => item.id !== deviceID)
    next.profiles = next.profiles.filter(profile => !ownedProfile(profile.id, deviceID) || next.devices.some(item => item.profile === profile.id))
    setPolicy(next)
    if (selectedDeviceID === deviceID) setSelectedDeviceID(next.devices[0]?.id ?? '')
    if (registrationSeed?.draft.id === deviceID) setRegistrationSeed(null)
    setError('')
    setMessage(`${name} 已从本地草稿移除；保存并重载后不再生效。`)
  }

  const discardDraft = async () => {
    if (!window.confirm('放弃当前尚未保存的设备修改并加载最新版本吗？')) return
    dirtyRef.current = false
    await refresh(true)
  }

  const editDeviceRouting = (deviceID: string) => {
    setSelectedDeviceID(deviceID)
    setRuleLibraryTab('device_routes')
    window.requestAnimationFrame(() => {
      ruleLibraryRef.current?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
      ruleLibraryRef.current?.focus({ preventScroll: true })
    })
  }

  return <>
    <PageHeader eyebrow="DEVICES" title="设备与规则" description="分别设置当前 Mac 和下游设备如何选择出口；两者互不影响。" />
    {message && <div className="notice ok-notice" role="status">{message}</div>}
    {error && <div className="notice warn" role="alert">{error}{revisionConflict && <button className="inline-action" type="button" onClick={() => void discardDraft()}>放弃本地修改并加载最新版本</button>}</div>}

    <section className="section live-section local-routing-section">
      <SectionTitle title="当前 Mac 的设备设置" subtitle="即时生效 · 与下游设备路由方式相互独立" />
      <LocalRoutingCard running={overview?.status.gateway === 'running'} interfaceName={overview?.status.interface} lanIP={overview?.status.lan_ip} healthByName={proxyHealth.byName} testing={proxyHealth.testing} onHealthTest={proxyHealth.test} onChanged={async () => { await onChanged(); await proxyHealth.refresh() }} onPolicies={() => onNavigate('policies')} />
    </section>

    {document ? <>
      <RegistrationPanel key={registrationSeed ? `edit-${registrationSeed.token}` : 'new'} initialDraft={registrationSeed?.draft} open={registrationOpen} onToggle={() => { setRegistrationOpen(value => !value); setRegistrationSeed(null) }} onRefresh={refreshDeviceObservation} topology={overview?.topology} routerBypass={routerBypass} routerBypassReady={routerBypassReady} onNetworkSettings={() => onNavigate('network')} leases={overview?.leases?.length ? overview.leases : data?.leases ?? []} observed={data?.observed_devices ?? []} observationError={data?.observation_error} policy={policy} candidates={candidates} onPolicyChange={setPolicy} onRegistered={id => { setSelectedDeviceID(id); setRegistrationOpen(false); setRegistrationSeed(null); setMessage('设备已加入本地草稿；保存后才会写入 desired 配置。') }} />

      <section className="section live-section device-outlet-section">
        <SectionTitle title="设备出口" subtitle="出口选择即时生效 · 路由方式保存后重载" />
        <div className="device-stack">
            {deviceViews(policy.devices, data?.applied_devices ?? (data?.applied ? data.devices : []), new Set(data?.changed_devices ?? []), new Set(data?.out_of_lan_devices ?? []), overview?.topology).map(view => <DeviceCard key={`${view.desired?.id ?? view.applied?.id}-${view.state}`} view={view} running={overview?.status.gateway === 'running'} topology={overview?.topology} lanPrefix={data?.lan_prefix ?? ''} routerBypass={routerBypass} routerBypassReady={routerBypassReady} onNetworkSettings={() => onNavigate('network')} leases={data?.leases ?? []} observed={data?.observed_devices ?? []} desiredDevices={policy.devices} groups={groups} healthByName={proxyHealth.byName} healthTesting={proxyHealth.testing} onHealthTest={proxyHealth.test} selected={selectedDeviceID === (view.desired?.id ?? view.applied?.id)} onSelect={() => view.desired && setSelectedDeviceID(view.desired.id)} onEditRouting={() => view.desired && editDeviceRouting(view.desired.id)} onEditIdentity={() => view.desired && editDeviceIdentity(view.desired.id)} onRemove={() => view.desired && removeDevice(view.desired.id)} onUseObservedIPv4={(deviceID, name, fromIPv4, toIPv4) => setRebindRequest({ deviceID, name, fromIPv4, toIPv4 })} onRouteModeChange={mode => {
              if (!view.desired) return
              const next = copyPolicy(policy)
              next.devices = next.devices.map(device => {
                if (device.id !== view.desired!.id) return device
                if (mode === 'upstream_router') return { ...device, gateway_target: 'upstream_router', egress_mode: device.egress_mode ?? 'inherit_global' }
                const { gateway_target: _gatewayTarget, ...rest } = device
                return { ...rest, egress_mode: mode }
              })
              setPolicy(next)
            }} onChanged={async () => { await onChanged(); await refresh(); await proxyHealth.refresh() }} />)}
        </div>
        {!policy.devices.length && !data?.devices.length && <Empty text={overview?.topology === 'same_lan' ? '尚未登记设备。使用上方“登记新设备”可从当前经过 Mac 的设备开始。' : '尚未登记设备。使用上方“登记新设备”可直接从当前 DHCP 租约开始。'} />}
      </section>

      <RuleLibrary libraryRef={ruleLibraryRef} activeTab={ruleLibraryTab} onTabChange={setRuleLibraryTab} selectedDeviceID={selectedDeviceID} onSelectedDeviceChange={setSelectedDeviceID} policy={policy} candidates={candidates} onPolicyChange={setPolicy} />
      {data?.drift && !dirty
        ? <PendingReloadBar data={data} running={overview?.status.gateway === 'running'} onReload={() => setReloadOpen(true)} onDashboard={() => onNavigate('dashboard')} />
        : <div className={`sticky-save ${dirty ? 'has-changes' : 'is-saved'}`}><div><strong>{dirty ? '有未保存的设备修改' : '设备配置已保存'}</strong><small>{dirty ? '保存只更新 desired；运行中还需重载' : `revision ${document.revision.slice(0, 10)}`}</small></div><button className="primary" type="button" disabled={!dirty || saving || rebinding} onClick={() => void save()}>{saving ? '正在验证并保存…' : '保存设备配置'}</button></div>}
    </> : <section className="section"><Empty text="当前 gateway config 尚未启用设备策略；请先在网络设置中启用。" /></section>}

    {reloadOpen && <ReloadDialog busy={reloading} routerBypassRenewalNames={routerBypassRenewalNames} openSurgeRenewalNames={openSurgeRenewalNames} onCancel={() => setReloadOpen(false)} onConfirm={() => void reload()} />}
    {rebindRequest && <RebindDialog request={rebindRequest} busy={rebinding} running={overview?.status.gateway === 'running'} includesDraft={dirty} onCancel={() => setRebindRequest(null)} onConfirm={() => void applyObservedIPv4()} />}
  </>
}

function PendingReloadBar({ data, running, onReload, onDashboard }: { data: DevicesResponse; running: boolean; onReload: () => void; onDashboard: () => void }) {
  return <div className="sticky-save needs-reload" role="status"><div><span className="effect-badge restart">需重载</span><strong>{running ? '设备配置已保存，但尚未应用' : '设备配置将在下次启动时应用'}</strong><small>desired {data.desired_digest?.slice(0, 8)} · applied {data.applied_digest?.slice(0, 8) || '尚无'}</small></div>{running ? <button className="primary" type="button" onClick={onReload}>应用并重载网关</button> : <button type="button" onClick={onDashboard}>前往总览启动</button>}</div>
}

function ReloadDialog({ busy, routerBypassRenewalNames, openSurgeRenewalNames, onCancel, onConfirm }: { busy: boolean; routerBypassRenewalNames: string[]; openSurgeRenewalNames: string[]; onCancel: () => void; onConfirm: () => void }) {
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === 'Escape' && !busy) onCancel() }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [busy, onCancel])
  return <dialog className="reload-dialog" open aria-modal="true" aria-labelledby="reload-title"><h2 id="reload-title">应用设备配置并重载网关？</h2><p>OpenSurge 会先验证完整候选配置。验证通过后，DHCP/DNS、mihomo、PF 与 IPv4 forwarding 会短暂重启。</p><ul><li>当前连接会中断并重新建立。</li><li>改变固定 IPv4 的设备可能需要重新连接网络。</li>{routerBypassRenewalNames.length > 0 && <li>应用后，请重新连接 {routerBypassRenewalNames.join('、')} 的网络，使新的主路由网关和 DNS 生效。</li>}{openSurgeRenewalNames.length > 0 && <li>应用后，请重新连接 {openSurgeRenewalNames.join('、')} 的网络，使新的 OpenSurge 网关和 DNS 生效。</li>}<li>若候选配置验证失败，现有网关不会被停止。</li></ul><div className="dialog-actions"><button type="button" disabled={busy} onClick={onCancel}>取消</button><button className="primary" type="button" autoFocus disabled={busy} onClick={onConfirm}>{busy ? '正在验证并重载…' : '确认应用并重载'}</button></div></dialog>
}

function RebindDialog({ request, busy, running, includesDraft, onCancel, onConfirm }: { request: DeviceRebindRequest; busy: boolean; running: boolean; includesDraft: boolean; onCancel: () => void; onConfirm: () => void }) {
  const titleID = `rebind-${request.deviceID}-title`
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === 'Escape' && !busy) onCancel() }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [busy, onCancel])
  return <dialog className="reload-dialog rebind-dialog" open aria-modal="true" aria-labelledby={titleID}>
    <h2 id={titleID}>更新 {request.name} 的设备 IP？</h2>
    <p>已通过相同 MAC 识别到这台设备，地址将从 <code>{request.fromIPv4}</code> 更新为 <code>{request.toIPv4}</code>。</p>
    <p>设备 ID、Profile、规则和出口选择都会保留。</p>
    {includesDraft && <div className="notice warn" role="status">当前页面还有未保存修改；确认后会与本次 IP 更新一起验证、保存{running ? '并应用' : ''}。</div>}
    <ul><li>不会删除或重新登记设备。</li>{running ? <li>保存成功后会安全重载网关，现有连接会短暂中断。</li> : <li>网关当前未运行，配置会在下次启动时应用。</li>}</ul>
    <div className="dialog-actions"><button type="button" disabled={busy} onClick={onCancel}>取消</button><button className="primary" type="button" autoFocus disabled={busy} onClick={onConfirm}>{busy ? (running ? '正在更新并重载…' : '正在更新…') : (running ? '更新并重载网关' : '更新设备 IP')}</button></div>
  </dialog>
}

type DeviceIdentity = { state: 'ready' | 'observed' | 'conflict' | 'address_changed' | 'waiting'; tone: string; text: string; observedIPv4?: string }
type DeviceView = { desired?: PolicyDevice; applied?: CompiledDevice; state: 'applied' | 'pending' | 'updated' | 'removing' | 'paused' | 'out_of_lan' }
function deviceViews(desired: PolicyDevice[], applied: CompiledDevice[], changed: Set<string>, outOfLAN: Set<string>, topology?: string): DeviceView[] {
  const appliedByID = new Map(applied.map(device => [device.id, device]))
  const views: DeviceView[] = desired.map(device => {
    const running = appliedByID.get(device.id)
    appliedByID.delete(device.id)
    // Nothing else about the device matters while its address is off-LAN: the
    // gateway cannot lease it, route it, or match its traffic.
    if (outOfLAN.has(device.id)) return { desired: device, applied: running, state: 'out_of_lan' }
    if (topology !== 'same_lan' && !device.mac.trim()) return { desired: device, state: 'paused' }
    if (!running) return { desired: device, state: 'pending' }
    const same = running.mac.toLowerCase() === device.mac.toLowerCase() && running.ipv4 === device.ipv4 && running.profile === device.profile && appliedEgressMode(running) === desiredEgressMode(device) && appliedGatewayTarget(running) === desiredGatewayTarget(device)
    return { desired: device, applied: running, state: same && !changed.has(device.id) ? 'applied' : 'updated' }
  })
  for (const device of appliedByID.values()) views.push({ applied: device, state: 'removing' })
  return views
}

type DeviceRouteMode = AppliedDeviceEgressMode | 'upstream_router'
type EditableDeviceRouteMode = DeviceEgressMode | 'upstream_router'
type RouterBypassSettings = { gateway: string; dns: string[] }

function DeviceCard({ view, running, topology, lanPrefix, routerBypass, routerBypassReady, onNetworkSettings, leases, observed, desiredDevices, groups, healthByName, healthTesting, onHealthTest, selected, onSelect, onEditRouting, onEditIdentity, onRemove, onUseObservedIPv4, onRouteModeChange, onChanged }: { view: DeviceView; running: boolean; topology?: string; lanPrefix: string; routerBypass: RouterBypassSettings; routerBypassReady: boolean; onNetworkSettings: () => void; leases: Lease[]; observed: ObservedDevice[]; desiredDevices: PolicyDevice[]; groups: ProxyGroup[]; healthByName: Map<string, ProxyHealthEntry>; healthTesting: Set<string>; onHealthTest: (names: string[]) => Promise<void>; selected: boolean; onSelect: () => void; onEditRouting: () => void; onEditIdentity: () => void; onRemove: () => void; onUseObservedIPv4: (deviceID: string, name: string, fromIPv4: string, toIPv4: string) => void; onRouteModeChange: (mode: EditableDeviceRouteMode) => void; onChanged: () => Promise<void> }) {
  const [rulesOpen, setRulesOpen] = useState(false)
  const device = view.desired ?? view.applied!
  const applied = view.applied
  const desiredMode = view.desired ? desiredEgressMode(view.desired) : undefined
  const runningMode = applied ? appliedEgressMode(applied) : undefined
  const desiredTarget = view.desired ? desiredGatewayTarget(view.desired) : undefined
  const runningTarget = appliedGatewayTarget(applied)
  const desiredRouteMode: DeviceRouteMode | undefined = desiredTarget === 'upstream_router' ? 'upstream_router' : desiredMode
  const runningRouteMode: DeviceRouteMode | undefined = runningTarget === 'upstream_router' ? 'upstream_router' : runningMode
  const identity = applied ? deviceIdentity(applied, topology, leases, observed) : null
  const entries = Object.entries(applied?.groups ?? {})
  const defaultEntry = entries.find(([slot]) => slot === 'default')
  const ruleEntries = entries.filter(([slot]) => slot !== 'default')
  const observedIPv4 = identity?.state === 'address_changed' ? identity.observedIPv4 : undefined
  const rebindOwner = observedIPv4 ? desiredDevices.find(item => item.id !== device.id && item.ipv4 === observedIPv4) : undefined
  const rebindAlreadyDrafted = Boolean(observedIPv4 && view.desired?.ipv4 === observedIPv4)
  const identityBlocked = identity?.state === 'address_changed' || identity?.state === 'conflict'
  const refreshReady = identity?.state === 'ready' || identity?.state === 'observed' || (topology === 'same_lan' && Boolean(applied?.mac.trim()) && identity?.state === 'waiting')
  return <article className={`device-card ${selected ? 'selected' : ''}`}>
    <div className="source-head"><button className="device-title" type="button" disabled={!view.desired} aria-pressed={selected} onClick={onSelect}><small>{device.profile}</small><strong>{view.desired ? displayDeviceName(view.desired) : device.id}</strong></button><span className={`pill ${view.state === 'applied' ? 'ok' : ''}`}>{deviceStateLabel(view.state)}</span></div>
    <div className="device-metadata">
      {view.desired?.name && <span className="device-meta-item"><small>设备 ID</small><span className="device-meta-value">{device.id}</span></span>}
      <span className="device-meta-item"><small>IPv4</small><span className="device-meta-value">{device.ipv4}</span></span>
      {identity?.state === 'waiting' && <span className="device-meta-item"><small>状态</small><span className="device-meta-value">设备按登记 IP 接入后生效</span></span>}
      <span className="device-meta-item"><small>MAC</small><span className="device-meta-value">{device.mac.trim() || (view.state === 'paused' ? '未登记 · 策略已暂停，补充后恢复' : '未登记 · 当前按固定 IPv4 匹配')}</span></span>
    </div>
    {view.state === 'out_of_lan' && <div className="device-out-of-lan" role="status">
      <strong>不在当前网段</strong>
      <small>{`登记地址 ${device.ipv4} 不属于当前网关网段${lanPrefix ? ` ${lanPrefix}` : ''}，OpenSurge 不会为它下发 DHCP 保留，也不会匹配它的流量。它保留在配置里不会阻止网关启动：用下面的“编辑身份与路由”换成当前网段的地址，或删除这台设备。`}</small>
    </div>}
    {identity?.state === 'address_changed' ? <div className="identity-rebind"><span className="identity-state changed"><strong>设备已识别，但 IP 已变化</strong><small>原地址 {applied!.ipv4} → 当前地址 {identity.observedIPv4}</small></span>{view.desired && !rebindOwner && !rebindAlreadyDrafted && <button className="primary" type="button" onClick={() => onUseObservedIPv4(view.desired!.id, displayDeviceName(view.desired!), applied!.ipv4, identity.observedIPv4!)}>使用当前 IP 并应用</button>}{rebindAlreadyDrafted && <small className="identity-rebind-note">当前 IP 已写入草稿；保存并重载后生效。</small>}{rebindOwner && <small className="identity-rebind-note conflict">当前地址已登记给 {displayDeviceName(rebindOwner)}，请先解决身份冲突。</small>}</div> : identity && <span className={`identity-state ${identity.tone}`}>{identity.text}</span>}
    {view.desired && <fieldset className={`device-routing-mode ${identityBlocked ? 'identity-blocked' : ''}`} disabled={identityBlocked}>
      <legend>设备路由方式 <span className="effect-badge restart">保存后重载</span></legend>
      {identityBlocked && <small className="identity-routing-blocked">{identity?.state === 'address_changed' ? '当前 IP 尚未绑定；请先使用上方按钮更新设备 IP。' : '当前登记 IP 存在身份冲突；解决冲突后才能切换。'}</small>}
      <div className="route-options"><label className={desiredRouteMode === 'inherit_global' ? 'active' : ''}><input type="radio" name={`route-${device.id}`} checked={desiredRouteMode === 'inherit_global'} onChange={() => onRouteModeChange('inherit_global')} /><span><strong>跟随网关规则</strong><small>继续使用订阅或托管的网关规则；不跟随 Mac 本机的规则 / 全局 / 直连开关。</small></span></label><label className={desiredRouteMode === 'dedicated' ? 'active' : ''}><input type="radio" name={`route-${device.id}`} checked={desiredRouteMode === 'dedicated'} onChange={() => onRouteModeChange('dedicated')} /><span><strong>独立设备出口</strong><small>公网流量优先使用设备出口；局域网和私网地址保持直连。还可以点击“编辑设备分流”，为域名、IP以及规则模版设定单独出口。</small></span></label>{topology === 'same_wifi_dhcp' && <label className={`${desiredRouteMode === 'upstream_router' ? 'active' : ''} ${!routerBypassReady || !device.mac.trim() ? 'unavailable' : ''}`}><input type="radio" name={`route-${device.id}`} disabled={!routerBypassReady || !device.mac.trim()} checked={desiredRouteMode === 'upstream_router'} onChange={() => onRouteModeChange('upstream_router')} /><span><strong>IPv4 直连主路由</strong><small>{routerBypassReady ? `仍由 OpenSurge 分配 IPv4；网关 ${routerBypass.gateway} · DNS ${routerBypass.dns.join(', ')}。启用下游 IPv6 时，该设备的 IPv6 出站会被阻止；设备仍可能保留 SLAAC 地址或 RDNSS。` : '请先在网络设置中确认主路由网关与 DNS。'}</small></span></label>}</div>
      {topology === 'same_wifi_dhcp' && !routerBypassReady && <button className="text-link router-bypass-settings-link" type="button" onClick={onNetworkSettings}>前往网络设置填写主路由信息</button>}
    </fieldset>}
    {desiredMode === 'legacy_fallback' && <div className="legacy-mode-warning" role="status"><strong>需要选择新的路由方式</strong><small>当前配置使用旧版兼容行为：先匹配全局规则，设备出口仅作兜底。</small></div>}
    {runningTarget === 'upstream_router' && <div className="runtime-route router-bypass"><span><strong>IPv4 直连主路由{applied?.ipv6_blocked ? ' · IPv6 出站已阻止' : ''}</strong><small>已配置网关 {routerBypass.gateway || '—'} · DNS {routerBypass.dns.join(', ') || '—'}；IPv4 在设备续租后生效，OpenSurge 不统计其 IPv4 流量。</small></span><span className="effect-badge restart">续租后生效</span></div>}
    {runningTarget !== 'upstream_router' && runningMode === 'inherit_global' && (identityBlocked ? <div className="runtime-route identity-blocked"><span><strong>{identity?.state === 'address_changed' ? '当前 IP 尚未绑定' : '当前身份存在冲突'}</strong><small>已应用配置仍对应 {applied!.ipv4}</small></span><span className="effect-badge restart">待修复</span></div> : <div className="runtime-route following"><span><strong>当前运行</strong><small>{identity?.state === 'waiting' ? '跟随网关规则 · 等待设备接入' : '跟随网关规则'}</small></span><span className="effect-badge live">{identity?.state === 'waiting' ? '已预设' : '已应用'}</span></div>)}
    {runningTarget !== 'upstream_router' && (runningMode === 'dedicated' || runningMode === 'legacy_fallback') && <div className={`default-slot ${runningMode === 'legacy_fallback' ? 'legacy' : ''}`}>{defaultEntry ? <DeviceOutletControl identity={identity} device={applied!.id} slot={defaultEntry[0]} groupName={defaultEntry[1]} groups={groups} title={runningMode === 'dedicated' ? '独立出口' : '兼容兜底出口'} ariaLabel={`${device.id} ${runningMode === 'dedicated' ? '独立出口' : '兼容兜底出口'} 当前摘要`} healthByName={healthByName} testing={healthTesting} onTest={onHealthTest} onChanged={onChanged} /> : <button className="outlet-summary unavailable" type="button" disabled><span className="outlet-summary-copy"><small>{runningMode === 'dedicated' ? '独立出口' : '兼容兜底出口'}</small><strong>重载后可用</strong></span></button>}</div>}
    {!runningRouteMode && desiredRouteMode && view.state !== 'paused' && view.state !== 'out_of_lan' && <div className="runtime-route"><span><strong>重载后应用</strong><small>{routeModeLabel(desiredRouteMode)}</small></span></div>}
    {runningRouteMode && desiredRouteMode && runningRouteMode !== desiredRouteMode && <small className="draft-mode-delta">草稿将改为“{routeModeLabel(desiredRouteMode)}”；保存并重载前仍按“{routeModeLabel(runningRouteMode)}”运行。</small>}
    {ruleEntries.length > 0 && <div className="rule-slots"><button className="rule-slots-toggle" type="button" aria-expanded={rulesOpen} onClick={() => setRulesOpen(value => !value)}>规则出口（{ruleEntries.length}）<span>{rulesOpen ? '收起' : '展开'}</span></button>{rulesOpen && ruleEntries.map(([slot, groupName]) => <div className="rule-outlet-summary" key={slot}><DeviceOutletControl identity={identity} device={applied!.id} slot={slot} groupName={groupName} groups={groups} title={slot} ariaLabel={`${device.id} ${slot} 出口当前摘要`} healthByName={healthByName} testing={healthTesting} onTest={onHealthTest} onChanged={onChanged} /></div>)}</div>}
    {applied && <ConnectionRefreshControl ariaLabel={`刷新 ${view.desired ? displayDeviceName(view.desired) : applied.id} 连接`} disabled={!running || runningTarget === 'upstream_router' || !refreshReady} disabledReason={!running ? '启动网关后可以刷新此设备的连接。' : runningTarget === 'upstream_router' ? '此设备直连主路由，没有由 OpenSurge 管理的连接。' : '确认设备当前身份后可以刷新连接。'} refresh={() => api.refreshDeviceConnections(applied.id)} onRefreshed={onChanged} />}
    {view.desired && <div className="device-card-actions">
      <button className="edit-device" type="button" onClick={onEditRouting}>{selected ? '正在编辑设备分流' : '编辑设备分流'}</button>
      <span className="device-card-manage">
        <button className="text-link" type="button" onClick={onEditIdentity}>编辑身份与路由</button>
        <button className="danger-link" type="button" onClick={onRemove}>删除设备</button>
      </span>
    </div>}
  </article>
}

function DeviceOutletControl({ identity, device, slot, groupName, groups, title, ariaLabel, healthByName, testing, onTest, onChanged }: { identity: DeviceIdentity | null; device: string; slot: string; groupName: string; groups: ProxyGroup[]; title: string; ariaLabel: string; healthByName: Map<string, ProxyHealthEntry>; testing: Set<string>; onTest: (names: string[]) => Promise<void>; onChanged: () => Promise<void> }) {
  const blocked = identity?.state === 'address_changed' || identity?.state === 'conflict'
  if (blocked) return <button className="outlet-summary unavailable" type="button" aria-label={ariaLabel} disabled><span className="outlet-summary-copy"><small>{title}</small><strong>{identity.state === 'address_changed' ? '先更新 IP 绑定' : '先解决身份冲突'}</strong></span></button>
  return <DeviceOutletSummary device={device} slot={slot} groupName={groupName} groups={groups} title={`${title} · ${identity?.state === 'waiting' ? '预设' : '即时切换'}`} ariaLabel={ariaLabel} healthByName={healthByName} testing={testing} onTest={onTest} onChanged={onChanged} />
}

function desiredEgressMode(device: PolicyDevice): AppliedDeviceEgressMode {
  return device.egress_mode ?? 'legacy_fallback'
}

function appliedEgressMode(device: CompiledDevice): AppliedDeviceEgressMode {
  return device.egress_mode || 'legacy_fallback'
}

function desiredGatewayTarget(device: PolicyDevice): DeviceGatewayTarget {
  return device.gateway_target || 'opensurge'
}

function appliedGatewayTarget(device?: CompiledDevice): DeviceGatewayTarget | undefined {
  if (!device) return undefined
  return device.gateway_target || 'opensurge'
}

function egressModeLabel(mode: AppliedDeviceEgressMode) {
  if (mode === 'inherit_global') return '跟随网关规则'
  if (mode === 'dedicated') return '独立设备出口'
  return '旧版兼容兜底'
}

function routeModeLabel(mode: DeviceRouteMode) {
  if (mode === 'upstream_router') return '直连主路由'
  return egressModeLabel(mode)
}

function deviceStateLabel(state: DeviceView['state']) {
  if (state === 'out_of_lan') return '不在当前网段'
  if (state === 'paused') return '等待 MAC'
  if (state === 'pending') return '待应用'
  if (state === 'updated') return '待更新'
  if (state === 'removing') return '待移除'
  return '已应用'
}

function deviceIdentity(applied: CompiledDevice, topology: string | undefined, leases: Lease[], observed: ObservedDevice[]): DeviceIdentity {
  if (topology === 'same_lan') {
    const current = observed.find(item => item.ip === applied.ipv4)
    if (current) {
      if (current.active_connections === 0) return { state: 'observed', tone: 'observed', text: current.mac ? `已观察到邻居 MAC ${current.mac}，等待该 IPv4 经过 Mac` : '固定 IPv4 已登记，等待流量经过 Mac' }
      if (!applied.mac.trim()) return current.mac
        ? { state: 'observed', tone: 'observed', text: `固定 IPv4 已生效 · 当前观察到 ${current.mac}` }
        : { state: 'observed', tone: 'observed', text: '当前有流量：按固定 IPv4 匹配，MAC 为可选身份信息' }
      if (!current.mac) return { state: 'observed', tone: 'observed', text: '当前有流量：固定 IPv4 已观察，MAC 尚未验证' }
      if (current.mac.toLowerCase() !== applied.mac.toLowerCase()) {
        return { state: 'conflict', tone: 'conflict', text: `身份冲突：邻居 MAC ${current.mac} 与登记不一致` }
      }
      return { state: 'ready', tone: 'ready', text: '流量与邻居已观察：MAC / IPv4 匹配' }
    }
    const sameMAC = applied.mac.trim() ? observed.filter(item => item.ip !== applied.ipv4 && item.active_connections > 0 && item.neighbor_observed && item.mac?.toLowerCase() === applied.mac.toLowerCase()) : []
    if (sameMAC.length === 1) return { state: 'address_changed', tone: 'changed', text: '设备已识别，但 IP 已变化', observedIPv4: sameMAC[0].ip }
    return { state: 'waiting', tone: '', text: applied.mac.trim() ? '静态配置身份：等待该 IPv4 经过 Mac' : '固定 IPv4 策略：等待该地址经过 Mac' }
  }
  const lease = leases.find(item => item.mac.toLowerCase() === applied.mac.toLowerCase() && item.ip === applied.ipv4 && item.online && (!item.expires_at || Date.parse(item.expires_at) > Date.now()))
  return lease
    ? { state: 'ready', tone: 'ready', text: 'DHCP 身份已验证' }
    : { state: 'waiting', tone: '', text: '身份待确认：需要在线且未过期的精确 MAC / IPv4 租约' }
}

type RegistrationDraft = { id: string; name: string; mac: string; ipv4: string; profile: string; gateway_target: DeviceGatewayTarget; egress_mode: DeviceEgressMode | '' }
type RegistrationCandidate = { ip: string; mac: string; hostname: string; source: 'dhcp' | 'traffic' | 'neighbor'; activeConnections: number; online: boolean }

function registrationCandidates(topology: string | undefined, leases: Lease[], observed: ObservedDevice[]): RegistrationCandidate[] {
  const byIP = new Map<string, RegistrationCandidate>()
  const leasesByIP = new Map(leases.map(lease => [lease.ip, lease]))
  if (topology !== 'same_lan') {
    for (const lease of leases) {
      byIP.set(lease.ip, { ip: lease.ip, mac: lease.mac, hostname: lease.registered_name || lease.hostname || '', source: 'dhcp', activeConnections: 0, online: lease.online })
    }
  }
  for (const device of observed) {
    const lease = leasesByIP.get(device.ip)
    byIP.set(device.ip, {
      ip: device.ip,
      mac: device.mac || lease?.mac || '',
      hostname: lease?.registered_name || lease?.hostname || '',
      source: device.active_connections > 0 ? 'traffic' : 'neighbor',
      activeConnections: device.active_connections,
      online: device.active_connections > 0,
    })
  }
  return [...byIP.values()].sort((left, right) => right.activeConnections - left.activeConnections || Number(right.online) - Number(left.online) || left.ip.localeCompare(right.ip, undefined, { numeric: true }))
}

function RegistrationPanel({ open, initialDraft, onToggle, onRefresh, topology, routerBypass, routerBypassReady, onNetworkSettings, leases, observed, observationError, policy, candidates, onPolicyChange, onRegistered }: { open: boolean; initialDraft?: RegistrationDraft; onToggle: () => void; onRefresh: () => Promise<void>; topology?: string; routerBypass: RouterBypassSettings; routerBypassReady: boolean; onNetworkSettings: () => void; leases: Lease[]; observed: ObservedDevice[]; observationError?: string; policy: PolicySet; candidates: string[]; onPolicyChange: (policy: PolicySet) => void; onRegistered: (id: string) => void }) {
  const sectionRef = useRef<HTMLElement>(null)
  const [draft, setDraft] = useState<RegistrationDraft>(initialDraft ?? { id: '', name: '', mac: '', ipv4: '', profile: '', gateway_target: 'opensurge', egress_mode: 'inherit_global' })
  const [defaults, setDefaults] = useState(['DIRECT'])
  const [useExisting, setUseExisting] = useState(Boolean(initialDraft?.profile))
  const [error, setError] = useState('')
  // The panel is remounted with a fresh key whenever a device card asks to edit
  // an existing registration, so bringing it into view belongs to mount.
  useEffect(() => {
    if (!initialDraft) return
    sectionRef.current?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
  }, [initialDraft])
  const editing = Boolean(draft.id && policy.devices.some(item => item.id === draft.id))
  const chooseCandidate = (candidate: RegistrationCandidate) => {
    const registered = policy.devices.find(item => (candidate.mac && item.mac.toLowerCase() === candidate.mac.toLowerCase()) || item.ipv4 === candidate.ip)
    setDraft({ id: registered?.id ?? '', name: registered ? displayDeviceName(registered) : candidate.hostname, mac: candidate.mac || registered?.mac || '', ipv4: candidate.ip, profile: registered?.profile ?? policy.profiles[0]?.id ?? '', gateway_target: registered?.gateway_target ?? 'opensurge', egress_mode: registered ? registered.egress_mode ?? '' : 'inherit_global' })
    setUseExisting(Boolean(registered))
  }
  const register = () => {
    const name = draft.name.trim()
    const normalizedMAC = draft.mac.trim().toLowerCase()
    const normalizedIPv4 = draft.ipv4.trim()
    if (!name || !normalizedIPv4 || (topology !== 'same_lan' && !normalizedMAC)) { setError(topology === 'same_lan' ? '请填写设备名称和固定 IPv4；MAC 可以留空。' : '请填写设备名称、MAC 和固定 IPv4。'); return }
    if (!draft.egress_mode) { setError('请选择“跟随网关规则”或“独立设备出口”。'); return }
    if (draft.gateway_target === 'upstream_router' && (!routerBypassReady || topology !== 'same_wifi_dhcp')) { setError('请先在局域网 DHCP 接管的网络设置中确认主路由网关与 DNS。'); return }
    if (useExisting && !draft.profile) { setError('请选择一个现有 Profile。'); return }
    let next = copyPolicy(policy)
    const registered = next.devices.find(item => item.id === draft.id || (normalizedMAC !== '' && item.mac.toLowerCase() === normalizedMAC) || item.ipv4 === normalizedIPv4)
    const deviceID = registered?.id ?? availableDeviceID(name, normalizedMAC || normalizedIPv4, next.devices)
    let profile = draft.profile || registered?.profile || ''
    const egressMode = draft.egress_mode as DeviceEgressMode
    if (!useExisting && !registered) {
      profile = uniqueProfileID(`${deviceID}-policy`, next.profiles)
      next.profiles.push({ id: profile, default_policies: defaults.length ? defaults : ['DIRECT'], on_unsupported: 'reject', rules: [] })
    }
    const gatewayTarget = draft.gateway_target === 'upstream_router' ? { gateway_target: 'upstream_router' as const } : {}
    next.devices = [...next.devices.filter(item => item.id !== deviceID && !(normalizedMAC !== '' && item.mac.toLowerCase() === normalizedMAC)), { id: deviceID, name, mac: normalizedMAC, ipv4: normalizedIPv4, profile, ...gatewayTarget, egress_mode: egressMode }]
    onPolicyChange(next); onRegistered(deviceID)
    setDraft({ id: '', name: '', mac: '', ipv4: '', profile: '', gateway_target: 'opensurge', egress_mode: 'inherit_global' }); setDefaults(['DIRECT']); setUseExisting(false); setError('')
  }
  const visibleCandidates = registrationCandidates(topology, leases, observed)
  const previewID = draft.id || (draft.name.trim() ? availableDeviceID(draft.name.trim(), draft.mac || draft.ipv4, policy.devices) : '')
  return <section className="section device-tools-section registration" ref={sectionRef}><button className="section-toggle" type="button" aria-expanded={open} onClick={onToggle}><span><strong>{editing ? '编辑设备身份与路由' : '登记新设备'}</strong><small>{editing ? '重新确认这台设备的名称、固定身份和路由方式；它的规则与出口选择会保留' : topology === 'same_lan' ? '从当前经过 Mac 的 LAN 流量发现设备，再确认静态身份与路由方式' : '从当前 DHCP 租约开始，确认身份与设备路由方式'}</small></span><span>{open ? '收起' : '展开'}</span></button>{open && <div className="registration-body"><div className="lease-picker"><div className="registration-picker-heading"><SectionTitle title={topology === 'same_lan' ? '当前经过 Mac 的设备' : '当前已接管设备'} subtitle={topology === 'same_lan' ? '来源是 mihomo 活跃连接；邻居表可补充 MAC，但固定 IPv4 可以独立登记' : '点击租约会自动填写 MAC 与当前 IPv4'} />{topology === 'same_lan' && <button className="text-link" type="button" onClick={() => void onRefresh()}>刷新当前设备</button>}</div>{observationError && topology === 'same_lan' && <div className="notice warn">实时设备观察不完整：{observationError}</div>}{visibleCandidates.length ? visibleCandidates.map(candidate => { const registered = policy.devices.find(item => (candidate.mac && item.mac.toLowerCase() === candidate.mac.toLowerCase()) || item.ipv4 === candidate.ip); return <button className="lease-choice" type="button" aria-label={`配置设备 ${candidate.ip}`} key={`${candidate.source}-${candidate.mac || 'unknown'}-${candidate.ip}`} onClick={() => chooseCandidate(candidate)}><span className={candidate.online ? 'pill ok' : 'pill'}>{candidate.source === 'traffic' ? '经过 Mac' : candidate.source === 'neighbor' ? '邻居记录' : candidate.online ? '在线' : '历史租约'}</span><span><strong>{registered ? displayDeviceName(registered) : candidate.hostname || `未登记设备 ${candidate.ip}`}</strong><small>{candidate.mac || 'MAC 尚未从邻居表解析'}{candidate.activeConnections > 0 ? ` · ${candidate.activeConnections} 个活跃连接` : ''}</small></span><code>{candidate.ip}</code><span>配置此设备</span></button> }) : <Empty text={topology === 'same_lan' ? '当前尚未观察到经过 Mac 的 LAN 设备；可以直接按固定 IPv4 手工登记。' : '当前没有 DHCP 租约；也可以手工填写。'} />}</div><div className="registration-form"><div className="utility-card-heading"><span><small>{editing ? 'EDIT DEVICE' : 'NEW DEVICE'}</small><h3>设备身份与路由</h3></span><span className="effect-badge restart">保存后重载</span></div><p className="card-help">确认设备名称、固定身份和路由方式；登记后仍需保存并重载设备配置。</p><label>设备名称<input aria-label="设备名称" value={draft.name} onChange={event => setDraft({ ...draft, name: event.target.value })} /></label><small className="registration-id-hint">{previewID ? <>内部 ID：<code>{previewID}</code>{draft.id ? '（保持不变）' : '（保存时自动生成）'}</> : '设备名称可包含空格；内部 ID 会在保存时自动生成。'}</small><label>{topology === 'same_lan' ? 'MAC 地址（可选身份信息）' : 'MAC 地址'}<input aria-label="设备 MAC" value={draft.mac} onChange={event => setDraft({ ...draft, mac: event.target.value })} /></label>{topology === 'same_lan' && !draft.mac.trim() && draft.ipv4.trim() && <small className="registration-id-hint">将只按固定 IPv4 匹配；请确保主路由不会把该地址分配给其他设备。</small>}<label>固定 IPv4<input aria-label="固定 IPv4" value={draft.ipv4} onChange={event => setDraft({ ...draft, ipv4: event.target.value })} /></label><fieldset className="registration-routing"><legend>设备路由方式</legend><label className={draft.gateway_target === 'opensurge' && draft.egress_mode === 'inherit_global' ? 'active' : ''}><input type="radio" name="registration-route" checked={draft.gateway_target === 'opensurge' && draft.egress_mode === 'inherit_global'} onChange={() => setDraft({ ...draft, gateway_target: 'opensurge', egress_mode: 'inherit_global' })} /><span><strong>跟随网关规则</strong><small>默认推荐；继续使用订阅或托管的网关规则，不跟随 Mac 本机模式。</small></span></label><label className={draft.gateway_target === 'opensurge' && draft.egress_mode === 'dedicated' ? 'active' : ''}><input type="radio" name="registration-route" checked={draft.gateway_target === 'opensurge' && draft.egress_mode === 'dedicated'} onChange={() => setDraft({ ...draft, gateway_target: 'opensurge', egress_mode: 'dedicated' })} /><span><strong>独立设备出口</strong><small>公网流量优先使用专属 selector，局域网和私网仍直连。</small></span></label>{topology === 'same_wifi_dhcp' && <label className={`${draft.gateway_target === 'upstream_router' ? 'active' : ''} ${!routerBypassReady ? 'unavailable' : ''}`}><input type="radio" name="registration-route" disabled={!routerBypassReady} checked={draft.gateway_target === 'upstream_router'} onChange={() => setDraft({ ...draft, gateway_target: 'upstream_router', egress_mode: draft.egress_mode || 'inherit_global' })} /><span><strong>IPv4 直连主路由</strong><small>{routerBypassReady ? `固定 IPv4 仍由 OpenSurge 分配；网关 ${routerBypass.gateway} · DNS ${routerBypass.dns.join(', ')}。启用下游 IPv6 时，IPv6 出站会被阻止。` : '请先在网络设置中确认主路由网关与 DNS。'}</small></span></label>}{topology === 'same_wifi_dhcp' && !routerBypassReady && <button className="text-link router-bypass-settings-link" type="button" onClick={onNetworkSettings}>前往网络设置填写主路由信息</button>}{!draft.egress_mode && <small className="field-error" role="status">这是旧版设备，请选择新的路由方式后再保存。</small>}</fieldset>{!useExisting && draft.gateway_target === 'opensurge' && draft.egress_mode === 'dedicated' && <CandidatePicker label="独立出口候选" values={defaults} candidates={candidates} onChange={setDefaults} />}
    <details className="inline-advanced"><summary>高级：使用已有 Profile</summary><label className="checkbox-field"><input type="checkbox" checked={useExisting} onChange={event => setUseExisting(event.target.checked)} /> 使用已有 Profile</label>{useExisting && <select aria-label="设备 Profile" value={draft.profile} onChange={event => setDraft({ ...draft, profile: event.target.value })}><option value="">选择 Profile</option>{policy.profiles.map(profile => <option key={profile.id}>{profile.id}</option>)}</select>}</details>{error && <small className="field-error" role="alert">{error}</small>}<button className="primary" type="button" onClick={register}>{editing ? '更新设备身份与路由' : topology === 'same_lan' && !draft.mac.trim() ? '按固定 IPv4 登记' : '登记或更新设备'}</button></div></div>}</section>
}


function CandidatePicker({ label, values, candidates, onChange }: { label: string; values: string[]; candidates: string[]; onChange: (values: string[]) => void }) {
  const [candidate, setCandidate] = useState('')
  const listID = useId()
  const available = candidates.filter(item => !values.includes(item))
  const validCandidate = available.includes(candidate)
  const add = () => { if (validCandidate) onChange([...values, candidate]); setCandidate('') }
  return <div className="candidate-picker"><label>{label}<span className="candidate-add"><input type="search" aria-label={label} list={listID} autoComplete="off" placeholder="搜索出口…" value={candidate} onChange={event => setCandidate(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') { event.preventDefault(); add() } }} /><datalist id={listID}>{available.map(item => <option key={item} value={item} label={policyDisplayName(item)} />)}</datalist><button type="button" disabled={!validCandidate} onClick={add}>添加</button></span></label><div className="token-list">{values.map(value => <span className="token" key={value}>{policyDisplayName(value)}<button type="button" disabled={values.length === 1} aria-label={`移除 ${policyDisplayName(value)}`} title={values.length === 1 ? '至少保留一个出口' : undefined} onClick={() => onChange(values.filter(item => item !== value))}>×</button></span>)}</div></div>
}


function RuleLibrary({ libraryRef, activeTab, onTabChange, selectedDeviceID, onSelectedDeviceChange, policy, candidates, onPolicyChange }: { libraryRef: RefObject<HTMLElement | null>; activeTab: RuleLibraryTab; onTabChange: (tab: RuleLibraryTab) => void; selectedDeviceID: string; onSelectedDeviceChange: (id: string) => void; policy: PolicySet; candidates: string[]; onPolicyChange: (policy: PolicySet) => void }) {
  const [presetTemplate, setPresetTemplate] = useState('')
  const useTemplate = (templateID: string) => {
    setPresetTemplate(templateID)
    onTabChange('device_routes')
  }
  const tabs: Array<{ id: RuleLibraryTab; label: string; count: number }> = [
    { id: 'rule_sets', label: '规则集', count: policy.rule_sets.length },
    { id: 'templates', label: '分流模版', count: policy.templates.filter(template => template.rule_sets?.length).length + (policy.templates.some(template => template.id === CLAUDE_CODE_TEMPLATE.id) ? 0 : 1) },
    { id: 'device_routes', label: '设备分流', count: policy.devices.length },
  ]
  return <section ref={libraryRef} tabIndex={-1} className="section device-tools-section rule-library">
    <div className="rule-library-heading"><div><strong>规则库</strong><small>编辑规则集、组合分流模版，再为设备设置命中后的单独出口</small></div><span className="effect-badge restart">保存后重载</span></div>
    <div className="rule-library-tabs" role="tablist" aria-label="规则库">{tabs.map(tab => <button key={tab.id} id={`rule-library-tab-${tab.id}`} type="button" role="tab" aria-selected={activeTab === tab.id} aria-controls={`rule-library-panel-${tab.id}`} onClick={() => onTabChange(tab.id)}>{tab.label}<span>{tab.count}</span></button>)}</div>
    <div id={`rule-library-panel-${activeTab}`} role="tabpanel" aria-labelledby={`rule-library-tab-${activeTab}`} className="rule-library-panel">
      {activeTab === 'rule_sets' && <RuleSetLibrary policy={policy} onPolicyChange={onPolicyChange} />}
      {activeTab === 'templates' && <TemplateLibrary policy={policy} onPolicyChange={onPolicyChange} onUseTemplate={useTemplate} />}
      {activeTab === 'device_routes' && <DeviceRoutesLibrary key={selectedDeviceID} policy={policy} selectedDeviceID={selectedDeviceID} onSelectedDeviceChange={onSelectedDeviceChange} candidates={candidates} presetTemplate={presetTemplate} onPresetConsumed={() => setPresetTemplate('')} onPolicyChange={onPolicyChange} />}
    </div>
  </section>
}

function RuleSetLibrary({ policy, onPolicyChange }: { policy: PolicySet; onPolicyChange: (policy: PolicySet) => void }) {
  const [editingID, setEditingID] = useState<string | 'new' | null>(null)
  const refs = (id: string) => {
    const templates = policy.templates.filter(template => template.rule_sets?.includes(id)).map(template => template.id)
    const rules = policy.profiles.filter(profile => profile.rules?.some(rule => rule.match.rule_sets?.includes(id))).map(profile => profile.id)
    return [...templates, ...rules]
  }
  const save = (ruleSet: PolicyRuleSet) => {
    const exists = policy.rule_sets.some(item => item.id === ruleSet.id)
    onPolicyChange({ ...policy, rule_sets: exists ? policy.rule_sets.map(item => item.id === ruleSet.id ? ruleSet : item) : [...policy.rule_sets, ruleSet] })
    setEditingID(null)
  }
  return <div className="library-pane">
    <div className="library-pane-heading"><div><h3>规则集</h3><p>维护不带出口的域名、IP CIDR 或经典规则列表。</p></div>{editingID === null && <button type="button" onClick={() => setEditingID('new')}>＋ 新建规则集</button>}</div>
    {editingID === 'new' && <RuleSetEditor existing={policy.rule_sets} onCancel={() => setEditingID(null)} onSave={save} />}
    <div className="library-list">{policy.rule_sets.map(ruleSet => { const usedBy = refs(ruleSet.id); return <div className="library-item" key={ruleSet.id}><div><strong>{CLAUDE_CODE_RULE_SET_NAMES[ruleSet.id] ?? ruleSet.id}</strong><small>{ruleSet.type ?? 'inline'} · {ruleSet.behavior} · {ruleSet.payload?.length ?? (ruleSet.url ? '远程' : 0)} 项{usedBy.length ? ` · 被 ${usedBy.join('、')} 使用` : ''}</small></div><div className="library-item-actions"><button type="button" onClick={() => setEditingID(editingID === ruleSet.id ? null : ruleSet.id)}>{editingID === ruleSet.id ? '收起' : '编辑'}</button><button className="danger-link" type="button" disabled={usedBy.length > 0} title={usedBy.length ? `被 ${usedBy.join('、')} 使用` : undefined} onClick={() => onPolicyChange({ ...policy, rule_sets: policy.rule_sets.filter(item => item.id !== ruleSet.id) })}>移除</button></div>{editingID === ruleSet.id && <RuleSetEditor initial={ruleSet} existing={policy.rule_sets} onCancel={() => setEditingID(null)} onSave={save} />}</div> })}</div>
    {!policy.rule_sets.length && editingID !== 'new' && <Empty text="尚未添加规则集；也可以先从“分流模版”查看 Claude Code 内置示例。" />}
  </div>
}

function RuleSetEditor({ initial, existing, onCancel, onSave }: { initial?: PolicyRuleSet; existing: PolicyRuleSet[]; onCancel: () => void; onSave: (ruleSet: PolicyRuleSet) => void }) {
  const [id, setID] = useState(initial?.id ?? '')
  const [type, setType] = useState<'inline' | 'http'>(initial?.type ?? 'inline')
  const [behavior, setBehavior] = useState<PolicyRuleSet['behavior']>(initial?.behavior ?? 'domain')
  const [format, setFormat] = useState(initial?.format ?? 'yaml')
  const [url, setURL] = useState(initial?.url ?? '')
  const [payload, setPayload] = useState((initial?.payload ?? []).join('\n'))
  const [error, setError] = useState('')
  const save = () => {
    const nextID = id.trim()
    const lines = payload.split(/\r?\n/).map(line => line.trim()).filter(Boolean)
    if (!nextID) { setError('请填写规则集名称。'); return }
    if (!initial && existing.some(item => item.id === nextID)) { setError('已经存在同名规则集。'); return }
    if (type === 'inline' && !lines.length) { setError('内联规则集至少需要一条规则。'); return }
    if (type === 'http' && !/^https?:\/\//.test(url.trim())) { setError('请填写有效的 HTTP 或 HTTPS 来源。'); return }
    onSave(type === 'inline'
      ? { id: nextID, type, behavior, payload: lines }
      : { id: nextID, type, behavior, format, url: url.trim(), interval: initial?.interval || 3600 })
  }
  return <div className="library-editor ruleset-editor"><div className="ruleset-primary-row"><label>名称<input aria-label="规则集名称" disabled={Boolean(initial)} placeholder="例如 work-domains" value={id} onChange={event => setID(event.target.value)} /></label><label>来源<select aria-label="规则集来源" value={type} onChange={event => setType(event.target.value as 'inline' | 'http')}><option value="inline">内联列表</option><option value="http">HTTP 来源</option></select></label><label>规则类型<select aria-label="规则集类型" value={behavior} onChange={event => setBehavior(event.target.value as PolicyRuleSet['behavior'])}><option value="domain">域名</option><option value="ipcidr">IP CIDR</option><option value="classical">经典规则</option></select></label></div>{type === 'inline' ? <label>每行一条规则<textarea aria-label="规则集内容" rows={8} placeholder="example.com" value={payload} onChange={event => setPayload(event.target.value)} /></label> : <div className="ruleset-source-row http"><label>来源地址<input aria-label="规则集 URL" placeholder="https://…" value={url} onChange={event => setURL(event.target.value)} /></label><label>格式<select aria-label="规则集格式" value={format} onChange={event => setFormat(event.target.value)}><option>yaml</option><option>text</option><option>mrs</option></select></label></div>}{error && <small className="field-error" role="alert">{error}</small>}<div className="editor-actions"><button type="button" onClick={onCancel}>取消</button><button className="primary" type="button" onClick={save}>保存到草稿</button></div></div>
}

function TemplateLibrary({ policy, onPolicyChange, onUseTemplate }: { policy: PolicySet; onPolicyChange: (policy: PolicySet) => void; onUseTemplate: (id: string) => void }) {
  const [editingID, setEditingID] = useState<string | 'new' | null>(null)
  const [exampleOpen, setExampleOpen] = useState(false)
  const routeRefs = (id: string) => policy.devices.filter(device => resolveProfile(policy, device.profile).rules?.some(rule => rule.match.template === id)).map(displayDeviceName)
  const save = (template: PolicyTemplate) => {
    const exists = policy.templates.some(item => item.id === template.id)
    onPolicyChange({ ...policy, templates: exists ? policy.templates.map(item => item.id === template.id ? template : item) : [...policy.templates, template] })
    setEditingID(null)
  }
  const claudeRefs = routeRefs(CLAUDE_CODE_TEMPLATE.id)
  return <div className="library-pane">
    <div className="library-pane-heading"><div><h3>分流模版</h3><p>把多个规则集组合为可复用的匹配对象；出口由设备分流决定。</p></div>{editingID === null && <button type="button" onClick={() => setEditingID('new')}>＋ 新建分流模版</button>}</div>
    <article className="builtin-template"><div className="builtin-template-heading"><div><span className="pill">内置示例 · {claudeRefs.length ? `已用于 ${claudeRefs.length} 台设备` : '未启用'}</span><h4>Claude Code</h4><p>包含核心域名、扩展服务、IP / ASN 兜底与 NTP 通用规则；不默认应用到任何设备。</p></div><div className="library-item-actions"><button type="button" onClick={() => setExampleOpen(value => !value)}>{exampleOpen ? '收起规则' : '查看规则'}</button><button className="primary" type="button" disabled={!policy.devices.length} title={!policy.devices.length ? '请先登记设备' : undefined} onClick={() => onUseTemplate(CLAUDE_CODE_TEMPLATE.id)}>用于设备</button></div></div><small className="builtin-source">社区示例，不是 Anthropic 官方规则 · 来源：<a href={CLAUDE_CODE_SOURCE.url} target="_blank" rel="noreferrer">{CLAUDE_CODE_SOURCE.label}</a> · 固定快照 {CLAUDE_CODE_SOURCE.snapshot}</small>{exampleOpen && <div className="builtin-rule-grid">{CLAUDE_CODE_RULE_SETS.map(ruleSet => <details key={ruleSet.id}><summary>{CLAUDE_CODE_RULE_SET_NAMES[ruleSet.id]} <span>{ruleSet.payload?.length ?? 0} 项</span></summary><pre>{ruleSet.payload?.join('\n')}</pre></details>)}</div>}</article>
    {editingID === 'new' && <TemplateEditor ruleSets={policy.rule_sets} existing={policy.templates} onCancel={() => setEditingID(null)} onSave={save} />}
    <div className="library-list">{policy.templates.filter(template => template.rule_sets?.length && template.id !== CLAUDE_CODE_TEMPLATE.id).map(template => { const usedBy = routeRefs(template.id); return <div className="library-item" key={template.id}><div><strong>{template.id}</strong><small>{template.rule_sets?.length ?? 0} 个规则集{usedBy.length ? ` · 设备：${usedBy.join('、')}` : ''}</small></div><div className="library-item-actions"><button type="button" onClick={() => setEditingID(editingID === template.id ? null : template.id)}>{editingID === template.id ? '收起' : '编辑'}</button><button className="danger-link" type="button" disabled={usedBy.length > 0} title={usedBy.length ? `被 ${usedBy.join('、')} 使用` : undefined} onClick={() => onPolicyChange({ ...policy, templates: policy.templates.filter(item => item.id !== template.id) })}>移除</button></div>{editingID === template.id && <TemplateEditor initial={template} ruleSets={policy.rule_sets} existing={policy.templates} onCancel={() => setEditingID(null)} onSave={save} />}</div> })}</div>
  </div>
}

function TemplateEditor({ initial, ruleSets, existing, onCancel, onSave }: { initial?: PolicyTemplate; ruleSets: PolicyRuleSet[]; existing: PolicyTemplate[]; onCancel: () => void; onSave: (template: PolicyTemplate) => void }) {
  const [id, setID] = useState(initial?.id ?? '')
  const [selected, setSelected] = useState(initial?.rule_sets ?? [])
  const [error, setError] = useState('')
  const save = () => {
    const nextID = id.trim()
    if (!nextID) { setError('请填写分流模版名称。'); return }
    if (!initial && existing.some(item => item.id === nextID)) { setError('已经存在同名分流模版。'); return }
    if (!selected.length) { setError('至少选择一个规则集。'); return }
    onSave({ id: nextID, rule_sets: selected })
  }
  return <div className="library-editor template-editor"><label>名称<input aria-label="分流模版名称" disabled={Boolean(initial)} placeholder="例如 Claude Code" value={id} onChange={event => setID(event.target.value)} /></label><fieldset><legend>包含的规则集</legend><div className="template-rule-set-options">{ruleSets.map(ruleSet => <label key={ruleSet.id}><input type="checkbox" checked={selected.includes(ruleSet.id)} onChange={event => setSelected(event.target.checked ? [...selected, ruleSet.id] : selected.filter(id => id !== ruleSet.id))} /> <span>{CLAUDE_CODE_RULE_SET_NAMES[ruleSet.id] ?? ruleSet.id}<small>{ruleSet.behavior}</small></span></label>)}</div></fieldset>{!ruleSets.length && <Empty text="请先在“规则集”中创建匹配列表。" />}{error && <small className="field-error" role="alert">{error}</small>}<div className="editor-actions"><button type="button" onClick={onCancel}>取消</button><button className="primary" type="button" onClick={save}>保存到草稿</button></div></div>
}

function DeviceRoutesLibrary({ policy, selectedDeviceID, onSelectedDeviceChange, candidates, presetTemplate, onPresetConsumed, onPolicyChange }: { policy: PolicySet; selectedDeviceID: string; onSelectedDeviceChange: (id: string) => void; candidates: string[]; presetTemplate: string; onPresetConsumed: () => void; onPolicyChange: (policy: PolicySet) => void }) {
  const device = policy.devices.find(item => item.id === selectedDeviceID) ?? policy.devices[0]
  const deviceID = device?.id ?? ''
  const effective = device ? resolveProfile(policy, device.profile) : null
  const [editing, setEditing] = useState<number | 'new' | null>(presetTemplate ? 'new' : null)
  useEffect(() => { if (presetTemplate) setEditing('new') }, [presetTemplate])
  if (!device || !effective) return <div className="library-pane"><Empty text="请先登记设备，再添加设备分流。" /></div>
  const changeProfile = (change: (profile: PolicyProfile) => PolicyProfile, base = policy) => {
    const { policy: privatePolicy, profileID } = ensurePrivateProfile(base, deviceID)
    const next = copyPolicy(privatePolicy)
    next.profiles = next.profiles.map(profile => profile.id === profileID ? change(profile) : profile)
    onPolicyChange(next)
  }
  const saveRoute = (rule: PolicyRule, index?: number) => {
    const base = rule.match.template === CLAUDE_CODE_TEMPLATE.id ? installClaudeCodeExample(policy) : policy
    changeProfile(profile => ({ ...profile, rules: index === undefined ? [...(profile.rules ?? []), rule] : (profile.rules ?? []).map((item, current) => current === index ? rule : item) }), base)
    setEditing(null)
    onPresetConsumed()
  }
  const move = (index: number, delta: number) => changeProfile(profile => {
    const rules = [...(profile.rules ?? [])]
    const target = index + delta
    if (target < 0 || target >= rules.length) return profile
    const [item] = rules.splice(index, 1); rules.splice(target, 0, item)
    return { ...profile, rules }
  })
  const remove = (index: number) => {
    if (!window.confirm('删除这条设备分流吗？保存并重载后它将不再生效。')) return
    changeProfile(profile => ({ ...profile, rules: (profile.rules ?? []).filter((_, current) => current !== index) }))
  }
  const mode = desiredEgressMode(device)
  return <div className="library-pane device-route-library">
    <div className="library-pane-heading"><div><h3>设备分流</h3><p>从上到下匹配；每条分流为规则集或分流模版指定单独出口。</p></div><label className="device-route-picker">设备<select aria-label="设备分流设备" value={deviceID} onChange={event => { setEditing(null); onSelectedDeviceChange(event.target.value) }}>{policy.devices.map(item => <option key={item.id} value={item.id}>{displayDeviceName(item)}</option>)}</select></label></div>
    {desiredGatewayTarget(device) === 'upstream_router' && <div className="notice info">直连主路由期间，设备分流和出口设置会保留但不生效；切回 OpenSurge 后恢复。</div>}
    {mode === 'inherit_global' ? <div className="device-defaults following"><strong>设备出口跟随网关规则</strong><small>未命中下方设备分流的流量继续使用网关规则。</small></div> : <div className={`device-defaults ${mode === 'legacy_fallback' ? 'legacy' : ''}`}><CandidatePicker label={mode === 'dedicated' ? '独立设备出口候选' : '兼容兜底出口候选'} values={effective.default_policies} candidates={candidates} onChange={values => changeProfile(profile => ({ ...profile, default_policies: values }))} /><small>候选成员变化需要保存并重载；应用后仍可在设备卡即时切换。</small></div>}
    <div className="flat-rules">{effective.rules?.map((rule, index) => <div className="flat-rule" key={rule.id}><div className="rule-summary"><div>{routeMatchChips(rule, policy).map(chip => <span className="rule-chip" key={chip}>{chip}</span>)}</div><span className="rule-arrow">→</span><strong>{rule.policies?.length ? rule.policies.map(value => policyDisplayName(value)).join(' / ') : policyDisplayName(rule.action ?? '')}</strong></div><div className="rule-actions"><button type="button" disabled={index === 0} aria-label={`上移设备分流 ${rule.id}`} onClick={() => move(index, -1)}>↑</button><button type="button" disabled={index === (effective.rules?.length ?? 0) - 1} aria-label={`下移设备分流 ${rule.id}`} onClick={() => move(index, 1)}>↓</button><button type="button" onClick={() => setEditing(editing === index ? null : index)}>编辑</button><button className="danger-link" type="button" onClick={() => remove(index)}>删除</button></div>{editing === index && <DeviceRouteEditor initial={rule} existing={effective.rules ?? []} policy={policy} candidates={candidates} onCancel={() => setEditing(null)} onSave={updated => saveRoute(updated, index)} />}</div>)}{!effective.rules?.length && <Empty text="尚未添加设备分流；未命中流量继续使用上方设备出口设置。" />}</div>
    {editing === 'new' ? <DeviceRouteEditor key={presetTemplate || 'new'} presetTemplate={presetTemplate} existing={effective.rules ?? []} policy={policy} candidates={candidates} onCancel={() => { setEditing(null); onPresetConsumed() }} onSave={rule => saveRoute(rule)} /> : <button className="add-rule" type="button" onClick={() => setEditing('new')}>＋ 添加设备分流</button>}
  </div>
}

function DeviceRouteEditor({ initial, presetTemplate = '', existing, policy, candidates, onCancel, onSave }: { initial?: PolicyRule; presetTemplate?: string; existing: PolicyRule[]; policy: PolicySet; candidates: string[]; onCancel: () => void; onSave: (rule: PolicyRule) => void }) {
  const initialKind = initial?.match.template ? 'template' : 'rule_set'
  const [kind, setKind] = useState<'template' | 'rule_set'>(presetTemplate ? 'template' : initialKind)
  const [sourceID, setSourceID] = useState(presetTemplate || initial?.match.template || initial?.match.rule_sets?.[0] || '')
  const [mode, setMode] = useState<'action' | 'selector'>(initial?.policies?.length ? 'selector' : 'action')
  const [action, setAction] = useState(initial?.action ?? 'DIRECT')
  const [policies, setPolicies] = useState(initial?.policies ?? ['DIRECT'])
  const [error, setError] = useState('')
  const templates = [...policy.templates.filter(template => template.rule_sets?.length), ...(policy.templates.some(template => template.id === CLAUDE_CODE_TEMPLATE.id) ? [] : [CLAUDE_CODE_TEMPLATE])]
  const save = () => {
    if (!sourceID) { setError(`请选择${kind === 'template' ? '分流模版' : '规则集'}。`); return }
    if (mode === 'selector' && !policies.length) { setError('独立 Selector 至少需要一个出口候选。'); return }
    const match = kind === 'template' ? { template: sourceID } : { rule_sets: [sourceID] }
    const id = initial?.id ?? nextRuleID(existing)
    onSave(mode === 'selector' ? { id, match, policies, on_unsupported: 'reject' } : { id, match, action, on_unsupported: 'reject' })
  }
  const sources = kind === 'template' ? templates : policy.rule_sets
  return <div className="library-editor device-route-editor"><fieldset className="route-source-kind"><legend>匹配对象</legend><label><input type="radio" checked={kind === 'template'} onChange={() => { setKind('template'); setSourceID('') }} /> 分流模版</label><label><input type="radio" checked={kind === 'rule_set'} onChange={() => { setKind('rule_set'); setSourceID('') }} /> 单个规则集</label></fieldset><label>{kind === 'template' ? '分流模版' : '规则集'}<select aria-label="设备分流匹配对象" value={sourceID} onChange={event => setSourceID(event.target.value)}><option value="">请选择</option>{sources.map(item => <option key={item.id} value={item.id}>{item.id === CLAUDE_CODE_TEMPLATE.id ? 'Claude Code（内置示例）' : CLAUDE_CODE_RULE_SET_NAMES[item.id] ?? item.id}</option>)}</select></label><fieldset className="egress-mode"><legend>命中后的出口</legend><label><input type="radio" checked={mode === 'action'} onChange={() => setMode('action')} /> 固定出口</label><label><input type="radio" checked={mode === 'selector'} onChange={() => setMode('selector')} /> 独立即时切换</label>{mode === 'action' ? <select aria-label="设备分流出口" value={action} onChange={event => setAction(event.target.value)}>{candidates.map(candidate => <option key={candidate} value={candidate}>{policyDisplayName(candidate)}</option>)}</select> : <CandidatePicker label="设备分流出口候选" values={policies} candidates={candidates} onChange={setPolicies} />}</fieldset>{error && <small className="field-error" role="alert">{error}</small>}<div className="editor-actions"><button type="button" onClick={onCancel}>取消</button><button className="primary" type="button" onClick={save}>添加到草稿</button></div></div>
}

function installClaudeCodeExample(policy: PolicySet): PolicySet {
  const next = copyPolicy(policy)
  const existingRuleSets = new Set(next.rule_sets.map(ruleSet => ruleSet.id))
  next.rule_sets.push(...CLAUDE_CODE_RULE_SETS.filter(ruleSet => !existingRuleSets.has(ruleSet.id)).map(ruleSet => structuredClone(ruleSet)))
  if (!next.templates.some(template => template.id === CLAUDE_CODE_TEMPLATE.id)) next.templates.push(structuredClone(CLAUDE_CODE_TEMPLATE))
  return next
}

function routeMatchChips(rule: PolicyRule, policy: PolicySet) {
  if (rule.match.template) {
    const template = policy.templates.find(item => item.id === rule.match.template) ?? (rule.match.template === CLAUDE_CODE_TEMPLATE.id ? CLAUDE_CODE_TEMPLATE : undefined)
    return [`模版 ${rule.match.template}`, `${template?.rule_sets?.length ?? 0} 个规则集`]
  }
  if (rule.match.rule_sets?.length) return rule.match.rule_sets.map(id => `规则集 ${CLAUDE_CODE_RULE_SET_NAMES[id] ?? id}`)
  return matchChips(rule)
}

function resolveProfile(policy: PolicySet, profileID: string): PolicyProfile {
  const profile = policy.profiles.find(item => item.id === profileID)
  if (!profile) return { id: profileID, default_policies: ['DIRECT'], rules: [] }
  const template = profile.template ? policy.templates.find(item => item.id === profile.template) : undefined
  return { id: profile.id, default_policies: profile.default_policies.length ? [...profile.default_policies] : [...(template?.default_policies ?? [])], on_unsupported: profile.on_unsupported || template?.on_unsupported, rules: [...(template?.rules ?? []).map(rule => structuredClone(rule)), ...(profile.rules ?? []).map(rule => structuredClone(rule))] }
}

function ensurePrivateProfile(policy: PolicySet, deviceID: string): { policy: PolicySet; profileID: string } {
  const device = policy.devices.find(item => item.id === deviceID)!
  const profile = policy.profiles.find(item => item.id === device.profile)
  const shared = policy.devices.filter(item => item.profile === device.profile).length > 1
  if (profile && !shared && !profile.template) return { policy, profileID: profile.id }
  const next = copyPolicy(policy)
  const effective = resolveProfile(policy, device.profile)
  const profileID = uniqueProfileID(`${device.id}-policy`, next.profiles)
  next.profiles.push({ ...effective, id: profileID, template: undefined })
  next.devices = next.devices.map(item => item.id === deviceID ? { ...item, profile: profileID } : item)
  return { policy: next, profileID }
}

// Registration derives a private profile ID from the device ID, so removing the
// device should take that profile with it unless something else adopted it.
function ownedProfile(profileID: string, deviceID: string) {
  return profileID === `${deviceID}-policy` || profileID.startsWith(`${deviceID}-policy-`)
}

function uniqueProfileID(base: string, profiles: PolicyProfile[]) {
  const used = new Set(profiles.map(profile => profile.id))
  if (!used.has(base)) return base
  let counter = 2
  while (used.has(`${base}-${counter}`)) counter++
  return `${base}-${counter}`
}

function displayDeviceName(device: PolicyDevice) {
  return device.name || device.id
}

function availableDeviceID(name: string, mac: string, devices: PolicyDevice[]) {
  const suffix = mac.replace(/[^A-Fa-f0-9]/g, '').slice(-6).toLowerCase() || 'new'
  const slug = name.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '')
  const base = slug || `device-${suffix}`
  const used = new Set(devices.map(item => item.id))
  if (!used.has(base)) return base
  let counter = 2
  while (used.has(`${base}-${counter}`)) counter++
  return `${base}-${counter}`
}

function nextRuleID(rules: PolicyRule[]) {
  const used = new Set(rules.map(rule => rule.id))
  let counter = 1
  while (used.has(`rule-${counter}`)) counter++
  return `rule-${counter}`
}

function matchChips(rule: PolicyRule) {
  return [
    ...(rule.match.domains ?? []).map(value => `域名 ${value}`),
    ...(rule.match.ip_cidrs ?? []).map(value => `目标 ${value}`),
    ...(rule.match.protocols ?? []).map(value => value.toUpperCase()),
    ...(rule.match.ports ?? []).map(value => `端口 ${value}`),
    ...(rule.match.rule_sets ?? []).map(value => `规则集 ${value}`),
  ]
}
