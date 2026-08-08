import { useCallback, useEffect, useMemo, useState } from 'react'
import { api, waitForOperation } from '../api'
import { ConnectivityCategory } from '../components/ConnectivityCategory'
import { Empty, PageHeader } from '../components/Common'
import { connectivityCategories, median } from '../connectivity'
import type { ConnectivityResponse, ConnectivityResult, Overview } from '../types'

const baselineKey = 'opensurge-connectivity-baseline'

export function ConnectivityPage({ overview, onChanged }: { overview: Overview | null; onChanged: () => Promise<void> }) {
  const [catalog, setCatalog] = useState<ConnectivityResponse | null>(null)
  const [results, setResults] = useState<Map<string, ConnectivityResult>>(new Map())
  const [testing, setTesting] = useState<Set<string>>(new Set())
  const [recovering, setRecovering] = useState(false)
  const [error, setError] = useState('')
  const [enforceBaseline, setEnforceBaseline] = useState(() => window.localStorage.getItem(baselineKey) !== 'observe')
  // The status API appends the live engine version when available, for example
  // "running (v1.19.27)". Keep that display detail from disabling probes.
  const mihomoRunning = overview?.status.mihomo.startsWith('running') ?? false
  const running = overview?.status.gateway === 'running' && mihomoRunning
  const runtimeInterrupted = overview?.status.runtime_state === 'interrupted'
  const runtimeActive = !runtimeInterrupted && (overview?.status.gateway === 'running' || overview?.status.gateway === 'degraded')
  const mihomoControllerRefused = overview?.warnings?.some(warning =>
    (warning.startsWith('mihomo policies unavailable:') || warning.startsWith('mihomo providers unavailable:') || warning.startsWith('mihomo TUN:')) &&
    warning.includes('connection refused')) ?? false
  const mihomoRecoveryNeeded = runtimeActive && (!mihomoRunning || mihomoControllerRefused)
  const canProbe = running && !mihomoRecoveryNeeded

  const loadCatalog = useCallback(async () => {
    try {
      setCatalog(await api.connectivity())
      setError('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    }
  }, [])

  useEffect(() => { void loadCatalog() }, [loadCatalog])

  const run = async (targetIDs: string[]) => {
    if (!canProbe || !targetIDs.length) return
    setTesting(current => new Set([...current, ...targetIDs])); setError('')
    try {
      const response = await api.testConnectivity(targetIDs)
      setResults(current => {
        const next = new Map(current)
        response.results.forEach(result => next.set(result.target_id, result))
        return next
      })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setTesting(current => {
        const next = new Set(current)
        targetIDs.forEach(id => next.delete(id))
        return next
      })
    }
  }

  const targets = catalog?.targets ?? []
  const tested = [...results.values()]
  const reachable = tested.filter(result => result.status === 'reachable' || result.status === 'degraded').length
  const mismatches = enforceBaseline ? tested.filter(result => result.route_match === false).length : 0
  const overallMedian = median(tested.map(result => result.median_ms ?? 0).filter(Boolean))
  const categories = useMemo(() => Object.keys(connectivityCategories) as Array<keyof typeof connectivityCategories>, [])

  const setBaseline = (enabled: boolean) => {
    setEnforceBaseline(enabled)
    window.localStorage.setItem(baselineKey, enabled ? 'split' : 'observe')
  }

  const recoverMihomo = async () => {
    if (!mihomoRecoveryNeeded || recovering) return
    if (!window.confirm('这会验证当前 applied 配置并恢复 Mihomo。DHCP/DNS、PF、IPv4 forwarding 和 Mac 网络设置不会改变；现有代理连接会重新建立。继续吗？')) return
    setRecovering(true); setError('')
    try {
      const operation = await api.gateway('restart-mihomo')
      await waitForOperation(operation.id)
      await onChanged()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setRecovering(false)
    }
  }

  return <>
    <PageHeader eyebrow="CONNECTIVITY" title="分流与网络连通性" description="通过当前 applied 配置与 Mac 本机运行模式访问真实服务，展示三轮中位延迟、命中规则和实际出口链。" action={<button className="primary" type="button" disabled={!canProbe || !targets.length || testing.size > 0} onClick={() => void run(targets.map(target => target.id))}>{testing.size ? `正在检测 ${testing.size} 项…` : '检测全部'}</button>} />
    <section className="probe-scope" aria-label="检测来源"><button className="active" type="button" aria-pressed="true"><span>◉</span><strong>Mac 本机运行路径</strong><small>opensurge-control → mihomo</small></button><a href="https://ip.net.coffee/link/" target="_blank" rel="noreferrer"><span>↗</span><strong>本机浏览器线路</strong><small>在 Net.Coffee 中打开</small></a><button type="button" disabled title="需要真实下游设备发起探测"><span>◇</span><strong>设备端检测</strong><small>后续：真实 DHCP / DNS / TUN</small></button></section>
    {overview?.drift && <div className="notice warn" role="status">当前存在未应用修改。本页只检测正在运行的 applied 配置；保存但未重载的规则不会反映在结果中。</div>}
    {!canProbe && !mihomoRecoveryNeeded && !runtimeInterrupted && <div className="notice warn" role="status">启动网关和 mihomo 后才能执行策略路径检测。浏览器线路测试仍可通过上方 Net.Coffee 打开。</div>}
    {runtimeInterrupted && <div className="notice warn" role="status">上一次网关运行已被系统重启中断。请先到“网络设置”安全清理旧 runtime，再重新启动完整网关；此时不能只重启 Mihomo。</div>}
    {mihomoRecoveryNeeded && <div className="notice actionable" role="status"><div><strong>Mihomo 本地服务需要恢复</strong><p>Mac 睡眠唤醒或 Wi-Fi 重连后，若进程已停止或本地 controller 拒绝连接，可重建 TUN 与出站 socket；不会停止 DHCP/DNS、卸载 PF 或修改 Mac 网络设置，旧 Mihomo 日志会先归档。</p></div><button className="primary" type="button" disabled={recovering || testing.size > 0} onClick={() => void recoverMihomo()}>{recovering ? '正在恢复…' : '恢复 Mihomo'}</button></div>}
    {error && <div className="error-banner" role="alert"><span>!</span><p>{error}</p><button type="button" onClick={() => void loadCatalog()}>重试</button></div>}
    <section className="connectivity-overview"><div className="connectivity-score"><span className={`score-orb ${tested.length ? mismatches || reachable < tested.length ? 'mixed' : 'healthy' : ''}`}><strong>{tested.length ? `${reachable}/${tested.length}` : '—'}</strong><small>可达</small></span><div><small>APPLIED ROUTING</small><h2>{tested.length ? mismatches ? `${mismatches} 项路径需要关注` : '当前分流符合所选基线' : '等待首次检测'}</h2><p>{tested.length ? `三轮探测 · 整体中位 ${overallMedian || '—'} ms` : '不会在打开页面时自动访问第三方服务'}</p></div></div><div className="connectivity-metrics"><span><small>已检测</small><strong>{tested.length}</strong></span><span><small>路径不符</small><strong className={mismatches ? 'attention' : ''}>{mismatches}</strong></span><span><small>中位延迟</small><strong>{overallMedian ? `${overallMedian} ms` : '—'}</strong></span></div><div className="baseline-control"><span><strong>分流判断基线</strong><small>只影响界面判断，不修改 mihomo 配置</small></span><div className="segmented"><button type="button" aria-pressed={enforceBaseline} onClick={() => setBaseline(true)}>国内直连 / 海外代理</button><button type="button" aria-pressed={!enforceBaseline} onClick={() => setBaseline(false)}>仅观察</button></div></div></section>
    {targets.length ? categories.map(category => {
      const items = targets.filter(target => target.category === category)
      return items.length ? <ConnectivityCategory key={category} category={category} targets={items} results={results} testing={testing} enforceBaseline={enforceBaseline} onTest={run} /> : null
    }) : !error && <Empty text="正在加载检测目录…" />}
    <p className="evidence-note"><strong>证据范围：</strong>这里的请求由 Mac 上的 Control Service 经 mihomo mixed-port 发起，会经过当前 Mac 本机规则 / 全局 / 直连模式。它不证明下游设备的网关规则、设备级 SRC-IP、DHCP、DNS 或 TUN；选择全局或直连时，分流判断基线出现差异可能正是当前模式的结果。HTTP 响应表示网络可达，不等同于已登录后的完整产品功能可用。</p>
  </>
}
