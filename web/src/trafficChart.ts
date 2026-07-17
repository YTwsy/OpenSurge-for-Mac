type ChartPoint = { x: number; y: number }

export type SmoothChart = {
  areaPath: string
  linePath: string
}

export function buildSmoothChart(values: number[], maximum: number, top: number, baseline: number): SmoothChart {
  const samples = values.length > 1 ? values : [0, values[0] ?? 0]
  const scale = Math.max(maximum, 1)
  const points = samples.map((value, index) => ({
    x: index / (samples.length - 1) * 100,
    y: baseline - Math.max(0, value) / scale * (baseline - top),
  }))
  const linePath = smoothPath(points, top, baseline)
  const first = points[0]
  const last = points.at(-1) ?? first
  return {
    linePath,
    areaPath: `${linePath} L ${format(last.x)} ${format(baseline)} L ${format(first.x)} ${format(baseline)} Z`,
  }
}

function smoothPath(points: ChartPoint[], top: number, bottom: number) {
  if (!points.length) return ''
  let path = `M ${format(points[0].x)} ${format(points[0].y)}`
  for (let index = 0; index < points.length - 1; index += 1) {
    const previous = points[Math.max(0, index - 1)]
    const current = points[index]
    const next = points[index + 1]
    const following = points[Math.min(points.length - 1, index + 2)]
    const control1 = {
      x: current.x + (next.x - previous.x) / 6,
      y: clamp(current.y + (next.y - previous.y) / 6, top, bottom),
    }
    const control2 = {
      x: next.x - (following.x - current.x) / 6,
      y: clamp(next.y - (following.y - current.y) / 6, top, bottom),
    }
    path += ` C ${format(control1.x)} ${format(control1.y)}, ${format(control2.x)} ${format(control2.y)}, ${format(next.x)} ${format(next.y)}`
  }
  return path
}

function clamp(value: number, minimum: number, maximum: number) {
  return Math.min(Math.max(value, minimum), maximum)
}

function format(value: number) {
  return value.toFixed(2)
}
