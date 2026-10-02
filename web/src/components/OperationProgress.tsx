import { useEffect, useState, useSyncExternalStore } from 'react'
import { waitForOperation, watchOperations } from '../api'
import { t } from '../i18n'
import { operationPhaseLabel } from '../status'
import { dismissOperation, getOperations, markOperationConnection, subscribeOperations } from '../operations'


const kindLabels: Record<string, string> = {
  start: '启动网关', stop: '停止网关', reload: '重载网关', 'restart-mihomo': '重启 Mihomo',
  'save-device-policy': '保存设备配置', 'apply-profile': '应用代理与规则源', 'apply-tailscale': '应用 Tailscale 配置',
  'dhcp-probe': '检查路由器 DHCP 是否已关闭', 'router-dhcp-restored': '检查路由器 DHCP 是否已恢复',
}

const successMessages: Record<string, string> = {
  'dhcp-probe': '本次探测未收到 DHCP OFFER，可以继续启动 OpenSurge。',
  'router-dhcp-restored': '已收到 DHCP OFFER，可以继续恢复 Mac 自动 DHCP。',
}

const noticeLabels: Record<string, string> = {
  tailscale_warmup_started: 'Tailscale 预热已发起，连接可能尚未就绪；可直接选择出口，首次访问可能需重试。',
  tailscale_warmup_unavailable: '未能发起 Tailscale 预热，但不影响网关操作完成；选择出口后仍由内核按需连接。',
}

export function OperationProgress({ onOpenDiagnostics }: { onOpenDiagnostics: () => void }) {
  const operations = useSyncExternalStore(subscribeOperations, getOperations)
  const [now, setNow] = useState(Date.now)
  useEffect(() => watchOperations(), [])

  const newestFirst = [...operations].reverse().sort((a, b) => Date.parse(b.created_at || '') - Date.parse(a.created_at || ''))
  const active = newestFirst.filter(operation => !operation.dismissed && operation.state === 'running')
  // Select the latest result before checking dismissal/expiry. Filtering first
  // would reveal older failures when that result disappears. Unfinished work
  // still takes priority, including operations whose outcome is unconfirmed.
  const candidate = active[0] || newestFirst[0]
  const operation = candidate && !candidate.dismissed && (candidate.state !== 'succeeded' || now - Date.parse(candidate.updated_at || '') < 6000) ? candidate : undefined
  const ticking = operation?.state === 'running' || operation?.state === 'succeeded'
  useEffect(() => {
    if (!ticking) return
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [ticking])
  if (!operation) return null

  const running = operation.state === 'running'
  const stale = running && now - Date.parse(operation.updated_at || operation.created_at || '') >= 180_000
  const uncertain = running && (operation.connection !== 'connected' || stale)
  const status = uncertain ? 'uncertain' : running ? 'running' : operation.state
  const elapsed = Math.max(0, Math.floor(((running ? now : Date.parse(operation.updated_at || '')) - Date.parse(operation.created_at || '')) / 1000)) || 0
  const phase = operationPhaseLabel(operation.phase)
  const recheck = () => {
    markOperationConnection(operation.id, 'reconnecting')
    void waitForOperation(operation.id).catch(() => { /* keep the known outcome on the card */ })
  }

  return <aside className={`operation-progress ${status}`} aria-label={t('当前操作进度')}>
    <div className="operation-progress-heading">
      <span className="operation-progress-symbol" aria-hidden="true">{running && !uncertain ? <span className="button-spinner" /> : status === 'succeeded' ? '✓' : '!'}</span>
      <div className="operation-progress-status" role="status" aria-live="polite" aria-atomic="true">
        <strong>{t(kindLabels[operation.kind] || '网关操作')}<span>{t(uncertain ? '结果尚未确认' : running ? '进行中' : operation.state === 'succeeded' ? '已完成' : '未完成')}</span></strong>
        <p>{running ? phase : operation.state === 'succeeded' ? t(successMessages[operation.kind] || '后台操作已完成') : t('最后执行阶段：{{phase}}', { phase })}</p>
      </div>
      {!running && <button type="button" className="operation-progress-dismiss" onClick={() => dismissOperation(operation.id)} aria-label={t('关闭操作进度')}>×</button>}
    </div>
    {running && <div className="operation-progress-track" role="progressbar" aria-label={phase}><span /></div>}
    <div className="operation-progress-meta"><span>{t('已用时 {{seconds}} 秒', { seconds: elapsed })}</span>{active.length > 1 && <span>{t('另有 {{count}} 个操作进行中', { count: active.length - 1 })}</span>}</div>
    {uncertain ? <p className="operation-progress-hint">{t('暂时无法确认执行结果。这里只查询原操作，不会重新启动或重载网关，请勿重复提交。')}</p>
      : running && <p className="operation-progress-hint">{t(elapsed >= 10 ? '仍在执行当前阶段，无需重复点击。可切换页面，进度会继续保留。' : '正在处理，请稍候。进度来自后台实际执行阶段。')}</p>}
    {operation.error && <p className="operation-progress-error">{t(operation.error)}</p>}
    {operation.notices?.map(notice => noticeLabels[notice] && <p className="operation-progress-hint" key={notice}>{t(noticeLabels[notice])}</p>)}
    {(uncertain || operation.state === 'failed') && <div className="operation-progress-actions">
      {uncertain && <button type="button" onClick={recheck}>{t('重新查询状态')}</button>}
      <button type="button" onClick={onOpenDiagnostics}>{t('查看诊断')}</button>
      {uncertain && <button type="button" onClick={() => dismissOperation(operation.id)}>{t('收起提示')}</button>}
    </div>}
  </aside>
}
