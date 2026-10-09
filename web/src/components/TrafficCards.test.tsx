// @vitest-environment jsdom
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { TrafficHistoryPoint } from '../types'
import { LiveRateCard } from './LiveRateCard'
import { TrafficTrendCard } from './TrafficTrendCard'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('traffic card sampling', () => {
  it('updates upload, peak and the smooth curve together without scheduling animation frames', () => {
    disableAnimationFrames()
    const initial = [point('2026-07-24T00:00:00Z', 1_000, 2_000)]
    const { rerender } = render(<LiveRateCard direction="upload" history={initial} value={1_000} />)
    const initialPath = screen.getByLabelText('上传当前速度 1 kB/s').querySelector('path.rate-line')?.getAttribute('d')

    rerender(<LiveRateCard
      direction="upload"
      history={[...initial, point('2026-07-24T00:00:02Z', 9_000, 8_000)]}
      value={9_000}
    />)

    const card = screen.getByLabelText('上传当前速度 9 kB/s')
    expect(within(card).getByText('9')).toBeTruthy()
    expect(within(card).getByText('峰值 9 kB/s')).toBeTruthy()
    expect(card.querySelector('path.rate-line')?.getAttribute('d')).not.toBe(initialPath)
    expect(card.querySelector('path.rate-line')?.getAttribute('d')).toContain(' C ')
    expect(requestAnimationFrame).not.toHaveBeenCalled()
  })

  it('updates trend numbers and curves together with a shared, labelled scale', () => {
    disableAnimationFrames()
    const initial = [point('2026-07-24T00:00:00Z', 1_000, 2_000)]
    const { rerender } = render(<TrafficTrendCard title="流量趋势" subtitle="测试" history={initial} />)
    const initialPath = document.querySelector('path.trend-line.upload')?.getAttribute('d')

    rerender(<TrafficTrendCard
      title="流量趋势"
      subtitle="测试"
      history={[...initial, point('2026-07-24T00:00:02Z', 9_000, 8_000)]}
    />)

    expect(screen.getByText('↑ 9 kB/s')).toBeTruthy()
    expect(screen.getByText('↓ 8 kB/s')).toBeTruthy()
    expect(document.querySelector('path.trend-line.upload')?.getAttribute('d')).not.toBe(initialPath)
    expect(screen.getByRole('img').getAttribute('aria-description')).toBe('纵轴范围：0–10 kB/s')
    expect(document.querySelector('.trend-y-axis')?.textContent).toBe('10 kB/s5 kB/s0')
    expect(requestAnimationFrame).not.toHaveBeenCalled()
  })
})

function disableAnimationFrames() {
  vi.stubGlobal('requestAnimationFrame', vi.fn(() => 1))
  vi.stubGlobal('cancelAnimationFrame', vi.fn())
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false })))
}

function point(sampled_at: string, upload: number, download: number): TrafficHistoryPoint {
  return { sampled_at, upload, download, devices: {} }
}
