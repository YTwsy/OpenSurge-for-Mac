import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import { t } from '../i18n'
import type { CompiledDevice, TailscaleDiscoveredNode, TailscaleDiscoveryResponse, TailscaleResponse, TailscaleSettings, TailscaleUpdate } from '../types'
import type { OperationNotification } from './OperationNotifications'

const tailscaleKeysURL = 'https://console.tailscale.com/admin/settings/keys'

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
  const [discovery, setDiscovery] = useState<TailscaleDiscoveryResponse | null>(null)
  const [devices, setDevices] = useState<CompiledDevice[]>([])
  const [draft, setDraft] = useState<Draft>({ ...defaultSettings })
  const [editing, setEditing] = useState(false)
  const [forgetting, setForgetting] = useState(false)
  const [busy, setBusy] = useState(false)
  const [discovering, setDiscovering] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  const refreshDiscovery = useCallback(async () => {
    setDiscovering(true)
    try {
      setDiscovery(await api.tailscaleDiscovery())
    } catch (cause) {
      setDiscovery({ schema_version: 1, available: false, magic_dns: false, peers: [], error: cause instanceof Error ? cause.message : String(cause) })
    } finally {
      setDiscovering(false)
    }
  }, [])

  const refresh = useCallback(async () => {
    try {
      const [tailscale, deviceResult, discoveryResult] = await Promise.all([
        api.tailscale(),
        api.devices().catch(() => null),
        api.tailscaleDiscovery().catch((cause: unknown) => ({ schema_version: 1, available: false, magic_dns: false, peers: [], error: cause instanceof Error ? cause.message : String(cause) } as TailscaleDiscoveryResponse)),
      ])
      setData(tailscale)
      setDevices(deviceResult?.desired_devices ?? deviceResult?.devices ?? [])
      setDiscovery(discoveryResult)
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
      setError(t('首次连接需要填写 Auth Key；OpenSurge 只会保存受限副本，不会在界面中回显。'))
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
      const confirmation = t(saved.gateway_active ? '配置已验证并重载；OpenSurge 已触发 Tailscale 预热。' : '配置已保存，将在下次启动网关时载入并预热。')
      setMessage(confirmation)
      onNotify({ tone: 'success', title: t(saved.settings.enabled ? 'Tailscale 托管出站已更新' : 'Tailscale 已停用'), message: confirmation })
      await Promise.resolve(onChanged())
    } catch (cause) {
      const failure = cause instanceof Error ? cause.message : String(cause)
      setError(failure)
      onNotify({ tone: 'error', title: t('Tailscale 配置未应用'), message: failure })
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
      setMessage(t('本地 Tailscale 身份已忘记；Tailnet 后台中的设备记录不会被自动删除。'))
      onNotify({ tone: 'success', title: t('已忘记本地身份'), message: t('下次启用时会用保存的 Auth Key 尝试重新注册；单次或已过期密钥需要先替换。Tailnet 后台设备需另行管理。') })
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
        <p>{t('把 Tailnet 当作一个可按域名、地址或设备选择的出站。未命中的普通流量继续使用现有代理策略。')}</p>
      </div>
      <div className="tailscale-actions">
        <button type="button" disabled={!data || busy} onClick={openEditor}>{t(data?.settings.enabled ? '配置' : '开始设置')}</button>
        <button className={data?.settings.enabled ? 'danger-soft' : 'primary'} type="button" disabled={!data || busy} onClick={toggle}>{t(busy ? '正在应用…' : data?.settings.enabled ? '停用' : '启用')}</button>
      </div>
    </div>

    <div className="tailscale-facts">
      <TailscaleFact label="节点身份" value={data?.identity_present ? data.settings.hostname : t(data?.auth_key_present ? '等待首次注册' : '尚未配置')} note={t(data?.identity_present ? '重启与重载后保持' : data?.auth_key_present ? '网关启动后预热并创建' : '需要 Auth Key')} />
      <TailscaleFact label="访问目标" value={t('{{count}} 项', { count: targetCount })} note={data?.settings.accept_routes ? t('{{count}} 个远端子网已批准', { count: data.settings.subnet_routes.length }) : t('未接受远端子网')} />
      <TailscaleFact label="Tailnet 使用者" value={access} note={t(data?.settings.allow_all_devices ? '仅已注册设备' : '显式授权，不向未知设备开放')} />
      <TailscaleFact label="出站角色" value={data?.selectable_exit ? 'Tailnet + Exit Node' : t('仅 Tailnet')} note={data?.selectable_exit ? t('可在设备出口中选择 {{name}}', { name: data.settings.display_name }) : t('不会作为公网出口出现')} />
    </div>

    <div className="tailscale-foot">
      <div className="tailscale-runtime"><span className={`status-dot ${data?.settings.enabled ? 'running' : 'stopped'}`} /><span><strong>{runtimeLabel(data)}</strong><small>{t(data?.settings.enabled ? 'OpenSurge 会主动预热；目标不可达时不会回退 DIRECT' : data?.identity_present ? '本地身份与密钥仍保留' : '不会加入 Tailnet 或接管流量')}</small></span></div>
      <button className="text-button" type="button" disabled={!data?.identity_present || data.settings.enabled || data.gateway_active || busy} title={data?.gateway_active ? t('请先停止网关') : data?.settings.enabled ? t('请先停用 Tailscale') : undefined} onClick={() => setForgetting(true)}>{t('忘记本地身份')}</button>
    </div>
    {(error || message) && <div className={`tailscale-feedback ${error ? 'error' : 'success'}`} role={error ? 'alert' : 'status'}>{error || message}</div>}

    {editing && data && <TailscaleDialog draft={draft} setDraft={setDraft} discovery={discovery} discovering={discovering} onRefreshDiscovery={() => void refreshDiscovery()} devices={devices} authKeyPresent={data.auth_key_present} identityPresent={data.identity_present} gatewayActive={data.gateway_active} busy={busy} error={error} onCancel={() => { setEditing(false); setError('') }} onSave={() => void save()} />}
    {forgetting && <dialog className="reload-dialog tailscale-forget-dialog" open aria-modal="true" aria-labelledby="tailscale-forget-title">
      <h2 id="tailscale-forget-title">{t('忘记这台 Mac 的本地 Tailscale 身份？')}</h2>
      <p>{t('这会删除 OpenSurge 保存的 tsnet 状态。下次启用时会重新注册一个节点。')}</p>
      <ul><li>{t('已保存的 Auth Key 会保留；单次或已过期密钥需要在下次启用前替换。')}</li><li>{t('Tailnet 或 Headscale 后台中的旧设备不会被自动删除。')}</li><li>{t('该操作只允许在网关停止且 Tailscale 已停用时执行。')}</li></ul>
      <div className="dialog-actions"><button type="button" disabled={busy} onClick={() => setForgetting(false)}>{t('取消')}</button><button className="danger" type="button" disabled={busy} onClick={() => void forgetIdentity()}>{t(busy ? '正在删除本地状态…' : '确认忘记本地身份')}</button></div>
    </dialog>}
  </section>
}

function TailscaleDialog({ draft, setDraft, discovery, discovering, onRefreshDiscovery, devices, authKeyPresent, identityPresent, gatewayActive, busy, error, onCancel, onSave }: { draft: Draft; setDraft: (value: Draft | ((current: Draft) => Draft)) => void; discovery: TailscaleDiscoveryResponse | null; discovering: boolean; onRefreshDiscovery: () => void; devices: CompiledDevice[]; authKeyPresent: boolean; identityPresent: boolean; gatewayActive: boolean; busy: boolean; error: string; onCancel: () => void; onSave: () => void }) {
  const controlKind = draft.control_url.includes('controlplane.tailscale.com') ? 'tailscale' : 'headscale'
  const [deviceScope, setDeviceScope] = useState<'none' | 'selected' | 'all'>(draft.allow_all_devices ? 'all' : draft.allowed_devices.length ? 'selected' : 'none')
  const update = <K extends keyof Draft>(key: K, value: Draft[K]) => setDraft(current => ({ ...current, [key]: value }))
  const setScope = (scope: 'none' | 'selected' | 'all') => {
    setDeviceScope(scope)
    setDraft(current => ({ ...current, allow_all_devices: scope === 'all', allowed_devices: scope === 'selected' ? current.allowed_devices : [] }))
  }
  const subnetOptions = discoveredSubnetOptions(discovery)
  const exitCandidates = discovery?.peers.filter(peer => peer.exit_node_option) ?? []
  const selectedPeerNames = discovery?.peers.filter(peer => peerCIDRs(peer).some(value => draft.peer_cidrs.includes(value))).map(peer => peer.name) ?? []
  const magicSuffix = normalizedSuffix(discovery?.magic_dns_suffix)

  const setPeer = (peer: TailscaleDiscoveredNode, enabled: boolean) => {
    const values = peerCIDRs(peer)
    update('peer_cidrs', enabled ? unique([...draft.peer_cidrs, ...values]) : draft.peer_cidrs.filter(value => !values.includes(value)))
  }
  const setMagicDNS = (enabled: boolean) => {
    if (!magicSuffix) return
    update('magic_dns_suffixes', enabled ? unique([...draft.magic_dns_suffixes, magicSuffix]) : draft.magic_dns_suffixes.filter(value => value !== magicSuffix))
  }
  const setSubnet = (route: string, enabled: boolean) => {
    const routes = enabled ? unique([...draft.subnet_routes, route]) : draft.subnet_routes.filter(value => value !== route)
    setDraft(current => ({ ...current, subnet_routes: routes, accept_routes: routes.length > 0 }))
  }
  const setExitNode = (value: string) => {
    setDraft(current => ({ ...current, exit_node: value, exit_node_allow_lan_access: value ? current.exit_node_allow_lan_access : false }))
  }

  return <dialog className="tailscale-dialog" open aria-modal="true" aria-labelledby="tailscale-dialog-title">
    <div className="tailscale-dialog-head"><div><small>MANAGED TAILNET NODE</small><h2 id="tailscale-dialog-title">{t('配置 Tailscale 出站')}</h2><p>{t('从本机发现建议，确认后再应用到 OpenSurge。')}</p></div><button type="button" aria-label={t('关闭 Tailscale 配置')} disabled={busy} onClick={onCancel}>×</button></div>
    <div className="tailscale-dialog-body">
      <DiscoveryBanner discovery={discovery} discovering={discovering} onRefresh={onRefreshDiscovery} />

      <section className="tailscale-form-section identity"><div className="tailscale-section-copy"><span>01</span><div><h3>{t('连接身份')}</h3><p>{t('本机 Tailscale App 只提供配置建议；OpenSurge 会注册并持久化独立节点身份。')}</p></div></div>
        <form className="tailscale-auth-card" onSubmit={event => event.preventDefault()}>
          <div className="tailscale-auth-heading"><span><strong>Auth Key</strong><small>{t(identityPresent ? '已有 OpenSurge 节点身份，通常无需再次填写。' : authKeyPresent ? '密钥已安全保存；留空保持不变。' : '首次连接需要一个以 tskey-auth- 开头的密钥。')}</small></span>{controlKind === 'tailscale' ? <a href={tailscaleKeysURL} target="_blank" rel="noopener noreferrer">{t(authKeyPresent || identityPresent ? '打开 Keys 页面' : '创建 Auth Key')} <span aria-hidden="true">↗</span></a> : <small>{t('请在 Headscale 管理端创建 preauth key')}</small>}</div>
          <label><span>{t('粘贴 Auth Key')}</span><input aria-label={t('Tailscale Auth Key')} type="password" autoComplete="new-password" value={draft.auth_key ?? ''} onChange={event => update('auth_key', event.target.value)} placeholder={authKeyPresent ? t('已安全保存；留空保持不变') : identityPresent ? t('已有本地身份；可留空') : 'tskey-auth-…'} /></label>
          {controlKind === 'tailscale' && !identityPresent && <ol className="tailscale-auth-steps"><li>{t('登录 Tailscale 管理后台并选择 Generate auth key。')}</li><li>{t('建议保持 Reusable 关闭，且不要启用 Ephemeral。')}</li><li>{t('生成后立即复制；完整密钥只显示一次。')}</li></ol>}
          <small className="tailscale-secret-note">{t('OpenSurge 仅把密钥写入 root 管理的受限文件，保存后不会回显。创建密钥需要 Tailnet 管理权限。')}</small>
        </form>
      </section>

      <section className="tailscale-form-section"><div className="tailscale-section-copy"><span>02</span><div><h3>{t('访问哪些 Tailnet 资源')}</h3><p>{t('选择具体设备最安全；建议项不会自动保存或扩大访问范围。')}</p></div></div>
        <div className="tailscale-resource-list">
          {discovery?.available && discovery.peers.length > 0 ? discovery.peers.map(peer => {
            const checked = peerCIDRs(peer).some(value => draft.peer_cidrs.includes(value))
            return <label className="tailscale-resource-row" key={peer.id || peer.dns_name || peer.name}>
              <input type="checkbox" checked={checked} onChange={event => setPeer(peer, event.target.checked)} aria-label={t('允许访问 {{name}}', { name: peer.name })} />
              <span className="tailscale-peer-symbol" aria-hidden="true">{peer.name.slice(0, 1).toUpperCase()}</span>
              <span className="tailscale-resource-copy"><strong>{peer.name}</strong><small>{peer.dns_name || peer.tailscale_ips.join(' · ')}</small><code>{peer.tailscale_ips.join(' · ') || t('未报告 Tailscale IP')}</code></span>
              <span className={`tailscale-resource-state ${peer.online ? 'online' : ''}`}>{t(peer.online ? '在线' : '离线')}</span>
            </label>
          }) : <div className="tailscale-resource-empty"><strong>{t(discovering ? '正在读取本机 Tailnet…' : '没有可供选择的 Tailnet 节点')}</strong><small>{t('你仍可在下方“高级手动配置”中填写 IP 或 CIDR。')}</small></div>}
        </div>

        {magicSuffix && <label className="tailscale-wide-option">
          <input type="checkbox" checked={draft.magic_dns_suffixes.includes(magicSuffix)} onChange={event => setMagicDNS(event.target.checked)} />
          <span><strong>{t('允许所有 MagicDNS 名称')}</strong><small>{t('匹配整个 {{suffix}}，不只上方已选择的节点。', { suffix: magicSuffix })}</small></span>
          <em>{t(discovery?.magic_dns ? '范围较宽' : '本机未启用')}</em>
        </label>}

        <div className="tailscale-subnet-box"><div><strong>{t('远端子网')}</strong><small>{t(subnetOptions.length ? '只选择确实需要访问的 subnet router 路由。' : '本机 Tailnet 当前未发现已接受的私网路由。')}</small></div>{subnetOptions.length ? <div className="tailscale-subnet-options">{subnetOptions.map(option => <label key={`${option.peer.id}-${option.route}`}><input type="checkbox" checked={draft.subnet_routes.includes(option.route)} onChange={event => setSubnet(option.route, event.target.checked)} /><span><strong>{option.route}</strong><small>{option.peer.name}</small></span></label>)}</div> : <span className="tailscale-resource-state">{t('无需配置')}</span>}</div>
      </section>

      <section className="tailscale-form-section"><div className="tailscale-section-copy"><span>03</span><div><h3>{t('允许谁使用')}</h3><p>{t('先用这台 Mac 验证，再按需开放给已登记的下游设备。')}</p></div></div>
        <label className="tailscale-check"><input type="checkbox" checked={draft.allow_mac} onChange={event => update('allow_mac', event.target.checked)} /><span><strong>{t('这台 Mac')}</strong><small>{t('包括 TUN 流量与显式代理流量。')}</small></span></label>
        <div className="tailscale-scope-picker"><span>{t('下游设备')}</span><div className="segmented" role="group" aria-label={t('Tailscale 下游设备范围')}><button type="button" aria-pressed={deviceScope === 'none'} onClick={() => setScope('none')}>{t('不允许')}</button><button type="button" aria-pressed={deviceScope === 'selected'} onClick={() => setScope('selected')}>{t('指定设备')}</button><button type="button" aria-pressed={deviceScope === 'all'} onClick={() => setScope('all')}>{t('全部已注册')}</button></div></div>
        {deviceScope === 'selected' && <div className="tailscale-device-list">{devices.length ? devices.map(device => <label key={device.id}><input type="checkbox" checked={draft.allowed_devices.includes(device.id)} onChange={event => update('allowed_devices', event.target.checked ? [...draft.allowed_devices, device.id] : draft.allowed_devices.filter(id => id !== device.id))} /><span><strong>{device.id}</strong><small>{device.ipv4}</small></span></label>) : <p>{t('还没有已注册设备。请先在“设备”页完成注册。')}</p>}</div>}
      </section>

      <section className="tailscale-exit-card"><div className="tailscale-section-copy"><span>04</span><div><h3>{t('公网出口（可选）')}</h3><p>{t('只有明确选择后，Tailscale 才会成为设备可选的公网出口。')}</p></div></div>
        <label><span>{t('Exit Node')}</span><select aria-label="Tailscale Exit Node" value={draft.exit_node} disabled={!exitCandidates.length && !draft.exit_node} onChange={event => setExitNode(event.target.value)}><option value="">{t(exitCandidates.length ? '不使用 Exit Node' : '未发现可用 Exit Node')}</option>{draft.exit_node && !exitCandidates.some(peer => exitNodeValue(peer) === draft.exit_node) && <option value={draft.exit_node}>{t('当前手动值：{{value}}', { value: draft.exit_node })}</option>}{exitCandidates.map(peer => <option key={peer.id || peer.name} value={exitNodeValue(peer)}>{peer.name} · {t(peer.online ? '在线' : '离线')}</option>)}</select></label>
        <label className="tailscale-check"><input type="checkbox" checked={draft.exit_node_allow_lan_access} disabled={!draft.exit_node.trim()} onChange={event => update('exit_node_allow_lan_access', event.target.checked)} /><span><strong>{t('使用 Exit Node 时保留本地 LAN 访问')}</strong><small>{t('本地 NAS、打印机和路由器继续走 DIRECT。')}</small></span></label>
        {!exitCandidates.length && <small className="tailscale-exit-empty">{t('当前节点都没有发布 Exit Node；留空就是正常的 Tailnet-only 配置。')}</small>}
      </section>

      <details className="tailscale-advanced"><summary><span><strong>{t('高级手动配置')}</strong><small>{t('节点名称、Control URL、后缀、CIDR 与 Headscale')}</small></span><i aria-hidden="true">›</i></summary><div className="tailscale-advanced-body">
        <div className="tailscale-form-grid">
          <label><span>{t('界面名称')}</span><input aria-label={t('Tailscale 界面名称')} value={draft.display_name} onChange={event => update('display_name', event.target.value)} placeholder={t('例如 Home Tailnet')} /></label>
          <label><span>{t('Tailnet 节点名')}</span><input aria-label={t('Tailscale 节点名')} value={draft.hostname} onChange={event => update('hostname', event.target.value.toLowerCase())} placeholder="opensurge-mac" /><small>{t('只能使用小写字母、数字与连字符')}</small></label>
          <div className="tailscale-control-plane"><span>{t('控制平面')}</span><div className="segmented"><button type="button" aria-pressed={controlKind === 'tailscale'} onClick={() => update('control_url', 'https://controlplane.tailscale.com')}>Tailscale</button><button type="button" aria-pressed={controlKind === 'headscale'} onClick={() => controlKind === 'tailscale' && update('control_url', 'https://headscale.example.com')}>Headscale</button></div></div>
          <label className="wide"><span>Control URL</span><input aria-label="Tailscale Control URL" value={draft.control_url} onChange={event => update('control_url', event.target.value)} /></label>
        </div>
        <div className="tailscale-target-grid">
          <ListEditor label="MagicDNS 后缀" hint="例如 home-name.ts.net" placeholder="home-name.ts.net" values={draft.magic_dns_suffixes} onChange={values => update('magic_dns_suffixes', values)} />
          <ListEditor label="Tailnet 节点 IP / CIDR" hint="建议填写精确节点 IP，避免接管整个 CGNAT" placeholder="100.82.10.7" values={draft.peer_cidrs} onChange={values => update('peer_cidrs', values)} />
          <ListEditor label="远端子网" hint="只批准确实由 subnet router 发布的私网" placeholder="10.20.0.0/16" values={draft.subnet_routes} onChange={values => { update('subnet_routes', values); update('accept_routes', values.length > 0) }} />
        </div>
        <label className="tailscale-manual-exit"><span>{t('手动 Exit Node 名称或 Tailscale IP')}</span><input aria-label={t('手动 Exit Node')} value={draft.exit_node} onChange={event => setExitNode(event.target.value)} placeholder={t('留空表示仅访问 Tailnet')} /></label>
        <label className="tailscale-check"><input type="checkbox" checked={draft.accept_routes} disabled={draft.subnet_routes.length > 0} onChange={event => update('accept_routes', event.target.checked)} /><span><strong>{t('接受 Tailnet 公布的子网路由')}</strong><small>{t('OpenSurge 仍只捕获上方明确批准的子网，并会拒绝与本地 LAN 重叠的配置。')}</small></span></label>
      </div></details>
    </div>
    <div className="tailscale-dialog-footer"><label className="tailscale-enable"><input type="checkbox" checked={draft.enabled} onChange={event => update('enabled', event.target.checked)} /><span><strong>{t('保存后启用')}</strong><small>{t(draft.enabled ? gatewayActive ? '验证通过后立即重载运行中的网关' : '保存并在下次启动网关时载入' : '保存配置但不载入 Tailscale 节点')}</small></span></label><div className="tailscale-save-summary"><span><strong>{guidedSummary(draft, selectedPeerNames)}</strong><small>{t(draft.exit_node ? 'Exit Node 只加入设备出口候选，不会自动切换现有设备。' : '普通公网出口保持不变。')}</small></span><div><button type="button" disabled={busy} onClick={onCancel}>{t('取消')}</button><button className="primary" type="button" disabled={busy || !draft.display_name.trim() || !draft.hostname.trim() || !draft.control_url.trim()} onClick={onSave}>{t(busy ? '正在验证并应用…' : !draft.enabled ? '仅保存配置' : gatewayActive ? '应用并重载' : '保存，随网关启动')}</button></div></div></div>
    {error && <div className="tailscale-dialog-error" role="alert">{error}</div>}
  </dialog>
}

function DiscoveryBanner({ discovery, discovering, onRefresh }: { discovery: TailscaleDiscoveryResponse | null; discovering: boolean; onRefresh: () => void }) {
  const peerCount = discovery?.peers.length ?? 0
  const onlineCount = discovery?.peers.filter(peer => peer.online).length ?? 0
  const ready = discovery?.available && discovery.backend_state === 'Running'
  return <div className={`tailscale-discovery ${ready ? 'ready' : discovery?.available ? 'warning' : ''}`}>
    <span className="tailscale-discovery-dot" aria-hidden="true" />
    <span><strong>{t(discovering ? '正在读取本机 Tailscale…' : ready ? '已发现本机 Tailscale' : discovery?.available ? '本机 Tailscale 当前未连接' : '未发现可读取的本机 Tailscale')}</strong><small>{ready ? t('{{suffix}} · {{online}}/{{count}} 台 peer 在线', { suffix: discovery.magic_dns_suffix || discovery.tailnet_name || 'Tailnet', online: onlineCount, count: peerCount }) : t('自动发现只提供建议；你仍可继续手动配置。')}</small></span>
    <button type="button" disabled={discovering} onClick={onRefresh}>{t(discovering ? '检测中…' : '重新检测')}</button>
  </div>
}

function ListEditor({ label, hint, placeholder, values, onChange }: { label: string; hint: string; placeholder: string; values: string[]; onChange: (values: string[]) => void }) {
  const [input, setInput] = useState('')
  const add = () => {
    const candidates = input.split(/[\s,]+/).map(value => value.trim()).filter(Boolean)
    if (!candidates.length) return
    onChange(unique([...values, ...candidates]))
    setInput('')
  }
  return <div className="tailscale-list-editor"><label><span>{t(label)}</span><small>{t(hint)}</small><span className="token-entry"><input aria-label={t(label)} value={input} placeholder={t(placeholder)} onChange={event => setInput(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') { event.preventDefault(); add() } }} /><button type="button" disabled={!input.trim()} onClick={add}>{t('添加')}</button></span></label><div className="token-list">{values.map(value => <span className="token" key={value}>{value}<button type="button" aria-label={t('移除 {{value}}', { value })} onClick={() => onChange(values.filter(item => item !== value))}>×</button></span>)}</div></div>
}

function TailscaleFact({ label, value, note }: { label: string; value: string; note: string }) {
  return <div><small>{t(label)}</small><strong>{value}</strong><span>{note}</span></div>
}

function peerCIDRs(peer: TailscaleDiscoveredNode): string[] {
  return peer.tailscale_ips.map(value => `${value}/${value.includes(':') ? 128 : 32}`)
}

function normalizedSuffix(value?: string): string {
  return value?.trim().replace(/^\*?\.?/, '').replace(/\.$/, '').toLowerCase() ?? ''
}

function discoveredSubnetOptions(discovery: TailscaleDiscoveryResponse | null): Array<{ peer: TailscaleDiscoveredNode; route: string }> {
  return discovery?.peers.flatMap(peer => peer.subnet_routes.map(route => ({ peer, route }))) ?? []
}

function exitNodeValue(peer: TailscaleDiscoveredNode): string {
  return peer.dns_name || peer.tailscale_ips.find(value => !value.includes(':')) || peer.tailscale_ips[0] || peer.name
}

function unique(values: string[]): string[] {
  return [...new Set(values)]
}

function guidedSummary(draft: Draft, selectedPeerNames: string[]): string {
  const sources = []
  if (draft.allow_mac) sources.push('Mac')
  if (draft.allow_all_devices) sources.push(t('全部设备'))
  else if (draft.allowed_devices.length) sources.push(t('{{count}} 台设备', { count: draft.allowed_devices.length }))
  const targets = []
  if (selectedPeerNames.length === 1) targets.push(selectedPeerNames[0])
  else if (selectedPeerNames.length > 1) targets.push(t('{{count}} 台 Tailnet 节点', { count: selectedPeerNames.length }))
  else if (draft.peer_cidrs.length) targets.push(t('{{count}} 个节点地址', { count: draft.peer_cidrs.length }))
  if (draft.magic_dns_suffixes.length) targets.push(t('MagicDNS 名称'))
  if (draft.subnet_routes.length) targets.push(t('{{count}} 个远端子网', { count: draft.subnet_routes.length }))
  return sources.length && targets.length ? t('{{sources}} → {{targets}}', { sources: sources.join(' + '), targets: targets.join(' + ') }) : t(!sources.length ? '尚未授权使用者' : '尚未选择 Tailnet 访问目标')
}

function tailscaleStatus(data: TailscaleResponse | null) {
  if (!data) return { label: t('正在读取'), tone: '' }
  if (!data.settings.enabled) return { label: t(data.identity_present ? '已停用 · 身份保留' : '未启用'), tone: '' }
  if (data.runtime_state === 'pending_gateway_start') return { label: t('等待网关启动'), tone: '' }
  return { label: t('已载入 · 按需连接'), tone: 'ok' }
}

function runtimeLabel(data: TailscaleResponse | null) {
  if (!data) return t('正在读取配置')
  if (!data.settings.enabled) return t(data.identity_present ? '已停用，身份已保留' : 'Tailscale 未启用')
  if (data.runtime_state === 'pending_gateway_start') return t('等待网关启动后载入')
  return t(data.selectable_exit ? 'Tailnet 与 Exit Node 已载入' : 'Tailnet 出站已载入')
}

function accessSummary(settings?: TailscaleSettings) {
  if (!settings) return '—'
  const scopes = []
  if (settings.allow_mac) scopes.push('Mac')
  if (settings.allow_all_devices) scopes.push(t('全部设备'))
  else if (settings.allowed_devices.length) scopes.push(t('{{count}} 台设备', { count: settings.allowed_devices.length }))
  return scopes.join(' + ') || t('无')
}
