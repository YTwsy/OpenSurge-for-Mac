import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import { Empty, PageHeader, SectionTitle } from '../components/Common'
import type { Overview, Source } from '../types'

type SourceAction =
  | { kind: 'import-url' | 'import-file' | 'apply'; sourceID?: string }
  | { kind: 'refresh'; sourceID: string }
  | null

export function SourcesPage({ overview, onChanged }: { overview: Overview | null; onChanged: () => void | Promise<void> }) {
  const [sources, setSources] = useState<Source[]>([])
  const [revision, setRevision] = useState('')
  const [name, setName] = useState('')
  const [url, setURL] = useState('')
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [activeAction, setActiveAction] = useState<SourceAction>(null)
  const [pending, setPending] = useState<Source | null>(null)
  const running = overview?.status.gateway === 'running'
  const busy = activeAction !== null

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

  const run = async (action: Exclude<SourceAction, null>, operation: () => Promise<unknown>, successMessage: string) => {
    setActiveAction(action)
    setMessage('')
    setError('')
    try {
      await operation()
      await refresh()
      setMessage(successMessage)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setActiveAction(null)
    }
  }

  const openApply = (source: Source) => {
    setPending(source)
    setMessage(`已选择 ${source.name}；确认后会先校验完整候选配置。`)
    setError('')
  }

  const apply = async () => {
    if (!pending) return
    const selected = pending
    setActiveAction({ kind: 'apply', sourceID: selected.id })
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
      setActiveAction(null)
    }
  }

  return <>
    <PageHeader eyebrow="SOURCES" title="代理与规则源" description="导入、校验、应用各自有明确状态；运行配置只会在完整校验成功后切换。" />
    <div className="source-feedback" aria-live="polite">
      {error && <div className="notice warn" role="alert"><span aria-hidden="true">!</span><div><strong>操作未完成</strong><p>{error}</p></div></div>}
      {message && <div className="ok-notice" role="status"><span aria-hidden="true">✓</span><div><strong>操作已确认</strong><p>{message}</p></div></div>}
    </div>
    <section className="section source-import-panel" aria-busy={activeAction?.kind === 'import-url' || activeAction?.kind === 'import-file'}>
      <SectionTitle title="添加配置来源" subtitle="导入只产生草稿，不会立即改变正在运行的 DHCP、DNS、TUN 或策略。" />
      <div className="source-import-grid">
        <article className="source-import-card">
          <div className="source-import-head"><span aria-hidden="true">↗</span><div><small>REMOTE PROFILE</small><h3>HTTPS 订阅</h3></div></div>
          <label><span>来源名称</span><input aria-label="来源名称" placeholder="例如 Home" value={name} onChange={event => setName(event.target.value)} /></label>
          <label><span>订阅地址</span><input aria-label="HTTPS 订阅 URL" placeholder="https://…" value={url} onChange={event => setURL(event.target.value)} /></label>
          <button className="primary source-action-button" type="button" disabled={busy || !url} onClick={() => void run({ kind: 'import-url' }, () => api.importURL(name, url), `${name || 'HTTPS 来源'} 已导入为草稿并完成结构校验。`)}>
            <ActionLabel active={activeAction?.kind === 'import-url'} idle="导入为草稿" pending="正在导入并校验…" />
          </button>
        </article>
        <article className="source-import-card local">
          <div className="source-import-head"><span aria-hidden="true">⇧</span><div><small>LOCAL PROFILE</small><h3>本地 mihomo YAML</h3></div></div>
          <label className={`dropzone source-dropzone ${activeAction?.kind === 'import-file' ? 'busy' : ''}`}>
            <span className="dropzone-icon" aria-hidden="true">＋</span>
            <strong>{activeAction?.kind === 'import-file' ? '正在读取并校验…' : '选择或拖入 YAML'}</strong>
            <small>.yaml / .yml · 仅创建草稿</small>
            <input aria-label="本地 mihomo YAML" type="file" accept=".yaml,.yml,text/yaml" disabled={busy} onChange={event => {
              const file = event.target.files?.[0]
              if (file) void run({ kind: 'import-file' }, () => api.importFile(file), `${file.name} 已导入为草稿并完成结构校验。`)
            }} />
          </label>
          <p className="source-guard-note"><span aria-hidden="true">⌁</span>DNS、TUN、Controller 与 LAN binding 始终由 OpenSurge 管理。</p>
        </article>
      </div>
    </section>
    <section className="section source-library">
      <SectionTitle title="已导入快照" subtitle="刷新只产生新草稿；应用到运行中的网关需要再次完整校验。" />
      {sources.length ? <div className="source-grid">{sources.map(source => {
        const previousApplied = source.versions.some(version => version.applied)
        const changed = source.diff.previous_digest && source.diff.previous_digest !== source.digest
        const state = source.applied ? '运行版本' : source.desired ? running ? '待重载' : '下次启动版本' : previousApplied ? '新草稿' : source.valid ? '结构有效' : '无效'
        const action = source.applied ? '已运行' : source.desired ? running ? '应用并重载网关' : '等待下次启动' : running ? '校验、应用并重载' : '设为下次启动版本'
        const refreshing = activeAction?.kind === 'refresh' && activeAction.sourceID === source.id
        return <article className="source-card" key={source.id}>
          <div className="source-head"><div><small>{source.kind}</small><h3>{source.name}</h3></div><span className={source.applied ? 'pill ok' : source.desired ? 'pill' : source.valid ? 'pill ok' : 'pill bad'}>{state}</span></div>
          <p className="source-origin" title={source.origin}><span aria-hidden="true">⌁</span>{source.origin}</p>
          <div className="source-inventory">
            <SourceMetric value={source.inventory.proxy_groups.length} label="策略组" />
            <SourceMetric value={source.inventory.proxy_providers.length} label="Provider" />
            <SourceMetric value={source.inventory.rule_count} label="规则" />
            <SourceMetric value={source.versions.length + 1} label="版本" />
          </div>
          {changed && <div className="source-diff"><strong>本次变化</strong><span>proxy +{source.diff.proxies_added.length}/-{source.diff.proxies_removed.length}</span><span>group +{source.diff.groups_added.length}/-{source.diff.groups_removed.length}</span><span>rules {source.diff.rule_count_delta >= 0 ? '+' : ''}{source.diff.rule_count_delta}</span></div>}
          <div className={`source-validation ${source.valid ? 'valid' : 'invalid'}`}><span aria-hidden="true">{source.valid ? '✓' : '!'}</span><div><strong>{source.valid ? '结构校验通过' : '结构校验失败'}</strong><small>{source.validation || (source.valid ? '可以进入完整候选配置校验' : '请修正来源后重新导入')}</small></div></div>
          {source.versions.length > 0 && <small className="source-history">历史：{source.versions.slice(-3).map(version => `${version.digest.slice(0, 8)}${version.applied ? ' (运行)' : version.desired ? ' (待应用)' : ''}`).join(' · ')}</small>}
          <div className="source-actions">
            {source.origin.startsWith('https://') && <button type="button" disabled={busy} onClick={() => void run({ kind: 'refresh', sourceID: source.id }, () => api.refreshSource(source.id), `${source.name} 已刷新；新内容已保存为草稿。`)}><ActionLabel active={refreshing} idle="刷新草稿" pending="正在刷新…" /></button>}
            <button className="primary" type="button" disabled={busy || !revision || !source.valid || source.applied || (source.desired && !running)} onClick={() => openApply(source)}>{action}</button>
          </div>
        </article>
      })}</div> : <Empty text="尚未导入任何来源" />}
    </section>
    {pending && <dialog className="reload-dialog" open aria-modal="true" aria-labelledby="source-apply-title">
      <h2 id="source-apply-title">{running ? '应用订阅并重载网关？' : '设为下次启动版本？'}</h2>
      <p>{running ? 'OpenSurge 会先验证完整候选配置，再短暂重启 DHCP/DNS、mihomo、PF 与 IPv4 forwarding。只有重载成功后才会标记为运行版本。' : '当前网关未运行。订阅会保存为 desired 配置，并在下次启动成功后成为运行版本。'}</p>
      {running && <ul><li>当前连接会中断并重新建立。</li><li>验证失败不会停止现有网关。</li><li>重载失败会恢复旧配置，并尽力恢复原网关。</li></ul>}
      <div className="dialog-actions"><button type="button" disabled={busy} onClick={() => setPending(null)}>取消</button><button className="primary" type="button" autoFocus disabled={busy} onClick={() => void apply()}><ActionLabel active={activeAction?.kind === 'apply'} idle={running ? '确认应用并重载' : '确认设为下次启动版本'} pending="正在验证并应用…" /></button></div>
    </dialog>}
  </>
}

function SourceMetric({ value, label }: { value: number; label: string }) {
  return <span><strong>{value}</strong><small>{label}</small></span>
}

function ActionLabel({ active, idle, pending }: { active: boolean; idle: string; pending: string }) {
  return <>{active && <span className="button-spinner" aria-hidden="true" />}{active ? pending : idle}</>
}
