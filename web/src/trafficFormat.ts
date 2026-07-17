export function formatBytes(value: number) {
  return formatTrafficValue(value, 1024, ['B', 'KB', 'MB', 'GB', 'TB'])
}

export function formatRate(value: number) {
  return `${formatTrafficValue(value, 1000, ['B', 'kB', 'MB', 'GB', 'TB'])}/s`
}

function formatTrafficValue(value: number, base: number, units: string[]) {
  if (!Number.isFinite(value) || value <= 0) return `0 ${units[0]}`
  const exponent = Math.min(Math.floor(Math.log(value) / Math.log(base)), units.length - 1)
  const amount = value / base ** exponent
  const precision = amount >= 100 || exponent === 0 ? 0 : amount >= 10 ? 1 : 2
  return `${amount.toFixed(precision).replace(/\.0+$|(?<=\.[0-9])0$/, '')} ${units[exponent]}`
}
