import { useEffect, useState } from 'react'
import { api } from '../api'
import { PageHeader, SectionTitle, StatusDot } from '../components/Common'
import type { Diagnostics, DoctorRunStatus, Overview } from '../types'

const doctorPollIntervalMs = 500

export function DiagnosticsPage({ overview }: { overview: Overview | null }) {
  const [details, setDetails] = useState<Diagnostics | null>(null)
  const [doctorStatus, setDoctorStatus] = useState<DoctorRunStatus | null>(null)
  const [doctorError, setDoctorError] = useState('')

  useEffect(() => {
    let active = true
    void api.diagnostics().then(value => { if (active) setDetails(value) }).catch(() => { if (active) setDetails(null) })
    return () => { active = false }
  }, [overview?.revision, overview?.status.gateway])

  useEffect(() => {
    let active = true
    setDoctorError('')
    void api.doctorStatus()
      .then(value => { if (active) setDoctorStatus(value) })
      .catch(cause => { if (active) setDoctorError(cause instanceof Error ? cause.message : String(cause)) })
    return () => { active = false }
  }, [overview?.revision])

  useEffect(() => {
    if (doctorStatus?.state !== 'running') return
    let active = true
    let timer = 0
    const poll = () => {
      timer = window.setTimeout(() => {
        void api.doctorStatus()
          .then(value => {
            if (!active) return
            setDoctorStatus(value)
            setDoctorError('')
            if (value.state === 'running') poll()
          })
          .catch(cause => {
            if (!active) return
            setDoctorError(cause instanceof Error ? cause.message : String(cause))
            poll()
          })
      }, doctorPollIntervalMs)
    }
    poll()
    return () => { active = false; window.clearTimeout(timer) }
  }, [doctorStatus?.state, doctorStatus?.started_at])

  const runDoctor = async () => {
    setDoctorError('')
    try {
      setDoctorStatus(await api.runDoctor())
    } catch (cause) {
      setDoctorError(cause instanceof Error ? cause.message : String(cause))
    }
  }

  const running = doctorStatus?.state === 'running'
  const checks = doctorStatus?.checks ?? []
  return <>
    <PageHeader eyebrow="DIAGNOSTICS" title="诊断、连接与 Provider" description="错误保持结构化；日志经过已知凭据脱敏，菜单栏只复制压缩摘要。" />
    <section className="split">
      <div>
        <div className="diagnostic-doctor-head">
          <SectionTitle title="Doctor" subtitle={doctorSubtitle(doctorStatus)} />
          <button className="primary" type="button" disabled={running} onClick={() => void runDoctor()}>{running ? <><span className="button-spinner" aria-hidden="true" />后台检查中…</> : doctorStatus?.state === 'idle' ? '运行 Doctor' : '重新运行 Doctor'}</button>
        </div>
        {doctorError && <div className="notice warn" role="alert">Doctor 状态暂不可用：{doctorError}</div>}
        {doctorStatus?.state === 'failed' && <div className="notice warn" role="alert">Doctor 后台任务失败：{doctorStatus.error || '未知错误'}</div>}
        {checks.map(check => <div className="check" key={check.name}><span className={check.ok ? 'ok-mark' : 'bad-mark'}>{check.ok ? '✓' : '!'}</span><div><strong>{check.name}</strong><small>{check.message}</small></div></div>)}
        {!checks.length && !doctorError && doctorStatus?.state !== 'failed' && <div className="empty">{running ? '检查在 Control Service 后台执行；离开本页不会启动第二份任务。' : '点击“运行 Doctor”后才会执行完整检查；总览与菜单栏刷新不会触发。'}</div>}
      </div>
      <div><SectionTitle title="Proxy Providers" subtitle="可从这里观察和刷新，不在菜单栏中执行" />{overview?.providers.proxy_providers.map(provider => <div className="row" key={provider.name}><StatusDot status={provider.proxies.some(proxy => proxy.alive) ? 'running' : 'degraded'} /><div className="grow"><strong>{provider.name}</strong><small>{provider.proxy_count} proxies · {provider.vehicle_type}</small></div><button onClick={() => void api.refreshProvider(provider.name)}>刷新</button></div>)}</div>
    </section>
    <section className="section"><SectionTitle title="Live Connections" subtitle={details?.connection_error || `${details?.connections.connections.length ?? 0} active connections`} /><div className="inventory"><span>↑ {details?.connections.upload_total ?? 0} bytes</span><span>↓ {details?.connections.download_total ?? 0} bytes</span>{details?.connections.connections.slice(0, 12).map(connection => <span key={connection.id}>{connection.rule || 'MATCH'} · {(connection.chains ?? []).join(' → ') || connection.id.slice(0, 8)}</span>)}</div></section>
    <section className="section"><SectionTitle title="Recent logs" subtitle="每个进程最多 80 行；API 会遮蔽 mihomo secret 与 upstream credentials" />{Object.entries(details?.logs ?? {}).map(([name, lines]) => <div key={name}><h3>{name}</h3><pre>{lines.join('\n') || 'No log output'}</pre></div>)}</section>
    <section className="section"><SectionTitle title="Operations 与恢复记录" subtitle={`Recovery: ${details?.recovery.stage ?? overview?.recovery.stage ?? 'idle'}`} />{details?.operations.length ? details.operations.map(operation => <div className="row" key={operation.id}><StatusDot status={operation.state === 'failed' ? 'degraded' : operation.state === 'succeeded' ? 'running' : 'stopped'} /><div className="grow"><strong>{operation.kind} · {operation.state}</strong><small>{operation.id} · {operation.updated_at}{operation.error ? ` · ${operation.error}` : ''}</small></div></div>) : <div className="empty">尚无生命周期操作记录</div>}</section>
  </>
}

function doctorSubtitle(status: DoctorRunStatus | null): string {
  if (!status) return '正在读取最近一次结果'
  if (status.state === 'idle') return '完整检查仅在你显式运行时执行，最长可能需要 90 秒'
  if (status.state === 'running') return '正在后台执行完整检查，不阻塞总览与菜单栏刷新'
  if (!status.current) return '配置已变化；以下是旧版本结果，请重新运行'
  if (status.state === 'failed') return '后台任务没有完成'
  return status.healthy ? '当前配置的基础检查通过' : '当前配置存在需要处理的问题'
}
