import { useId } from 'react'
import type { TrafficHistoryPoint } from '../types'
import { formatRate } from '../trafficFormat'
import { buildSmoothChart, trafficChartMaximum } from '../trafficChart'
import { t } from '../i18n'

type LiveRateCardProps = {
  direction: 'upload' | 'download'
  history: TrafficHistoryPoint[]
  value: number
}

export function LiveRateCard({ direction, history, value }: LiveRateCardProps) {
  const gradientID = useId().replace(/:/g, '')
  const label = t(direction === 'upload' ? '上传' : '下载')
  const values = history.map(point => point[direction])
  if (values.length === 0) values.push(value)
  else values[values.length - 1] = value
  const chartMaximum = trafficChartMaximum(values)
  const peak = Math.max(...values, value, 0)
  const { amount, unit } = rateParts(value)
  const chart = buildSmoothChart(values, chartMaximum, 7, 35)
  return <article className={`live-rate-card ${direction}`} aria-label={t('{{direction}}当前速度 {{rate}}', { direction: label, rate: formatRate(value) })}>
    <header><span className="rate-direction" aria-hidden="true">{direction === 'upload' ? '↑' : '↓'}</span><span><small>{direction.toUpperCase()}</small><b>{label}</b></span><em><i />LIVE</em></header>
    <div className="rate-reading"><strong>{amount}</strong><span>{unit}</span></div>
    <div className="rate-meta"><span>{t('实时速率')}</span><span>{t('峰值 {{rate}}', { rate: formatRate(peak) })}</span></div>
    <div className="rate-mini-chart">
      <svg viewBox="0 0 100 38" preserveAspectRatio="none" role="img" aria-label={t('{{direction}}最近 60 秒趋势', { direction: label })}>
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
