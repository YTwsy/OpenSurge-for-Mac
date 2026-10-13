import { describe, expect, it } from 'vitest'
import { buildSmoothChart, trafficChartMaximum } from './trafficChart'

describe('trafficChartMaximum', () => {
  it('holds small fluctuations in readable ranges without clipping new peaks', () => {
    expect(trafficChartMaximum([8_001, 9_999])).toBe(10_000)
    expect(trafficChartMaximum([10_001])).toBe(20_000)
    expect(trafficChartMaximum([20_001])).toBe(50_000)
    expect(trafficChartMaximum([850_000, 1_000_000])).toBe(1_000_000)
    expect(trafficChartMaximum([0, 0])).toBe(1)
    expect(trafficChartMaximum([])).toBe(1)
  })
})

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
