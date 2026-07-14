import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import { Empty, PageHeader, SectionTitle } from '../components/Common'
import type { Overview, Source } from '../types'

export function SourcesPage({ overview, onChanged }: { overview: Overview | null; onChanged: () => void | Promise<void> }) {
  const [sources, setSources] = useState<Source[]>([])
  const [revision, setRevision] = useState('')
  const [name, setName] = useState('')
  const [url, setURL] = useState('')
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)
  const [pending, setPending] = useState<Source | null>(null)
  const running = overview?.status.gateway === 'running'

  const refresh = useCallback(async () => {
    try {
      const response = await api.sources()
      setSources(response.sources ?? [])
      setRevision(response.revision)
      setError('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    }
  }, [])

  useEffect(() => { void refresh() }, [refresh])

  const run = async (action: () => Promise<unknown>) => {
    setBusy(true)
    setMessage('')
    try {
      await action()
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setBusy(false)
    }
  }

  const apply = async () => {
    if (!pending) return
    const selected = pending
    setBusy(true)
    setError('')
    setMessage('')
    try {
      const applied = await api.applySource(selected.id, revision)
      setPending(null)
      setMessage(applied.applied ? '订阅已应用，网关已使用新的运行配置。' : '订阅已保存，将在下次启动网关时应用。')
      await Promise.all([refresh(), Promise.resolve(onChanged())])
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setBusy(false)
    }
  }

  return <>
    <PageHeader eyebrow="SOURCES" title="代理与规则源" description="导入先生成只读快照，通过结构和 mihomo 引擎校验后才允许应用。" />
    {error && <div className="notice warn" role="alert">{error}</div>}
    {message && <div className="ok-notice" role="status">{message}</div>}
    <section className="section import-panel"><div><h3>HTTPS 订阅 URL</h3><input aria-label="来源名称" placeholder="名称" value={name} onChange={event => setName(event.target.value)} /><input aria-label="HTTPS 订阅 URL" placeholder="https://…" value={url} onChange={event => setURL(event.target.value)} /><button className="primary" disabled={busy || !url} onClick={() => void run(() => api.importURL(name, url))}>导入为草稿</button></div><div><h3>本地 mihomo YAML</h3><label className="dropzone">选择或拖入 YAML<input aria-label="本地 mihomo YAML" type="file" accept=".yaml,.yml,text/yaml" onChange={event => { const file = event.target.files?.[0]; if (file) void run(() => api.importFile(file)) }} /></label><p>导入内容不能覆盖 OpenSurge 管理的 DNS、TUN、Controller 与 LAN binding。</p></div></section>
    <section className="section"><SectionTitle title="已导入快照" subtitle="刷新只产生新草稿；应用到运行中的网关需要完整校验并重载" />{sources.length ? <div className="source-grid">{sources.map(source => {
      const previousApplied = source.versions.some(version => version.applied)
      const changed = source.diff.previous_digest && source.diff.previous_digest !== source.digest
      const state = source.applied ? '运行版本' : source.desired ? running ? '待重载' : '下次启动版本' : previousApplied ? '新草稿' : source.valid ? '结构有效' : '无效'
      const action = source.applied ? '已运行' : source.desired ? running ? '应用并重载网关' : '等待下次启动' : running ? '校验、应用并重载' : '设为下次启动版本'
      return <article className="source-card" key={source.id}><div className="source-head"><div><small>{source.kind}</small><h3>{source.name}</h3></div><span className={source.applied ? 'pill ok' : source.desired ? 'pill' : source.valid ? 'pill ok' : 'pill bad'}>{state}</span></div><p>{source.origin}</p><div className="inventory"><span>{source.inventory.proxy_groups.length} 策略组</span><span>{source.inventory.proxy_providers.length} Proxy Provider</span><span>{source.inventory.rule_count} 条规则</span><span>{source.versions.length + 1} 个版本</span></div>{changed && <div className="notice">Diff：proxy +{source.diff.proxies_added.length}/-{source.diff.proxies_removed.length}，group +{source.diff.groups_added.length}/-{source.diff.groups_removed.length}，rules {source.diff.rule_count_delta >= 0 ? '+' : ''}{source.diff.rule_count_delta}</div>}<small>{source.validation}</small>{source.versions.length > 0 && <small>历史：{source.versions.slice(-3).map(version => `${version.digest.slice(0, 8)}${version.applied ? ' (运行)' : version.desired ? ' (待应用)' : ''}`).join(' · ')}</small>}<div className="actions">{source.origin.startsWith('https://') && <button disabled={busy} onClick={() => void run(() => api.refreshSource(source.id))}>刷新</button>}<button className="primary" disabled={busy || !revision || !source.valid || source.applied || (source.desired && !running)} onClick={() => setPending(source)}>{action}</button></div></article>
    })}</div> : <Empty text="尚未导入任何来源" />}</section>
    {pending && <dialog className="reload-dialog" open aria-modal="true" aria-labelledby="source-apply-title"><h2 id="source-apply-title">{running ? '应用订阅并重载网关？' : '设为下次启动版本？'}</h2><p>{running ? 'OpenSurge 会先验证完整候选配置，再短暂重启 DHCP/DNS、mihomo、PF 与 IPv4 forwarding。只有重载成功后才会标记为运行版本。' : '当前网关未运行。订阅会保存为 desired 配置，并在下次启动成功后成为运行版本。'}</p>{running && <ul><li>当前连接会中断并重新建立。</li><li>验证失败不会停止现有网关。</li><li>重载失败会恢复旧配置，并尽力恢复原网关。</li></ul>}<div className="dialog-actions"><button type="button" disabled={busy} onClick={() => setPending(null)}>取消</button><button className="primary" type="button" autoFocus disabled={busy} onClick={() => void apply()}>{busy ? '正在验证并应用…' : running ? '确认应用并重载' : '确认设为下次启动版本'}</button></div></dialog>}
  </>
}
