import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import { Empty, PageHeader, SectionTitle } from '../components/Common'
import type { CompiledDevice, DevicePolicyDocument, DevicesResponse, Overview, PolicyRule, PolicySet, ProxyGroup } from '../types'

const normalizePolicy = (value: PolicySet): PolicySet => ({ devices: value.devices ?? [], profiles: value.profiles ?? [], templates: value.templates ?? [], rule_sets: value.rule_sets ?? [] })

export function DevicesPage({ overview }: { overview: Overview | null }) {
  const [data, setData] = useState<DevicesResponse | null>(null)
  const [policyDocument, setPolicyDocument] = useState<DevicePolicyDocument | null>(null)
  const [importedCandidates, setImportedCandidates] = useState<string[]>([])
  const [error, setError] = useState('')
  const groups = overview?.policies ?? []
  const candidates = useMemo(() => [...new Set(['DIRECT', 'REJECT', ...importedCandidates, ...groups.flatMap(group => [group.name, ...group.options])])], [groups, importedCandidates])
  const refresh = useCallback(async () => {
    try {
      const [devices, config, sources] = await Promise.all([api.devices(), api.config(), api.sources().catch(() => ({ revision: '', sources: [] }))])
      const policy = config.device_policy.enabled ? await api.devicePolicy() : null
      const imported = sources.sources.filter(source => source.applied && source.valid).flatMap(source => [...source.inventory.proxies, ...source.inventory.proxy_groups])
      setData(devices); setPolicyDocument(policy); setImportedCandidates(imported); setError('')
    } catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
  }, [])
  useEffect(() => { void refresh() }, [refresh])
  return <>
    <PageHeader eyebrow="DEVICES" title="每设备策略" description="设备身份来自稳定 Wi‑Fi MAC、固定 IPv4 DHCP reservation 和 applied policy。" />
    {data?.drift && <div className="notice warn">设备策略 desired/applied 不一致；编辑已保存，但需要重启网关才会生效。</div>}{error && <div className="notice warn" role="alert">{error}</div>}
    <section className="device-layout"><article className="this-mac"><small>THIS MAC</small><h3>OpenSurge 网关</h3><p>{overview?.status.interface} · {overview?.status.lan_ip}</p><span className="pill">本机不属于下游 DHCP 设备</span></article>{data?.devices.map(device => <DeviceCard key={device.id} device={device} leases={data.leases} groups={groups} onChanged={refresh} />)}</section>
    {!data?.devices.length && <Empty text="尚未登记下游设备；先在下方创建 profile，再登记设备。" />}
    {policyDocument ? <PolicyEditor document={policyDocument} candidates={candidates} onSaved={refresh} /> : <section className="section"><Empty text="当前 gateway config 尚未启用设备策略；请先在网络设置中启用。" /></section>}
  </>
}

function PolicyEditor({ document, candidates, onSaved }: { document: DevicePolicyDocument; candidates: string[]; onSaved: () => Promise<void> }) {
  const [policy, setPolicy] = useState<PolicySet>(() => normalizePolicy(structuredClone(document.policy)))
  const [profileID, setProfileID] = useState('')
  const [profilePolicies, setProfilePolicies] = useState('DIRECT')
  const [profileTemplate, setProfileTemplate] = useState('')
  const [device, setDevice] = useState({ id: '', mac: '', ipv4: '', profile: '' })
  const [rule, setRule] = useState({ profile: '', id: '', domains: '', ipCidrs: '', protocols: '', ports: '', ruleSets: '', action: 'REJECT', policies: '' })
  const [template, setTemplate] = useState({ id: '', policies: 'DIRECT' })
  const [ruleSet, setRuleSet] = useState<{ id: string; type: 'inline' | 'http'; behavior: 'domain' | 'ipcidr' | 'classical'; format: string; url: string; payload: string }>({ id: '', type: 'inline', behavior: 'domain', format: 'yaml', url: '', payload: '' })
  const [message, setMessage] = useState('')
  useEffect(() => { setPolicy(normalizePolicy(structuredClone(document.policy))) }, [document])
  const list = (value: string) => value.split(',').map(item => item.trim()).filter(Boolean)
  const addProfile = () => {
    if (!profileID || policy.profiles.some(item => item.id === profileID)) return
    setPolicy({ ...policy, profiles: [...policy.profiles, { id: profileID, template: profileTemplate || undefined, default_policies: list(profilePolicies), on_unsupported: 'reject', rules: [] }] })
    setProfileID('')
  }
  const addDevice = () => {
    if (!device.id || !device.mac || !device.ipv4 || !device.profile) return
    setPolicy({ ...policy, devices: [...policy.devices, device] })
    setDevice({ id: '', mac: '', ipv4: '', profile: '' })
  }
  const addRule = () => {
    if (!rule.profile || !rule.id) return
    const next: PolicyRule = { id: rule.id, match: { domains: list(rule.domains), ip_cidrs: list(rule.ipCidrs), protocols: list(rule.protocols), ports: list(rule.ports), rule_sets: list(rule.ruleSets) } }
    if (rule.policies.trim()) next.policies = list(rule.policies); else next.action = rule.action
    setPolicy({ ...policy, profiles: policy.profiles.map(profile => profile.id === rule.profile ? { ...profile, rules: [...(profile.rules ?? []), next] } : profile) })
    setRule({ profile: rule.profile, id: '', domains: '', ipCidrs: '', protocols: '', ports: '', ruleSets: '', action: 'REJECT', policies: '' })
  }
  const addTemplate = () => { if (template.id) { setPolicy({ ...policy, templates: [...policy.templates, { id: template.id, default_policies: list(template.policies), on_unsupported: 'reject', rules: [] }] }); setTemplate({ id: '', policies: 'DIRECT' }) } }
  const addRuleSet = () => { if (ruleSet.id) { setPolicy({ ...policy, rule_sets: [...policy.rule_sets, ruleSet.type === 'http' ? { id: ruleSet.id, type: 'http', behavior: ruleSet.behavior, format: ruleSet.format, url: ruleSet.url, interval: 3600 } : { id: ruleSet.id, type: 'inline', behavior: ruleSet.behavior, payload: list(ruleSet.payload) }] }); setRuleSet({ id: '', type: 'inline', behavior: 'domain', format: 'yaml', url: '', payload: '' }) } }
  const save = async () => {
    try { await api.saveDevicePolicy(policy, document.revision); setMessage('Desired policy 已保存；运行中的 gateway 需要重启后应用。'); await onSaved() }
    catch (cause) { setMessage(cause instanceof Error ? cause.message : String(cause)) }
  }
  return <section className="section policy-editor"><SectionTitle title="设备策略编辑器" subtitle="结构化编辑 desired policy；保存不会热改正在运行的 DHCP/mihomo bundle" />
    {message && <div className="notice" role="status">{message}</div>}
    <div className="editor-grid"><div><h3>1. Profiles</h3><div className="form-stack"><input aria-label="Profile ID" placeholder="profile id" value={profileID} onChange={event => setProfileID(event.target.value)} /><input aria-label="Profile 默认策略" placeholder="DIRECT, Proxy" value={profilePolicies} onChange={event => setProfilePolicies(event.target.value)} /><select aria-label="Profile template" value={profileTemplate} onChange={event => setProfileTemplate(event.target.value)}><option value="">无模板</option>{policy.templates.map(item => <option key={item.id}>{item.id}</option>)}</select><button onClick={addProfile}>添加</button></div>{policy.profiles.map(profile => <div className="editor-item" key={profile.id}><strong>{profile.id}</strong><span>{profile.template ? `template ${profile.template}` : profile.default_policies.join(' / ')}</span><button onClick={() => setPolicy({ ...policy, profiles: policy.profiles.filter(item => item.id !== profile.id) })}>移除</button></div>)}</div>
      <DeviceRegistration policy={policy} device={device} setDevice={setDevice} addDevice={addDevice} setPolicy={setPolicy} />
      <RuleEditor policy={policy} candidates={candidates} rule={rule} setRule={setRule} addRule={addRule} />
      <div className="wide"><h3>4. Templates 与 Rule Sets</h3><div className="form-row"><input aria-label="Template ID" placeholder="template id" value={template.id} onChange={event => setTemplate({ ...template, id: event.target.value })} /><input aria-label="Template policies" list="policy-candidates" value={template.policies} onChange={event => setTemplate({ ...template, policies: event.target.value })} /><button onClick={addTemplate}>添加模板</button></div><div className="rule-form"><input aria-label="Rule set ID" placeholder="rule set id" value={ruleSet.id} onChange={event => setRuleSet({ ...ruleSet, id: event.target.value })} /><select aria-label="Rule set type" value={ruleSet.type} onChange={event => setRuleSet({ ...ruleSet, type: event.target.value as 'inline' | 'http' })}><option>inline</option><option>http</option></select><select aria-label="Rule set behavior" value={ruleSet.behavior} onChange={event => setRuleSet({ ...ruleSet, behavior: event.target.value as 'domain' | 'ipcidr' | 'classical' })}><option>domain</option><option>ipcidr</option><option>classical</option></select>{ruleSet.type === 'http' ? <><input aria-label="Rule set URL" placeholder="https://…" value={ruleSet.url} onChange={event => setRuleSet({ ...ruleSet, url: event.target.value })} /><select aria-label="Rule set format" value={ruleSet.format} onChange={event => setRuleSet({ ...ruleSet, format: event.target.value })}><option>yaml</option><option>text</option><option>mrs</option></select></> : <input aria-label="Rule set payload" placeholder="comma-separated payload" value={ruleSet.payload} onChange={event => setRuleSet({ ...ruleSet, payload: event.target.value })} />}<button onClick={addRuleSet}>添加 Rule Set</button></div><div className="inventory">{policy.templates.map(item => <span key={item.id}>template: {item.id}</span>)}{policy.rule_sets.map(item => <span key={item.id}>rule-set: {item.id}</span>)}</div><datalist id="policy-candidates">{candidates.map(item => <option key={item} value={item} />)}</datalist></div>
    </div>
    <div className="editor-footer"><span>revision {document.revision.slice(0, 10)}</span><button className="primary" onClick={() => void save()}>保存 Desired Policy</button></div>
  </section>
}

type DeviceDraft = { id: string; mac: string; ipv4: string; profile: string }
function DeviceRegistration({ policy, device, setDevice, addDevice, setPolicy }: { policy: PolicySet; device: DeviceDraft; setDevice: (value: DeviceDraft) => void; addDevice: () => void; setPolicy: (value: PolicySet) => void }) {
  return <div><h3>2. Devices</h3><div className="form-stack"><input aria-label="Device ID" placeholder="device id" value={device.id} onChange={event => setDevice({ ...device, id: event.target.value })} /><input aria-label="Wi-Fi MAC" placeholder="Wi-Fi MAC" value={device.mac} onChange={event => setDevice({ ...device, mac: event.target.value })} /><input aria-label="固定 IPv4" placeholder="固定 IPv4" value={device.ipv4} onChange={event => setDevice({ ...device, ipv4: event.target.value })} /><select aria-label="设备 Profile" value={device.profile} onChange={event => setDevice({ ...device, profile: event.target.value })}><option value="">选择 profile</option>{policy.profiles.map(profile => <option key={profile.id}>{profile.id}</option>)}</select><button onClick={addDevice}>登记设备</button></div>{policy.devices.map(item => <div className="editor-item" key={item.id}><strong>{item.id}</strong><span>{item.ipv4} · {item.profile}</span><button onClick={() => setPolicy({ ...policy, devices: policy.devices.filter(candidate => candidate.id !== item.id) })}>移除</button></div>)}</div>
}

type RuleDraft = { profile: string; id: string; domains: string; ipCidrs: string; protocols: string; ports: string; ruleSets: string; action: string; policies: string }
function RuleEditor({ policy, candidates, rule, setRule, addRule }: { policy: PolicySet; candidates: string[]; rule: RuleDraft; setRule: (value: RuleDraft) => void; addRule: () => void }) {
  return <div className="wide"><h3>3. 设备覆盖规则</h3><div className="rule-form"><select aria-label="规则 Profile" value={rule.profile} onChange={event => setRule({ ...rule, profile: event.target.value })}><option value="">选择 profile</option>{policy.profiles.map(profile => <option key={profile.id}>{profile.id}</option>)}</select><input aria-label="Rule ID" placeholder="rule id" value={rule.id} onChange={event => setRule({ ...rule, id: event.target.value })} /><input aria-label="Domains" placeholder="domains" value={rule.domains} onChange={event => setRule({ ...rule, domains: event.target.value })} /><input aria-label="Target CIDRs" placeholder="target CIDRs" value={rule.ipCidrs} onChange={event => setRule({ ...rule, ipCidrs: event.target.value })} /><input aria-label="Protocols" placeholder="tcp,udp" value={rule.protocols} onChange={event => setRule({ ...rule, protocols: event.target.value })} /><input aria-label="Ports" placeholder="80,443" value={rule.ports} onChange={event => setRule({ ...rule, ports: event.target.value })} /><input aria-label="Rule set IDs" placeholder="rule set ids" value={rule.ruleSets} onChange={event => setRule({ ...rule, ruleSets: event.target.value })} /><input aria-label="Selector candidates" list="policy-candidates" placeholder="selector candidates" value={rule.policies} onChange={event => setRule({ ...rule, policies: event.target.value })} /><select aria-label="Rule action" value={rule.action} onChange={event => setRule({ ...rule, action: event.target.value })}><option>REJECT</option><option>DIRECT</option></select><button onClick={addRule}>添加规则</button></div><small>候选由已应用且验证通过的 imported inventory、运行中 group 与内置 DIRECT/REJECT 组成：{candidates.join(', ')}</small></div>
}

function DeviceCard({ device, leases, groups, onChanged }: { device: CompiledDevice; leases: DevicesResponse['leases']; groups: ProxyGroup[]; onChanged: () => Promise<void> }) {
  const lease = leases.find(item => item.mac.toLowerCase() === device.mac.toLowerCase() && item.ip === device.ipv4)
  return <article className="device-card"><div className="source-head"><div><small>{device.profile}</small><h3>{device.id}</h3></div><span className={lease?.online ? 'pill ok' : 'pill'}>{lease?.online ? '身份就绪' : '等待精确租约'}</span></div><code>{device.ipv4}</code><small>{device.mac}</small><div className="slots">{Object.entries(device.groups).map(([slot, groupName]) => { const group = groups.find(item => item.name === groupName); return <label key={slot}><span>{slot}</span><select value={group?.selected ?? ''} disabled={!group} onChange={event => void api.selectDevicePolicy(device.id, slot, event.target.value).then(onChanged)}><option value="">不可用</option>{group?.options.map(option => <option key={option}>{option}</option>)}</select></label> })}</div></article>
}
