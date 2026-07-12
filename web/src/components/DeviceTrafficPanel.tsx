import { useEffect, useState } from 'react'
import { api } from '../api'
import type { DeviceTraffic, DeviceTrafficRow } from '../types'
import { Empty, SectionTitle, StatusDot } from './Common'

const refreshIntervalMs = 5_000

export function DeviceTrafficPanel({ gateway }: { gateway?: string }) {
  const [traffic, setTraffic] = useState<DeviceTraffic | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    const refresh = async () => {
      try {
        const next = await api.deviceTraffic()
        if (active) { setTraffic(next); setError('') }
      } catch (cause) {
        if (active) setError(cause instanceof Error ? cause.message : String(cause))
      }
    }
    void refresh()
    const timer = window.setInterval(() => void refresh(), refreshIntervalMs)
    return () => { active = false; window.clearInterval(timer) }
  }, [gateway])

  return <section className="section traffic-section">
    <SectionTitle title="设备流量" subtitle="当前活跃会话累计 · 每 5 秒刷新；不是持久化历史流量" />
    {error && !traffic ? <Empty text={`暂时无法读取设备流量：${error}`} /> : <>
      {traffic?.connection_error && <div className="notice warn">{gateway === 'running' || gateway === 'degraded' ? 'mihomo 连接数据暂时不可用；DHCP 租约设备仍会显示。' : '网关未运行；启动后这里会显示各 DHCP 设备的活跃连接流量。'}</div>}
      {traffic?.devices.length ? <div className="traffic-table-wrap"><table className="traffic-table">
        <thead><tr><th>设备</th><th>IP</th><th>活跃连接</th><th>↑ 上行</th><th>↓ 下行</th><th>主出口</th></tr></thead>
        <tbody>{traffic.devices.map(device => <TrafficRow key={`${device.mac}-${device.ip}`} device={device} />)}</tbody>
      </table></div> : <Empty text={traffic ? '暂无 OpenSurge DHCP 租约设备' : '正在读取设备流量…'} />}
      {traffic && <div className="traffic-summary"><strong>合计 {traffic.totals.devices} 台 · {traffic.totals.active_connections} 个活跃连接 · ↑ {formatBytes(traffic.totals.upload)} · ↓ {formatBytes(traffic.totals.download)}</strong>{traffic.unmatched_connections > 0 && <small>另有 {traffic.unmatched_connections} 个连接无法匹配 DHCP 租约，未计入设备合计。</small>}</div>}
      {error && traffic && <small className="traffic-refresh-error">刷新失败：{error}</small>}
    </>}
  </section>
}

function TrafficRow({ device }: { device: DeviceTrafficRow }) {
  const hasTraffic = device.active_connections > 0
  return <tr>
    <td><div className="traffic-device"><StatusDot status={device.online ? 'running' : 'stopped'} /><div><strong>{deviceName(device)}</strong><small>{device.mac}</small></div></div></td>
    <td><code>{device.ip}</code></td>
    <td>{device.active_connections}</td>
    <td>{hasTraffic ? formatBytes(device.upload) : '—'}</td>
    <td>{hasTraffic ? formatBytes(device.download) : '—'}</td>
    <td className="traffic-egress" title={device.primary_egress}>{device.primary_egress || '—'}</td>
  </tr>
}

function deviceName(device: DeviceTrafficRow) {
  if (device.hostname) return device.hostname
  const parts = device.mac.toLowerCase().split(':')
  return `未知设备 ${parts.length > 3 ? `${parts.slice(0, 3).join(':')}:…` : device.mac.toLowerCase()}`
}

export function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const exponent = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  const amount = value / 1024 ** exponent
  const rounded = Math.round(amount * 10) / 10
  return `${Number.isInteger(rounded) ? rounded.toFixed(0) : rounded.toFixed(1)} ${units[exponent]}`
}
