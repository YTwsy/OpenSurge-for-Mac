import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import { api, RequestError, waitForOperation } from '../api'
import { Empty, PageHeader, SectionTitle } from '../components/Common'
import { DeviceOutletSummary } from '../components/DeviceOutletSummary'
import { OutletSummary } from '../components/OutletSummary'
import { useProxyHealth } from '../hooks/useProxyHealth'
import type { AppliedDeviceEgressMode, CompiledDevice, DeviceEgressMode, DevicePolicyDocument, DevicesResponse, Lease, ObservedDevice, Overview, PolicyDevice, PolicyProfile, PolicyRule, PolicyRuleSet, PolicySet, ProxyGroup, ProxyHealthEntry } from '../types'

const emptyPolicy = (): PolicySet => ({ devices: [], profiles: [], templates: [], rule_sets: [] })
const normalizePolicy = (value: PolicySet): PolicySet => ({ devices: value.devices ?? [], profiles: value.profiles ?? [], templates: value.templates ?? [], rule_sets: value.rule_sets ?? [] })
const copyPolicy = (value: PolicySet) => normalizePolicy(structuredClone(value))

type DevicesPageProps = {
  overview: Overview | null
  onChanged: () => Promise<void>
  onNavigate: (page: 'dashboard' | 'policies') => void
  onDirtyChange: (dirty: boolean) => void
}

export function DevicesPage({ overview, onChanged, onNavigate, onDirtyChange }: DevicesPageProps) {
  const proxyHealth = useProxyHealth()
  const [data, setData] = useState<DevicesResponse | null>(null)
  const [document, setDocument] = useState<DevicePolicyDocument | null>(null)
  const [policy, setPolicy] = useState<PolicySet>(emptyPolicy)
  const [importedCandidates, setImportedCandidates] = useState<string[]>([])
  const [selectedDeviceID, setSelectedDeviceID] = useState('')
  const [registrationOpen, setRegistrationOpen] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [reloadOpen, setReloadOpen] = useState(false)
  const [reloading, setReloading] = useState(false)
  const [revisionConflict, setRevisionConflict] = useState(false)
  const dirty = Boolean(document && JSON.stringify(policy) !== JSON.stringify(normalizePolicy(document.policy)))
  const dirtyRef = useRef(dirty)
  dirtyRef.current = dirty

  const groups = overview?.policies ?? []
  const globalGroups = useMemo(() => groups.filter(group => !group.name.startsWith('device/')), [groups])
  const candidates = useMemo(() => [...new Set(['DIRECT', 'REJECT', ...globalGroups.map(group => group.name), ...importedCandidates])], [globalGroups, importedCandidates])

  const refresh = useCallback(async (discardDraft = false) => {
    try {
      const [devices, config, sources] = await Promise.all([api.devices(), api.config(), api.sources().catch(() => ({ revision: '', sources: [] }))])
      const nextDocument = config.device_policy.enabled ? await api.devicePolicy() : null
      const imported = sources.sources.filter(source => source.applied && source.valid).flatMap(source => [...source.inventory.proxies, ...source.inventory.proxy_groups])
      setData(devices)
      setImportedCandidates(imported)
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
      setMessage('设备配置已保存。运行中的网关仍使用 applied 配置；请使用上方按钮应用并重载。')
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
      setMessage('网关已使用最新设备配置重新启动。改变固定 IPv4 的设备可能需要重新连接以获取新租约。')
    } catch (cause) {
      const failure = cause instanceof Error ? cause.message : String(cause)
      setReloadOpen(false)
      await refresh()
      await onChanged()
      setError(failure)
    } finally { setReloading(false) }
  }

  const discardDraft = async () => {
    if (!window.confirm('放弃当前尚未保存的设备修改并加载最新版本吗？')) return
    dirtyRef.current = false
    await refresh(true)
  }

  return <>
    <PageHeader eyebrow="DEVICES" title="设备与规则" description="设备可以跟随本机 / 全局规则，也可以使用独立出口；路由方式保存后需要重载，已应用的出口选择即时生效。" />
    {data?.drift && <DriftBanner data={data} running={overview?.status.gateway === 'running'} onReload={() => setReloadOpen(true)} onDashboard={() => onNavigate('dashboard')} />}
    {message && <div className="notice ok-notice" role="status">{message}</div>}
    {error && <div className="notice warn" role="alert">{error}{revisionConflict && <button className="inline-action" type="button" onClick={() => void discardDraft()}>放弃本地修改并加载最新版本</button>}</div>}

    {document ? <>
      <RegistrationPanel open={registrationOpen} onToggle={() => setRegistrationOpen(value => !value)} onRefresh={refreshDeviceObservation} topology={overview?.topology} leases={overview?.leases?.length ? overview.leases : data?.leases ?? []} observed={data?.observed_devices ?? []} observationError={data?.observation_error} policy={policy} candidates={candidates} onPolicyChange={setPolicy} onRegistered={id => { setSelectedDeviceID(id); setRegistrationOpen(false); setMessage('设备已加入本地草稿；保存后才会写入 desired 配置。') }} />

      <section className="section live-section">
        <SectionTitle title="设备出口" subtitle="即时生效 · 只切换已经应用的 mihomo selector，不改变规则结构" />
        <div className="device-layout">
          <ThisMacCard overview={overview} groups={globalGroups} healthByName={proxyHealth.byName} testing={proxyHealth.testing} onHealthTest={proxyHealth.test} onChanged={async () => { await onChanged(); await refresh(); await proxyHealth.refresh() }} onPolicies={() => onNavigate('policies')} />
          <div className="device-stack">
            {deviceViews(policy.devices, data?.applied_devices ?? (data?.applied ? data.devices : []), new Set(data?.changed_devices ?? [])).map(view => <DeviceCard key={`${view.desired?.id ?? view.applied?.id}-${view.state}`} view={view} topology={overview?.topology} leases={data?.leases ?? []} observed={data?.observed_devices ?? []} groups={groups} healthByName={proxyHealth.byName} healthTesting={proxyHealth.testing} onHealthTest={proxyHealth.test} selected={selectedDeviceID === (view.desired?.id ?? view.applied?.id)} onSelect={() => view.desired && setSelectedDeviceID(view.desired.id)} onEgressModeChange={mode => {
              if (!view.desired) return
              const next = copyPolicy(policy)
              next.devices = next.devices.map(device => device.id === view.desired!.id ? { ...device, egress_mode: mode } : device)
              setPolicy(next)
            }} onChanged={async () => { await onChanged(); await refresh(); await proxyHealth.refresh() }} />)}
          </div>
        </div>
        {!policy.devices.length && !data?.devices.length && <Empty text={overview?.topology === 'same_lan' ? '尚未登记设备。使用上方“登记新设备”可从当前经过 Mac 的设备开始。' : '尚未登记设备。使用上方“登记新设备”可直接从当前 DHCP 租约开始。'} />}
      </section>

      {selectedDeviceID && policy.devices.some(device => device.id === selectedDeviceID)
        ? <DeviceRulesPanel key={selectedDeviceID} deviceID={selectedDeviceID} policy={policy} candidates={candidates} onPolicyChange={setPolicy} />
        : <section className="section"><Empty text="选择一台 desired 设备后，可在这里编辑它的规则。" /></section>}

      <AdvancedPolicyTools policy={policy} candidates={candidates} onPolicyChange={setPolicy} />
      <div className={`sticky-save ${dirty ? 'has-changes' : 'is-saved'}`}><div><strong>{dirty ? '有未保存的设备修改' : '设备配置已保存'}</strong><small>{dirty ? '保存只更新 desired；运行中还需重载' : `revision ${document.revision.slice(0, 10)}`}</small></div><button className="primary" type="button" disabled={!dirty || saving} onClick={() => void save()}>{saving ? '正在验证并保存…' : '保存设备配置'}</button></div>
    </> : <section className="section"><Empty text="当前 gateway config 尚未启用设备策略；请先在网络设置中启用。" /></section>}

    {reloadOpen && <ReloadDialog busy={reloading} onCancel={() => setReloadOpen(false)} onConfirm={() => void reload()} />}
  </>
}

function DriftBanner({ data, running, onReload, onDashboard }: { data: DevicesResponse; running: boolean; onReload: () => void; onDashboard: () => void }) {
  return <div className="drift-banner" role="status"><div><span className="effect-badge restart">需重载</span><strong>{running ? '设备配置已保存，但尚未应用' : '设备配置将在下次启动时应用'}</strong><p>desired {data.desired_digest?.slice(0, 8)} · applied {data.applied_digest?.slice(0, 8) || '尚无'}</p></div>{running ? <button className="primary" type="button" onClick={onReload}>应用并重载网关</button> : <button type="button" onClick={onDashboard}>前往总览启动</button>}</div>
}

function ReloadDialog({ busy, onCancel, onConfirm }: { busy: boolean; onCancel: () => void; onConfirm: () => void }) {
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === 'Escape' && !busy) onCancel() }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [busy, onCancel])
  return <dialog className="reload-dialog" open aria-modal="true" aria-labelledby="reload-title"><h2 id="reload-title">应用设备配置并重载网关？</h2><p>OpenSurge 会先验证完整候选配置。验证通过后，DHCP/DNS、mihomo、PF 与 IPv4 forwarding 会短暂重启。</p><ul><li>当前连接会中断并重新建立。</li><li>改变固定 IPv4 的设备可能需要重新连接网络。</li><li>若候选配置验证失败，现有网关不会被停止。</li></ul><div className="dialog-actions"><button type="button" disabled={busy} onClick={onCancel}>取消</button><button className="primary" type="button" autoFocus disabled={busy} onClick={onConfirm}>{busy ? '正在验证并重载…' : '确认应用并重载'}</button></div></dialog>
}

function ThisMacCard({ overview, groups, healthByName, testing, onHealthTest, onChanged, onPolicies }: { overview: Overview | null; groups: ProxyGroup[]; healthByName: Map<string, ProxyHealthEntry>; testing: Set<string>; onHealthTest: (names: string[]) => Promise<void>; onChanged: () => Promise<void>; onPolicies: () => void }) {
  return <article className="this-mac"><div className="source-head"><div><small>THIS MAC</small><h3>本机 / 全局策略组</h3></div><span className="effect-badge live">即时生效</span></div><p>{overview?.status.interface} · {overview?.status.lan_ip}</p><small className="card-help">仅显示当前出口摘要；打开后可查看候选节点健康并切换，不等同于 macOS 系统代理开关。</small><div className="global-groups">{groups.map(group => <OutletSummary key={group.name} title={group.name} ariaLabel={`${group.name} 当前出口 ${group.selected}`} group={group} healthByName={healthByName} testing={testing} onTest={onHealthTest} onSelect={async policy => { await api.selectPolicy(group.name, policy); await onChanged() }} />)}{!groups.length && <Empty text="mihomo 未运行或没有全局可选策略组" />}</div><button className="text-link" type="button" onClick={onPolicies}>完整节点健康见策略页 →</button></article>
}

type DeviceView = { desired?: PolicyDevice; applied?: CompiledDevice; state: 'applied' | 'pending' | 'updated' | 'removing' }
function deviceViews(desired: PolicyDevice[], applied: CompiledDevice[], changed: Set<string>): DeviceView[] {
  const appliedByID = new Map(applied.map(device => [device.id, device]))
  const views: DeviceView[] = desired.map(device => {
    const running = appliedByID.get(device.id)
    appliedByID.delete(device.id)
    if (!running) return { desired: device, state: 'pending' }
    const same = running.mac.toLowerCase() === device.mac.toLowerCase() && running.ipv4 === device.ipv4 && running.profile === device.profile && appliedEgressMode(running) === desiredEgressMode(device)
    return { desired: device, applied: running, state: same && !changed.has(device.id) ? 'applied' : 'updated' }
  })
  for (const device of appliedByID.values()) views.push({ applied: device, state: 'removing' })
  return views
}

function DeviceCard({ view, topology, leases, observed, groups, healthByName, healthTesting, onHealthTest, selected, onSelect, onEgressModeChange, onChanged }: { view: DeviceView; topology?: string; leases: Lease[]; observed: ObservedDevice[]; groups: ProxyGroup[]; healthByName: Map<string, ProxyHealthEntry>; healthTesting: Set<string>; onHealthTest: (names: string[]) => Promise<void>; selected: boolean; onSelect: () => void; onEgressModeChange: (mode: DeviceEgressMode) => void; onChanged: () => Promise<void> }) {
  const [rulesOpen, setRulesOpen] = useState(false)
  const device = view.desired ?? view.applied!
  const applied = view.applied
  const desiredMode = view.desired ? desiredEgressMode(view.desired) : undefined
  const runningMode = applied ? appliedEgressMode(applied) : undefined
  const identity = applied ? deviceIdentity(applied, topology, leases, observed) : null
  const entries = Object.entries(applied?.groups ?? {})
  const defaultEntry = entries.find(([slot]) => slot === 'default')
  const ruleEntries = entries.filter(([slot]) => slot !== 'default')
  return <article className={`device-card ${selected ? 'selected' : ''}`}>
    <div className="source-head"><button className="device-title" type="button" disabled={!view.desired} aria-pressed={selected} onClick={onSelect}><small>{device.profile}</small><strong>{view.desired ? displayDeviceName(view.desired) : device.id}</strong>{view.desired?.name && <code>{device.id}</code>}</button><span className={`pill ${view.state === 'applied' ? 'ok' : ''}`}>{deviceStateLabel(view.state)}</span></div>
    <div className="device-identity"><code>{device.ipv4}</code><small>{device.mac}</small></div>
    {identity && <span className={`identity-state ${identity.tone}`}>{identity.text}</span>}
    {view.desired && <fieldset className="device-routing-mode"><legend>设备路由方式 <span className="effect-badge restart">保存后重载</span></legend><div className="route-options"><label className={desiredMode === 'inherit_global' ? 'active' : ''}><input type="radio" name={`route-${device.id}`} checked={desiredMode === 'inherit_global'} onChange={() => onEgressModeChange('inherit_global')} /><span><strong>跟随本机 / 全局规则</strong><small>按与本机相同的全局规则选择出口；设备专属规则仍优先。</small></span></label><label className={desiredMode === 'dedicated' ? 'active' : ''}><input type="radio" name={`route-${device.id}`} checked={desiredMode === 'dedicated'} onChange={() => onEgressModeChange('dedicated')} /><span><strong>独立设备出口</strong><small>公网流量优先使用设备出口；局域网和私网地址保持直连。</small></span></label></div></fieldset>}
    {desiredMode === 'legacy_fallback' && <div className="legacy-mode-warning" role="status"><strong>需要选择新的路由方式</strong><small>当前配置使用旧版兼容行为：先匹配全局规则，设备出口仅作兜底。</small></div>}
    {runningMode === 'inherit_global' && <div className="runtime-route following"><span><strong>当前运行</strong><small>跟随本机 / 全局规则</small></span><span className="effect-badge live">已应用</span></div>}
    {(runningMode === 'dedicated' || runningMode === 'legacy_fallback') && <div className={`default-slot ${runningMode === 'legacy_fallback' ? 'legacy' : ''}`}>{defaultEntry ? <DeviceOutletSummary device={applied!.id} slot={defaultEntry[0]} groupName={defaultEntry[1]} groups={groups} title={runningMode === 'dedicated' ? '独立出口 · 即时切换' : '兼容兜底出口 · 即时切换'} ariaLabel={`${device.id} ${runningMode === 'dedicated' ? '独立出口' : '兼容兜底出口'} 当前摘要`} healthByName={healthByName} testing={healthTesting} onTest={onHealthTest} onChanged={onChanged} /> : <button className="outlet-summary unavailable" type="button" disabled><span className="outlet-summary-copy"><small>{runningMode === 'dedicated' ? '独立出口' : '兼容兜底出口'}</small><strong>重载后可用</strong></span></button>}</div>}
    {!runningMode && desiredMode && <div className="runtime-route"><span><strong>重载后应用</strong><small>{egressModeLabel(desiredMode)}</small></span></div>}
    {runningMode && desiredMode && runningMode !== desiredMode && <small className="draft-mode-delta">草稿将改为“{egressModeLabel(desiredMode)}”；保存并重载前仍按“{egressModeLabel(runningMode)}”运行。</small>}
    {ruleEntries.length > 0 && <div className="rule-slots"><button className="rule-slots-toggle" type="button" aria-expanded={rulesOpen} onClick={() => setRulesOpen(value => !value)}>规则出口（{ruleEntries.length}）<span>{rulesOpen ? '收起' : '展开'}</span></button>{rulesOpen && ruleEntries.map(([slot, groupName]) => <div className="rule-outlet-summary" key={slot}><DeviceOutletSummary device={applied!.id} slot={slot} groupName={groupName} groups={groups} title={slot} ariaLabel={`${device.id} ${slot} 出口当前摘要`} healthByName={healthByName} testing={healthTesting} onTest={onHealthTest} onChanged={onChanged} /></div>)}</div>}
    {view.desired && <button className="edit-device" type="button" onClick={onSelect}>{selected ? '正在编辑此设备规则' : '编辑此设备规则'}</button>}
  </article>
}

function desiredEgressMode(device: PolicyDevice): AppliedDeviceEgressMode {
  return device.egress_mode ?? 'legacy_fallback'
}

function appliedEgressMode(device: CompiledDevice): AppliedDeviceEgressMode {
  return device.egress_mode || 'legacy_fallback'
}

function egressModeLabel(mode: AppliedDeviceEgressMode) {
  if (mode === 'inherit_global') return '跟随本机 / 全局规则'
  if (mode === 'dedicated') return '独立设备出口'
  return '旧版兼容兜底'
}

function deviceStateLabel(state: DeviceView['state']) {
  if (state === 'pending') return '待应用'
  if (state === 'updated') return '待更新'
  if (state === 'removing') return '待移除'
  return '已应用'
}

function deviceIdentity(applied: CompiledDevice, topology: string | undefined, leases: Lease[], observed: ObservedDevice[]) {
  if (topology === 'same_lan') {
    const current = observed.find(item => item.ip === applied.ipv4)
    if (!current) return { tone: '', text: '静态配置身份：等待该 IPv4 经过 Mac' }
    if (!current.mac) return { tone: 'observed', text: '当前有流量：固定 IPv4 已观察，MAC 尚未验证' }
    if (current.mac.toLowerCase() !== applied.mac.toLowerCase()) {
      return { tone: 'conflict', text: `身份冲突：邻居 MAC ${current.mac} 与登记不一致` }
    }
    return { tone: 'ready', text: '流量与邻居已观察：MAC / IPv4 匹配' }
  }
  const lease = leases.find(item => item.mac.toLowerCase() === applied.mac.toLowerCase() && item.ip === applied.ipv4 && item.online && (!item.expires_at || Date.parse(item.expires_at) > Date.now()))
  return lease
    ? { tone: 'ready', text: 'DHCP 身份已验证' }
    : { tone: '', text: '身份待确认：需要在线且未过期的精确 MAC / IPv4 租约' }
}

type RegistrationDraft = { id: string; name: string; mac: string; ipv4: string; profile: string; egress_mode: DeviceEgressMode | '' }
type RegistrationCandidate = { ip: string; mac: string; hostname: string; source: 'dhcp' | 'traffic'; activeConnections: number; online: boolean }

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
      source: 'traffic',
      activeConnections: device.active_connections,
      online: true,
    })
  }
  return [...byIP.values()].sort((left, right) => right.activeConnections - left.activeConnections || Number(right.online) - Number(left.online) || left.ip.localeCompare(right.ip, undefined, { numeric: true }))
}

function RegistrationPanel({ open, onToggle, onRefresh, topology, leases, observed, observationError, policy, candidates, onPolicyChange, onRegistered }: { open: boolean; onToggle: () => void; onRefresh: () => Promise<void>; topology?: string; leases: Lease[]; observed: ObservedDevice[]; observationError?: string; policy: PolicySet; candidates: string[]; onPolicyChange: (policy: PolicySet) => void; onRegistered: (id: string) => void }) {
  const [draft, setDraft] = useState<RegistrationDraft>({ id: '', name: '', mac: '', ipv4: '', profile: '', egress_mode: 'inherit_global' })
  const [defaults, setDefaults] = useState(['DIRECT'])
  const [useExisting, setUseExisting] = useState(false)
  const [error, setError] = useState('')
  const chooseCandidate = (candidate: RegistrationCandidate) => {
    const registered = policy.devices.find(item => (candidate.mac && item.mac.toLowerCase() === candidate.mac.toLowerCase()) || item.ipv4 === candidate.ip)
    setDraft({ id: registered?.id ?? '', name: registered ? displayDeviceName(registered) : candidate.hostname, mac: candidate.mac || registered?.mac || '', ipv4: candidate.ip, profile: registered?.profile ?? policy.profiles[0]?.id ?? '', egress_mode: registered ? registered.egress_mode ?? '' : 'inherit_global' })
    setUseExisting(Boolean(registered))
  }
  const register = () => {
    const name = draft.name.trim()
    if (!name || !draft.mac || !draft.ipv4) { setError('请填写设备名称、MAC 和固定 IPv4。'); return }
    if (!draft.egress_mode) { setError('请选择“跟随本机 / 全局规则”或“独立设备出口”。'); return }
    if (useExisting && !draft.profile) { setError('请选择一个现有 Profile。'); return }
    let next = copyPolicy(policy)
    const normalizedMAC = draft.mac.trim().toLowerCase()
    const registered = next.devices.find(item => item.id === draft.id || item.mac.toLowerCase() === normalizedMAC)
    const deviceID = registered?.id ?? availableDeviceID(name, normalizedMAC, next.devices)
    let profile = draft.profile
    const egressMode = draft.egress_mode as DeviceEgressMode
    if (!useExisting) {
      profile = uniqueProfileID(`${deviceID}-policy`, next.profiles)
      next.profiles.push({ id: profile, default_policies: defaults.length ? defaults : ['DIRECT'], on_unsupported: 'reject', rules: [] })
    }
    next.devices = [...next.devices.filter(item => item.id !== deviceID && item.mac.toLowerCase() !== normalizedMAC), { id: deviceID, name, mac: normalizedMAC, ipv4: draft.ipv4.trim(), profile, egress_mode: egressMode }]
    onPolicyChange(next); onRegistered(deviceID)
    setDraft({ id: '', name: '', mac: '', ipv4: '', profile: '', egress_mode: 'inherit_global' }); setDefaults(['DIRECT']); setUseExisting(false); setError('')
  }
  const visibleCandidates = registrationCandidates(topology, leases, observed)
  const previewID = draft.id || (draft.name.trim() ? availableDeviceID(draft.name.trim(), draft.mac, policy.devices) : '')
  return <section className="section device-tools-section registration"><button className="section-toggle" type="button" aria-expanded={open} onClick={onToggle}><span><strong>登记新设备</strong><small>{topology === 'same_lan' ? '从当前经过 Mac 的 LAN 流量发现设备，再确认静态身份与路由方式' : '从当前 DHCP 租约开始，确认身份与设备路由方式'}</small></span><span>{open ? '收起' : '展开'}</span></button>{open && <div className="registration-body"><div className="lease-picker"><div className="registration-picker-heading"><SectionTitle title={topology === 'same_lan' ? '当前经过 Mac 的设备' : '当前已接管设备'} subtitle={topology === 'same_lan' ? '来源是 mihomo 活跃连接；邻居表可补充 MAC，但不等同于 DHCP 身份验证' : '点击租约会自动填写 MAC 与当前 IPv4'} />{topology === 'same_lan' && <button className="text-link" type="button" onClick={() => void onRefresh()}>刷新当前设备</button>}</div>{observationError && topology === 'same_lan' && <div className="notice warn">实时设备观察不完整：{observationError}</div>}{visibleCandidates.length ? visibleCandidates.map(candidate => { const registered = policy.devices.find(item => (candidate.mac && item.mac.toLowerCase() === candidate.mac.toLowerCase()) || item.ipv4 === candidate.ip); return <button className="lease-choice" type="button" aria-label={`配置设备 ${candidate.ip}`} key={`${candidate.source}-${candidate.mac || 'unknown'}-${candidate.ip}`} onClick={() => chooseCandidate(candidate)}><span className={candidate.online ? 'pill ok' : 'pill'}>{candidate.source === 'traffic' ? '经过 Mac' : candidate.online ? '在线' : '历史租约'}</span><span><strong>{registered ? displayDeviceName(registered) : candidate.hostname || `未登记设备 ${candidate.ip}`}</strong><small>{candidate.mac || 'MAC 尚未从邻居表解析'}{candidate.activeConnections > 0 ? ` · ${candidate.activeConnections} 个活跃连接` : ''}</small></span><code>{candidate.ip}</code><span>配置此设备</span></button> }) : <Empty text={topology === 'same_lan' ? '当前尚未观察到经过 Mac 的 LAN 设备；让客户端产生网络流量后点击“刷新当前设备”，或直接手工填写。' : '当前没有 DHCP 租约；也可以手工填写设备。'} />}</div><div className="registration-form"><div className="utility-card-heading"><span><small>NEW DEVICE</small><h3>设备身份与路由</h3></span><span className="effect-badge restart">保存后重载</span></div><p className="card-help">确认设备名称、固定身份和路由方式；登记后仍需保存并重载设备配置。</p><label>设备名称<input aria-label="设备名称" value={draft.name} onChange={event => setDraft({ ...draft, name: event.target.value })} /></label><small className="registration-id-hint">{previewID ? <>内部 ID：<code>{previewID}</code>{draft.id ? '（保持不变）' : '（保存时自动生成）'}</> : '设备名称可包含空格；内部 ID 会在保存时自动生成。'}</small><label>MAC 地址<input aria-label="设备 MAC" value={draft.mac} onChange={event => setDraft({ ...draft, mac: event.target.value })} /></label><label>固定 IPv4<input aria-label="固定 IPv4" value={draft.ipv4} onChange={event => setDraft({ ...draft, ipv4: event.target.value })} /></label><fieldset className="registration-routing"><legend>设备路由方式</legend><label className={draft.egress_mode === 'inherit_global' ? 'active' : ''}><input type="radio" name="registration-route" checked={draft.egress_mode === 'inherit_global'} onChange={() => setDraft({ ...draft, egress_mode: 'inherit_global' })} /><span><strong>跟随本机 / 全局规则</strong><small>默认推荐；按全局规则路由，其余流量使用全局 MATCH。</small></span></label><label className={draft.egress_mode === 'dedicated' ? 'active' : ''}><input type="radio" name="registration-route" checked={draft.egress_mode === 'dedicated'} onChange={() => setDraft({ ...draft, egress_mode: 'dedicated' })} /><span><strong>独立设备出口</strong><small>公网流量优先使用专属 selector，局域网和私网仍直连。</small></span></label>{!draft.egress_mode && <small className="field-error" role="status">这是旧版设备，请选择新的路由方式后再保存。</small>}</fieldset>{!useExisting && draft.egress_mode === 'dedicated' && <CandidatePicker label="独立出口候选" values={defaults} candidates={candidates} onChange={setDefaults} />}
    <details className="inline-advanced"><summary>高级：使用已有 Profile</summary><label className="checkbox-field"><input type="checkbox" checked={useExisting} onChange={event => setUseExisting(event.target.checked)} /> 使用已有 Profile</label>{useExisting && <select aria-label="设备 Profile" value={draft.profile} onChange={event => setDraft({ ...draft, profile: event.target.value })}><option value="">选择 Profile</option>{policy.profiles.map(profile => <option key={profile.id}>{profile.id}</option>)}</select>}</details>{error && <small className="field-error" role="alert">{error}</small>}<button className="primary" type="button" onClick={register}>登记或更新设备</button></div></div>}</section>
}

function DeviceRulesPanel({ deviceID, policy, candidates, onPolicyChange }: { deviceID: string; policy: PolicySet; candidates: string[]; onPolicyChange: (policy: PolicySet) => void }) {
  const device = policy.devices.find(item => item.id === deviceID)!
  const mode = desiredEgressMode(device)
  const effective = resolveProfile(policy, device.profile)
  const [editing, setEditing] = useState<number | 'new' | null>(null)
  const changeProfile = (change: (profile: PolicyProfile) => PolicyProfile) => {
    const { policy: privatePolicy, profileID } = ensurePrivateProfile(policy, deviceID)
    const next = copyPolicy(privatePolicy)
    next.profiles = next.profiles.map(profile => profile.id === profileID ? change(profile) : profile)
    onPolicyChange(next)
  }
  const move = (index: number, delta: number) => changeProfile(profile => {
    const rules = [...(profile.rules ?? [])]
    const target = index + delta
    if (target < 0 || target >= rules.length) return profile
    const [item] = rules.splice(index, 1); rules.splice(target, 0, item)
    return { ...profile, rules }
  })
  const remove = (index: number) => {
    if (!window.confirm('删除这条设备规则吗？保存并重载后它将不再生效。')) return
    changeProfile(profile => ({ ...profile, rules: (profile.rules ?? []).filter((_, current) => current !== index) }))
  }
  const emptyRulesText = mode === 'inherit_global'
    ? '这台设备还没有覆盖规则；其余流量跟随本机 / 全局规则。'
    : mode === 'dedicated'
      ? '这台设备还没有覆盖规则；其余公网流量使用独立设备出口。'
      : '这台设备还没有覆盖规则；全局规则优先，兼容兜底出口仅处理尚未命中的流量。'
  return <section className="section device-rules"><div className="section-heading-row"><SectionTitle title={`${displayDeviceName(device)} 的规则`} subtitle="保存后重载 · 这些规则只属于当前设备" /><span className="effect-badge restart">需重载</span></div>{mode === 'inherit_global' ? <div className="device-defaults following"><strong>默认出口跟随本机 / 全局规则</strong><small>设备专属规则仍然优先。若希望其余公网流量固定到这台设备的 selector，请在上方改为“独立设备出口”。</small></div> : <div className={`device-defaults ${mode === 'legacy_fallback' ? 'legacy' : ''}`}><CandidatePicker label={mode === 'dedicated' ? '独立出口候选' : '兼容兜底出口候选'} values={effective.default_policies} candidates={candidates} onChange={values => changeProfile(profile => ({ ...profile, default_policies: values }))} /><small>{mode === 'legacy_fallback' ? '这是旧版兼容配置：全局规则仍优先。请在上方明确选择新的路由方式。' : '候选成员变化需要重载；重载后可在上方设备卡即时选择。'}</small></div>}<div className="flat-rules">{effective.rules?.map((rule, index) => <div className="flat-rule" key={rule.id}><div className="rule-summary"><div>{matchChips(rule).map(chip => <span className="rule-chip" key={chip}>{chip}</span>)}</div><span className="rule-arrow">→</span><strong>{rule.policies?.length ? rule.policies.join(' / ') : rule.action}</strong></div><div className="rule-actions"><button type="button" disabled={index === 0} aria-label={`上移规则 ${rule.id}`} onClick={() => move(index, -1)}>↑</button><button type="button" disabled={index === (effective.rules?.length ?? 0) - 1} aria-label={`下移规则 ${rule.id}`} onClick={() => move(index, 1)}>↓</button><button type="button" onClick={() => setEditing(index)}>编辑</button><button className="danger-link" type="button" onClick={() => remove(index)}>删除</button></div>{editing === index && <RuleForm initial={rule} candidates={candidates} existing={effective.rules ?? []} onCancel={() => setEditing(null)} onSave={updated => { changeProfile(profile => ({ ...profile, rules: (profile.rules ?? []).map((item, current) => current === index ? updated : item) })); setEditing(null) }} />}</div>)}{!effective.rules?.length && <Empty text={emptyRulesText} />}</div>{editing === 'new' ? <RuleForm candidates={candidates} existing={effective.rules ?? []} onCancel={() => setEditing(null)} onSave={rule => { changeProfile(profile => ({ ...profile, rules: [...(profile.rules ?? []), rule] })); setEditing(null) }} /> : <button className="add-rule" type="button" onClick={() => setEditing('new')}>＋ 添加设备规则</button>}</section>
}

function RuleForm({ initial, candidates, existing, onCancel, onSave }: { initial?: PolicyRule; candidates: string[]; existing: PolicyRule[]; onCancel: () => void; onSave: (rule: PolicyRule) => void }) {
  const [match, setMatch] = useState(() => structuredClone(initial?.match ?? {}))
  const [mode, setMode] = useState<'action' | 'selector'>(initial?.policies?.length ? 'selector' : 'action')
  const [action, setAction] = useState(initial?.action ?? 'REJECT')
  const [policies, setPolicies] = useState(initial?.policies ?? ['DIRECT'])
  const [error, setError] = useState('')
  const save = () => {
    if (![match.domains, match.ip_cidrs, match.protocols, match.ports, match.rule_sets].some(values => values?.length)) { setError('至少添加一个匹配条件。'); return }
    if (mode === 'selector' && !policies.length) { setError('Selector 至少需要一个出口候选。'); return }
    const id = initial?.id ?? nextRuleID(existing)
    onSave(mode === 'selector' ? { id, match, policies, on_unsupported: initial?.on_unsupported } : { id, match, action, on_unsupported: initial?.on_unsupported })
  }
  return <div className="rule-editor-form"><div className="match-grid"><TokenInput label="域名后缀" values={match.domains ?? []} placeholder="youtube.example" onChange={values => setMatch({ ...match, domains: values })} /><TokenInput label="目标 CIDR" values={match.ip_cidrs ?? []} placeholder="203.0.113.0/24" onChange={values => setMatch({ ...match, ip_cidrs: values })} /><TokenInput label="协议" values={match.protocols ?? []} placeholder="tcp 或 udp" suggestions={['tcp', 'udp']} onChange={values => setMatch({ ...match, protocols: values })} /><TokenInput label="目标端口" values={match.ports ?? []} placeholder="443" onChange={values => setMatch({ ...match, ports: values })} /><TokenInput label="规则集" values={match.rule_sets ?? []} placeholder="rule-set id" onChange={values => setMatch({ ...match, rule_sets: values })} /></div><fieldset className="egress-mode"><legend>匹配后的出口</legend><label><input type="radio" checked={mode === 'action'} onChange={() => setMode('action')} /> 固定动作</label><label><input type="radio" checked={mode === 'selector'} onChange={() => setMode('selector')} /> 可即时切换的 Selector</label>{mode === 'action' ? <select aria-label="Rule action" value={action} onChange={event => setAction(event.target.value)}>{candidates.map(candidate => <option key={candidate}>{candidate}</option>)}</select> : <CandidatePicker label="Selector 候选" values={policies} candidates={candidates} onChange={setPolicies} />}</fieldset>{error && <small className="field-error" role="alert">{error}</small>}<details className="rule-technical"><summary>技术信息</summary><code>{initial?.id ?? '保存时自动生成 rule-*'}</code></details><div className="editor-actions"><button type="button" onClick={onCancel}>取消</button><button className="primary" type="button" onClick={save}>保存到草稿</button></div></div>
}

function CandidatePicker({ label, values, candidates, onChange }: { label: string; values: string[]; candidates: string[]; onChange: (values: string[]) => void }) {
  const [candidate, setCandidate] = useState('')
  const listID = useId()
  const available = candidates.filter(item => !values.includes(item))
  const validCandidate = available.includes(candidate)
  const add = () => { if (validCandidate) onChange([...values, candidate]); setCandidate('') }
  return <div className="candidate-picker"><label>{label}<span className="candidate-add"><input type="search" aria-label={label} list={listID} autoComplete="off" placeholder="搜索出口…" value={candidate} onChange={event => setCandidate(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') { event.preventDefault(); add() } }} /><datalist id={listID}>{available.map(item => <option key={item} value={item} />)}</datalist><button type="button" disabled={!validCandidate} onClick={add}>添加</button></span></label><div className="token-list">{values.map(value => <span className="token" key={value}>{value}<button type="button" disabled={values.length === 1} aria-label={`移除 ${value}`} title={values.length === 1 ? '至少保留一个出口' : undefined} onClick={() => onChange(values.filter(item => item !== value))}>×</button></span>)}</div></div>
}

function TokenInput({ label, values, placeholder, suggestions = [], onChange }: { label: string; values: string[]; placeholder: string; suggestions?: string[]; onChange: (values: string[]) => void }) {
  const [value, setValue] = useState('')
  const add = () => { const next = value.trim(); if (next && !values.includes(next)) onChange([...values, next]); setValue('') }
  return <label className="token-input"><span>{label}</span><span className="token-entry"><input aria-label={label} list={suggestions.length ? `${label}-suggestions` : undefined} placeholder={placeholder} value={value} onChange={event => setValue(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') { event.preventDefault(); add() } }} /><button type="button" onClick={add}>添加</button></span>{suggestions.length > 0 && <datalist id={`${label}-suggestions`}>{suggestions.map(item => <option key={item}>{item}</option>)}</datalist>}<span className="token-list">{values.map(item => <span className="token" key={item}>{item}<button type="button" aria-label={`移除 ${item}`} onClick={() => onChange(values.filter(valueItem => valueItem !== item))}>×</button></span>)}</span></label>
}

function AdvancedPolicyTools({ policy, candidates, onPolicyChange }: { policy: PolicySet; candidates: string[]; onPolicyChange: (policy: PolicySet) => void }) {
  const [open, setOpen] = useState(false)
  const [profileID, setProfileID] = useState('')
  const [profilePolicies, setProfilePolicies] = useState(['DIRECT'])
  const [templateID, setTemplateID] = useState('')
  const [templatePolicies, setTemplatePolicies] = useState(['DIRECT'])
  const [ruleSet, setRuleSet] = useState<PolicyRuleSet>({ id: '', type: 'inline', behavior: 'domain', format: 'yaml', url: '', payload: [] })
  const profileRefs = (id: string) => policy.devices.filter(device => device.profile === id).map(device => device.id)
  const templateRefs = (id: string) => policy.profiles.filter(profile => profile.template === id).map(profile => profile.id)
  const ruleSetRefs = (id: string) => [...policy.profiles, ...policy.templates].filter(owner => owner.rules?.some(rule => rule.match.rule_sets?.includes(id))).map(owner => owner.id)
  const addRuleSet = () => {
    if (!ruleSet.id || policy.rule_sets.some(item => item.id === ruleSet.id)) return
    onPolicyChange({ ...policy, rule_sets: [...policy.rule_sets, ruleSet.type === 'http' ? { ...ruleSet, interval: 3600, payload: undefined } : { ...ruleSet, url: undefined, format: undefined }] })
    setRuleSet({ id: '', type: 'inline', behavior: 'domain', format: 'yaml', url: '', payload: [] })
  }
  return <section className="section device-tools-section advanced-policy"><button className="section-toggle" type="button" aria-expanded={open} onClick={() => setOpen(value => !value)}><span><strong>高级 / 复用机制</strong><small>Profiles、Templates 与 Rule Sets；普通设备配置无需进入这里</small></span><span>{open ? '收起' : '展开'}</span></button>{open && <div className="advanced-grid">
    <div className="advanced-tool-card">
      <div className="utility-card-heading"><span><small>DEVICE POLICY</small><h3>Profiles</h3></span><span className="effect-badge live">设备入口</span></div>
      <p className="card-help">设备实际关联的策略入口，集中保存默认出口和该设备的覆盖规则。</p>
      <input aria-label="Profile ID" placeholder="profile id" value={profileID} onChange={event => setProfileID(event.target.value)} />
      <CandidatePicker label="Profile 默认策略" values={profilePolicies} candidates={candidates} onChange={setProfilePolicies} />
      <button type="button" onClick={() => { if (!profileID || policy.profiles.some(item => item.id === profileID)) return; onPolicyChange({ ...policy, profiles: [...policy.profiles, { id: profileID, default_policies: profilePolicies, on_unsupported: 'reject', rules: [] }] }); setProfileID('') }}>添加 Profile</button>
      {policy.profiles.map(profile => { const refs = profileRefs(profile.id); return <div className="editor-item" key={profile.id}><strong>{profile.id}</strong><span>{refs.length ? `设备：${refs.join(', ')}` : profile.default_policies.join(' / ')}</span><button type="button" disabled={refs.length > 0} title={refs.length ? `被设备 ${refs.join(', ')} 引用` : undefined} onClick={() => onPolicyChange({ ...policy, profiles: policy.profiles.filter(item => item.id !== profile.id) })}>移除</button></div> })}
    </div>
    <div className="advanced-tool-card">
      <div className="utility-card-heading"><span><small>SHARED BASE</small><h3>Templates</h3></span><span className="effect-badge live">共享基础</span></div>
      <p className="card-help">可由多个 Profile 继承的基础策略，适合复用默认出口和共同规则。</p>
      <input aria-label="Template ID" placeholder="template id" value={templateID} onChange={event => setTemplateID(event.target.value)} />
      <CandidatePicker label="Template policies" values={templatePolicies} candidates={candidates} onChange={setTemplatePolicies} />
      <button type="button" onClick={() => { if (!templateID || policy.templates.some(item => item.id === templateID)) return; onPolicyChange({ ...policy, templates: [...policy.templates, { id: templateID, default_policies: templatePolicies, on_unsupported: 'reject', rules: [] }] }); setTemplateID('') }}>添加模板</button>
      {policy.templates.map(template => { const refs = templateRefs(template.id); return <div className="editor-item" key={template.id}><strong>template: {template.id}</strong><span>{refs.length ? `Profiles：${refs.join(', ')}` : template.default_policies.join(' / ')}</span><button type="button" disabled={refs.length > 0} title={refs.length ? `被 Profile ${refs.join(', ')} 引用` : undefined} onClick={() => onPolicyChange({ ...policy, templates: policy.templates.filter(item => item.id !== template.id) })}>移除</button></div> })}
    </div>
    <div className="advanced-tool-card wide">
      <div className="utility-card-heading"><span><small>REUSABLE MATCH</small><h3>Rule Sets</h3></span><span className="effect-badge live">匹配列表</span></div>
      <p className="card-help">为设备规则提供可复用的域名、IP CIDR 或经典匹配列表，支持内联内容和 HTTPS 来源。</p>
      <div className="ruleset-form">
        <div className="ruleset-primary-row"><input aria-label="Rule set ID" placeholder="rule set id" value={ruleSet.id} onChange={event => setRuleSet({ ...ruleSet, id: event.target.value })} /><select aria-label="Rule set type" value={ruleSet.type} onChange={event => setRuleSet({ ...ruleSet, type: event.target.value as 'inline' | 'http' })}><option value="inline">内联列表</option><option value="http">HTTPS 来源</option></select><select aria-label="Rule set behavior" value={ruleSet.behavior} onChange={event => setRuleSet({ ...ruleSet, behavior: event.target.value as PolicyRuleSet['behavior'] })}><option value="domain">域名</option><option value="ipcidr">IP CIDR</option><option value="classical">经典规则</option></select><button type="button" onClick={addRuleSet}>添加 Rule Set</button></div>
        <div className={`ruleset-source-row ${ruleSet.type}`}>{ruleSet.type === 'http' ? <><input aria-label="Rule set URL" placeholder="https://…" value={ruleSet.url} onChange={event => setRuleSet({ ...ruleSet, url: event.target.value })} /><select aria-label="Rule set format" value={ruleSet.format} onChange={event => setRuleSet({ ...ruleSet, format: event.target.value })}><option>yaml</option><option>text</option><option>mrs</option></select></> : <TokenInput label="Rule set payload" values={ruleSet.payload ?? []} placeholder="一项内容" onChange={payload => setRuleSet({ ...ruleSet, payload })} />}</div>
      </div>
      {policy.rule_sets.map(item => { const refs = ruleSetRefs(item.id); return <div className="editor-item" key={item.id}><strong>rule-set: {item.id}</strong><span>{refs.length ? `规则引用：${refs.join(', ')}` : `${item.type ?? 'inline'} · ${item.behavior}`}</span><button type="button" disabled={refs.length > 0} title={refs.length ? `被规则 ${refs.join(', ')} 引用` : undefined} onClick={() => onPolicyChange({ ...policy, rule_sets: policy.rule_sets.filter(candidate => candidate.id !== item.id) })}>移除</button></div> })}
    </div>
  </div>}</section>
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
