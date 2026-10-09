// @vitest-environment jsdom
import { act, cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { TrafficTrendCard } from '../components/TrafficTrendCard'
import type { DeviceTraffic } from '../types'
import { appendTrafficPoint, DeviceTrafficProvider, useDeviceTraffic } from './useDeviceTraffic'

vi.mock('../api', () => ({ api: { deviceTraffic: vi.fn() } }))

const start = Date.parse('2026-10-09T00:00:00Z')

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(start)
  vi.mocked(api.deviceTraffic).mockImplementation(async () => sample(Date.now(), 10_000))
})
afterEach(() => {
  cleanup()
  delete window.__opensurgeWindowVisible
  vi.useRealTimers()
  vi.clearAllMocks()
})

function Chart() {
  const { history } = useDeviceTraffic()
  return <><output>{history.length}</output><TrafficTrendCard title="流量趋势" subtitle="" history={history} /></>
}

function Shell({ page = 'dashboard', gateway = 'running' }: { page?: string; gateway?: string }) {
  return <DeviceTrafficProvider gateway={gateway}>{page === 'dashboard' ? <Chart key={page} /> : <div>Another page</div>}</DeviceTrafficProvider>
}

const scale = () => screen.getByRole('img').getAttribute('aria-description')
const curve = () => document.querySelector('.trend-line.download')?.getAttribute('d')
const tick = (ms = 0) => act(async () => { await vi.advanceTimersByTimeAsync(ms) })

describe('traffic history across navigation', () => {
  it('keeps the same curve and scale on a quick round trip and continues sampling on other pages', async () => {
    vi.mocked(api.deviceTraffic).mockResolvedValueOnce(sample(start, 900_000))
    const { rerender } = render(<Shell />)
    await tick()
    await tick(2_000)
    const before = curve()
    expect(scale()).toBe('纵轴范围：0–1 MB/s')
    rerender(<Shell page="network" />)
    rerender(<Shell />)
    expect(curve()).toBe(before)
    expect(scale()).toBe('纵轴范围：0–1 MB/s')
    expect(api.deviceTraffic).toHaveBeenCalledTimes(2)

    rerender(<Shell page="network" />)
    await tick(6_000)
    rerender(<Shell />)
    expect(screen.getByRole('status').textContent).toBe('5')
    expect(scale()).toBe('纵轴范围：0–1 MB/s')
  })

  it('pauses hidden-window reads and drops expired samples after a long absence', async () => {
    vi.mocked(api.deviceTraffic).mockResolvedValueOnce(sample(start, 900_000))
    render(<Shell />)
    await tick()
    window.__opensurgeWindowVisible = false
    document.dispatchEvent(new Event('visibilitychange'))
    await tick(70_000)
    expect(api.deviceTraffic).toHaveBeenCalledOnce()
    window.__opensurgeWindowVisible = true
    await act(async () => { document.dispatchEvent(new Event('visibilitychange')) })
    expect(screen.getByRole('status').textContent).toBe('1')
    expect(scale()).toBe('纵轴范围：0–10 kB/s')
  })

  it('does not rerender unrelated pages as the retained history receives samples', async () => {
    const OtherPage = vi.fn(() => <div>Another page</div>)
    render(<DeviceTrafficProvider gateway="running"><OtherPage /></DeviceTrafficProvider>)
    await tick(6_000)
    expect(api.deviceTraffic).toHaveBeenCalledTimes(4)
    expect(OtherPage).toHaveBeenCalledOnce()
  })

  it('keeps history through degraded status, but rejects late reads from the previous gateway lifecycle', async () => {
    vi.mocked(api.deviceTraffic).mockResolvedValueOnce(sample(start, 900_000))
    const { rerender } = render(<Shell />)
    await tick()
    rerender(<Shell gateway="degraded" />)
    expect(scale()).toBe('纵轴范围：0–1 MB/s')
    let finish!: (value: DeviceTraffic) => void
    vi.mocked(api.deviceTraffic).mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await tick(2_000)
    rerender(<Shell gateway="stopped" />)
    await tick()
    await act(async () => finish(sample(start + 1_000, 5_000_000)))
    expect(scale()).toBe('纵轴范围：0–10 kB/s')
    expect(screen.getByRole('status').textContent).toBe('1')
  })

  it('bounds history by both time and count, ignoring duplicate and older responses', () => {
    let history = appendTrafficPoint([], sample(start, 100))
    expect(appendTrafficPoint(history, sample(start, 500))).toBe(history)
    expect(appendTrafficPoint(history, sample(start - 2_000, 500))).toBe(history)
    for (let i = 1; i <= 40; i++) history = appendTrafficPoint(history, sample(start + i * 1_000, i))
    expect(history).toHaveLength(30)
    history = appendTrafficPoint(history, sample(start + 90_000, 50))
    expect(history).toHaveLength(11)
    expect(history[0].sampled_at).toBe(new Date(start + 31_000).toISOString())
    history = appendTrafficPoint(history, sample(start + 151_000, 60))
    expect(history).toHaveLength(1)
  })
})

function sample(at: number, download: number): DeviceTraffic {
  return {
    schema_version: 1, revision: 'test', sampled_at: new Date(at).toISOString(), scope: 'active_sessions',
    gateway_local: { ip: '192.0.2.10', mac: '', online: true, active_connections: 1, upload: 0, download: 0, upload_rate: 0, download_rate: download },
    devices: [], totals: { devices: 0, active_connections: 0, upload: 0, download: 0, upload_rate: 0, download_rate: 0 },
    gateway_rates: { upload: 0, download }, unidentified_device_connections: 0, unclassified_connections: 0, unmatched_connections: 0,
  }
}
