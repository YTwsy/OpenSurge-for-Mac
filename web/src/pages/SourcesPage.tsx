import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import { Empty, PageHeader, SectionTitle } from '../components/Common'
import type { Source } from '../types'

export function SourcesPage() {
  const [sources, setSources] = useState<Source[]>([])
  const [name, setName] = useState('')
  const [url, setURL] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const refresh = useCallback(async () => {
    try { setSources((await api.sources()).sources ?? []); setError('') }
    catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
  }, [])
  useEffect(() => { void refresh() }, [refresh])
  const run = async (action: () => Promise<unknown>) => {
    setBusy(true)
    try { await action(); await refresh() }
    catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setBusy(false) }
  }
  return <>
    <PageHeader eyebrow="SOURCES" title="代理与规则源" description="导入先生成只读快照，通过结构和 mihomo 引擎校验后才允许应用。" />
    {error && <div className="notice warn" role="alert">{error}</div>}
    <section className="section import-panel"><div><h3>HTTPS 订阅 URL</h3><input aria-label="来源名称" placeholder="名称" value={name} onChange={event => setName(event.target.value)} /><input aria-label="HTTPS 订阅 URL" placeholder="https://…" value={url} onChange={event => setURL(event.target.value)} /><button className="primary" disabled={busy || !url} onClick={() => void run(() => api.importURL(name, url))}>导入为草稿</button></div><div><h3>本地 mihomo YAML</h3><label className="dropzone">选择或拖入 YAML<input aria-label="本地 mihomo YAML" type="file" accept=".yaml,.yml,text/yaml" onChange={event => { const file = event.target.files?.[0]; if (file) void run(() => api.importFile(file)) }} /></label><p>导入内容不能覆盖 OpenSurge 管理的 DNS、TUN、Controller 与 LAN binding。</p></div></section>
    <section className="section"><SectionTitle title="已导入快照" subtitle="刷新只产生新版本，不会自动替换运行配置" />{sources.length ? <div className="source-grid">{sources.map(source => <article className="source-card" key={source.id}><div className="source-head"><div><small>{source.kind}</small><h3>{source.name}</h3></div><span className={source.valid ? 'pill ok' : 'pill bad'}>{source.valid ? '结构有效' : '无效'}</span></div><p>{source.origin}</p><div className="inventory"><span>{source.inventory.proxy_groups.length} 策略组</span><span>{source.inventory.proxy_providers.length} Proxy Provider</span><span>{source.inventory.rule_count} 条规则</span></div><small>{source.validation}</small><div className="actions">{source.origin.startsWith('https://') && <button disabled={busy} onClick={() => void run(() => api.refreshSource(source.id))}>刷新</button>}<button className="primary" disabled={busy || !source.valid || source.applied} onClick={() => void run(() => api.applySource(source.id))}>{source.applied ? '已应用' : '校验并应用'}</button></div></article>)}</div> : <Empty text="尚未导入任何来源" />}</section>
  </>
}
