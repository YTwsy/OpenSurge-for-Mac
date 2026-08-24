import { useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import { t } from '../i18n'
import type { ProfileOverlay, ProfileOverlayDocument, ProfileOverlayGroupPatch, ProfileOverlayPreview, Source } from '../types'

type EditorMode = 'guided' | 'yaml'
type PreviewTab = 'changes' | 'source' | 'overlay' | 'effective' | 'final'

export function ProfileOverlayPanel({ overlay, sources, onSaved }: { overlay: ProfileOverlay | null; sources: Source[]; onSaved: (saved: ProfileOverlay) => void | Promise<void> }) {
  const [document, setDocument] = useState<ProfileOverlayDocument | null>(null)
  const [yaml, setYAML] = useState('')
  const [mode, setMode] = useState<EditorMode>('guided')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [previewSource, setPreviewSource] = useState('')
  const [preview, setPreview] = useState<ProfileOverlayPreview | null>(null)
  const [previewTab, setPreviewTab] = useState<PreviewTab>('changes')

  useEffect(() => {
    if (!overlay) return
    setDocument(structuredClone(overlay.document))
    setYAML(overlay.yaml)
    setError('')
    setMessage('')
  }, [overlay])

  const documentDirty = Boolean(document && overlay && JSON.stringify(document) !== JSON.stringify(overlay.document))
  const yamlDirty = Boolean(overlay && yaml !== overlay.yaml)
  const dirty = mode === 'guided' ? documentDirty : yamlDirty
  const sourceGroups = useMemo(() => unique(sources.flatMap(source => source.inventory?.proxy_groups ?? [])), [sources])
  const sourceTargets = useMemo(() => unique(sources.flatMap(source => [...(source.inventory?.proxies ?? []), ...(source.inventory?.proxy_groups ?? [])])), [sources])
  const sourceProviders = useMemo(() => unique(sources.flatMap(source => source.inventory?.proxy_providers ?? [])), [sources])
  const compatible = sources.filter(source => source.overlay_compatible !== false).length

  if (!overlay || !document) {
    return <section className="section profile-overlay-panel"><div className="profile-overlay-loading"><span className="button-spinner" aria-hidden="true" />{t('正在读取全局附加配置…')}</div></section>
  }

  const update = (change: (current: ProfileOverlayDocument) => void) => {
    setDocument(current => {
      if (!current) return current
      const next = structuredClone(current)
      change(next)
      return next
    })
    setMessage('')
  }

  const save = async () => {
    setBusy(true)
    setError('')
    setMessage('')
    try {
      const saved = mode === 'guided'
        ? await api.saveProfileOverlayDocument(document, overlay.revision)
        : await api.saveProfileOverlayYAML(yaml, overlay.revision)
      setDocument(structuredClone(saved.document))
      setYAML(saved.yaml)
      setMessage(t(saved.document.enabled ? '附加配置草稿已保存。请在下方选择来源并应用，运行网关才会改变。' : '附加配置已停用并保存；如果运行版本曾使用它，请重新应用来源以移除附加内容。'))
      await onSaved(saved)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setBusy(false)
    }
  }

  const openPreview = async () => {
    if (!previewSource) return
    setBusy(true)
    setError('')
    try {
      const result = await api.sourcePreview(previewSource)
      setPreview(result)
      setPreviewTab('changes')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setBusy(false)
    }
  }

  const changeMode = (next: EditorMode) => {
    if (next === mode) return
    if (dirty) {
      setError(t('请先保存或撤销当前编辑，再切换编辑模式。'))
      return
    }
    setMode(next)
    setError('')
  }

  const reset = () => {
    setDocument(structuredClone(overlay.document))
    setYAML(overlay.yaml)
    setError('')
    setMessage('')
  }

  const status = t(!document.enabled ? '未启用' : !overlay.desired ? '草稿待应用' : overlay.applied ? '运行中' : '下次启动生效')
  const statusTone = !document.enabled ? 'off' : !overlay.desired ? 'draft' : overlay.applied ? 'live' : 'ready'

  return <>
    <section className={`section profile-overlay-panel ${document.enabled ? 'enabled' : ''}`}>
      <header className="profile-overlay-hero">
        <div className="profile-overlay-symbol" aria-hidden="true">⌘</div>
        <div><small>GLOBAL EXTENSION</small><h2>{t('全局附加配置')}</h2><p>{t('订阅刷新后自动重新组合，但不会自动重载网关。OpenSurge 始终保留 DNS、TUN、Controller 与 LAN 的所有权。')}</p></div>
        <div className="profile-overlay-controls">
          <span className={`overlay-status ${statusTone}`}>{status}</span>
          <button className={`overlay-switch ${document.enabled ? 'on' : ''}`} type="button" role="switch" aria-checked={document.enabled} onClick={() => update(current => { current.enabled = !current.enabled })}><i aria-hidden="true" /><span>{t(document.enabled ? '已启用' : '已停用')}</span></button>
        </div>
      </header>

      <div className="overlay-summary" aria-label={t('附加配置摘要')}>
        <OverlayMetric value={document.rules.prepend.length} label="前置规则" />
        <OverlayMetric value={document.rules.append_before_match.length} label="尾部规则" />
        <OverlayMetric value={document.proxies.add.length} label="新增节点" />
        <OverlayMetric value={document.proxy_groups.patch.length + document.proxy_groups.add.length} label="策略组变化" />
        <OverlayMetric value={Object.keys(document.proxy_providers.add).length + Object.keys(document.rule_providers.add).length} label="新增 Provider" />
        <OverlayMetric value={`${compatible}/${sources.length}`} label="来源兼容" />
      </div>

      <div className="overlay-editor-toolbar">
        <div className="segmented-control" aria-label={t('附加配置编辑模式')}>
          <button type="button" className={mode === 'guided' ? 'active' : ''} aria-pressed={mode === 'guided'} onClick={() => changeMode('guided')}>{t('引导编辑')}</button>
          <button type="button" className={mode === 'yaml' ? 'active' : ''} aria-pressed={mode === 'yaml'} onClick={() => changeMode('yaml')}>{t('高级 YAML')}</button>
        </div>
        <small>{t(mode === 'guided' ? '常见操作会生成显式、可审计的附加指令。' : '支持 add / replace / patch；受保护字段会在保存时拒绝。')}</small>
      </div>

      {mode === 'guided' ? <div className="overlay-guided-editor">
        <RuleOperationsEditor value={document.rules} onChange={rules => update(current => { current.rules = rules })} />
        <ManualProxyEditor proxies={document.proxies.add} onChange={proxies => update(current => { current.proxies.add = proxies })} />
        <ProviderOperationsEditor title="代理 Provider" kind="proxy" providers={document.proxy_providers.add} onChange={providers => update(current => { current.proxy_providers.add = providers })} />
        <ProviderOperationsEditor title="规则 Provider" kind="rule" providers={document.rule_providers.add} onChange={providers => update(current => { current.rule_providers.add = providers })} />
        <ProxyGroupOperationsEditor additions={document.proxy_groups.add} patches={document.proxy_groups.patch} sourceGroups={sourceGroups} sourceTargets={sourceTargets} sourceProviders={sourceProviders} onAdditionsChange={groups => update(current => { current.proxy_groups.add = groups })} onPatchesChange={patches => update(current => { current.proxy_groups.patch = patches })} />
        <DNSOperationsEditor document={document} onChange={dns => update(current => { current.dns = dns })} />
      </div> : <div className="overlay-yaml-editor"><label htmlFor="profile-overlay-yaml">{t('附加配置 YAML')}</label><textarea id="profile-overlay-yaml" spellCheck={false} rows={24} value={yaml} onChange={event => { setYAML(event.target.value); setMessage('') }} /><p><strong>{t('边界：')}</strong>{t('不能设置 mixed-port、TUN、Controller、LAN binding，也不能覆盖 DNS 的监听、IPv6、模式或 Fake-IP 地址段。')}</p></div>}

      <div className="overlay-source-compatibility">
        <div><strong>{t('来源兼容性')}</strong><small>{t('保存草稿后，每个订阅独立检查名称冲突和策略组引用。')}</small></div>
        <div className="compatibility-chips">{sources.length ? sources.map(source => <span key={source.id} className={source.overlay_compatible === false ? 'bad' : 'ok'} title={source.overlay_validation ? t(source.overlay_validation) : undefined}>{source.overlay_compatible === false ? '!' : '✓'} {source.name}</span>) : <span className="muted">{t('导入来源后可检查')}</span>}</div>
      </div>

      <div className="overlay-preview-row">
        <label>{t('最终配置预览')}<select aria-label={t('选择要预览的来源')} value={previewSource} onChange={event => setPreviewSource(event.target.value)}><option value="">{t('选择一个来源')}</option>{sources.map(source => <option key={source.id} value={source.id}>{source.name}{source.overlay_compatible === false ? t(' · 存在冲突') : ''}</option>)}</select></label>
        <button type="button" disabled={busy || !previewSource || dirty} onClick={() => void openPreview()}>{t('查看组合结果')}</button>
        {dirty ? <small>{t('保存草稿后才能生成权威预览。')}</small> : <small>{t('预览包含原始来源、附加层、有效来源和 OpenSurge 最终配置。')}</small>}
      </div>

      {error ? <div className="overlay-feedback error" role="alert"><span aria-hidden="true">!</span><div><strong>{t('附加配置未完成')}</strong><p>{error}</p></div></div> : null}
      {message ? <div className="overlay-feedback success" role="status"><span aria-hidden="true">✓</span><div><strong>{t('草稿已保存')}</strong><p>{message}</p></div></div> : null}

      <footer className="overlay-save-bar"><span className={dirty ? 'dirty' : ''}><i aria-hidden="true">{dirty ? '•' : '✓'}</i>{dirty ? t('有未保存的附加配置修改') : t(overlay.validation)}</span><div><button type="button" disabled={busy || !dirty} onClick={reset}>{t('撤销修改')}</button><button className="primary" type="button" disabled={busy || !dirty} onClick={() => void save()}>{busy ? <><span className="button-spinner" aria-hidden="true" />{t('正在保存…')}</> : t('保存附加配置草稿')}</button></div></footer>
    </section>
    {preview ? <ProfileOverlayPreviewDialog preview={preview} tab={previewTab} onTabChange={setPreviewTab} onClose={() => setPreview(null)} /> : null}
  </>
}

function OverlayMetric({ value, label }: { value: number | string; label: string }) {
  return <span><strong>{value}</strong><small>{t(label)}</small></span>
}

function RuleOperationsEditor({ value, onChange }: { value: ProfileOverlayDocument['rules']; onChange: (value: ProfileOverlayDocument['rules']) => void }) {
  return <details className="overlay-editor-section" open><summary><span>01</span><div><strong>{t('规则附写')}</strong><small>{t('前置规则优先于订阅；尾部规则固定插入最终 MATCH 之前。')}</small></div><i aria-hidden="true">⌄</i></summary><div className="overlay-section-body two-columns"><label>{t('订阅规则之前')}<textarea aria-label={t('全局前置规则')} rows={7} placeholder={'DOMAIN-SUFFIX,example.com,DIRECT\nRULE-SET,private,DIRECT'} value={lines(value.prepend)} onChange={event => onChange({ ...value, prepend: parseLines(event.target.value) })} /></label><label>{t('最终 MATCH 之前')}<textarea aria-label={t('全局尾部规则')} rows={7} placeholder="DOMAIN,internal.example,Proxy" value={lines(value.append_before_match)} onChange={event => onChange({ ...value, append_before_match: parseLines(event.target.value) })} /></label></div></details>
}

function ManualProxyEditor({ proxies, onChange }: { proxies: Array<Record<string, unknown>>; onChange: (proxies: Array<Record<string, unknown>>) => void }) {
  const [name, setName] = useState('')
  const [type, setType] = useState('http')
  const [server, setServer] = useState('')
  const [port, setPort] = useState('')
  const [credential, setCredential] = useState('')
  const [password, setPassword] = useState('')
  const parsedPort = Number(port)
  const canAdd = Boolean(name.trim() && server.trim() && Number.isInteger(parsedPort) && parsedPort >= 1 && parsedPort <= 65535 && (type !== 'vless' || credential.trim()))
  const add = () => {
    if (!canAdd) return
    const proxy: Record<string, unknown> = { name: name.trim(), type, server: server.trim(), port: parsedPort }
    if (type === 'vless') proxy.uuid = credential.trim()
    else if (credential.trim()) proxy.username = credential.trim()
    if (password) passwordField(proxy, type, password)
    if (type === 'socks5') proxy.udp = true
    onChange([...proxies, proxy])
    setName(''); setServer(''); setPort(''); setCredential(''); setPassword('')
  }
  return <details className="overlay-editor-section"><summary><span>02</span><div><strong>{t('新增手工节点')}</strong><small>{t('引导添加 HTTP、SOCKS5 或基础 VLESS；复杂传输参数可在高级 YAML 中补充。')}</small></div><i aria-hidden="true">⌄</i></summary><div className="overlay-section-body"><ResourceList resources={proxies} onRemove={index => onChange(proxies.filter((_, current) => current !== index))} empty="尚未新增节点" /><div className="overlay-resource-form proxy"><label>{t('名称')}<input aria-label={t('附加节点名称')} value={name} onChange={event => setName(event.target.value)} placeholder={t('例如 LAN-Proxy')} /></label><label>{t('类型')}<select aria-label={t('附加节点类型')} value={type} onChange={event => setType(event.target.value)}><option value="http">HTTP</option><option value="socks5">SOCKS5</option><option value="vless">VLESS</option></select></label><label>{t('服务器')}<input aria-label={t('附加节点服务器')} value={server} onChange={event => setServer(event.target.value)} placeholder="192.168.1.10" /></label><label>{t('端口')}<input aria-label={t('附加节点端口')} inputMode="numeric" value={port} onChange={event => setPort(event.target.value)} placeholder="1080" /></label><label>{t(type === 'vless' ? 'UUID' : '用户名（可选）')}<input aria-label={t('附加节点凭据')} value={credential} onChange={event => setCredential(event.target.value)} /></label><label>{t(type === 'vless' ? 'Flow（可选）' : '密码（可选）')}<input aria-label={t('附加节点密码或 Flow')} type={type === 'vless' ? 'text' : 'password'} autoComplete="new-password" value={password} onChange={event => setPassword(event.target.value)} /></label><button type="button" disabled={!canAdd} onClick={add}>{t('添加节点')}</button></div></div></details>
}

function ProviderOperationsEditor({ title, kind, providers, onChange }: { title: string; kind: 'proxy' | 'rule'; providers: Record<string, Record<string, unknown>>; onChange: (providers: Record<string, Record<string, unknown>>) => void }) {
  const [name, setName] = useState('')
  const [url, setURL] = useState('')
  const [behavior, setBehavior] = useState('domain')
  const canAdd = Boolean(name.trim() && url.startsWith('https://'))
  const add = () => {
    if (!canAdd) return
    const value: Record<string, unknown> = { type: 'http', url, path: `./providers/${name.trim()}.yaml`, interval: 86400 }
    if (kind === 'rule') value.behavior = behavior
    onChange({ ...providers, [name.trim()]: value })
    setName(''); setURL('')
  }
  return <details className="overlay-editor-section"><summary><span>{kind === 'proxy' ? '03' : '04'}</span><div><strong>{t(title)}</strong><small>{t('新增 HTTPS Provider；同名覆盖必须在高级 YAML 中显式使用 replace。')}</small></div><i aria-hidden="true">⌄</i></summary><div className="overlay-section-body"><ProviderList providers={providers} onRemove={key => { const next = { ...providers }; delete next[key]; onChange(next) }} /><div className={`overlay-resource-form provider ${kind}`}><label>{t('名称')}<input aria-label={t('{{title}}名称', { title: t(title) })} value={name} onChange={event => setName(event.target.value)} placeholder={kind === 'proxy' ? 'Airport' : 'private-domains'} /></label>{kind === 'rule' ? <label>{t('规则类型')}<select aria-label={t('附加规则 Provider 类型')} value={behavior} onChange={event => setBehavior(event.target.value)}><option value="domain">{t('域名')}</option><option value="ipcidr">IP CIDR</option><option value="classical">{t('经典规则')}</option></select></label> : null}<label className="wide">{t('HTTPS 地址')}<input aria-label={t('{{title}}地址', { title: t(title) })} value={url} onChange={event => setURL(event.target.value)} placeholder="https://example.com/provider.yaml" /></label><button type="button" disabled={!canAdd} onClick={add}>{t('添加 Provider')}</button></div></div></details>
}

function ProxyGroupOperationsEditor({ additions, patches, sourceGroups, sourceTargets, sourceProviders, onAdditionsChange, onPatchesChange }: { additions: Array<Record<string, unknown>>; patches: ProfileOverlayGroupPatch[]; sourceGroups: string[]; sourceTargets: string[]; sourceProviders: string[]; onAdditionsChange: (groups: Array<Record<string, unknown>>) => void; onPatchesChange: (patches: ProfileOverlayGroupPatch[]) => void }) {
  const [newName, setNewName] = useState('')
  const [newMembers, setNewMembers] = useState('DIRECT')
  const [patchName, setPatchName] = useState('')
  const [patchMembers, setPatchMembers] = useState('')
  const [patchProviders, setPatchProviders] = useState('')
  const canAddGroup = Boolean(newName.trim() && parseLines(newMembers).length)
  const canAddPatch = Boolean(patchName && parseLines(patchMembers).length + parseLines(patchProviders).length)
  const addGroup = () => {
    if (!canAddGroup) return
    onAdditionsChange([...additions, { name: newName.trim(), type: 'select', proxies: parseLines(newMembers) }])
    setNewName(''); setNewMembers('DIRECT')
  }
  const addPatch = () => {
    const appendProxies = parseLines(patchMembers)
    const appendUse = parseLines(patchProviders)
    if (!canAddPatch) return
    onPatchesChange([...patches, { name: patchName, append_proxies: appendProxies, append_use: appendUse }])
    setPatchName(''); setPatchMembers(''); setPatchProviders('')
  }
  return <details className="overlay-editor-section"><summary><span>05</span><div><strong>{t('策略组扩展')}</strong><small>{t('可以创建新 Select 组，或向订阅已有组追加节点和 Provider。')}</small></div><i aria-hidden="true">⌄</i></summary><div className="overlay-section-body group-operations"><div className="group-operation-column"><h4>{t('创建新策略组')}</h4><ResourceList resources={additions} onRemove={index => onAdditionsChange(additions.filter((_, current) => current !== index))} empty="尚未创建新组" /><label>{t('组名')}<input aria-label={t('新增策略组名称')} value={newName} onChange={event => setNewName(event.target.value)} placeholder="Manual" /></label><label>{t('候选成员（每行一个）')}<textarea aria-label={t('新增策略组候选')} rows={5} value={newMembers} onChange={event => setNewMembers(event.target.value)} placeholder={['DIRECT', ...sourceTargets.slice(0, 2)].join('\n')} /></label><button type="button" disabled={!canAddGroup} onClick={addGroup}>{t('创建 Select 组')}</button></div><div className="group-operation-column"><h4>{t('扩展订阅已有组')}</h4>{patches.length ? <ul className="overlay-patch-list">{patches.map((patch, index) => <li key={`${patch.name}-${index}`}><span><strong>{patch.name}</strong><small>{t('节点 +{{nodes}} · Provider +{{providers}}', { nodes: patch.append_proxies.length, providers: patch.append_use.length })}</small></span><button type="button" aria-label={t('移除 {{name}} 策略组扩展', { name: patch.name })} onClick={() => onPatchesChange(patches.filter((_, current) => current !== index))}>{t('移除')}</button></li>)}</ul> : <p className="overlay-empty">{t('尚未扩展订阅组')}</p>}<label>{t('目标策略组')}<select aria-label={t('要扩展的订阅策略组')} value={patchName} onChange={event => setPatchName(event.target.value)}><option value="">{t('选择策略组')}</option>{sourceGroups.map(group => <option key={group}>{group}</option>)}</select></label><label>{t('追加节点/策略组（每行一个）')}<textarea aria-label={t('策略组追加候选')} rows={4} value={patchMembers} onChange={event => setPatchMembers(event.target.value)} placeholder={sourceTargets.slice(0, 3).join('\n')} /></label><label>{t('追加 Proxy Provider（每行一个）')}<textarea aria-label={t('策略组追加 Provider')} rows={3} value={patchProviders} onChange={event => setPatchProviders(event.target.value)} placeholder={sourceProviders.slice(0, 2).join('\n')} /></label><button type="button" disabled={!canAddPatch} onClick={addPatch}>{t('添加组扩展')}</button></div></div></details>
}

function DNSOperationsEditor({ document, onChange }: { document: ProfileOverlayDocument; onChange: (dns: ProfileOverlayDocument['dns']) => void }) {
  const filters = (document.dns.append['fake-ip-filter'] ?? []).filter((value): value is string => typeof value === 'string')
  const nameservers = (document.dns.append.nameserver ?? []).filter((value): value is string => typeof value === 'string')
  return <details className="overlay-editor-section"><summary><span>06</span><div><strong>{t('DNS 保留字段')}</strong><small>{t('只附加解析与过滤策略；监听、模式、IPv6 与 Fake-IP 地址段不可修改。')}</small></div><i aria-hidden="true">⌄</i></summary><div className="overlay-section-body two-columns"><label>{t('追加 Fake-IP Filter')}<textarea aria-label={t('追加 Fake-IP Filter')} rows={6} value={lines(filters)} onChange={event => onChange({ ...document.dns, append: { ...document.dns.append, 'fake-ip-filter': parseLines(event.target.value) } })} placeholder={'+.lan\n+.local'} /></label><label>{t('追加 Nameserver')}<textarea aria-label={t('追加 Nameserver')} rows={6} value={lines(nameservers)} onChange={event => onChange({ ...document.dns, append: { ...document.dns.append, nameserver: parseLines(event.target.value) } })} placeholder={'https://dns.alidns.com/dns-query\nhttps://1.1.1.1/dns-query'} /></label></div></details>
}

function ResourceList({ resources, onRemove, empty }: { resources: Array<Record<string, unknown>>; onRemove: (index: number) => void; empty: string }) {
  if (!resources.length) return <p className="overlay-empty">{t(empty)}</p>
  return <ul className="overlay-resource-list">{resources.map((resource, index) => <li key={`${String(resource.name)}-${index}`}><span><strong>{String(resource.name || t('未命名'))}</strong><small>{String(resource.type || 'resource')}{resource.server ? ` · ${String(resource.server)}:${String(resource.port ?? '')}` : ''}</small></span><button type="button" aria-label={t('移除 {{name}}', { name: String(resource.name) })} onClick={() => onRemove(index)}>{t('移除')}</button></li>)}</ul>
}

function ProviderList({ providers, onRemove }: { providers: Record<string, Record<string, unknown>>; onRemove: (key: string) => void }) {
  const entries = Object.entries(providers)
  if (!entries.length) return <p className="overlay-empty">{t('尚未新增 Provider')}</p>
  return <ul className="overlay-resource-list">{entries.map(([name, provider]) => <li key={name}><span><strong>{name}</strong><small>{String(provider.behavior ?? 'proxy')} · {String(provider.url ?? '')}</small></span><button type="button" aria-label={t('移除 {{name}}', { name })} onClick={() => onRemove(name)}>{t('移除')}</button></li>)}</ul>
}

function ProfileOverlayPreviewDialog({ preview, tab, onTabChange, onClose }: { preview: ProfileOverlayPreview; tab: PreviewTab; onTabChange: (tab: PreviewTab) => void; onClose: () => void }) {
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [onClose])
  const content = tab === 'source' ? preview.source_yaml : tab === 'overlay' ? preview.overlay_yaml : tab === 'effective' ? preview.effective_profile_yaml : preview.final_mihomo_yaml
  return <dialog className="profile-overlay-preview-dialog" open aria-modal="true" aria-labelledby="overlay-preview-title"><header><div><small>COMPOSITION PREVIEW</small><h2 id="overlay-preview-title">{t('最终配置预览')}</h2><p>{t(preview.validation)}</p></div><button type="button" autoFocus aria-label={t('关闭最终配置预览')} onClick={onClose}>×</button></header><nav aria-label={t('配置预览层级')}><button type="button" className={tab === 'changes' ? 'active' : ''} onClick={() => onTabChange('changes')}>{t('变化摘要')}</button><button type="button" className={tab === 'source' ? 'active' : ''} onClick={() => onTabChange('source')}>{t('原始来源')}</button><button type="button" className={tab === 'overlay' ? 'active' : ''} onClick={() => onTabChange('overlay')}>{t('附加配置')}</button><button type="button" className={tab === 'effective' ? 'active' : ''} onClick={() => onTabChange('effective')}>{t('有效来源')}</button><button type="button" className={tab === 'final' ? 'active' : ''} onClick={() => onTabChange('final')}>{t('最终 mihomo')}</button></nav>{tab === 'changes' ? <div className="overlay-preview-changes"><OverlayMetric value={`+${preview.diff.proxies_added.length}/-${preview.diff.proxies_removed.length}`} label="节点" /><OverlayMetric value={`+${preview.diff.groups_added.length}/-${preview.diff.groups_removed.length}`} label="策略组" /><OverlayMetric value={signed(preview.diff.rule_count_delta)} label="规则" /><OverlayMetric value={preview.effective_inventory.rule_count} label="最终规则数" /><div><strong>{t('组合顺序')}</strong><p>{t('OpenSurge 系统与设备规则 → 附加前置规则 → 订阅规则 → 附加尾部规则 → 最终 MATCH')}</p></div></div> : <pre tabIndex={0}>{content}</pre>}<footer><span>{t('预览可能包含节点参数，仅通过本机认证控制面展示。')}</span><button type="button" onClick={onClose}>{t('完成')}</button></footer></dialog>
}

function passwordField(proxy: Record<string, unknown>, type: string, value: string) {
  if (type === 'vless') proxy.flow = value
  else proxy.password = value
}

function lines(values: string[]) {
  return values.join('\n')
}

function parseLines(value: string) {
  return unique(value.split('\n').map(line => line.trim()).filter(Boolean))
}

function unique(values: string[]) {
  return [...new Set(values)]
}

function signed(value: number) {
  return value >= 0 ? `+${value}` : String(value)
}
