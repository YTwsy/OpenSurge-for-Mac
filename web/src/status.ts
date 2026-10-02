import { t } from './i18n'
import type { GatewayPresentation } from './types'

export function gatewayDisplayState(presentation?: GatewayPresentation, gateway?: string, runtimeState?: string) {
  if (presentation) return presentation.state
  if (runtimeState === 'interrupted') return 'interrupted'
  return gateway === 'running' || gateway === 'stopped' || gateway === 'degraded' ? gateway : 'unknown'
}

export function gatewayStatusLabel(state: string) {
  const label = ({
    running: '正在运行', stopped: '已停止', starting: '正在启动', reloading: '正在应用配置',
    stopping: '正在停止', recovering: '正在恢复代理引擎', rolling_back: '正在回滚网络改动',
    changing: '正在更新网关', degraded: '运行异常', recovery: '待恢复网络',
    interrupted: '重启后待清理', unknown: '状态暂不可用', connecting: '正在连接…', unreachable: '无法连接后台服务',
  } as Record<string, string>)[state]
  return t(label || '状态暂不可用')
}

export function gatewayIsTransitioning(state: string) {
  return ['starting', 'reloading', 'stopping', 'recovering', 'rolling_back', 'changing'].includes(state)
}

export function gatewayStatusDetail(presentation?: GatewayPresentation, drift = false, gateway?: string) {
  if (presentation?.phase && gatewayIsTransitioning(presentation.state)) return operationPhaseLabel(presentation.phase)
  if (presentation?.state === 'recovery') return recoveryLabel(presentation.reason ?? '')
  const reason = ({
    dns_stopped: 'DHCP / DNS 服务已停止', mihomo_stopped: '代理引擎已停止', tun_failed: 'TUN 未就绪',
    ipv6_failed: '下游 IPv6 数据面异常', pf_unloaded: '网关防火墙规则未加载', forwarding_disabled: 'IPv4 转发未启用',
    takeover_failed: '网关接管未就绪，请查看网络设置', runtime_interrupted: '系统重启中断了网关，请先清理遗留状态',
    status_unavailable: '暂时无法确认运行状态，正在重新查询', status_refreshing: '操作状态已变化，正在重新确认',
    operation_unconfirmed: '操作结果尚未确认，请查看全局进度或诊断', external_operation: '另一网关操作正在执行，完成后会自动更新',
  } as Record<string, string>)[presentation?.reason ?? '']
  if (reason) return t(reason)
  if (presentation?.config_pending ?? drift) return t(gateway === 'stopped' ? '配置将在下次启动时应用' : '有配置待应用，当前仍使用原配置')
  if (presentation?.diagnosis_warning) return t('上次诊断有待检查项')
  return ''
}

export function statusLabel(status?: string, runtimeState?: string) {
  if (runtimeState === 'interrupted') return t('重启后待清理')
  return status === 'running' ? t('正在运行')
    : status === 'degraded' ? t('运行异常')
      : status === 'stopped' ? t('已停止')
        : t('无法连接')
}

export function takeoverLabel(status?: string) {
  const label = ({
    ready: '正在接管',
    waiting: '等待上游 IPv6',
    stopped: '已停止',
    disabled: '已关闭',
    failed: '运行异常',
    interrupted: '重启后待清理',
  } as Record<string, string>)[status ?? '']
  return label ? t(label) : t('未知')
}

export function recoveryLabel(stage: string) {
  const label = ({
    prepared: '恢复资料已准备',
    mac_static: 'Mac 已使用固定 IPv4',
    router_dhcp_disabled_confirmed: '路由器 DHCP 已关闭',
    gateway_active: 'OpenSurge 已接管',
    client_validated: '客户端 DHCP、DNS 与 TUN 已验收',
    client_validation_skipped: '客户端验收已跳过',
    gateway_stopped_waiting_router_dhcp: '已停止，等待恢复路由器 DHCP',
    router_dhcp_restored: '路由器 DHCP 已恢复',
    complete: 'Mac 与客户端已恢复自动获取',
    complete_static: '流程已结束，Mac 保持静态 IPv4',
    idle: '尚未开始',
  } as Record<string, string>)[stage]
  return label ? t(label) : stage
}

// A running takeover still has a recovery plan, but it is the intended steady
// state rather than an unfinished restoration. Reserve the cross-page warning
// for interrupted setup and the post-stop path that needs operator action.
export function needsNetworkRecoveryWarning(stage: string) {
  return !['idle', 'prepared', 'gateway_active', 'client_validated', 'client_validation_skipped', 'complete', 'complete_static'].includes(stage)
}

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
  enabling_system_dns: '接管 Mac 系统 DNS',
  initiating_tailscale: '发起 Tailscale 预热',
  restoring_system_proxy: '恢复原有系统代理设置',
  restoring_system_dns: '恢复 Mac 系统 DNS',
  stopping_dns: '停止 DHCP / DNS 服务',
  stopping_ipv6: '撤销下游 IPv6 接管',
  stopping_mihomo: '停止 Mihomo 进程',
  restoring_network: '恢复防火墙与转发设置',
  clearing_runtime: '清理本次运行状态',
  rolling_back: '操作未完成，正在回滚网络改动',
  restoring_config: '恢复之前的配置与网关',
  probing_dhcp: '正在探测 DHCP OFFER',
}

export function operationPhaseLabel(phase?: string) {
  return t(phaseLabels[phase ?? ""] || "正在执行操作")
}
