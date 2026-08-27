import { useEffect, useState } from 'react'
import { api } from '../api'
import { t } from '../i18n'
import { parseProxyShareLinks, type ProxyShareLinkError } from '../proxyShareLinks'
import type { ProfileOverlay, ProfileOverlayDocument, ProfileOverlayPreview, Source } from '../types'

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
  const compatible = sources.filter(source => source.overlay_compatible !== false).length
  const expertOperations = document ? expertOperationCount(document) : 0

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
      setMessage(t(saved.document.enabled ? '附加配置草稿已保存。请在来源卡片应用，运行网关才会改变。' : '附加配置已停用并保存；如果运行版本曾使用它，请重新应用来源以移除附加内容。'))
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
    <details className={`section profile-overlay-panel ${document.enabled ? 'enabled' : ''}`}>
      <summary className="profile-overlay-hero">
        <div className="profile-overlay-symbol" aria-hidden="true">⌘</div>
        <div><small>ADVANCED · GLOBAL EXTENSION</small><h2>{t('高级：全局附加配置')}</h2><p>{t('默认折叠；只在需要跨订阅保留个人规则或节点时启用。')}</p></div>
        <div className="profile-overlay-controls"><span className={`overlay-status ${statusTone}`}>{status}</span><i aria-hidden="true">⌄</i></div>
      </summary>

      <div className="profile-overlay-body">
        <div className="overlay-enable-row">
          <div><strong>{t('全局应用')}</strong><small>{t('保存只产生草稿；订阅刷新会重新检查，应用来源后才改变网关。')}</small></div>
          <button className={`overlay-switch ${document.enabled ? 'on' : ''}`} type="button" role="switch" aria-checked={document.enabled} onClick={() => update(current => { current.enabled = !current.enabled })}><i aria-hidden="true" /><span>{t(document.enabled ? '已启用' : '已停用')}</span></button>
        </div>

        <div className="overlay-summary" aria-label={t('附加配置摘要')}>
          <OverlayMetric value={document.rules.prepend.length} label="自定义规则" />
          <OverlayMetric value={document.proxies.add.length} label="自定义节点" />
          <OverlayMetric value={expertOperations} label="专家操作" />
          <OverlayMetric value={`${compatible}/${sources.length}`} label="来源兼容" />
        </div>

        {mode === 'guided' ? <div className="overlay-guided-editor">
          <RuleOperationsEditor value={document.rules.prepend} onChange={prepend => update(current => { current.rules.prepend = prepend })} />
          <ManualProxyEditor proxies={document.proxies.add} onChange={proxies => update(current => { current.proxies.add = proxies })} />
        </div> : null}

        <section className={`overlay-expert-editor ${mode === 'yaml' ? 'open' : ''}`}>
          <button className="overlay-expert-toggle" type="button" aria-expanded={mode === 'yaml'} onClick={() => changeMode(mode === 'yaml' ? 'guided' : 'yaml')}>
            <span><strong>{t('专家 Overlay YAML')}</strong><small>{t(expertOperations ? '当前包含 {{count}} 项专家操作。Provider、策略组、DNS 和低优先级规则在此维护。' : 'Provider、策略组、DNS 和低优先级规则仅在此维护。', { count: expertOperations })}</small></span><i aria-hidden="true">⌄</i>
          </button>
          {mode === 'yaml' ? <div className="overlay-yaml-editor"><label htmlFor="profile-overlay-yaml">{t('附加配置 YAML')}</label><textarea id="profile-overlay-yaml" spellCheck={false} rows={24} value={yaml} onChange={event => { setYAML(event.target.value); setMessage('') }} /><p><strong>{t('边界：')}</strong>{t('不能设置 mixed-port、TUN、Controller、LAN binding，也不能覆盖 DNS 的监听、IPv6、模式或 Fake-IP 地址段。')}</p></div> : null}
        </section>

        <div className="overlay-source-compatibility">
          <div><strong>{t('来源兼容性')}</strong><small>{t('保存草稿后，每个订阅独立检查名称冲突和高级引用。')}</small></div>
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
      </div>
    </details>
    {preview ? <ProfileOverlayPreviewDialog preview={preview} tab={previewTab} onTabChange={setPreviewTab} onClose={() => setPreview(null)} /> : null}
  </>
}

function OverlayMetric({ value, label }: { value: number | string; label: string }) {
  return <span><strong>{value}</strong><small>{t(label)}</small></span>
}

function RuleOperationsEditor({ value, onChange }: { value: string[]; onChange: (value: string[]) => void }) {
  return <section className="overlay-editor-section open"><header><span>01</span><div><strong>{t('高优先级自定义规则')}</strong><small>{t('固定插入订阅规则之前，先命中的规则优先生效。')}</small></div></header><div className="overlay-section-body"><label>{t('自定义规则（每行一条）')}<textarea aria-label={t('高优先级自定义规则')} rows={7} placeholder={'DOMAIN-SUFFIX,example.com,DIRECT\nRULE-SET,private,DIRECT'} value={lines(value)} onChange={event => onChange(parseLines(event.target.value))} /></label></div></section>
}

function ManualProxyEditor({ proxies, onChange }: { proxies: Array<Record<string, unknown>>; onChange: (proxies: Array<Record<string, unknown>>) => void }) {
  const [shareLinks, setShareLinks] = useState('')
  const [shareError, setShareError] = useState('')
  const [name, setName] = useState('')
  const [type, setType] = useState('http')
  const [server, setServer] = useState('')
  const [port, setPort] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const parsedPort = Number(port)
  const canAddManual = Boolean(name.trim() && server.trim() && Number.isInteger(parsedPort) && parsedPort >= 1 && parsedPort <= 65535)

  const addShareLinks = () => {
    const result = parseProxyShareLinks(shareLinks)
    if (result.errors.length) {
      setShareError(result.errors.map(shareLinkErrorMessage).join('\n'))
      return
    }
    const existing = new Set(proxies.map(proxy => String(proxy.name ?? '')))
    const duplicate = result.proxies.find(proxy => existing.has(String(proxy.name ?? '')))
    if (duplicate) {
      setShareError(t('节点名称 {{name}} 已存在。', { name: String(duplicate.name) }))
      return
    }
    const incoming = new Set<string>()
    const repeated = result.proxies.find(proxy => {
      const candidate = String(proxy.name ?? '')
      if (incoming.has(candidate)) return true
      incoming.add(candidate)
      return false
    })
    if (repeated) {
      setShareError(t('分享链接中包含重复节点名称 {{name}}。', { name: String(repeated.name) }))
      return
    }
    onChange([...proxies, ...result.proxies])
    setShareLinks('')
    setShareError('')
  }

  const addManual = () => {
    if (!canAddManual) return
    const proxy: Record<string, unknown> = { name: name.trim(), type, server: server.trim(), port: parsedPort }
    if (username.trim()) proxy.username = username.trim()
    if (password) proxy.password = password
    if (type === 'socks5') proxy.udp = true
    onChange([...proxies, proxy])
    setName(''); setServer(''); setPort(''); setUsername(''); setPassword('')
  }

  return <section className="overlay-editor-section open"><header><span>02</span><div><strong>{t('自定义节点')}</strong><small>{t('优先粘贴分享链接；这里只添加节点，不会创建新的订阅或 YAML 配置来源。')}</small></div></header><div className="overlay-section-body">
    <ResourceList resources={proxies} onRemove={index => onChange(proxies.filter((_, current) => current !== index))} empty="尚未新增节点" />
    <div className="overlay-share-form"><label>{t('节点分享链接（每行一条）')}<textarea aria-label={t('节点分享链接')} rows={5} value={shareLinks} onChange={event => { setShareLinks(event.target.value); setShareError('') }} placeholder={'ss://…\nvmess://…\nvless://…\ntrojan://…\nhysteria2://…'} /></label><div><small>{t('支持 SS、VMess、VLESS、Trojan、Hysteria2、HTTP 和 SOCKS5 分享链接。复杂或非标准节点可使用专家 Overlay YAML。')}</small><button type="button" disabled={!shareLinks.trim()} onClick={addShareLinks}>{t('解析并添加')}</button></div>{shareError ? <p className="field-error multiline" role="alert">{shareError}</p> : null}</div>
    <div className="overlay-manual-proxy"><h4>{t('手工添加 HTTP / SOCKS5')}</h4><div className="overlay-resource-form proxy"><label>{t('名称')}<input aria-label={t('附加节点名称')} value={name} onChange={event => setName(event.target.value)} placeholder={t('例如 LAN-Proxy')} /></label><label>{t('类型')}<select aria-label={t('附加节点类型')} value={type} onChange={event => setType(event.target.value)}><option value="http">HTTP</option><option value="socks5">SOCKS5</option></select></label><label>{t('服务器')}<input aria-label={t('附加节点服务器')} value={server} onChange={event => setServer(event.target.value)} placeholder="192.168.1.10" /></label><label>{t('端口')}<input aria-label={t('附加节点端口')} inputMode="numeric" value={port} onChange={event => setPort(event.target.value)} placeholder="1080" /></label><label>{t('用户名（可选）')}<input aria-label={t('附加节点凭据')} value={username} onChange={event => setUsername(event.target.value)} /></label><label>{t('密码（可选）')}<input aria-label={t('附加节点密码')} type="password" autoComplete="new-password" value={password} onChange={event => setPassword(event.target.value)} /></label><button type="button" disabled={!canAddManual} onClick={addManual}>{t('添加节点')}</button></div></div>
  </div></section>
}

function ResourceList({ resources, onRemove, empty }: { resources: Array<Record<string, unknown>>; onRemove: (index: number) => void; empty: string }) {
  if (!resources.length) return <p className="overlay-empty">{t(empty)}</p>
  return <ul className="overlay-resource-list">{resources.map((resource, index) => <li key={`${String(resource.name)}-${index}`}><span><strong>{String(resource.name || t('未命名'))}</strong><small>{String(resource.type || 'resource')}{resource.server ? ` · ${String(resource.server)}:${String(resource.port ?? '')}` : ''}</small></span><button type="button" aria-label={t('移除 {{name}}', { name: String(resource.name) })} onClick={() => onRemove(index)}>{t('移除')}</button></li>)}</ul>
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
  return <dialog className="profile-overlay-preview-dialog" open aria-modal="true" aria-labelledby="overlay-preview-title"><header><div><small>COMPOSITION PREVIEW</small><h2 id="overlay-preview-title">{t('最终配置预览')}</h2><p>{t(preview.validation)}</p></div><button type="button" autoFocus aria-label={t('关闭最终配置预览')} onClick={onClose}>×</button></header><nav aria-label={t('配置预览层级')}><button type="button" className={tab === 'changes' ? 'active' : ''} onClick={() => onTabChange('changes')}>{t('变化摘要')}</button><button type="button" className={tab === 'source' ? 'active' : ''} onClick={() => onTabChange('source')}>{t('原始来源')}</button><button type="button" className={tab === 'overlay' ? 'active' : ''} onClick={() => onTabChange('overlay')}>{t('附加配置')}</button><button type="button" className={tab === 'effective' ? 'active' : ''} onClick={() => onTabChange('effective')}>{t('有效来源')}</button><button type="button" className={tab === 'final' ? 'active' : ''} onClick={() => onTabChange('final')}>{t('最终 mihomo')}</button></nav>{tab === 'changes' ? <div className="overlay-preview-changes"><OverlayMetric value={`+${preview.diff.proxies_added.length}/-${preview.diff.proxies_removed.length}`} label="节点" /><OverlayMetric value={`+${preview.diff.groups_added.length}/-${preview.diff.groups_removed.length}`} label="策略组" /><OverlayMetric value={signed(preview.diff.rule_count_delta)} label="规则" /><OverlayMetric value={preview.effective_inventory.rule_count} label="最终规则数" /><div><strong>{t('组合顺序')}</strong><p>{t('OpenSurge 系统与设备规则 → 高优先级自定义规则 → 订阅规则 → 专家低优先级规则 → 最终 MATCH')}</p></div></div> : <pre tabIndex={0}>{content}</pre>}<footer><span>{t('预览可能包含节点参数，仅通过本机认证控制面展示。')}</span><button type="button" onClick={onClose}>{t('完成')}</button></footer></dialog>
}

function shareLinkErrorMessage(error: ProxyShareLinkError) {
  if (error.reason === 'unsupported') return t('第 {{line}} 行：不支持 {{scheme}} 分享链接。', { line: error.line, scheme: error.scheme || t('未知协议') })
  if (error.reason === 'missing_server') return t('第 {{line}} 行：分享链接缺少服务器。', { line: error.line })
  if (error.reason === 'missing_port') return t('第 {{line}} 行：分享链接缺少有效端口。', { line: error.line })
  if (error.reason === 'missing_credentials') return t('第 {{line}} 行：分享链接缺少凭据。', { line: error.line })
  return t('第 {{line}} 行：分享链接格式无效。', { line: error.line })
}

function expertOperationCount(document: ProfileOverlayDocument) {
  return document.rules.append_before_match.length
    + document.proxies.replace.length
    + Object.keys(document.proxy_providers.add).length
    + Object.keys(document.proxy_providers.replace).length
    + document.proxy_groups.add.length
    + document.proxy_groups.replace.length
    + document.proxy_groups.patch.length
    + Object.keys(document.rule_providers.add).length
    + Object.keys(document.rule_providers.replace).length
    + Object.keys(document.dns.merge).length
    + Object.keys(document.dns.append).length
}

function lines(values: string[]) {
  return values.join('\n')
}

function parseLines(value: string) {
  return [...new Set(value.split('\n').map(line => line.trim()).filter(Boolean))]
}

function signed(value: number) {
  return value >= 0 ? `+${value}` : String(value)
}
