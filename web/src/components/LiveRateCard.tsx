import { useId } from 'react'
import type { TrafficHistoryPoint } from '../types'
import { formatRate } from '../trafficFormat'
import { buildSmoothChart } from '../trafficChart'
import { useAnimatedTrafficSeries } from '../hooks/useAnimatedTrafficSeries'

type LiveRateCardProps = {
  direction: 'upload' | 'download'
  history: TrafficHistoryPoint[]
  value: number
}

export function LiveRateCard({ direction, history, value }: LiveRateCardProps) {
  const gradientID = useId().replace(/:/g, '')
  const label = direction === 'upload' ? '上传' : '下载'
  const target = history.map(point => ({ upload: point.upload, download: point.download }))
  if (target.length === 0) target.push({ upload: 0, download: 0 })
  target[target.length - 1] = { ...target[target.length - 1], [direction]: value }
  const animated = useAnimatedTrafficSeries(target, `${direction}:${history.at(-1)?.sampled_at ?? 'empty'}:${value}`)
  const values = animated.map(point => point[direction])
  const chartMaximum = Math.max(...values, 0)
  const peak = Math.max(...target.map(point => point[direction]), value, 0)
  const { amount, unit } = rateParts(value)
  const chart = buildSmoothChart(values, chartMaximum, 7, 35)
  return <article className={`live-rate-card ${direction}`} aria-label={`${label}当前速度 ${formatRate(value)}`}>
    <header><span className="rate-direction" aria-hidden="true">{direction === 'upload' ? '↑' : '↓'}</span><span><small>{direction.toUpperCase()}</small><b>{label}</b></span><em><i />LIVE</em></header>
    <div className="rate-reading"><strong>{amount}</strong><span>{unit}</span></div>
    <div className="rate-meta"><span>实时速率</span><span>峰值 {formatRate(peak)}</span></div>
    <div className="rate-mini-chart">
      <svg viewBox="0 0 100 38" preserveAspectRatio="none" role="img" aria-label={`${label}最近 60 秒趋势`}>
        <defs><linearGradient id={`${gradientID}-rate`} x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor="currentColor" stopOpacity=".24" /><stop offset="1" stopColor="currentColor" stopOpacity="0" /></linearGradient></defs>
        <line x1="0" y1="12" x2="100" y2="12" />
        <line x1="0" y1="25" x2="100" y2="25" />
        <path className="rate-area" d={chart.areaPath} fill={`url(#${gradientID}-rate)`} />
        <path className="rate-line" d={chart.linePath} />
      </svg>
    </div>
  </article>
}

function rateParts(value: number) {
  const [amount, unit = 'B/s'] = formatRate(value).split(' ')
  return { amount, unit }
}
