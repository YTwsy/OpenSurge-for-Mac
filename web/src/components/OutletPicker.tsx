import { useEffect, useId, useMemo, useState } from 'react'
import { testedAgo } from '../proxyHealth'
import type { ProxyGroup, ProxyHealthEntry } from '../types'
import { Empty } from './Common'
import { ProxyHealthBadge } from './ProxyHealthBadge'

type OutletPickerProps = {
  open: boolean
  title: string
  group: ProxyGroup
  healthByName: Map<string, ProxyHealthEntry>
  testing: Set<string>
  onTest: (names: string[]) => Promise<void>
  onSelect: (policy: string) => Promise<void>
  onClose: () => void
}

export function OutletPicker({ open, title, group, healthByName, testing, onTest, onSelect, onClose }: OutletPickerProps) {
  const titleID = useId()
  const [search, setSearch] = useState('')
  const [switching, setSwitching] = useState('')
  const [error, setError] = useState('')
  const options = useMemo(() => group.options.filter(option => option.toLowerCase().includes(search.trim().toLowerCase())), [group.options, search])
  const probeable = useMemo(() => group.options.filter(option => healthByName.get(option)?.probeable), [group.options, healthByName])
  const selectable = ['selector', 'select'].includes(group.type.toLowerCase())

  useEffect(() => {
    if (!open) return
    setSearch('')
    setError('')
  }, [open])

  useEffect(() => {
    if (!open) return
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === 'Escape' && !switching) onClose() }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [open, onClose, switching])

  if (!open) return null

  const select = async (policy: string) => {
    setSwitching(policy); setError('')
    try {
      await onSelect(policy)
      onClose()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setSwitching('')
    }
  }

  return <dialog className="outlet-dialog" open aria-modal="true" aria-labelledby={titleID}>
    <div className="outlet-dialog-head"><div><small>{group.type}</small><h2 id={titleID}>{title}</h2><p>{group.name} · 当前 {group.selected || '未选择'}</p></div><button className="icon-button" type="button" aria-label="关闭出口选择" disabled={Boolean(switching)} onClick={onClose}>×</button></div>
    <div className="outlet-toolbar"><label><span className="sr-only">搜索出口</span><input type="search" value={search} placeholder="搜索节点或策略组" onChange={event => setSearch(event.target.value)} /></label><button type="button" disabled={!probeable.length || probeable.some(name => testing.has(name))} onClick={() => void onTest(probeable)}>检测候选</button></div>
    {error && <div className="notice warn" role="alert">{error}</div>}
    <div className="outlet-options">{options.map(option => {
      const health = healthByName.get(option)
      const selected = option === group.selected
      return <button className={`outlet-option ${selected ? 'selected' : ''}`} type="button" key={option} aria-pressed={selected} disabled={!selectable || Boolean(switching)} onClick={() => void select(option)}>
        <span className="option-check" aria-hidden="true">{selected ? '✓' : ''}</span>
        <span className="outlet-option-main"><strong>{option}</strong><span>{health?.selected && health.selected !== option ? `当前链路 → ${health.selected}` : health?.provider || health?.type || '策略候选'}</span><small>{testedAgo(health?.tested_at)}</small></span>
        <span className="option-meta">{health?.type && <span className="protocol-chip">{health.type}</span>}{health?.udp && <span className="protocol-chip">UDP</span>}<ProxyHealthBadge health={health} testing={testing.has(option)} /></span>
        {switching === option && <span className="switch-overlay">正在切换…</span>}
      </button>
    })}</div>
    {!options.length && <Empty text="没有匹配的出口" />}
    <p className="dialog-footnote">{selectable ? '选择 Selector 候选会即时生效。' : '这是自动策略组，候选仅供查看，不能手动选择。'} 延迟由网关 Mac 上的 mihomo 发起探测，只表示节点到检测地址的可达性。</p>
  </dialog>
}
