import { describe, expect, it } from 'vitest'
import { buildSmoothChart } from './trafficChart'

describe('buildSmoothChart', () => {
  it('builds a bounded cubic curve and a closed area', () => {
    const chart = buildSmoothChart([0, 50, 10, 100], 100, 8, 46)
    expect(chart.linePath).toContain(' C ')
    expect(chart.linePath).not.toContain('NaN')
    expect(chart.areaPath).toMatch(/L 100\.00 46\.00 L 0\.00 46\.00 Z$/)
  })

  it('creates a valid baseline when only one sample exists', () => {
    const chart = buildSmoothChart([0], 0, 7, 35)
    expect(chart.linePath).toContain('M 0.00 35.00 C ')
    expect(chart.areaPath).not.toContain('NaN')
  })
})
