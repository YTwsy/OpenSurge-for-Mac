import { api } from '../api'
import type { ProxyGroup, ProxyHealthEntry } from '../types'
import { OutletSummary } from './OutletSummary'

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
}

export function DeviceOutletSummary({ device, slot, groupName, groups, title, ariaLabel, healthByName, testing, onTest, onChanged }: DeviceOutletSummaryProps) {
  const group = groups.find(item => item.name === groupName)
  if (!group) return <button className="outlet-summary unavailable" type="button" aria-label={ariaLabel} disabled><span className="outlet-summary-copy"><small>{title}</small><strong>重载后可用</strong></span></button>
  return <OutletSummary title={title} ariaLabel={ariaLabel} group={group} healthByName={healthByName} testing={testing} onTest={onTest} onSelect={async policy => { await api.selectDevicePolicy(device, slot, policy); await onChanged() }} />
}
