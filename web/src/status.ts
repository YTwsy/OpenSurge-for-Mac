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

// Saving a recovery card deliberately locks the desired configuration, but it
// does not change the Mac, router, or DHCP service. Keep that safe preflight
// distinct from the stages that require an operator to restore networking.
export function needsNetworkRecoveryWarning(stage: string) {
  return !['idle', 'prepared', 'complete'].includes(stage)
}
