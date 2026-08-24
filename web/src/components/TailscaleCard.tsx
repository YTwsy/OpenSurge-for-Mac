import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import type { CompiledDevice, TailscaleResponse, TailscaleSettings, TailscaleUpdate } from '../types'
import type { OperationNotification } from './OperationNotifications'

const defaultSettings: TailscaleSettings = {
  enabled: false,
  display_name: 'Tailnet',
  hostname: 'opensurge-mac',
  control_url: 'https://controlplane.tailscale.com',
  accept_routes: false,
  magic_dns_suffixes: [],
  peer_cidrs: [],
  subnet_routes: [],
  allow_mac: true,
  allow_all_devices: false,
  allowed_devices: [],
  exit_node: '',
  exit_node_allow_lan_access: false,
}

type Draft = TailscaleUpdate

export function TailscaleCard({ onChanged, onNotify }: { onChanged: () => void | Promise<void>; onNotify: (notification: OperationNotification) => void }) {
  const [data, setData] = useState<TailscaleResponse | null>(null)
  const [devices, setDevices] = useState<CompiledDevice[]>([])
  const [draft, setDraft] = useState<Draft>({ ...defaultSettings })
  const [editing, setEditing] = useState(false)
  const [forgetting, setForgetting] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  const refresh = useCallback(async () => {
    try {
      const [tailscale, deviceResult] = await Promise.all([
        api.tailscale(),
        api.devices().catch(() => null),
      ])
      setData(tailscale)
      setDevices(deviceResult?.desired_devices ?? deviceResult?.devices ?? [])
      setError('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    }
  }, [])

  useEffect(() => { void refresh() }, [refresh])

  const openEditor = () => {
    setDraft({ ...(data?.settings ?? defaultSettings), auth_key: '' })
    setError('')
    setMessage('')
    setEditing(true)
  }

  const save = async (candidate: Draft = draft) => {
    if (!data || busy) return
    if (candidate.enabled && !data.auth_key_present && !data.identity_present && !candidate.auth_key?.trim()) {
      setError('首次连接需要填写 Auth Key；OpenSurge 只会保存受限副本，不会在界面中回显。')
      return
    }
    setBusy(true)
    setError('')
    setMessage('')
    try {
      const saved = await api.saveTailscale(data.revision, candidate)
      setData(saved)
      setDraft({ ...saved.settings, auth_key: '' })
      setEditing(false)
      const confirmation = saved.gateway_active ? '配置已验证并重载；OpenSurge 已触发 Tailscale 预热。' : '配置已保存，将在下次启动网关时载入并预热。'
      setMessage(confirmation)
      onNotify({ tone: 'success', title: saved.settings.enabled ? 'Tailscale 托管出站已更新' : 'Tailscale 已停用', message: confirmation })
      await Promise.resolve(onChanged())
    } catch (cause) {
      const failure = cause instanceof Error ? cause.message : String(cause)
      setError(failure)
      onNotify({ tone: 'error', title: 'Tailscale 配置未应用', message: failure })
    } finally {
      setBusy(false)
    }
  }

  const toggle = () => {
    if (!data) return
    if (!data.settings.enabled && !data.auth_key_present && !data.identity_present) {
      openEditor()
      setDraft(current => ({ ...current, enabled: true }))
      return
    }
    void save({ ...data.settings, enabled: !data.settings.enabled })
  }

  const forgetIdentity = async () => {
    if (!data || busy) return
    setBusy(true)
    setError('')
    try {
      const next = await api.forgetTailscaleIdentity(data.revision)
      setData(next)
      setForgetting(false)
      setMessage('本地 Tailscale 身份已忘记；Tailnet 后台中的设备记录不会被自动删除。')
      onNotify({ tone: 'success', title: '已忘记本地身份', message: '下次启用时会用保存的 Auth Key 尝试重新注册；单次或已过期密钥需要先替换。Tailnet 后台设备需另行管理。' })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setBusy(false)
    }
  }

  const status = tailscaleStatus(data)
  const targetCount = (data?.settings.magic_dns_suffixes.length ?? 0) + (data?.settings.peer_cidrs.length ?? 0) + (data?.settings.subnet_routes.length ?? 0)
  const access = accessSummary(data?.settings)

  return <section className="section tailscale-section" aria-busy={busy}>
    <div className="tailscale-hero">
      <div className="tailscale-mark" aria-hidden="true"><span /><span /><span /><span /><span /><span /><span /><span /><span /></div>
      <div className="tailscale-heading">
        <small>OPENSURGE MANAGED OUTBOUND</small>
        <div className="tailscale-title-line"><h2>Tailscale</h2><span className={`pill ${status.tone}`}>{status.label}</span></div>
        <p>把 Tailnet 当作一个可按域名、地址或设备选择的出站。未命中的普通流量继续使用现有代理策略。</p>
      </div>
      <div className="tailscale-actions">
        <button type="button" disabled={!data || busy} onClick={openEditor}>{data?.settings.enabled ? '配置' : '开始设置'}</button>
        <button className={data?.settings.enabled ? 'danger-soft' : 'primary'} type="button" disabled={!data || busy} onClick={toggle}>{busy ? '正在应用…' : data?.settings.enabled ? '停用' : '启用'}</button>
      </div>
    </div>

    <div className="tailscale-facts">
      <TailscaleFact label="节点身份" value={data?.identity_present ? data.settings.hostname : data?.auth_key_present ? '等待首次注册' : '尚未配置'} note={data?.identity_present ? '重启与重载后保持' : data?.auth_key_present ? '网关启动后预热并创建' : '需要 Auth Key'} />
      <TailscaleFact label="访问目标" value={`${targetCount} 项`} note={data?.settings.accept_routes ? `${data.settings.subnet_routes.length} 个远端子网已批准` : '未接受远端子网'} />
      <TailscaleFact label="Tailnet 使用者" value={access} note={data?.settings.allow_all_devices ? '仅已注册设备' : '显式授权，不向未知设备开放'} />
      <TailscaleFact label="出站角色" value={data?.selectable_exit ? 'Tailnet + Exit Node' : '仅 Tailnet'} note={data?.selectable_exit ? `可在设备出口中选择 ${data.settings.display_name}` : '不会作为公网出口出现'} />
    </div>

    <div className="tailscale-foot">
      <div className="tailscale-runtime"><span className={`status-dot ${data?.settings.enabled ? 'running' : 'stopped'}`} /><span><strong>{runtimeLabel(data)}</strong><small>{data?.settings.enabled ? 'OpenSurge 会主动预热；目标不可达时不会回退 DIRECT' : data?.identity_present ? '本地身份与密钥仍保留' : '不会加入 Tailnet 或接管流量'}</small></span></div>
      <button className="text-button" type="button" disabled={!data?.identity_present || data.settings.enabled || data.gateway_active || busy} title={data?.gateway_active ? '请先停止网关' : data?.settings.enabled ? '请先停用 Tailscale' : undefined} onClick={() => setForgetting(true)}>忘记本地身份</button>
    </div>
    {(error || message) && <div className={`tailscale-feedback ${error ? 'error' : 'success'}`} role={error ? 'alert' : 'status'}>{error || message}</div>}

    {editing && data && <TailscaleDialog draft={draft} setDraft={setDraft} devices={devices} authKeyPresent={data.auth_key_present} identityPresent={data.identity_present} busy={busy} error={error} onCancel={() => { setEditing(false); setError('') }} onSave={() => void save()} />}
    {forgetting && <dialog className="reload-dialog tailscale-forget-dialog" open aria-modal="true" aria-labelledby="tailscale-forget-title">
      <h2 id="tailscale-forget-title">忘记这台 Mac 的本地 Tailscale 身份？</h2>
      <p>这会删除 OpenSurge 保存的 tsnet 状态。下次启用时会重新注册一个节点。</p>
      <ul><li>已保存的 Auth Key 会保留；单次或已过期密钥需要在下次启用前替换。</li><li>Tailnet 或 Headscale 后台中的旧设备不会被自动删除。</li><li>该操作只允许在网关停止且 Tailscale 已停用时执行。</li></ul>
      <div className="dialog-actions"><button type="button" disabled={busy} onClick={() => setForgetting(false)}>取消</button><button className="danger" type="button" disabled={busy} onClick={() => void forgetIdentity()}>{busy ? '正在删除本地状态…' : '确认忘记本地身份'}</button></div>
    </dialog>}
  </section>
}

function TailscaleDialog({ draft, setDraft, devices, authKeyPresent, identityPresent, busy, error, onCancel, onSave }: { draft: Draft; setDraft: (value: Draft | ((current: Draft) => Draft)) => void; devices: CompiledDevice[]; authKeyPresent: boolean; identityPresent: boolean; busy: boolean; error: string; onCancel: () => void; onSave: () => void }) {
  const controlKind = draft.control_url.includes('controlplane.tailscale.com') ? 'tailscale' : 'headscale'
  const [deviceScope, setDeviceScope] = useState<'none' | 'selected' | 'all'>(draft.allow_all_devices ? 'all' : draft.allowed_devices.length ? 'selected' : 'none')
  const update = <K extends keyof Draft>(key: K, value: Draft[K]) => setDraft(current => ({ ...current, [key]: value }))
  const setScope = (scope: 'none' | 'selected' | 'all') => {
    setDeviceScope(scope)
    setDraft(current => ({ ...current, allow_all_devices: scope === 'all', allowed_devices: scope === 'selected' ? current.allowed_devices : [] }))
  }
  return <dialog className="tailscale-dialog" open aria-modal="true" aria-labelledby="tailscale-dialog-title">
    <div className="tailscale-dialog-head"><div><small>MANAGED TAILNET NODE</small><h2 id="tailscale-dialog-title">配置 Tailscale 出站</h2><p>OpenSurge 生成规则并交给 mihomo 内置 tsnet 数据面。</p></div><button type="button" aria-label="关闭 Tailscale 配置" disabled={busy} onClick={onCancel}>×</button></div>
    <div className="tailscale-dialog-body">
      <section className="tailscale-form-section identity"><div className="tailscale-section-copy"><span>01</span><div><h3>节点身份</h3><p>身份状态会持久化；普通重启和重载不会产生新设备。</p></div></div>
        <div className="tailscale-form-grid">
          <label><span>界面名称</span><input aria-label="Tailscale 界面名称" value={draft.display_name} onChange={event => update('display_name', event.target.value)} placeholder="例如 Home Tailnet" /></label>
          <label><span>Tailnet 节点名</span><input aria-label="Tailscale 节点名" value={draft.hostname} onChange={event => update('hostname', event.target.value.toLowerCase())} placeholder="opensurge-mac" /><small>只能使用小写字母、数字与连字符</small></label>
          <div className="tailscale-control-plane"><span>控制平面</span><div className="segmented"><button type="button" aria-pressed={controlKind === 'tailscale'} onClick={() => update('control_url', 'https://controlplane.tailscale.com')}>Tailscale</button><button type="button" aria-pressed={controlKind === 'headscale'} onClick={() => controlKind === 'tailscale' && update('control_url', 'https://headscale.example.com')}>Headscale</button></div></div>
          <label className="wide"><span>Control URL</span><input aria-label="Tailscale Control URL" value={draft.control_url} onChange={event => update('control_url', event.target.value)} /></label>
          <label className="wide"><span>Auth Key</span><input aria-label="Tailscale Auth Key" type="password" autoComplete="new-password" value={draft.auth_key ?? ''} onChange={event => update('auth_key', event.target.value)} placeholder={authKeyPresent ? '已安全保存；留空保持不变' : identityPresent ? '已有本地身份；可留空' : 'tskey-auth-…'} /><small>仅写入 root 管理的受限文件，保存后不会回显；重新注册时单次密钥可能需要替换。</small></label>
        </div>
      </section>

      <section className="tailscale-form-section"><div className="tailscale-section-copy"><span>02</span><div><h3>Tailnet 目标</h3><p>只有这些目标会进入 Tailscale；普通网页和订阅分流保持不变。</p></div></div>
        <div className="tailscale-target-grid">
          <ListEditor label="MagicDNS 后缀" hint="例如 home-name.ts.net" placeholder="home-name.ts.net" values={draft.magic_dns_suffixes} onChange={values => update('magic_dns_suffixes', values)} />
          <ListEditor label="Tailnet 节点 IP / CIDR" hint="建议填写精确节点 IP，避免接管整个 CGNAT" placeholder="100.82.10.7" values={draft.peer_cidrs} onChange={values => update('peer_cidrs', values)} />
          <ListEditor label="远端子网" hint="只批准确实由 subnet router 发布的私网" placeholder="10.20.0.0/16" values={draft.subnet_routes} onChange={values => { update('subnet_routes', values); if (values.length) update('accept_routes', true) }} />
        </div>
        <label className="tailscale-check"><input type="checkbox" checked={draft.accept_routes} disabled={draft.subnet_routes.length > 0} onChange={event => update('accept_routes', event.target.checked)} /><span><strong>接受 Tailnet 公布的子网路由</strong><small>OpenSurge 仍只捕获上方明确批准的子网，并会拒绝与本地 LAN 重叠的配置。</small></span></label>
      </section>

      <section className="tailscale-form-section"><div className="tailscale-section-copy"><span>03</span><div><h3>允许谁访问 Tailnet 目标</h3><p>只控制上方域名、peer 和远端子网；Exit Node 的公网出口由“设备”页单独选择。</p></div></div>
        <label className="tailscale-check"><input type="checkbox" checked={draft.allow_mac} onChange={event => update('allow_mac', event.target.checked)} /><span><strong>这台 Mac</strong><small>包括 TUN 流量与显式代理流量。</small></span></label>
        <div className="tailscale-scope-picker"><span>下游设备</span><div className="segmented" role="group" aria-label="Tailscale 下游设备范围"><button type="button" aria-pressed={deviceScope === 'none'} onClick={() => setScope('none')}>不允许</button><button type="button" aria-pressed={deviceScope === 'selected'} onClick={() => setScope('selected')}>指定设备</button><button type="button" aria-pressed={deviceScope === 'all'} onClick={() => setScope('all')}>全部已注册</button></div></div>
        {deviceScope === 'selected' && <div className="tailscale-device-list">{devices.length ? devices.map(device => <label key={device.id}><input type="checkbox" checked={draft.allowed_devices.includes(device.id)} onChange={event => update('allowed_devices', event.target.checked ? [...draft.allowed_devices, device.id] : draft.allowed_devices.filter(id => id !== device.id))} /><span><strong>{device.id}</strong><small>{device.ipv4}</small></span></label>) : <p>还没有已注册设备。请先在“设备”页完成注册。</p>}</div>}
      </section>

      <section className="tailscale-form-section exit"><div className="tailscale-section-copy"><span>04</span><div><h3>可选 Exit Node</h3><p>填写后，这个托管出站才会成为可供设备选择的公网出口。</p></div></div>
        <label><span>Exit Node 名称或 Tailscale IP</span><input aria-label="Tailscale Exit Node" value={draft.exit_node} onChange={event => update('exit_node', event.target.value)} placeholder="留空表示仅访问 Tailnet" /></label>
        <label className="tailscale-check"><input type="checkbox" checked={draft.exit_node_allow_lan_access} disabled={!draft.exit_node.trim()} onChange={event => update('exit_node_allow_lan_access', event.target.checked)} /><span><strong>使用 Exit Node 时保留本地 LAN 访问</strong><small>本地 NAS、打印机和路由器继续走 DIRECT。</small></span></label>
        <div className="tailscale-fail-closed"><span aria-hidden="true">◇</span><p><strong>Fail closed</strong> · Exit Node 离线或目标不在 Tailnet 路由内时连接直接失败，不会从本地公网泄漏。</p></div>
      </section>
    </div>
    <div className="tailscale-dialog-footer"><label className="tailscale-enable"><input type="checkbox" checked={draft.enabled} onChange={event => update('enabled', event.target.checked)} /><span><strong>保存后启用</strong><small>{draft.enabled ? '验证通过后立即重载运行中的网关' : '保存配置但不载入 Tailscale 节点'}</small></span></label><div><button type="button" disabled={busy} onClick={onCancel}>取消</button><button className="primary" type="button" disabled={busy || !draft.display_name.trim() || !draft.hostname.trim() || !draft.control_url.trim()} onClick={onSave}>{busy ? '正在验证并应用…' : '保存 Tailscale 配置'}</button></div></div>
    {error && <div className="tailscale-dialog-error" role="alert">{error}</div>}
  </dialog>
}

function ListEditor({ label, hint, placeholder, values, onChange }: { label: string; hint: string; placeholder: string; values: string[]; onChange: (values: string[]) => void }) {
  const [input, setInput] = useState('')
  const add = () => {
    const candidates = input.split(/[\s,]+/).map(value => value.trim()).filter(Boolean)
    if (!candidates.length) return
    onChange([...new Set([...values, ...candidates])])
    setInput('')
  }
  return <div className="tailscale-list-editor"><label><span>{label}</span><small>{hint}</small><span className="token-entry"><input aria-label={label} value={input} placeholder={placeholder} onChange={event => setInput(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') { event.preventDefault(); add() } }} /><button type="button" disabled={!input.trim()} onClick={add}>添加</button></span></label><div className="token-list">{values.map(value => <span className="token" key={value}>{value}<button type="button" aria-label={`移除 ${value}`} onClick={() => onChange(values.filter(item => item !== value))}>×</button></span>)}</div></div>
}

function TailscaleFact({ label, value, note }: { label: string; value: string; note: string }) {
  return <div><small>{label}</small><strong>{value}</strong><span>{note}</span></div>
}

function tailscaleStatus(data: TailscaleResponse | null) {
  if (!data) return { label: '正在读取', tone: '' }
  if (!data.settings.enabled) return { label: data.identity_present ? '已停用 · 身份保留' : '未启用', tone: '' }
  if (data.runtime_state === 'pending_gateway_start') return { label: '等待网关启动', tone: '' }
  return { label: '已载入 · 按需连接', tone: 'ok' }
}

function runtimeLabel(data: TailscaleResponse | null) {
  if (!data) return '正在读取配置'
  if (!data.settings.enabled) return data.identity_present ? '已停用，身份已保留' : 'Tailscale 未启用'
  if (data.runtime_state === 'pending_gateway_start') return '等待网关启动后载入'
  return data.selectable_exit ? 'Tailnet 与 Exit Node 已载入' : 'Tailnet 出站已载入'
}

function accessSummary(settings?: TailscaleSettings) {
  if (!settings) return '—'
  const scopes = []
  if (settings.allow_mac) scopes.push('Mac')
  if (settings.allow_all_devices) scopes.push('全部设备')
  else if (settings.allowed_devices.length) scopes.push(`${settings.allowed_devices.length} 台设备`)
  return scopes.join(' + ') || '无'
}
