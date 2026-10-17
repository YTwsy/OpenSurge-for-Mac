import { useEffect, useRef, useState } from 'react'
import { api } from '../api'
import { t } from '../i18n'
import { desktopAction, isDesktop } from '../desktop'
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

export function CustomDNSCard({ contextKey }: { contextKey: string }) {
  const [saved, setSaved] = useState<CustomDNSDraft | null>(null)
  const [draft, setDraft] = useState<CustomDNSDocument | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [pending, setPending] = useState<(CustomDNSImport & { name: string }) | null>(null)
  const [preview, setPreview] = useState<CustomDNSPreview | null>(null)
  const savedRef = useRef<CustomDNSDraft | null>(null)
  const hostsFile = useRef<HTMLInputElement>(null)
  const yamlFile = useRef<HTMLInputElement>(null)
  const dirty = Boolean(draft && saved && JSON.stringify(draft) !== JSON.stringify(saved.document))

  useEffect(() => {
    let alive = true
    void api.customDNS().then(value => {
      if (!alive) return
      const previous = savedRef.current
      setDraft(current => !current || JSON.stringify(current) === JSON.stringify(previous?.document) ? value.document : current)
      savedRef.current = value; setSaved(value)
    }).catch(reason => { if (alive) setError(String(reason)) })
    return () => { alive = false }
  }, [contextKey])

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

  return <section className="section custom-dns-section">
    <details className="custom-dns-card">
      <summary><strong>{t('自定义 DNS')}</strong><span>{saved ? t(saved.document.enabled ? '已启用' : '已禁用') : t('正在加载…')}</span>
        {saved && <span>{t('{{hosts}} 个映射 · {{policies}} 个上游策略', { hosts: saved.hosts, policies: saved.policies })}</span>}
        {saved && <span className="pill">{t(dirty ? '有未保存的修改' : saved.applied ? '已应用' : saved.desired ? '已保存，待启动' : '未应用修改')}</span>}
      </summary>
      <p>{t('导入 hosts 或仅含 hosts / dns 的 YAML，同时用于 Mac TUN 与使用 OpenSurge DNS 的下游设备。')}</p>
      <p className="muted">{t('保存只更新草稿。通过停止后启动网关，或重新应用来源使其生效；禁用也需应用。')}</p>
      {draft && <fieldset disabled={busy || Boolean(pending)}>
        <label className="custom-dns-enable"><input type="checkbox" checked={draft.enabled} onChange={event => edit({ ...draft, enabled: event.target.checked })} />{t('启用自定义 DNS')}</label>
        <div className="custom-dns-actions">
          <button type="button" onClick={() => hostsFile.current?.click()}>{t('导入 hosts')}</button>
          <button type="button" onClick={() => yamlFile.current?.click()}>{t('导入 YAML')}</button>
          <button type="button" onClick={() => void work(() => download('custom-dns-example.yaml', example))}>{t('下载示例')}</button>
          <button type="button" onClick={() => void work(async () => {
            const result = await api.importCustomDNS(draft, 'yaml', draft.yaml)
            await download('custom-dns.yaml', result.document.yaml)
          })}>{t('导出草稿')}</button>
        </div>
        <input hidden ref={hostsFile} type="file" onChange={event => { void importFile(event.target.files?.[0], 'hosts'); event.target.value = '' }} />
        <input hidden ref={yamlFile} type="file" accept=".yaml,.yml,text/yaml" onChange={event => { void importFile(event.target.files?.[0], 'yaml'); event.target.value = '' }} />
        <label className="custom-dns-editor">{t('自定义 DNS YAML')}<textarea spellCheck={false} rows={14} value={draft.yaml} onChange={event => edit({ ...draft, yaml: event.target.value })} /></label>
        <p className="muted">{t('同名自定义项覆盖订阅及全局附加配置；删除覆盖项恢复继承。DHCP 名称保留给租约解析。')}</p>
        <div className="custom-dns-actions">
          <button type="button" onClick={() => void work(async () => setPreview(await api.previewCustomDNS(draft)))}>{t('预览')}</button>
          <button className="primary" type="button" disabled={!saved || !dirty} onClick={() => void work(async () => {
            if (!saved) return
            const value = await api.saveCustomDNS(draft, saved.revision)
            setSaved(value); savedRef.current = value; setDraft(value.document); setPreview(null)
            setMessage(t('草稿已保存，运行中的 DNS 配置未改变。'))
          })}>{t('保存草稿')}</button>
          <button type="button" onClick={() => void work(async () => {
            const value = await api.customDNS(); setSaved(value); savedRef.current = value; setDraft(value.document); setPreview(null)
          })}>{t('重新载入草稿')}</button>
        </div>
      </fieldset>}
      {pending && <div className="custom-dns-import" role="region" aria-label={t('导入替换预览')}>
        <h3>{t('导入替换预览')}</h3><p>{t('文件 {{name}} 将替换草稿中的 {{sections}}，其他区块保留。', { name: pending.name, sections: pending.sections.join(', ') })}</p>
        <div className="custom-dns-comparison"><div><strong>{t('当前草稿')}</strong><pre>{draft?.yaml}</pre></div><div><strong>{t('导入后草稿')}</strong><pre>{pending.document.yaml}</pre></div></div>
        <div className="custom-dns-actions"><button type="button" onClick={() => { edit(pending.document); setPending(null) }}>{t('采用导入内容')}</button><button type="button" onClick={() => setPending(null)}>{t('取消')}</button></div>
      </div>}
      {preview && <div className="custom-dns-preview" role="region" aria-label={t('最终 DNS 配置')}>
        <h3>{t('最终 DNS 配置')}</h3>
        <p>{t('预览以已保存的网络设置为准；配置检查不代表真实查询验收。')}</p>
        <div className="custom-dns-actions"><span className={`pill ${preview.mac_ready ? 'ok' : ''}`}>{t('Mac TUN')} · {t(preview.mac_ready ? '配置前提满足' : '配置前提未满足')}</span><span className={`pill ${preview.downstream_ready ? 'ok' : ''}`}>{t('下游 DNS')} · {t(preview.downstream_ready ? '配置前提满足' : '配置前提未满足')}</span></div>
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
      {!draft && <button type="button" disabled={busy} onClick={() => void work(async () => { const value = await api.customDNS(); savedRef.current = value; setSaved(value); setDraft(value.document) })}>{t('重新载入草稿')}</button>}
      {error && <p role="alert" className="notice warn">{error}</p>}
    </details>
  </section>
}

function DNSChanges({ title, changes }: { title: string; changes: CustomDNSChange[] }) {
  if (!changes.length) return null
  return <details><summary>{title}</summary><div className="table-scroll"><table><thead><tr><th>{t('字段')}</th><th>{t('覆盖前')}</th><th>{t('覆盖后')}</th></tr></thead><tbody>{changes.map((change, i) => <tr key={i}><td><code>{change.field}</code></td><td><pre>{change.before || '—'}</pre></td><td><pre>{change.after}</pre></td></tr>)}</tbody></table></div></details>
}
