import { api } from '../api'
import { Empty, PageHeader } from '../components/Common'
import type { Overview } from '../types'

export function PoliciesPage({ overview, onChanged }: { overview: Overview | null; onChanged: () => Promise<void> }) {
  const groups = overview?.policies ?? []
  return <>
    <PageHeader eyebrow="POLICIES" title="策略组" description="已有 selector 的成员切换即时生效；规则结构或设备身份修改需要重启。" />
    <section className="policy-list">{groups.map(group => <article key={group.name}><div><small>{group.type}</small><h3>{group.name}</h3></div><select aria-label={`${group.name} selected policy`} value={group.selected} onChange={event => void api.selectPolicy(group.name, event.target.value).then(onChanged)}>{group.options.map(option => <option key={option}>{option}</option>)}</select></article>)}</section>
    {!groups.length && <Empty text="mihomo 未运行或没有可选择的策略组" />}
  </>
}
