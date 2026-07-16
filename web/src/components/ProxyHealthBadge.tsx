import { healthLabel, healthTone } from '../proxyHealth'
import type { ProxyHealthEntry } from '../types'

type ProxyHealthBadgeProps = {
  health?: ProxyHealthEntry
  testing?: boolean
  compact?: boolean
}

export function ProxyHealthBadge({ health, testing = false, compact = false }: ProxyHealthBadgeProps) {
  const label = healthLabel(health, testing)
  return <span className={`health-badge ${healthTone(health, testing)} ${compact ? 'compact' : ''}`} title={health?.error || label}>
    <span className="health-indicator" aria-hidden="true" />
    <span>{label}</span>
  </span>
}
