import { useId } from 'react'
import type { TrafficHistoryPoint, TrafficRates } from '../types'
import { formatRate } from '../trafficFormat'
import { buildSmoothChart } from '../trafficChart'
import { useAnimatedTrafficSeries } from '../hooks/useAnimatedTrafficSeries'

type TrafficTrendCardProps = {
  title: string
  subtitle: string
  history: TrafficHistoryPoint[]
  deviceKey?: string
  className?: string
}

export function TrafficTrendCard({ title, subtitle, history, deviceKey, className = '' }: TrafficTrendCardProps) {
  const gradientID = useId().replace(/:/g, '')
  const target = history.map(point => deviceKey ? point.devices[deviceKey] ?? zeroRates : point)
  const samples = useAnimatedTrafficSeries(target, `${deviceKey ?? 'gateway'}:${history.at(-1)?.sampled_at ?? 'empty'}`)
  const upload = samples.map(point => point.upload)
  const download = samples.map(point => point.download)
  const maximum = Math.max(...upload, ...download, 1)
  const uploadChart = buildSmoothChart(upload, maximum, 8, 46)
  const downloadChart = buildSmoothChart(download, maximum, 8, 46)
  const current = samples.at(-1) ?? zeroRates
  const firstTime = history[0]?.sampled_at
  const lastTime = history.at(-1)?.sampled_at

  return <article className={`traffic-trend-card ${className}`.trim()}>
    <header className="trend-card-header"><div><small>LIVE TRAFFIC</small><h2>{title}</h2><p>{subtitle}</p></div><div className="trend-live-dot"><span />实时</div></header>
    <div className="trend-current">
      <span className="upload"><i />↑ {formatRate(current.upload)}</span>
      <span className="download"><i />↓ {formatRate(current.download)}</span>
    </div>
    <div className="trend-chart">
      <svg viewBox="0 0 100 52" preserveAspectRatio="none" role="img" aria-label={`${title}最近 60 秒上传下载趋势`}>
        <defs>
          <linearGradient id={`${gradientID}-up`} x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor="#8b7cf6" stopOpacity=".24" /><stop offset="1" stopColor="#8b7cf6" stopOpacity="0" /></linearGradient>
          <linearGradient id={`${gradientID}-down`} x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor="#58bce8" stopOpacity=".22" /><stop offset="1" stopColor="#58bce8" stopOpacity="0" /></linearGradient>
        </defs>
        <line x1="0" y1="27" x2="100" y2="27" className="chart-grid-line" />
        <path d={uploadChart.areaPath} fill={`url(#${gradientID}-up)`} />
        <path d={downloadChart.areaPath} fill={`url(#${gradientID}-down)`} />
        <path d={uploadChart.linePath} className="trend-line upload" />
        <path d={downloadChart.linePath} className="trend-line download" />
      </svg>
      <div className="trend-axis"><span>{firstTime ? formatSampleTime(firstTime) : '等待采样'}</span><span>近 60 秒</span><span>{lastTime ? formatSampleTime(lastTime) : '现在'}</span></div>
    </div>
  </article>
}

const zeroRates: TrafficRates = { upload: 0, download: 0 }

function formatSampleTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
