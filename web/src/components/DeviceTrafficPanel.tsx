import { useEffect, useRef, useState } from 'react'
import type { DeviceTraffic, DeviceTrafficRow, TrafficHistoryPoint } from '../types'
import { formatBytes, formatRate } from '../trafficFormat'
import { deviceKey, gatewayLocalDeviceKey } from '../hooks/useDeviceTraffic'
import { Empty, StatusDot } from './Common'
import { TrafficTrendCard } from './TrafficTrendCard'

type DeviceTrafficPanelProps = {
  gateway?: string
  traffic: DeviceTraffic | null
  history: TrafficHistoryPoint[]
  error: string
}

const detailTransitionMs = 460

export function DeviceTrafficPanel({ gateway, traffic, history, error }: DeviceTrafficPanelProps) {
  const [selectedKey, setSelectedKey] = useState('')
  const [detailOpen, setDetailOpen] = useState(false)
  const closeTimer = useRef<number | null>(null)
  const visibleDevices = traffic ? [traffic.gateway_local, ...traffic.devices] : []
  const selectedDevice = visibleDevices.find(device => trafficRowKey(device) === selectedKey) ?? null

  useEffect(() => () => {
    if (closeTimer.current !== null) window.clearTimeout(closeTimer.current)
  }, [])

  const selectDevice = (device: DeviceTrafficRow) => {
    const key = trafficRowKey(device)
    if (closeTimer.current !== null) window.clearTimeout(closeTimer.current)
    if (key === selectedKey && detailOpen) {
      setDetailOpen(false)
      closeTimer.current = window.setTimeout(() => {
        setSelectedKey('')
        closeTimer.current = null
      }, detailTransitionMs)
      return
    }
    setSelectedKey(key)
    setDetailOpen(true)
  }

  return <section className="section traffic-section">
    <div className="traffic-section-heading"><div><h2>活跃设备</h2><p>实时速度来自相邻连接样本；累计值仅覆盖当前活跃会话</p></div>{selectedDevice && <button type="button" onClick={() => selectDevice(selectedDevice)}>{detailOpen ? '收起趋势' : '展开趋势'}</button>}</div>
    {error && !traffic ? <Empty text={`暂时无法读取设备流量：${error}`} /> : <>
      {traffic?.connection_error && <div className="notice warn">{gateway === 'running' || gateway === 'degraded' ? 'mihomo 连接数据暂时不可用；已有设备清单仍会显示。' : '网关未运行；DHCP 租约或已应用静态登记仍会显示，启动后才有活跃连接流量。'}</div>}
      <div className={`device-traffic-layout ${detailOpen ? 'expanded' : ''}`}>
        <div className="device-traffic-list">
          {visibleDevices.length ? <div className="device-traffic-grid" aria-label="活跃设备流量">
            <div className="device-traffic-grid-head">
              <span>设备</span><span>IP</span><span>连接</span><span>↑ 当前</span><span>↓ 当前</span><span>主出口</span>
            </div>
            {visibleDevices.map(device => {
              const key = trafficRowKey(device)
              const name = deviceName(device)
              const expanded = key === selectedKey && detailOpen
              const local = device.identity_source === 'gateway_local'
              const online = local ? gatewayActive(gateway) : device.online
              return <button className={`device-traffic-row ${expanded ? 'selected' : ''}`} type="button" key={key} aria-label={`查看 ${name} ${device.ip} 流量趋势`} aria-expanded={expanded} onClick={() => selectDevice(device)}>
                <span className="traffic-device"><StatusDot status={online ? 'running' : 'stopped'} /><span><strong>{name}</strong><small>{deviceIdentityDetail(device, gateway)}</small></span></span>
                <span className="traffic-ip"><code>{device.ip || '—'}</code></span>
                <span className="traffic-connections">{device.active_connections}</span>
                <RateCell rate={device.upload_rate} total={device.upload} />
                <RateCell rate={device.download_rate} total={device.download} />
                <span className="traffic-egress" title={device.primary_egress}><strong>{compactEgress(device.primary_egress)}</strong><small>{device.primary_egress || '暂无出口'}</small></span>
              </button>
            })}
          </div> : <Empty text={traffic ? '暂无 DHCP、静态登记或当前流量观察到的 LAN 设备' : '正在读取设备流量…'} />}
          {traffic && <div className="traffic-summary">
            <strong>合计 {traffic.totals.devices} 台设备接入 · {traffic.totals.active_connections} 个连接 · ↑ {formatRate(traffic.totals.upload_rate)} · ↓ {formatRate(traffic.totals.download_rate)}</strong>
            {traffic.unidentified_device_connections > 0 && <small>其中 {traffic.unidentified_device_connections} 个待识别设备连接，仅确认了当前 LAN 源 IP。</small>}
            {traffic.unclassified_connections > 0 && <small>另有 {traffic.unclassified_connections} 个连接无法判断来源，请在诊断中查看。</small>}
          </div>}
          {error && traffic && <small className="traffic-refresh-error">刷新失败：{error}</small>}
        </div>
        <aside className="device-trend-shell" aria-hidden={!detailOpen}>
          {selectedDevice && <TrafficTrendCard
            title={`${deviceName(selectedDevice)} 流量趋势`}
            subtitle={`${selectedDevice.ip} · ${selectedDevice.primary_egress || '暂无出口信息'}`}
            history={history}
            deviceKey={trafficRowKey(selectedDevice)}
            className="device-trend-card"
          />}
        </aside>
      </div>
    </>}
  </section>
}

function RateCell({ rate = 0, total = 0 }: { rate?: number; total?: number }) {
  return <span className="traffic-rate"><strong>{formatRate(rate)}</strong><small>累计 {formatBytes(total)}</small></span>
}

function deviceName(device: DeviceTrafficRow) {
  if (device.identity_source === 'gateway_local') return '本机 Mac'
  if (device.name) return device.name
  if (device.hostname) return device.hostname
  if (!device.mac) return `当前设备 ${device.ip}`
  const parts = device.mac.toLowerCase().split(':')
  return `未知设备 ${parts.length > 3 ? `${parts.slice(0, 3).join(':')}:…` : device.mac.toLowerCase()}`
}

function deviceIdentityDetail(device: DeviceTrafficRow, gateway?: string) {
  if (device.identity_source === 'gateway_local') return gatewayLocalDetail(device, gateway)
  const source = device.identity_source === 'dhcp_lease'
    ? 'DHCP 已验证'
    : device.identity_source === 'registered_static'
      ? '静态登记'
      : device.identity_source === 'observed_traffic'
        ? '流量已观察'
        : '身份来源未标记'
  return device.mac ? `${source} · ${device.mac}` : `${source} · MAC 待识别`
}

function gatewayLocalDetail(device: DeviceTrafficRow, gateway?: string) {
  if (!gatewayActive(gateway)) return '网关本机 · 网关未运行'
  switch (device.transport) {
  case 'tun':
    return '网关本机 · TUN'
  case 'explicit_proxy':
    return '网关本机 · 显式代理'
  case 'tun_and_explicit_proxy':
    return '网关本机 · TUN / 显式代理'
  case 'other':
    return '网关本机 · 本机流量'
  default:
    return '网关本机 · 暂无活跃连接'
  }
}

function gatewayActive(gateway?: string) {
  return gateway?.startsWith('running') === true || gateway === 'degraded'
}

function trafficRowKey(device: DeviceTrafficRow) {
  return device.identity_source === 'gateway_local' ? gatewayLocalDeviceKey : deviceKey(device.mac, device.ip)
}

function compactEgress(egress?: string) {
  if (!egress) return '—'
  const parts = egress.split(' → ').map(part => part.trim()).filter(Boolean)
  return parts.at(-1) ?? egress
}
