// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'
import { ConnectionRefreshControl } from './ConnectionRefreshControl'

afterEach(() => cleanup())

it('reports when there are no active connections to refresh', async () => {
  const refresh = vi.fn(async () => ({ schema_version: 1, scope: 'device' as const, device_id: 'phone', matched_connections: 0, closed_connections: 0 }))
  render(<ConnectionRefreshControl ariaLabel="刷新测试设备连接" refresh={refresh} />)

  await userEvent.click(screen.getByRole('button', { name: '刷新测试设备连接' }))

  expect(refresh).toHaveBeenCalledTimes(1)
  expect(await screen.findByText('当前没有需要刷新的连接。')).toBeTruthy()
})

it('keeps a failed refresh visible as an alert', async () => {
  const refresh = vi.fn(async () => { throw new Error('Mihomo 暂时不可用') })
  render(<ConnectionRefreshControl ariaLabel="刷新测试设备连接" refresh={refresh} />)

  await userEvent.click(screen.getByRole('button', { name: '刷新测试设备连接' }))

  expect((await screen.findByRole('alert')).textContent).toContain('Mihomo 暂时不可用')
})
