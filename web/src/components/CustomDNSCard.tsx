import { useEffect, useRef, useState } from 'react'
import { api } from '../api'
import { t } from '../i18n'
import { desktopAction, isDesktop } from '../desktop'
import { customDNSStatus } from './CustomDNSStatus'
import type { CustomDNSChange, CustomDNSDocument, CustomDNSDraft, CustomDNSImport, CustomDNSPreview } from '../types'

const example = `hosts:
  nas.home.arpa: 192.168.50.10

dns:
  use-hosts: true
  nameserver-policy:
    '+.corp.example':
      - 10.20.0.53
  fake-ip-filter-mode: blacklist
  fake-ip-filter:
    - '+.corp.example'
`

async function download(name: string, content: string) {
  if (isDesktop()) { await desktopAction('save-dns-file', { filename: name, text: content }); return }
  const url = URL.createObjectURL(new Blob([content], { type: 'text/yaml;charset=utf-8' }))
  const a = document.createElement('a')
  a.href = url; a.download = name; a.click()
  setTimeout(() => URL.revokeObjectURL(url), 0)
}

export function CustomDNSCard({ saved, onSaved, running, onApply, applying = false, applyDisabled = false, loadError = '', onOpenNetwork, onCheckNetwork = onOpenNetwork, onDirtyChange, navigationRequest = 0 }: {
  saved: CustomDNSDraft | null
  onSaved: (value: CustomDNSDraft) => void | Promise<void>
  running: boolean
  onApply?: () => void
  applying?: boolean
  applyDisabled?: boolean
  loadError?: string
  onOpenNetwork: () => void
  onCheckNetwork?: () => void
  onDirtyChange?: (dirty: boolean) => void
  navigationRequest?: number
}) {
  const [draft, setDraft] = useState<CustomDNSDocument | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [pending, setPending] = useState<(CustomDNSImport & { name: string }) | null>(null)
  const [preview, setPreview] = useState<CustomDNSPreview | null>(null)
  const savedRef = useRef<CustomDNSDraft | null>(null)
  const hostsFile = useRef<HTMLInputElement>(null)
  const yamlFile = useRef<HTMLInputElement>(null)
  const details = useRef<HTMLDetailsElement>(null)
  const summary = useRef<HTMLElement>(null)
  const dirty = Boolean(draft && saved && JSON.stringify(draft) !== JSON.stringify(saved.document))

  useEffect(() => {
    if (!saved) return
    const previous = savedRef.current
    setDraft(current => !current || JSON.stringify(current) === JSON.stringify(previous?.document) ? saved.document : current)
    savedRef.current = saved
  }, [saved])

  useEffect(() => { onDirtyChange?.(dirty) }, [dirty, onDirtyChange])

  useEffect(() => {
    if (window.location.hash !== '#custom-dns' || !details.current) return
    details.current.open = true
    const reducedMotion = typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
    details.current.scrollIntoView?.({ behavior: reducedMotion ? 'auto' : 'smooth', block: 'start' })
    summary.current?.focus({ preventScroll: true })
  }, [navigationRequest, Boolean(saved)])

  async function work(action: () => Promise<void>) {
    setBusy(true); setError(''); setMessage('')
    try { await action() } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)) }
    finally { setBusy(false) }
  }

  async function importFile(file: File | undefined, format: 'hosts' | 'yaml') {
    if (!file || !draft) return
    await work(async () => {
      if (file.size > 2 * 1024 * 1024) throw new Error(t('文件不能超过 2 MiB。'))
      let content: string
      try { content = new TextDecoder('utf-8', { fatal: true }).decode(await file.arrayBuffer()) }
      catch { throw new Error(t('文件必须使用 UTF-8 编码。')) }
      const result = await api.importCustomDNS(draft, format, content)
      setPending({ ...result, name: file.name }); setPreview(null)
    })
  }

  function edit(value: CustomDNSDocument) { setDraft(value); setPreview(null); setMessage('') }

  const status = loadError ? '状态未知' : dirty ? '有未保存的修改' : saved ? customDNSStatus(saved, running) : '正在加载…'
  const statusTone = loadError ? 'off' : dirty || (saved && !saved.desired) ? 'draft' : saved?.applied ? 'live' : saved?.document.enabled ? 'ready' : 'off'
  const showApply = saved && (dirty || onApply || !saved.desired || (!saved.applied && saved.document.enabled))

  return <section id="custom-dns" className="custom-dns-section">
    <details className={`section profile-overlay-panel custom-dns-card ${saved?.document.enabled ? 'enabled' : ''}`} ref={details}>
      <summary className="profile-overlay-hero" ref={summary}>
        <div className="profile-overlay-symbol"><DNSIcon name="network" /></div>
        <div>
          <small>ADVANCED · CUSTOM DNS</small>
          <div className="custom-dns-title"><h2>{t('自定义 DNS')}</h2>{saved && <span>{t('{{hosts}} 个映射 · {{policies}} 个上游策略', { hosts: saved.hosts, policies: saved.policies })}</span>}</div>
          <p>{t('固定域名地址，或为指定域名设置 DNS 上游。')}</p>
        </div>
        <div className="profile-overlay-controls"><span className={`overlay-status ${statusTone}`}>{t(status)}</span><i aria-hidden="true">⌄</i></div>
      </summary>
      <div className="custom-dns-body">
      {draft && <fieldset disabled={busy || applying || Boolean(pending)}>
        <label className="custom-dns-enable">
          <span><strong>{t('启用自定义 DNS')}</strong><small>{t('同时用于 Mac TUN 与使用 OpenSurge DNS 的下游设备。')}</small></span>
          <span className="custom-dns-toggle">
            <span>{t(draft.enabled ? '草稿已启用' : '草稿已禁用')}</span>
            <input type="checkbox" aria-label={t('启用自定义 DNS')} checked={draft.enabled} onChange={event => edit({ ...draft, enabled: event.target.checked })} />
            <i aria-hidden="true" />
          </span>
        </label>
        <div className="custom-dns-editor">
          <div className="custom-dns-editor-toolbar">
            <label htmlFor="custom-dns-yaml"><DNSIcon name="file" />{t('自定义 DNS YAML')}</label>
            <div className="custom-dns-tools">
              <div>
                <button type="button" onClick={() => hostsFile.current?.click()}><DNSIcon name="upload" />{t('导入 hosts')}</button>
                <button type="button" onClick={() => yamlFile.current?.click()}><DNSIcon name="upload" />{t('导入 YAML')}</button>
              </div>
              <div>
                <button type="button" onClick={() => void work(() => download('custom-dns-example.yaml', example))}><DNSIcon name="download" />{t('下载示例')}</button>
                <button type="button" onClick={() => void work(async () => {
                  const result = await api.importCustomDNS(draft, 'yaml', draft.yaml)
                  await download('custom-dns.yaml', result.document.yaml)
                })}><DNSIcon name="export" />{t('导出草稿')}</button>
              </div>
            </div>
          </div>
          <textarea id="custom-dns-yaml" spellCheck={false} autoCapitalize="off" autoCorrect="off" rows={12} value={draft.yaml} onChange={event => edit({ ...draft, yaml: event.target.value })} />
          <div className="custom-dns-editor-caption"><span>{t('仅支持 hosts / dns 区块')}</span><span>YAML · UTF-8</span></div>
        </div>
        <input hidden ref={hostsFile} type="file" onChange={event => { void importFile(event.target.files?.[0], 'hosts'); event.target.value = '' }} />
        <input hidden ref={yamlFile} type="file" accept=".yaml,.yml,text/yaml" onChange={event => { void importFile(event.target.files?.[0], 'yaml'); event.target.value = '' }} />
        <details className="custom-dns-help">
          <summary><DNSIcon name="info" />{t('导入格式与覆盖规则')}<DNSIcon name="chevron" /></summary>
          <div>
            <p>{t('导入 hosts 或仅含 hosts / dns 的 YAML，同时用于 Mac TUN 与使用 OpenSurge DNS 的下游设备。')}</p>
            <p>{t('同名自定义项覆盖订阅及全局附加配置；删除覆盖项恢复继承。DHCP 名称保留给租约解析。')}</p>
            <p>{t('保存只更新草稿。通过停止后启动网关，或重新应用来源使其生效；禁用也需应用。')}</p>
          </div>
        </details>
        <div className="custom-dns-save-bar">
          <span>{busy ? t('正在处理…') : t('保存草稿后，再应用到网关。')}</span>
          <div className="custom-dns-actions">
            <button className="custom-dns-reset" type="button" onClick={() => void work(async () => {
              const value = await api.customDNS(); setDraft(value.document); setPreview(null); await onSaved(value)
            })}><DNSIcon name="refresh" />{t('重新载入草稿')}</button>
            <button type="button" onClick={() => void work(async () => setPreview(await api.previewCustomDNS(draft)))}><DNSIcon name="preview" />{t('预览')}</button>
            <button className="primary" type="button" disabled={!saved || !dirty} onClick={() => void work(async () => {
              if (!saved) return
              const value = await api.saveCustomDNS(draft, saved.revision)
              setDraft(value.document); setPreview(null)
              await onSaved(value)
              setMessage(t('草稿已保存，运行中的 DNS 配置未改变。'))
            })}><DNSIcon name="check" />{t('保存草稿')}</button>
          </div>
        </div>
      </fieldset>}
      {saved && showApply && <div className={`custom-dns-apply ${dirty ? 'unsaved' : ''}`}>
        <div className="custom-dns-apply-copy"><span className="custom-dns-apply-icon"><DNSIcon name={dirty ? 'file' : 'info'} /></span><div>
          <strong>{t('应用到网关')}</strong>
          {dirty ? <p>{t('请先保存自定义 DNS 草稿，再应用配置。')}</p> : !saved.desired && <p>{t('自定义 DNS 有待应用的修改；关闭开关也需要应用。')}</p>}
          {!onApply && (!saved.desired || (!saved.applied && saved.document.enabled)) && <p>{t(running ? '保持当前配置来源，前往网络设置停止后启动网关，应用已保存的草稿。' : '下次启动会组合已保存的自定义 DNS 和全局附加配置，并校验后生效。')}</p>}
        </div></div>
        {onApply ? <button className="primary" type="button" disabled={busy || applying || applyDisabled || dirty || Boolean(pending)} onClick={onApply}>{t(running ? '校验并应用自定义 DNS' : '校验并保存到下次启动')}</button>
          : (!saved.desired || (!saved.applied && saved.document.enabled)) && <button type="button" disabled={busy || applying || dirty || Boolean(pending)} onClick={onOpenNetwork}>{t(running ? '前往网络设置应用' : '前往网络设置启动')}<DNSIcon name="export" /></button>}
      </div>}
      {pending && <div className="custom-dns-import" role="region" aria-label={t('导入替换预览')}>
        <h3>{t('导入替换预览')}</h3><p>{t('文件 {{name}} 将替换草稿中的 {{sections}}，其他区块保留。', { name: pending.name, sections: pending.sections.join(', ') })}</p>
        <div className="custom-dns-comparison"><div><strong>{t('当前草稿')}</strong><pre>{draft?.yaml}</pre></div><div><strong>{t('导入后草稿')}</strong><pre>{pending.document.yaml}</pre></div></div>
        <div className="custom-dns-actions"><button className="primary" type="button" onClick={() => { edit(pending.document); setPending(null) }}>{t('采用导入内容')}</button><button type="button" onClick={() => setPending(null)}>{t('取消')}</button></div>
      </div>}
      {preview && <div className="custom-dns-preview" role="region" aria-label={t('最终 DNS 配置')}>
        <h3>{t('最终 DNS 配置')}</h3>
        <p>{t('预览以已保存的网络设置为准；配置检查不代表真实查询验收。')}</p>
        <div className="custom-dns-actions"><span className={`pill ${preview.mac_ready ? 'ok' : ''}`}>{t('Mac TUN')} · {t(preview.mac_ready ? '配置前提满足' : '配置前提未满足')}</span><span className={`pill ${preview.downstream_ready ? 'ok' : ''}`}>{t('下游 DNS')} · {t(preview.downstream_ready ? '配置前提满足' : '配置前提未满足')}</span></div>
        {(!preview.mac_ready || !preview.downstream_ready) && <button type="button" onClick={onCheckNetwork}>{t('前往网络设置检查')}</button>}
        {preview.blockers.map((item, i) => <p key={i} className="notice warn">{item}</p>)}
        {!preview.blockers.length && <p className="notice">{t('配置检查通过；应用时仍需通过引擎校验。')}</p>}
        <DNSChanges title={t('自定义内容变化')} changes={preview.changes} />
        <DNSChanges title={t('受管设置与系统 DNS 协同')} changes={preview.managed_changes} />
        <details><summary>{t('基础 DNS 内容')}</summary><pre>{preview.base_yaml || t('没有可用的原始快照')}</pre></details>
        <details><summary>{t('全局附加配置')}</summary><pre>{preview.overlay_yaml}</pre></details>
        <pre>{preview.final_yaml}</pre>
        <p className="muted">{t('hosts 先于上游策略匹配；更具体的域名可能优先于通配。节点和 Provider 也可能使用 hosts。')}</p>
        {preview.warnings.map((item, i) => <p key={i} className="notice">{item}</p>)}
        {preview.dns_references.length > 0 && <details><summary>{t('DNS 服务引用')}</summary><pre>{preview.dns_references.map(item => `${item.field}: ${item.target} → ${item.effective} (${item.source})`).join('\n')}</pre></details>}
      </div>}
      {message && <p role="status" className="notice">{message}</p>}
      {!draft && <button type="button" disabled={busy} onClick={() => void work(async () => { const value = await api.customDNS(); await onSaved(value) })}>{t('重新载入草稿')}</button>}
      {(error || loadError) && <p role="alert" className="notice warn">{error || loadError}</p>}
      </div>
    </details>
  </section>
}

function DNSIcon({ name }: { name: 'network' | 'chevron' | 'file' | 'upload' | 'download' | 'export' | 'refresh' | 'preview' | 'check' | 'info' }) {
  const paths = {
    network: <><rect x="8" y="3" width="8" height="6" rx="1.5" /><path d="M12 9v7M5 16v-3h14v3" /><rect x="2" y="16" width="6" height="5" rx="1" /><rect x="9" y="16" width="6" height="5" rx="1" /><rect x="16" y="16" width="6" height="5" rx="1" /></>,
    chevron: <path d="m6 9 6 6 6-6" />,
    file: <><path d="M14 3H6a1 1 0 0 0-1 1v16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8zM14 3v5h5M9 12h6M9 16h6" /></>,
    upload: <path d="M12 16V3m-4 4 4-4 4 4M4 15v5h16v-5" />,
    download: <path d="M12 3v13m-4-4 4 4 4-4M4 17v4h16v-4" />,
    export: <path d="M14 3h7v7m0-7L10 14M10 5H4v15h15v-6" />,
    refresh: <><path d="M20 7v5h-5M4 17v-5h5" /><path d="M6.1 6a7 7 0 0 1 11.2 0L20 12M4 12l2.7 6a7 7 0 0 0 11.2 0" /></>,
    preview: <><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" /><circle cx="12" cy="12" r="3" /></>,
    check: <path d="m5 12 4 4L19 6" />,
    info: <><circle cx="12" cy="12" r="9" /><path d="M12 11v6M12 7v.01" /></>,
  }
  return <svg className="custom-dns-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{paths[name]}</svg>
}

function DNSChanges({ title, changes }: { title: string; changes: CustomDNSChange[] }) {
  if (!changes.length) return null
  return <details><summary>{title}</summary><div className="table-scroll"><table><thead><tr><th>{t('字段')}</th><th>{t('覆盖前')}</th><th>{t('覆盖后')}</th></tr></thead><tbody>{changes.map((change, i) => <tr key={i}><td><code>{change.field}</code></td><td><pre>{change.before || '—'}</pre></td><td><pre>{change.after}</pre></td></tr>)}</tbody></table></div></details>
}
