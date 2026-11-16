import { t } from './i18n'
import type { CompiledDevice, Lease, ObservedDevice, PolicyDevice } from './types'

export type DeviceIdentity = {
  state: 'ready' | 'observed' | 'conflict' | 'confirmation' | 'address_changed' | 'waiting'
  tone: string
  text: string
  detail?: string
  observedIPv4?: string
  observedMAC?: string
  observations?: Array<{ ip: string; mac: string }>
  privateMAC?: boolean
}

export const normalizeDeviceMAC = (value: string) => value.trim().toLowerCase()
const validMAC = (value: string) => /^(?:[0-9a-f]{2}:){5}[0-9a-f]{2}$/.test(value) && (parseInt(value.slice(0, 2), 16) & 1) === 0 && value !== '00:00:00:00:00:00'
export const identityNeedsConfirmation = (identity: DeviceIdentity | null) => Boolean(identity && ['confirmation', 'address_changed', 'conflict'].includes(identity.state))

export function deviceIdentity(applied: CompiledDevice, topology: string | undefined, leases: Lease[], observed: ObservedDevice[], owners: Array<PolicyDevice | CompiledDevice> = []): DeviceIdentity {
  if (topology !== 'same_lan') {
    const lease = leases.find(item => normalizeDeviceMAC(item.mac) === normalizeDeviceMAC(applied.mac) && item.ip === applied.ipv4 && item.online && (!item.expires_at || Date.parse(item.expires_at) > Date.now()))
    return lease ? { state: 'ready', tone: 'ready', text: t('DHCP 身份已验证') }
      : { state: 'waiting', tone: '', text: t('身份待确认：需要在线且未过期的精确 MAC / IPv4 租约') }
  }
  const conflict = (detail: string): DeviceIdentity => ({ state: 'conflict', tone: 'conflict', text: t('这个地址的设备归属需要检查'), detail,
    observations: observed.filter(item => item.active_connections > 0 && (item.ip === applied.ipv4 || normalizeDeviceMAC(item.mac ?? '') === normalizeDeviceMAC(applied.mac)))
      .flatMap(item => (item.observed_macs?.length ? item.observed_macs : [item.mac ?? '']).map(mac => ({ ip: item.ip, mac }))),
  })
  const ownerOf = (ip: string, mac: string) => owners.find(item => item.id !== applied.id && (item.ipv4 === ip || (mac && normalizeDeviceMAC(item.mac) === mac)))
  const ownerConflict = (owner: PolicyDevice | CompiledDevice) => conflict(t('当前地址已登记给“{{name}}”。请检查两台设备的登记信息后再更新。', { name: ('name' in owner && owner.name) || owner.id }))
  const active = observed.filter(item => item.active_connections > 0)
  const currentRows = active.filter(item => item.ip === applied.ipv4)
  if (currentRows.length > 1 || currentRows.some(item => item.mac_conflict)) return conflict(t('检测到同一 IP 的多个设备地址交替出现。请检查是否有设备使用了相同 IP；稳定后可重新检测。'))
  const current = currentRows[0] ?? observed.find(item => item.ip === applied.ipv4)
  const registeredMAC = normalizeDeviceMAC(applied.mac)
  if (current) {
    if (current.active_connections === 0) return { state: 'observed', tone: 'observed', text: current.mac ? t('已观察到邻居 MAC {{mac}}，等待该 IPv4 经过 Mac', { mac: current.mac }) : t('固定 IPv4 已登记，等待流量经过 Mac') }
    if (!registeredMAC) return { state: 'observed', tone: 'observed', text: current.mac ? t('固定 IPv4 已生效 · 当前观察到 {{mac}}', { mac: current.mac }) : t('当前有流量：按固定 IPv4 匹配，MAC 为可选身份信息') }
    const mac = normalizeDeviceMAC(current.mac ?? '')
    if (!validMAC(mac) || !current.neighbor_observed) return { state: 'observed', tone: 'observed', text: t('当前有流量：固定 IPv4 已观察，MAC 尚未验证') }
    if (mac !== registeredMAC) {
      const owner = ownerOf(current.ip, mac)
      if (owner) return ownerConflict(owner)
      if (active.some(item => item.ip !== current.ip && [registeredMAC, mac].includes(normalizeDeviceMAC(item.mac ?? '')))) return conflict(t('原设备或当前设备地址仍出现在其他 IP，暂时无法确认归属。请核对设备后重新检测。'))
      return { state: 'confirmation', tone: 'changed', text: t('需要确认这台设备'), observedIPv4: current.ip, observedMAC: mac, privateMAC: (parseInt(mac.slice(0, 2), 16) & 2) !== 0 }
    }
    return { state: 'ready', tone: 'ready', text: t('流量与邻居已观察：MAC / IPv4 匹配') }
  }
  const sameMAC = registeredMAC ? active.filter(item => item.ip !== applied.ipv4 && item.neighbor_observed && normalizeDeviceMAC(item.mac ?? '') === registeredMAC) : []
  if (sameMAC.length > 1 || sameMAC.some(item => item.mac_conflict)) return conflict(t('同一设备地址出现在多个 IP，暂时无法确定当前地址。请核对设备后重新检测。'))
  if (sameMAC.length === 1) {
    const owner = ownerOf(sameMAC[0].ip, registeredMAC)
    if (owner) return ownerConflict(owner)
    return { state: 'address_changed', tone: 'changed', text: t('发现设备的当前 IP 已变化'), observedIPv4: sameMAC[0].ip, observedMAC: registeredMAC }
  }
  return { state: 'waiting', tone: '', text: t(registeredMAC ? '静态配置身份：等待该 IPv4 经过 Mac' : '固定 IPv4 策略：等待该地址经过 Mac') }
}
