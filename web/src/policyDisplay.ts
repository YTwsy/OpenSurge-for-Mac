import type { ProxyHealthEntry } from './types'
import { t } from './i18n'

export const TAILSCALE_POLICY = 'open-surge/tailscale'
export const TAILSCALE_EXIT_POLICY = 'open-surge/tailscale-exit'

export function policyDisplayName(name: string, health?: ProxyHealthEntry, tailscaleDisplayName = '') {
  if (health?.display_name) return health.display_name
  if (name === TAILSCALE_EXIT_POLICY) return tailscaleDisplayName ? `${tailscaleDisplayName} · Exit Node` : t('Tailscale Exit Node')
  if (name === TAILSCALE_POLICY) return tailscaleDisplayName || t('Tailscale 出口节点')
  return name
}
