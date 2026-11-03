import type { DeviceTrafficRow, ObservedConnection } from './types'
import { t } from './i18n'

export type ConnectionsViewState = {
  owner: string
  search: string
  protocol: string
  family: string
  route: string
  sort: 'newest' | 'download' | 'upload'
  page: number
  connection: string
}

export function initialConnectionsView(): ConnectionsViewState {
  return { owner: new URLSearchParams(window.location.search).get('owner') || 'all', search: '', protocol: 'all', family: 'all', route: 'all', sort: 'newest', page: 0, connection: '' }
}

export function connectionOwnerKey(device: DeviceTrafficRow): string {
  if (device.key) return device.key
  if (device.identity_source === 'gateway_local') return 'gateway-local'
  if (device.device_id) return `device:${device.device_id}`
  if (device.mac) return `lease:${device.mac.toLowerCase()}`
  return `ip:${device.ip}`
}

export function connectionDeviceName(device: DeviceTrafficRow): string {
  if (device.identity_source === 'gateway_local') return t('本机 Mac')
  if (device.identity_source === 'unclassified') return t('无法归属')
  return device.name || device.hostname || t('设备 {{address}}', { address: device.ip || device.mac })
}

export function connectionMetadata(connection: ObservedConnection, key: string): string {
  const value = connection.metadata?.[key]
  return typeof value === 'string' || typeof value === 'number' ? String(value) : ''
}

export function connectionRoute(connection: ObservedConnection): string {
  const outbound = connection.chains?.[0]?.toUpperCase()
  if (outbound === 'DIRECT') return 'direct'
  if (outbound === 'REJECT' || outbound === 'REJECT-DROP') return 'reject'
  return outbound ? 'proxy' : 'unknown'
}

export function connectionTarget(connection: ObservedConnection): string {
  return connectionMetadata(connection, 'sniffHost') || connectionMetadata(connection, 'host') || connectionMetadata(connection, 'destinationIP') || '—'
}

export function connectionChain(connection: ObservedConnection): string {
  // Mihomo returns the leaf outbound first; display the path from group to leaf.
  return connection.chains?.slice().reverse().join(' → ') || '—'
}

export function filterConnections(connections: ObservedConnection[], devices: DeviceTrafficRow[], view: ConnectionsViewState): ObservedConnection[] {
  const names = new Map(devices.map(device => [connectionOwnerKey(device), connectionDeviceName(device)]))
  const selected = devices.find(device => connectionOwnerKey(device) === view.owner || (device.device_id && `device:${device.device_id}` === view.owner))
  const owner = selected ? connectionOwnerKey(selected) : view.owner
  const query = view.search.trim().toLowerCase()
  return connections.filter(connection => {
    if (owner !== 'all' && connection.owner_key !== owner) return false
    if (view.protocol !== 'all' && connectionMetadata(connection, 'network').toLowerCase() !== view.protocol) return false
    if (view.family !== 'all' && connection.source_family !== view.family) return false
    if (view.route !== 'all' && connectionRoute(connection) !== view.route) return false
    return !query || [names.get(connection.owner_key), connectionTarget(connection), connectionMetadata(connection, 'host'), connectionMetadata(connection, 'sourceIP'), connectionMetadata(connection, 'destinationIP'), connectionMetadata(connection, 'process'), connectionMetadata(connection, 'processPath'), connectionChain(connection), connection.rule, connection.rule_payload].join(' ').toLowerCase().includes(query)
  }).sort((a, b) => {
    const difference = view.sort === 'download' ? b.download_rate - a.download_rate : view.sort === 'upload' ? b.upload_rate - a.upload_rate : (Date.parse(b.start || '') || 0) - (Date.parse(a.start || '') || 0)
    return difference || a.id.localeCompare(b.id)
  })
}

export function connectionDuration(connection: ObservedConnection, sampledAt: string): string {
  const started = Date.parse(connection.start || '')
  const sampled = Date.parse(sampledAt)
  if (!Number.isFinite(started) || !Number.isFinite(sampled)) return '—'
  const seconds = Math.max(0, Math.floor((sampled - started) / 1000))
  if (seconds < 60) return t('{{count}} 秒', { count: seconds })
  if (seconds < 3600) return t('{{minutes}} 分 {{seconds}} 秒', { minutes: Math.floor(seconds / 60), seconds: seconds % 60 })
  return t('{{hours}} 时 {{minutes}} 分', { hours: Math.floor(seconds / 3600), minutes: Math.floor(seconds / 60) % 60 })
}
