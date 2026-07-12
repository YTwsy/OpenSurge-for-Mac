import { describe, expect, it } from 'vitest'
import { needsNetworkRecoveryWarning, recoveryLabel, statusLabel } from './status'

describe('status labels', () => {
  it('does not confuse an unreachable control service with a stopped gateway', () => {
    expect(statusLabel()).toBe('无法连接')
    expect(statusLabel('stopped')).toBe('已停止')
  })

  it('keeps the dangerous stopped recovery stage explicit', () => {
    expect(recoveryLabel('gateway_stopped_waiting_router_dhcp')).toContain('等待恢复路由器 DHCP')
  })

  it('distinguishes a saved recovery card from changed network state', () => {
    expect(needsNetworkRecoveryWarning('prepared')).toBe(false)
    expect(needsNetworkRecoveryWarning('mac_static')).toBe(true)
  })

  it('treats active takeover as steady state and post-stop as recovery', () => {
    expect(needsNetworkRecoveryWarning('gateway_active')).toBe(false)
    expect(needsNetworkRecoveryWarning('client_validated')).toBe(false)
    expect(needsNetworkRecoveryWarning('gateway_stopped_waiting_router_dhcp')).toBe(true)
    expect(needsNetworkRecoveryWarning('router_dhcp_restored')).toBe(true)
  })
})
