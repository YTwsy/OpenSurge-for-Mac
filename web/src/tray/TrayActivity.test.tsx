// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { api } from '../api'
import { desktopAction } from '../desktop'
import { activateLanguage, prepareLanguage } from '../i18n'
import type { DeviceTraffic, DeviceTrafficRow } from '../types'
import { TrayActivity } from './TrayActivity'

vi.mock('../api', () => ({ api: { localRouting: vi.fn() } }))
vi.mock('../desktop', () => ({ desktopAction: vi.fn() }))
const row = (key: string, name: string, rate: number, connections = 1): DeviceTrafficRow => ({ key, name, ip: '192.0.2.20', mac: '', identity_source: 'registered_static', online: true, active_connections: connections, upload: 0, download: 0, upload_rate: 0, download_rate: rate })
const fixture: DeviceTraffic = {
 schema_version: 1, revision: 'sample', sampled_at: '', scope: 'active_sessions',
 gateway_local: { ...row('gateway-local', 'Mac', 1_000_000, 4), identity_source: 'gateway_local' },
 devices: [row('device:tv', 'Apple TV', 8_000_000, 3), row('device:ps5', 'PS5', 3_000_000, 2), row('device:idle', 'Idle phone', 0, 0)],
 totals: { devices: 3, active_connections: 5, upload: 0, download: 0, upload_rate: 0, download_rate: 11_000_000 },
 gateway_rates: { upload: 100_000, download: 12_000_000 }, unidentified_device_connections: 0, unclassified_connections: 1, unmatched_connections: 0,
}
beforeEach(async () => {
 await prepareLanguage('en'); activateLanguage('en')
 vi.useFakeTimers()
 vi.mocked(desktopAction).mockImplementation(async () => ({ traffic: { ...fixture, sampled_at: new Date().toISOString() }, failed: false, history: [
  { time: Date.now() - 2000, ...fixture.gateway_rates }, { time: Date.now(), ...fixture.gateway_rates },
 ] }))
 vi.mocked(api.localRouting).mockResolvedValue({ schema_version: 1, mode: 'rule', available_modes: ['rule', 'direct'], udp_behavior: 'rules', transports: ['tun'], new_connections_only: true, consistent: true })
})
afterEach(() => { cleanup(); vi.useRealTimers(); vi.resetAllMocks(); delete window.__opensurgeWindowVisible; activateLanguage('zh-Hans') })

it('shows live rates, counts active downstream devices, and expands details before navigating', async () => {
 const open = vi.fn()
 await act(async () => { render(<TrayActivity gateway="running" onOpen={open} />) })
 expect(screen.getByText('12 MB/s')).toBeTruthy()
 expect(screen.getByText('2 active LAN devices · 10 total connections')).toBeTruthy()
 expect(screen.queryByText('Idle phone')).toBeNull()
 const devices = screen.getAllByRole('button', { name: /^Device details: / })
 expect(devices[0].textContent).toContain('This Mac')
 fireEvent.click(screen.getByRole('button', { name: /View all/ }))
 expect(open).toHaveBeenCalledWith('dashboard', { section: 'active-devices' })
 open.mockClear()
 fireEvent.click(screen.getByRole('button', { name: /This Mac’s routing/ }))
 expect(open).not.toHaveBeenCalled()
 expect(screen.getByText('Applies to this Mac only. LAN devices keep their own routing settings.')).toBeTruthy()
 fireEvent.click(screen.getByRole('button', { name: 'Configure in desktop window' }))
 expect(open).toHaveBeenCalledWith('devices')
 open.mockClear()
 fireEvent.click(screen.getByRole('button', { name: 'Device details: Apple TV' }))
 expect(open).not.toHaveBeenCalled()
 expect(screen.queryByRole('button', { name: 'Configure in desktop window' })).toBeNull()
 fireEvent.click(screen.getByRole('button', { name: /connections for Apple TV/ }))
 expect(open).toHaveBeenCalledWith('connections', { owner: 'device:tv' })
 expect(document.body.textContent).not.toMatch(/[\u3400-\u9fff]/)
})

it('pauses renderer reads but displays retained native history immediately on reopening', async () => {
 const view = render(<TrayActivity gateway="running" onOpen={vi.fn()} />)
 await act(async () => { await vi.advanceTimersByTimeAsync(2000) })
 expect(screen.getByText('12 MB/s')).toBeTruthy()
 window.__opensurgeWindowVisible = false
 const calls = vi.mocked(desktopAction).mock.calls.length
 await act(async () => { await vi.advanceTimersByTimeAsync(20_000) })
 expect(desktopAction).toHaveBeenCalledTimes(calls)
 window.__opensurgeWindowVisible = true
 await act(async () => { document.dispatchEvent(new Event('visibilitychange')) })
 expect(screen.queryByText('Sampling traffic…')).toBeNull()
 expect(screen.getByText('12 MB/s')).toBeTruthy()
 expect(document.querySelector('.download-line')?.getAttribute('d')).toContain(' C ')
 vi.mocked(desktopAction).mockResolvedValue({ traffic: null, history: null, failed: true })
 await act(async () => { await vi.advanceTimersByTimeAsync(2000) })
 expect(screen.getByText('Traffic unavailable')).toBeTruthy()
 expect(screen.queryByText('Apple TV')).toBeNull()
 expect(screen.queryByText('12 MB/s')).toBeNull()
 view.rerender(<TrayActivity gateway="stopped" onOpen={vi.fn()} />)
 const stoppedCalls = vi.mocked(desktopAction).mock.calls.length
 await act(async () => { await vi.advanceTimersByTimeAsync(4000) })
 expect(desktopAction).toHaveBeenCalledTimes(stoppedCalls)
})

it('renders cached traffic without waiting for a stalled routing request', async () => {
 vi.mocked(api.localRouting).mockReturnValue(new Promise(() => {}))
 await act(async () => { render(<TrayActivity gateway="running" onOpen={vi.fn()} />) })
 expect(screen.getByText('12 MB/s')).toBeTruthy()
 expect(screen.getByText('Loading…')).toBeTruthy()
 await act(async () => { await vi.advanceTimersByTimeAsync(4000) })
 expect(desktopAction).toHaveBeenCalledTimes(5)
 expect(api.localRouting).toHaveBeenCalledOnce()
})

it('does not turn the first baseline sample into a zero rate', async () => {
 vi.mocked(desktopAction).mockResolvedValueOnce({ traffic: fixture, history: [{ time: Date.now(), upload: 0, download: 0 }], failed: false })
 await act(async () => { render(<TrayActivity gateway="running" onOpen={vi.fn()} />) })
 expect(screen.getByText('Sampling traffic…')).toBeTruthy()
 expect(screen.queryByText('12 MB/s')).toBeNull()
 await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
 expect(screen.getByText('12 MB/s')).toBeTruthy()
})
