import { useEffect, useState, useSyncExternalStore } from 'react'
import { waitForOperation, watchOperations } from '../api'
import { t } from '../i18n'
import { dismissOperation, getOperations, markOperationConnection, subscribeOperations } from '../operations'

const phaseLabels: Record<string, string> = {
  submitting: '正在提交操作',
  waiting_helper: '等待网关服务响应',
  checking_runtime: '检查当前运行状态',
  validating_network: '检查网络接口与启动条件',
  checking_reservations: '检查设备固定地址冲突',
  preparing_config: '生成候选运行配置',
  validating_config: '校验 Mihomo 配置',
  validating_device_policy: '校验设备身份与路由规则',
  saving_config: '保存已校验的配置',
  saving_runtime: '保存网络恢复快照',
  enabling_forwarding: '启用网关转发',
  starting_mihomo: '启动 Mihomo 并等待就绪',
  starting_ipv6: '启动下游 IPv6 数据面',
  starting_dns: '启动 DHCP / DNS 服务',
  applying_firewall: '应用网关防火墙规则',
  enabling_system_proxy: '启用本机系统代理协同',
  initiating_tailscale: '发起 Tailscale 预热',
  restoring_system_proxy: '恢复原有系统代理设置',
  stopping_dns: '停止 DHCP / DNS 服务',
  stopping_ipv6: '撤销下游 IPv6 接管',
  stopping_mihomo: '停止 Mihomo 进程',
  restoring_network: '恢复防火墙与转发设置',
  clearing_runtime: '清理本次运行状态',
  rolling_back: '操作未完成，正在回滚网络改动',
  restoring_config: '恢复之前的配置与网关',
}

const kindLabels: Record<string, string> = {
  start: '启动网关', stop: '停止网关', reload: '重载网关', 'restart-mihomo': '重启 Mihomo',
  'save-device-policy': '保存设备配置', 'apply-profile': '应用代理与规则源', 'apply-tailscale': '应用 Tailscale 配置',
}

const noticeLabels: Record<string, string> = {
  tailscale_warmup_started: 'Tailscale 预热已发起，连接可能尚未就绪；可直接选择出口，首次访问可能需重试。',
  tailscale_warmup_unavailable: '未能发起 Tailscale 预热，但不影响网关操作完成；选择出口后仍由内核按需连接。',
}

export function OperationProgress({ onOpenDiagnostics }: { onOpenDiagnostics: () => void }) {
  const operations = useSyncExternalStore(subscribeOperations, getOperations)
  const [now, setNow] = useState(Date.now)
  useEffect(() => watchOperations(), [])

  const visible = operations.filter(operation => !operation.dismissed && (operation.state !== 'succeeded' || now - Date.parse(operation.updated_at || '') < 6000))
  const active = visible.filter(operation => operation.state === 'running')
  const operation = [...(active.length ? active : visible)].sort((a, b) => Date.parse(b.created_at || '') - Date.parse(a.created_at || ''))[0]
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
  const phase = t(phaseLabels[operation.phase || ''] || '正在执行操作')
  const recheck = () => {
    markOperationConnection(operation.id, 'reconnecting')
    void waitForOperation(operation.id).catch(() => { /* keep the known outcome on the card */ })
  }

  return <aside className={`operation-progress ${status}`} aria-label={t('当前操作进度')}>
    <div className="operation-progress-heading">
      <span className="operation-progress-symbol" aria-hidden="true">{running && !uncertain ? <span className="button-spinner" /> : status === 'succeeded' ? '✓' : '!'}</span>
      <div className="operation-progress-status" role="status" aria-live="polite" aria-atomic="true">
        <strong>{t(kindLabels[operation.kind] || '网关操作')}<span>{t(uncertain ? '结果尚未确认' : running ? '进行中' : operation.state === 'succeeded' ? '已完成' : '未完成')}</span></strong>
        <p>{running ? phase : operation.state === 'succeeded' ? t('后台操作已完成') : t('最后执行阶段：{{phase}}', { phase })}</p>
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
