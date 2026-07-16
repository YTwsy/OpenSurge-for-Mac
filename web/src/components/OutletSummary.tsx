import { useState } from 'react'
import type { ProxyGroup, ProxyHealthEntry } from '../types'
import { OutletPicker } from './OutletPicker'
import { ProxyHealthBadge } from './ProxyHealthBadge'

type OutletSummaryProps = {
  title: string
  group: ProxyGroup
  healthByName: Map<string, ProxyHealthEntry>
  testing: Set<string>
  onTest: (names: string[]) => Promise<void>
  onSelect: (policy: string) => Promise<void>
  ariaLabel: string
}

export function OutletSummary({ title, group, healthByName, testing, onTest, onSelect, ariaLabel }: OutletSummaryProps) {
  const [open, setOpen] = useState(false)
  const selectedHealth = healthByName.get(group.selected)
  const leafName = selectedHealth?.selected && selectedHealth.selected !== group.selected ? selectedHealth.selected : ''
  const displayedHealth = leafName ? healthByName.get(leafName) ?? selectedHealth : selectedHealth

  return <>
    <button className="outlet-summary" type="button" aria-label={ariaLabel} aria-haspopup="dialog" onClick={() => setOpen(true)}>
      <span className="outlet-summary-copy"><small>{title}</small><strong>{group.selected || '未选择'}</strong>{leafName && <span>当前链路 → {leafName}</span>}</span>
      <span className="outlet-summary-state"><ProxyHealthBadge health={displayedHealth} testing={testing.has(leafName || group.selected)} compact /><span className="summary-action">更换</span></span>
    </button>
    <OutletPicker open={open} title={title} group={group} healthByName={healthByName} testing={testing} onTest={onTest} onSelect={onSelect} onClose={() => setOpen(false)} />
  </>
}
