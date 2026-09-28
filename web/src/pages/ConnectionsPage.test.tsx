// @vitest-environment jsdom
import { selectOption } from '../test/select'
import { useState } from 'react'
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ConnectionsPage } from './ConnectionsPage'
import { initialConnectionsView } from '../connections'
import { useConnections } from '../hooks/useConnections'
import type { ConnectionObservation, DeviceTrafficRow, ObservedConnection } from '../types'

vi.mock('../hooks/useConnections', () => ({ useConnections: vi.fn() }))

function device(key: string, name: string, ip: string, active: number, extra: Partial<DeviceTrafficRow> = {}): DeviceTrafficRow {
  return { key, name, ip, mac: '', active_connections: active, online: active > 0, upload: 50, download: 500, upload_rate: 5, download_rate: 50, identity_source: 'registered_static', ...extra }
}
function connection(id: string, owner: string, family: ObservedConnection['source_family'], chain = 'DIRECT'): ObservedConnection {
  return { id, owner_key: owner, source_family: family, upload: 20, download: 200, upload_rate: 2, download_rate: 20, start: '2026-09-20T10:00:00Z', chains: [chain, 'Device group'], rule: 'RuleSet', rule_payload: 'media', metadata: { host: `${id}.example.invalid`, sourceIP: family === 'ipv6' ? 'fdfe:dcba:9878::51' : '192.168.1.151', network: family === 'ipv6' ? 'udp' : 'tcp', process: owner === 'gateway-local' ? 'Safari' : undefined } }
}
function observation(): ConnectionObservation {
  return {
    schema_version: 1, revision: 'r', scope: 'active_sessions', sampled_at: '2026-09-20T10:01:30Z',
    gateway_local: device('gateway-local', '', '192.168.1.20', 1, { identity_source: 'gateway_local' }),
    devices: [device('device:phone', 'Phone', '192.168.1.151', 2, { device_id: 'phone' }), device('device:idle', 'PS5', '192.168.1.152', 0), device('device:printer', 'Printer', '192.168.1.153', 0, { gateway_target: 'upstream_router' })],
    unclassified: device('unclassified', '', '', 0, { identity_source: 'unclassified' }),
    totals: { devices: 3, active_connections: 2, upload: 40, download: 400, upload_rate: 4, download_rate: 40 },
    gateway_totals: { devices: 0, active_connections: 3, upload: 60, download: 600, upload_rate: 6, download_rate: 60 },
    gateway_rates: { upload: 6, download: 60 }, unidentified_device_connections: 0, unclassified_connections: 0, unmatched_connections: 1,
    connections: [connection('phone-v4', 'device:phone', 'ipv4'), connection('phone-v6', 'device:phone', 'ipv6', 'HK'), connection('mac', 'gateway-local', 'ipv4')],
  }
}
function Harness() {
  const [view, setView] = useState(initialConnectionsView)
  return <ConnectionsPage overview={null} view={view} onViewChange={patch => setView(current => ({ ...current, ...patch }))} />
}
beforeEach(() => {
  window.history.replaceState({}, '', '/connections')
  vi.mocked(useConnections).mockReturnValue({ snapshot: observation(), error: '' })
})
afterEach(() => { cleanup(); vi.clearAllMocks() })

describe('ConnectionsPage', () => {
  it('combines both families under the deep-linked device and composes filters', async () => {
    window.history.replaceState({}, '', '/connections?owner=device%3Aphone')
    render(<Harness />)
    expect(screen.getAllByRole('row')).toHaveLength(3)
    expect(screen.queryByRole('button', { name: 'mac.example.invalid' })).toBeNull()
    await selectOption(screen.getByLabelText('来源地址族'), 'ipv6')
    expect(screen.getAllByRole('row')).toHaveLength(2)
    await selectOption(screen.getByLabelText('出口类型'), 'direct')
    expect(screen.getAllByRole('row')).toHaveLength(1)
    await selectOption(screen.getByLabelText('出口类型'), 'proxy')
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'HK' } })
    expect(screen.getAllByRole('row')).toHaveLength(2)
    fireEvent.click(screen.getByRole('button', { name: 'phone-v6.example.invalid' }))
    expect(within(screen.getByRole('region', { name: '连接详情' })).getByText('Device group → HK')).toBeTruthy()
    expect(screen.queryByText('本机进程')).toBeNull()
  })

  it('keeps idle and bypass inventory without claiming offline or zero bypass speed', () => {
    render(<Harness />)
    fireEvent.click(screen.getByRole('button', { name: '查看 PS5 的连接' }))
    expect(screen.getByText('当前筛选下没有活跃连接；没有连接不代表设备离线。')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '查看 Printer 的连接' }))
    expect(screen.getByText('IPv4 直连主路由，该路径不在统计范围内；列表仅展示核心实际观察到的连接。')).toBeTruthy()
    expect(screen.getByText('0 个活跃连接 · ↑ — · ↓ —')).toBeTruthy()
  })

  it('retains the selected connection after it leaves a later snapshot', () => {
    const view = render(<Harness />)
    fireEvent.click(screen.getByRole('button', { name: 'mac.example.invalid' }))
    expect(screen.getByText('Safari')).toBeTruthy()
    const next = observation()
    next.connections = next.connections.filter(item => item.id !== 'mac')
    vi.mocked(useConnections).mockReturnValue({ snapshot: next, error: '' })
    view.rerender(<Harness />)
    expect(screen.getByText('当前快照中已无此连接，以下保留最后一次观察详情。')).toBeTruthy()
    expect(screen.getByText('Safari')).toBeTruthy()
  })

  it('paginates large samples and resets the page when changing filters', () => {
    const next = observation()
    next.connections = Array.from({ length: 61 }, (_, index) => connection(`session-${index}`, 'device:phone', 'ipv4'))
    vi.mocked(useConnections).mockReturnValue({ snapshot: next, error: '' })
    render(<Harness />)
    expect(screen.getAllByRole('row')).toHaveLength(51)
    fireEvent.click(screen.getByRole('button', { name: '下一页' }))
    expect(screen.getAllByRole('row')).toHaveLength(12)
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'session-60.' } })
    expect(screen.getAllByRole('row')).toHaveLength(2)
    expect(screen.getByText('筛选后 1 个连接 · 第 1 / 1 页')).toBeTruthy()
  })

  it('labels stale samples and lets pause freeze the displayed sample', () => {
    vi.mocked(useConnections).mockReturnValue({ snapshot: observation(), error: 'offline' })
    render(<Harness />)
    expect(screen.getByRole('alert').textContent).toContain('上次采样')
    fireEvent.click(screen.getByRole('button', { name: '暂停画面更新' }))
    expect(vi.mocked(useConnections).mock.lastCall?.[0]).toBe(true)
    expect(screen.getByRole('button', { name: '恢复实时更新' }).getAttribute('aria-pressed')).toBe('true')
  })

  it('keeps the inventory when the core is unavailable without claiming no traffic', () => {
    const next = observation()
    next.connection_error = 'core unavailable'
    next.connections = []
    vi.mocked(useConnections).mockReturnValue({ snapshot: next, error: '' })
    render(<Harness />)
    expect(screen.getByRole('button', { name: '查看 PS5 的连接' })).toBeTruthy()
    expect(screen.getByText('连接数据暂不可用，请等待恢复。')).toBeTruthy()
    expect(screen.queryByText('当前筛选下没有活跃连接；没有连接不代表设备离线。')).toBeNull()
    expect(screen.getByText('— 个活跃连接 · ↑ — · ↓ —')).toBeTruthy()
  })
})
