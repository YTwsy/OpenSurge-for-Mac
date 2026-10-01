import { useId, useRef, useState } from 'react'
import type { ProxyGroup, ProxyHealthEntry } from '../types'
import { OutletPicker } from './OutletPicker'
import { ProxyHealthBadge } from './ProxyHealthBadge'
import { policyDisplayName } from '../policyDisplay'
import { t } from '../i18n'

type OutletSummaryProps = {
  title: string
  group: ProxyGroup
  healthByName: Map<string, ProxyHealthEntry>
  testing: Set<string>
  onTest: (names: string[]) => Promise<void>
  onSelect: (policy: string) => Promise<void>
  ariaLabel: string
  inline?: boolean
  onEditCandidates?: () => void
}

export function OutletSummary({ title, group, healthByName, testing, onTest, onSelect, ariaLabel, inline = false, onEditCandidates }: OutletSummaryProps) {
  const [open, setOpen] = useState(false)
  const panelID = useId()
  const trigger = useRef<HTMLButtonElement>(null)
  const close = () => { setOpen(false); trigger.current?.focus({ preventScroll: true }) }
  const selectedHealth = healthByName.get(group.selected)
  const leafName = selectedHealth?.selected && selectedHealth.selected !== group.selected ? selectedHealth.selected : ''
  const displayedHealth = leafName ? healthByName.get(leafName) ?? selectedHealth : selectedHealth

  return <>
    <button ref={trigger} className="outlet-summary" type="button" aria-label={t(ariaLabel)} aria-haspopup={inline ? undefined : 'dialog'} aria-expanded={inline ? open : undefined} aria-controls={open ? panelID : undefined} onClick={() => setOpen(value => !value)}>
      <span className="outlet-summary-copy"><small>{t(title)}</small><strong>{group.selected ? policyDisplayName(group.selected, selectedHealth) : t('未选择')}</strong>{leafName && <span>{t('当前链路')} → {policyDisplayName(leafName, displayedHealth)}</span>}</span>
      <span className="outlet-summary-state"><ProxyHealthBadge health={displayedHealth} testing={testing.has(leafName || group.selected)} compact /><span className="summary-action">{t(open && inline ? '收起' : '更换')}</span></span>
    </button>
    <OutletPicker id={panelID} inline={inline} open={open} title={title} group={group} healthByName={healthByName} testing={testing} onTest={onTest} onSelect={onSelect} onClose={close} onEditCandidates={onEditCandidates ? () => { setOpen(false); onEditCandidates() } : undefined} />
  </>
}
