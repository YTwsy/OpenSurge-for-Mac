import { connectivityCategories, median } from '../connectivity'
import type { ConnectivityResult, ConnectivityTarget } from '../types'
import { ConnectivityTargetCard } from './ConnectivityTargetCard'
import { t } from '../i18n'

type ConnectivityCategoryProps = {
  category: keyof typeof connectivityCategories
  targets: ConnectivityTarget[]
  results: Map<string, ConnectivityResult>
  testing: Set<string>
  enforceBaseline: boolean
  onTest: (targetIDs: string[]) => Promise<void>
}

export function ConnectivityCategory({ category, targets, results, testing, enforceBaseline, onTest }: ConnectivityCategoryProps) {
  const meta = connectivityCategories[category]
  const categoryResults = targets.map(target => results.get(target.id)).filter((result): result is ConnectivityResult => Boolean(result))
  const reachable = categoryResults.filter(result => result.status === 'reachable' || result.status === 'degraded').length
  const average = median(categoryResults.map(result => result.median_ms ?? 0).filter(Boolean))
  const busy = targets.some(target => testing.has(target.id))

  return <section className="connectivity-category">
    <header className="connectivity-category-head"><span className="category-mark" aria-hidden="true">{meta.flag}</span><div><h2>{t(meta.title)}</h2><p>{t(meta.description)}</p></div><div className="category-summary"><span>{categoryResults.length ? t('可达 {{reachable}}/{{total}}{{median}}', { reachable, total: categoryResults.length, median: average ? t(' · 中位 {{value}} ms', { value: average }) : '' }) : t('准备检测')}</span><button type="button" disabled={busy} onClick={() => void onTest(targets.map(target => target.id))}>{t(busy ? '检测中…' : '刷新')}</button></div></header>
    <div className="connectivity-grid">{targets.map(target => <ConnectivityTargetCard key={target.id} target={target} result={results.get(target.id)} testing={testing.has(target.id)} enforceBaseline={enforceBaseline} />)}</div>
  </section>
}
