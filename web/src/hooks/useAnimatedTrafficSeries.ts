import { useEffect, useRef, useState } from 'react'
import type { TrafficRates } from '../types'

const transitionDurationMs = 700

export function useAnimatedTrafficSeries(target: TrafficRates[], transitionKey: string) {
  const [series, setSeries] = useState(target)
  const current = useRef(target)

  useEffect(() => {
    let frame = 0
    const publish = (next: TrafficRates[]) => {
      current.current = next
      setSeries(next)
    }
    const reduceMotion = typeof window.matchMedia === 'function'
      && window.matchMedia('(prefers-reduced-motion: reduce)').matches

    if (reduceMotion || target.length === 0 || current.current.length === 0) {
      publish(target)
      return
    }

    const previous = fitTrafficSeries(current.current, target.length)
    if (sameTrafficSeries(previous, target)) {
      publish(target)
      return
    }

    const startedAt = window.performance.now()
    const animate = (now: number) => {
      const elapsed = Math.min(Math.max((now - startedAt) / transitionDurationMs, 0), 1)
      const eased = 1 - Math.pow(1 - elapsed, 3)
      publish(interpolateTrafficSeries(previous, target, eased))
      if (elapsed < 1) frame = window.requestAnimationFrame(animate)
    }
    frame = window.requestAnimationFrame(animate)
    return () => window.cancelAnimationFrame(frame)
  }, [transitionKey])

  return series
}

export function interpolateTrafficSeries(previous: TrafficRates[], target: TrafficRates[], progress: number) {
  const from = fitTrafficSeries(previous, target.length)
  const amount = Math.min(Math.max(progress, 0), 1)
  return target.map((point, index) => ({
    upload: from[index].upload + (point.upload - from[index].upload) * amount,
    download: from[index].download + (point.download - from[index].download) * amount,
  }))
}

function fitTrafficSeries(series: TrafficRates[], length: number) {
  if (length === 0) return []
  if (series.length === 0) return Array.from({ length }, () => ({ upload: 0, download: 0 }))
  if (series.length === 1 || length === 1) return Array.from({ length }, () => ({ ...series.at(-1)! }))
  if (series.length === length) return series.map(point => ({ ...point }))

  return Array.from({ length }, (_, index) => {
    const position = index / (length - 1) * (series.length - 1)
    const left = Math.floor(position)
    const right = Math.min(Math.ceil(position), series.length - 1)
    const amount = position - left
    return {
      upload: series[left].upload + (series[right].upload - series[left].upload) * amount,
      download: series[left].download + (series[right].download - series[left].download) * amount,
    }
  })
}

function sameTrafficSeries(left: TrafficRates[], right: TrafficRates[]) {
  return left.length === right.length
    && left.every((point, index) => point.upload === right[index].upload && point.download === right[index].download)
}
