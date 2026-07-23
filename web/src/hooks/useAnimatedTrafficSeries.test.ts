import { describe, expect, it } from 'vitest'
import { interpolateTrafficSeries } from './useAnimatedTrafficSeries'

describe('interpolateTrafficSeries', () => {
  it('resamples the previous curve before interpolating a longer series', () => {
    const result = interpolateTrafficSeries(
      [{ upload: 0, download: 100 }, { upload: 100, download: 200 }],
      [{ upload: 100, download: 200 }, { upload: 200, download: 300 }, { upload: 300, download: 400 }],
      0.5,
    )

    expect(result).toEqual([
      { upload: 50, download: 150 },
      { upload: 125, download: 225 },
      { upload: 200, download: 300 },
    ])
  })

  it('clamps progress and supports an empty target', () => {
    expect(interpolateTrafficSeries([{ upload: 10, download: 20 }], [{ upload: 30, download: 40 }], 2))
      .toEqual([{ upload: 30, download: 40 }])
    expect(interpolateTrafficSeries([{ upload: 10, download: 20 }], [], 0.5)).toEqual([])
  })
})
