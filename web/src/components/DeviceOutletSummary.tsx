import { api } from '../api'
import type { ProxyGroup, ProxyHealthEntry } from '../types'
import { OutletSummary } from './OutletSummary'
import { t } from '../i18n'

type DeviceOutletSummaryProps = {
  device: string
  slot: string
  groupName: string
  groups: ProxyGroup[]
  title: string
  ariaLabel: string
  healthByName: Map<string, ProxyHealthEntry>
  testing: Set<string>
  onTest: (names: string[]) => Promise<void>
  onChanged: () => Promise<void>
  onSelectionChanged?: (policy: string) => void
  onEditCandidates?: () => void
}

export function DeviceOutletSummary({ device, slot, groupName, groups, title, ariaLabel, healthByName, testing, onTest, onChanged, onSelectionChanged, onEditCandidates }: DeviceOutletSummaryProps) {
  const group = groups.find(item => item.name === groupName)
  if (!group) return <button className="outlet-summary unavailable" type="button" aria-label={t(ariaLabel)} disabled><span className="outlet-summary-copy"><small>{t(title)}</small><strong>{t('重载后可用')}</strong></span></button>
  return <OutletSummary inline title={title} ariaLabel={ariaLabel} group={group} healthByName={healthByName} testing={testing} onTest={onTest} onEditCandidates={onEditCandidates} onSelect={async policy => {
    const changed = policy !== group.selected
    await api.selectDevicePolicy(device, slot, policy)
    if (changed) onSelectionChanged?.(policy)
    await onChanged()
  }} />
}
