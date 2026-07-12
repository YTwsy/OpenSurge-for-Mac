export function statusLabel(status?: string) {
  return status === 'running' ? '正在运行'
    : status === 'degraded' ? '运行异常'
      : status === 'stopped' ? '已停止'
        : '无法连接'
}

export function recoveryLabel(stage: string) {
  return ({
    prepared: '恢复资料已准备',
    mac_static: 'Mac 已使用固定 IPv4',
    router_dhcp_disabled_confirmed: '路由器 DHCP 已关闭',
    gateway_active: 'OpenSurge 已接管',
    client_validated: '客户端 DHCP、DNS 与 TUN 已验收',
    gateway_stopped_waiting_router_dhcp: '已停止，等待恢复路由器 DHCP',
    router_dhcp_restored: '路由器 DHCP 已恢复',
    complete: 'Mac 与客户端已恢复自动获取',
    idle: '尚未开始',
  } as Record<string, string>)[stage] ?? stage
}

// A running takeover still has a recovery plan, but it is the intended steady
// state rather than an unfinished restoration. Reserve the cross-page warning
// for interrupted setup and the post-stop path that needs operator action.
export function needsNetworkRecoveryWarning(stage: string) {
  return !['idle', 'prepared', 'gateway_active', 'client_validated', 'complete'].includes(stage)
}
