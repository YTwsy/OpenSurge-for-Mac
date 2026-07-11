import { describe, expect, it } from 'vitest'
import { recoveryLabel, statusLabel } from './status'

describe('status labels', () => {
  it('does not confuse an unreachable control service with a stopped gateway', () => {
    expect(statusLabel()).toBe('无法连接')
    expect(statusLabel('stopped')).toBe('已停止')
  })

  it('keeps the dangerous stopped recovery stage explicit', () => {
    expect(recoveryLabel('gateway_stopped_waiting_router_dhcp')).toContain('等待恢复路由器 DHCP')
  })
})
