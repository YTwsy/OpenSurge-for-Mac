import type { ProxyHealthEntry } from './types'

export const TAILSCALE_POLICY = 'open-surge/tailscale'

export function policyDisplayName(name: string, health?: ProxyHealthEntry) {
  if (health?.display_name) return health.display_name
  if (name === TAILSCALE_POLICY) return 'Tailscale Exit Node'
  return name
}
