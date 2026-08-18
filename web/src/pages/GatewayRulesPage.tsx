import { useCallback, useEffect, useState } from 'react'
import { api, waitForOperation } from '../api'
import { PageHeader, SectionTitle } from '../components/Common'
import type { OperationNotification } from '../components/OperationNotifications'
import type { GatewayRules, GatewayRulesDocument, Overview } from '../types'

type RuleListName = 'prepend' | 'append' | 'delete'

export function GatewayRulesPage({ overview, onChanged, onNotify, onDirtyChange }: { overview: Overview | null; onChanged: () => void | Promise<void>; onNotify: (notification: OperationNotification) => void; onDirtyChange: (dirty: boolean) => void }) {
  const [document, setDocument] = useState<GatewayRulesDocument | null>(null)
  const [draft, setDraft] = useState<GatewayRules | null>(null)
  const [newRules, setNewRules] = useState<Record<'prepend' | 'append' | 'delete', string>>({ prepend: '', append: '', delete: '' })
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)
  const [reloading, setReloading] = useState(false)
  const running = overview?.status.gateway === 'running'
  const pendingInput = Object.values(newRules).some(rule => Boolean(rule.trim()))
  const dirty = document !== null && draft !== null && (JSON.stringify(document.rules) !== JSON.stringify(draft) || pendingInput)

  useEffect(() => { onDirtyChange(dirty) }, [dirty, onDirtyChange])
  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => {
      if (!dirty) return
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [dirty])

  const refresh = useCallback(async () => {
    try {
      const next = await api.gatewayRules()
      setDocument(next)
      setDraft(next.rules)
      setError('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    }
  }, [])

  useEffect(() => { void refresh() }, [refresh])

  const updateList = (name: RuleListName, update: (rules: string[]) => string[]) => {
    setDraft(current => current ? { ...current, [name]: update(current[name]) } : current)
  }

  const addRule = (name: 'prepend' | 'append' | 'delete') => {
    const value = newRules[name].trim()
    if (!value) return
    updateList(name, rules => [...rules, value])
    setNewRules(current => ({ ...current, [name]: '' }))
  }

  const removeRule = (name: RuleListName, index: number) => updateList(name, rules => rules.filter((_, candidate) => candidate !== index))

  const moveRule = (name: RuleListName, index: number, offset: -1 | 1) => updateList(name, rules => {
    const next = [...rules]
    const target = index + offset
    if (target < 0 || target >= next.length) return next
    const [rule] = next.splice(index, 1)
    next.splice(target, 0, rule)
    return next
  })

  const save = async () => {
    if (!document || !draft || !dirty) return
    const candidate = {
      ...draft,
      prepend: appendPendingRule(draft.prepend, newRules.prepend),
      append: appendPendingRule(draft.append, newRules.append),
      delete: appendPendingRule(draft.delete, newRules.delete),
    }
    setBusy(true)
    setError('')
    setMessage('')
    try {
      const saved = await api.saveGatewayRules(candidate, document.revision)
      setDocument(saved)
      setDraft(saved.rules)
      setNewRules({ prepend: '', append: '', delete: '' })
      setMessage(running ? '规则已保存；重载网关后生效。' : '规则已保存，将在下次启动网关时生效。')
      await onChanged()
      if (running) onNotify({ tone: 'success', title: '自定义规则已保存', message: '订阅不会覆盖这些规则；请重载网关使它们进入当前 mihomo 进程。' })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setBusy(false)
    }
  }

  const reload = async () => {
    setReloading(true)
    setError('')
    try {
      const operation = await api.gateway('reload')
      await waitForOperation(operation.id)
      setMessage('网关已重载，自定义规则正在生效。')
      onNotify({ tone: 'success', title: '重载网关成功', message: '自定义规则已加载到当前 mihomo 配置。' })
      await onChanged()
    } catch (cause) {
      const failure = cause instanceof Error ? cause.message : String(cause)
      setError(failure)
      onNotify({ tone: 'error', title: '重载网关失败', message: failure })
    } finally {
      setReloading(false)
    }
  }

  return <>
    <PageHeader eyebrow="GATEWAY RULES" title="网关规则" description="补充当前 mihomo 订阅规则；规则单独持久化，刷新或替换订阅不会覆盖。" />
    <div className="source-feedback" aria-live="polite">
      {error && <div className="notice warn" role="alert"><span aria-hidden="true">!</span><div><strong>操作未完成</strong><p>{error}</p></div></div>}
      {message && <div className="ok-notice" role="status"><span aria-hidden="true">✓</span><div><strong>操作已确认</strong><p>{message}</p></div></div>}
    </div>
    <section className="section gateway-rules-intro">
      <SectionTitle title="规则合并顺序" subtitle="OpenSurge 内置的网关保护与设备规则仍会先执行；自定义规则再与订阅规则合并。" />
      <div className="gateway-rule-flow"><span>内置规则</span><b>→</b><span>前置规则</span><b>→</b><span>订阅规则</span><b>→</b><span>追加规则</span><b>→</b><span>默认规则</span></div>
      <p className="gateway-rule-note">例如：<code>DOMAIN-SUFFIX,example.com,Proxy</code>。末尾目标名必须是当前 mihomo 配置中的策略组或代理名称。</p>
      {overview && (overview.policies?.length || overview.providers?.rule_providers?.length) ? <details className="gateway-rule-reference"><summary><span>查看当前可用目标</span><small>{overview.policies?.length ?? 0} 个策略组 · {overview.providers?.rule_providers?.length ?? 0} 个规则集 Provider</small></summary>{overview.policies?.length ? <div className="gateway-rule-targets"><small>当前可见策略组</small>{overview.policies.map(group => <code key={group.name}>{group.name}</code>)}</div> : null}{overview.providers?.rule_providers?.length ? <div className="gateway-rule-targets"><small>当前规则集 Provider</small>{overview.providers.rule_providers.map(provider => <code key={provider.name}>{provider.name}</code>)}</div> : null}</details> : null}
    </section>
    {draft && <>
      <RuleListEditor name="prepend" title="前置规则" description="插入订阅规则之前，适合覆盖订阅中的通用匹配。" rules={draft.prepend} newRule={newRules.prepend} busy={busy || reloading} onNewRule={value => setNewRules(current => ({ ...current, prepend: value }))} onAdd={() => addRule('prepend')} onRemove={index => removeRule('prepend', index)} onMove={(index, offset) => moveRule('prepend', index, offset)} />
      <RuleListEditor name="append" title="追加规则" description="插入订阅规则之后、OpenSurge 默认 MATCH 之前。" rules={draft.append} newRule={newRules.append} busy={busy || reloading} onNewRule={value => setNewRules(current => ({ ...current, append: value }))} onAdd={() => addRule('append')} onRemove={index => removeRule('append', index)} onMove={(index, offset) => moveRule('append', index, offset)} />
      <RuleListEditor name="delete" title="从订阅中删除" description="按完整规则文本精确删除订阅中的规则；订阅刷新后仍会继续删除。" rules={draft.delete} newRule={newRules.delete} busy={busy || reloading} onNewRule={value => setNewRules(current => ({ ...current, delete: value }))} onAdd={() => addRule('delete')} onRemove={index => removeRule('delete', index)} onMove={(index, offset) => moveRule('delete', index, offset)} />
      <div className={`sticky-save ${dirty ? 'needs-reload' : ''}`}>
        <span><i aria-hidden="true">{dirty ? '!' : '✓'}</i><strong>{dirty ? '有尚未保存的规则修改' : '规则配置已保存'}</strong>{dirty && <small>保存后不会自动改写订阅快照</small>}</span>
        {running && !dirty && <button className="primary" type="button" disabled={reloading} onClick={() => void reload()}>{reloading ? '正在重载…' : '重载网关使规则生效'}</button>}
        <button className="primary" type="button" disabled={busy || reloading || !dirty} onClick={() => void save()}>{busy ? '正在校验并保存…' : '保存规则'}</button>
      </div>
    </>}
  </>
}

function appendPendingRule(rules: string[], pending: string) {
  const value = pending.trim()
  return value ? [...rules, value] : rules
}

function RuleListEditor({ name, title, description, rules, newRule, busy, onNewRule, onAdd, onRemove, onMove }: { name: RuleListName; title: string; description: string; rules: string[]; newRule: string; busy: boolean; onNewRule: (value: string) => void; onAdd: () => void; onRemove: (index: number) => void; onMove: (index: number, offset: -1 | 1) => void }) {
  const [expanded, setExpanded] = useState(false)
  const titleID = `gateway-rules-${name}`
  const bodyID = `${titleID}-body`

  return <section className={`section gateway-rule-editor ${expanded ? 'expanded' : 'collapsed'}`} aria-labelledby={titleID}>
    <div className="gateway-rule-editor-head"><div className="section-title"><h2 id={titleID}>{title}</h2><p>{description}</p></div><div className="gateway-rule-editor-summary"><span><strong>{rules.length}</strong> 条规则</span><button className="gateway-rule-editor-toggle" type="button" aria-expanded={expanded} aria-controls={bodyID} aria-label={`${expanded ? '收起' : '展开'}${title}`} onClick={() => setExpanded(value => !value)}><svg className="gateway-rule-chevron" viewBox="0 0 16 16" aria-hidden="true"><path d="m3 6 5 5 5-5" /></svg></button></div></div>
    {expanded && <div className="gateway-rule-editor-body" id={bodyID}>
      <div className="gateway-rule-add"><input aria-label={`${title}新规则`} value={newRule} disabled={busy} placeholder="例如 DOMAIN-SUFFIX,example.com,Proxy" onChange={event => onNewRule(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') { event.preventDefault(); onAdd() } }} /><button type="button" disabled={busy || !newRule.trim()} onClick={onAdd}>添加规则</button></div>
      {rules.length ? <ol className="gateway-rule-list" aria-label={`${title}列表`}>{rules.map((rule, index) => <li key={`${rule}-${index}`}><code>{rule}</code><div><button type="button" aria-label={`上移第 ${index + 1} 条规则`} disabled={busy || index === 0} onClick={() => onMove(index, -1)}>↑</button><button type="button" aria-label={`下移第 ${index + 1} 条规则`} disabled={busy || index === rules.length - 1} onClick={() => onMove(index, 1)}>↓</button><button type="button" className="danger-link" disabled={busy} onClick={() => onRemove(index)}>删除</button></div></li>)}</ol> : <p className="gateway-rule-empty">还没有规则</p>}
    </div>}
  </section>
}
