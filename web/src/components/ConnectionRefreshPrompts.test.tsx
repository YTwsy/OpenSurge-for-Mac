// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

vi.mock('../api', () => ({
  api: {
    refreshLocalConnections: vi.fn(),
    refreshDeviceConnections: vi.fn(),
    refreshPolicyConnections: vi.fn(),
  },
}))

import { api } from '../api'
import { ConnectionRefreshPrompts, queueConnectionRefreshSuggestion, type ConnectionRefreshSuggestionItem } from './ConnectionRefreshPrompts'

beforeEach(() => {
  vi.mocked(api.refreshLocalConnections).mockResolvedValue({ schema_version: 1, scope: 'gateway_local', matched_connections: 2, closed_connections: 2 })
  vi.mocked(api.refreshDeviceConnections).mockResolvedValue({ schema_version: 1, scope: 'device', device_id: 'phone', matched_connections: 0, closed_connections: 0 })
  vi.mocked(api.refreshPolicyConnections).mockResolvedValue({ schema_version: 1, scope: 'policy_group', policy_group: '共享/香港 策略', matched_connections: 3, closed_connections: 3 })
})

afterEach(() => { cleanup(); vi.clearAllMocks() })

it('refreshes only the suggested Mac scope and reports the result', async () => {
  const dismiss = vi.fn()
  const refreshed = vi.fn()
  const suggestion = { id: 1, key: 'gateway_local', scope: 'gateway_local' as const, subject: 'Mac 本机', selection: 'Proxy-B' }
  render(<ConnectionRefreshPrompts suggestions={[suggestion]} onDismiss={dismiss} onRefreshed={refreshed} />)

  expect(screen.getByText('新连接将使用“Proxy-B”；已有连接可能继续使用原链路。刷新会关闭 Mac 本机当前由 OpenSurge 管理的连接，下载、通话等可能中断。')).toBeTruthy()
  await userEvent.click(screen.getByRole('button', { name: '刷新 Mac 本机连接' }))

  expect(api.refreshLocalConnections).toHaveBeenCalledOnce()
  expect(api.refreshDeviceConnections).not.toHaveBeenCalled()
  expect(await screen.findByText('已关闭 2 个连接，等待客户端建立新连接。')).toBeTruthy()
  await waitFor(() => expect(refreshed).toHaveBeenCalledOnce())
})

it('keeps the outlet switch successful when a device refresh fails and allows retry', async () => {
  vi.mocked(api.refreshDeviceConnections).mockRejectedValueOnce(new Error('Mihomo 暂时不可用'))
  const suggestion = { id: 2, key: 'device:phone', scope: 'device' as const, deviceID: 'phone', subject: '客厅电视', selection: '日本节点' }
  render(<ConnectionRefreshPrompts suggestions={[suggestion]} onDismiss={vi.fn()} />)

  await userEvent.click(screen.getByRole('button', { name: '刷新 客厅电视 连接' }))

  expect(await screen.findByText('出口已切换，连接刷新失败')).toBeTruthy()
  expect(screen.getByText('Mihomo 暂时不可用')).toBeTruthy()
  await userEvent.click(screen.getByRole('button', { name: '重试刷新连接' }))
  expect(api.refreshDeviceConnections).toHaveBeenCalledTimes(2)
  expect(api.refreshDeviceConnections).toHaveBeenLastCalledWith('phone')
  expect(await screen.findByText('当前没有需要刷新的连接。')).toBeTruthy()
})

it('replaces the same target with its latest selection and keeps distinct targets', () => {
  let current: ConnectionRefreshSuggestionItem[] = []
  current = queueConnectionRefreshSuggestion(current, { key: 'gateway_local', scope: 'gateway_local', subject: 'Mac 本机', selection: 'Proxy-A' }, 1)
  current = queueConnectionRefreshSuggestion(current, { key: 'device:phone', scope: 'device', deviceID: 'phone', subject: '手机', selection: 'Proxy-B' }, 2)
  current = queueConnectionRefreshSuggestion(current, { key: 'gateway_local', scope: 'gateway_local', subject: 'Mac 本机', selection: 'DIRECT' }, 3)

  expect(current).toEqual([
    expect.objectContaining({ key: 'device:phone', selection: 'Proxy-B', id: 2 }),
    expect.objectContaining({ key: 'gateway_local', selection: 'DIRECT', id: 3 }),
  ])
})

it('explains the shared group scope and waits for explicit consent before refreshing it', async () => {
  const suggestion = { id: 3, key: 'policy_group:共享/香港 策略', scope: 'policy_group' as const, group: '共享/香港 策略', subject: '香港策略', selection: '香港 B' }
  const dismiss = vi.fn()
  const refreshed = vi.fn()
  render(<ConnectionRefreshPrompts suggestions={[suggestion]} onDismiss={dismiss} onRefreshed={refreshed} />)

  expect(screen.getByText(/Mac 本机及下游设备当前经过此组的连接，包括跟随网关规则/)).toBeTruthy()
  expect(screen.getByText(/已使用新出口的连接也会关闭/)).toBeTruthy()
  expect(api.refreshPolicyConnections).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: '暂不刷新' }))
  expect(dismiss).toHaveBeenCalledWith(3)
  expect(api.refreshPolicyConnections).not.toHaveBeenCalled()

  await userEvent.click(screen.getByRole('button', { name: '刷新经过此策略组的连接' }))
  expect(api.refreshPolicyConnections).toHaveBeenCalledExactlyOnceWith('共享/香港 策略')
  expect(api.refreshLocalConnections).not.toHaveBeenCalled()
  expect(api.refreshDeviceConnections).not.toHaveBeenCalled()
  expect(await screen.findByText('已关闭 3 个连接，等待客户端建立新连接。')).toBeTruthy()
  await waitFor(() => expect(refreshed).toHaveBeenCalledOnce())
})

it('retries a group refresh without changing the selected outlet or widening its scope', async () => {
  vi.mocked(api.refreshPolicyConnections).mockRejectedValueOnce(new Error('closed 1 of 3 matching connections'))
  const suggestion = { id: 4, key: 'policy_group:Main', scope: 'policy_group' as const, group: 'Main', subject: 'Main', selection: 'Proxy-B' }
  render(<ConnectionRefreshPrompts suggestions={[suggestion]} onDismiss={vi.fn()} />)
  await userEvent.click(screen.getByRole('button', { name: '刷新经过此策略组的连接' }))
  expect(await screen.findByText('出口已切换，连接刷新失败')).toBeTruthy()
  expect(screen.getByText('closed 1 of 3 matching connections')).toBeTruthy()
  await userEvent.click(screen.getByRole('button', { name: '重试刷新连接' }))
  expect(api.refreshPolicyConnections).toHaveBeenCalledTimes(2)
  expect(api.refreshPolicyConnections).toHaveBeenLastCalledWith('Main')
  expect(api.refreshLocalConnections).not.toHaveBeenCalled()
  expect(api.refreshDeviceConnections).not.toHaveBeenCalled()
})

it('replaces a group prompt with its latest selection while retaining other scopes', () => {
  const group = { key: 'policy_group:Main', scope: 'policy_group' as const, group: 'Main', subject: 'Main', selection: 'Proxy-A' }
  const mac = { key: 'gateway_local', scope: 'gateway_local' as const, subject: 'Mac 本机', selection: 'DIRECT' }
  let current = queueConnectionRefreshSuggestion([], group, 1)
  current = queueConnectionRefreshSuggestion(current, mac, 2)
  current = queueConnectionRefreshSuggestion(current, { ...group, selection: 'Proxy-B' }, 3)
  expect(current).toEqual([{ ...mac, id: 2 }, { ...group, selection: 'Proxy-B', id: 3 }])
})
