import { useEffect, useRef, useState } from 'react'
import type { DeviceTraffic, DeviceTrafficRow, TrafficHistoryPoint } from '../types'
import { formatBytes, formatRate } from '../trafficFormat'
import { deviceKey } from '../hooks/useDeviceTraffic'
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
  const selectedDevice = traffic?.devices.find(device => deviceKey(device.mac, device.ip) === selectedKey) ?? null

  useEffect(() => () => {
    if (closeTimer.current !== null) window.clearTimeout(closeTimer.current)
  }, [])

  const selectDevice = (device: DeviceTrafficRow) => {
    const key = deviceKey(device.mac, device.ip)
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
          {traffic?.devices.length ? <div className="device-traffic-grid" aria-label="活跃设备流量">
            <div className="device-traffic-grid-head">
              <span>设备</span><span>IP</span><span>连接</span><span>↑ 当前</span><span>↓ 当前</span><span>主出口</span>
            </div>
            {traffic.devices.map(device => {
              const key = deviceKey(device.mac, device.ip)
              const name = deviceName(device)
              const expanded = key === selectedKey && detailOpen
              return <button className={`device-traffic-row ${expanded ? 'selected' : ''}`} type="button" key={key} aria-label={`查看 ${name} ${device.ip} 流量趋势`} aria-expanded={expanded} onClick={() => selectDevice(device)}>
                <span className="traffic-device"><StatusDot status={device.online ? 'running' : 'stopped'} /><span><strong>{name}</strong><small>{deviceIdentityDetail(device)}</small></span></span>
                <span className="traffic-ip"><code>{device.ip}</code></span>
                <span className="traffic-connections">{device.active_connections}</span>
                <RateCell rate={device.upload_rate} total={device.upload} />
                <RateCell rate={device.download_rate} total={device.download} />
                <span className="traffic-egress" title={device.primary_egress}><strong>{compactEgress(device.primary_egress)}</strong><small>{device.primary_egress || '暂无出口'}</small></span>
              </button>
            })}
          </div> : <Empty text={traffic ? '暂无 DHCP、静态登记或当前流量观察到的 LAN 设备' : '正在读取设备流量…'} />}
          {traffic && <div className="traffic-summary"><strong>合计 {traffic.totals.devices} 台 · {traffic.totals.active_connections} 个设备连接 · ↑ {formatRate(traffic.totals.upload_rate)} · ↓ {formatRate(traffic.totals.download_rate)}</strong>{traffic.unmatched_connections > 0 && <small>另有 {traffic.unmatched_connections} 个连接无法匹配当前 LAN 设备身份，未计入设备合计。</small>}</div>}
          {error && traffic && <small className="traffic-refresh-error">刷新失败：{error}</small>}
        </div>
        <aside className="device-trend-shell" aria-hidden={!detailOpen}>
          {selectedDevice && <TrafficTrendCard
            title={`${deviceName(selectedDevice)} 流量趋势`}
            subtitle={`${selectedDevice.ip} · ${selectedDevice.primary_egress || '暂无出口信息'}`}
            history={history}
            deviceKey={deviceKey(selectedDevice.mac, selectedDevice.ip)}
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
  if (device.name) return device.name
  if (device.hostname) return device.hostname
  if (!device.mac) return `当前设备 ${device.ip}`
  const parts = device.mac.toLowerCase().split(':')
  return `未知设备 ${parts.length > 3 ? `${parts.slice(0, 3).join(':')}:…` : device.mac.toLowerCase()}`
}

function deviceIdentityDetail(device: DeviceTrafficRow) {
  const source = device.identity_source === 'dhcp_lease'
    ? 'DHCP 已验证'
    : device.identity_source === 'registered_static'
      ? '静态登记'
      : device.identity_source === 'observed_traffic'
        ? '流量已观察'
        : '身份来源未标记'
  return device.mac ? `${source} · ${device.mac}` : `${source} · MAC 待识别`
}

function compactEgress(egress?: string) {
  if (!egress) return '—'
  const parts = egress.split(' → ').map(part => part.trim()).filter(Boolean)
  return parts.at(-1) ?? egress
}
