// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LocalRouting, Overview, ProxyHealthSnapshot } from '../types'

vi.mock('../api', () => ({
  api: {
    proxyHealth: vi.fn(),
    testProxyHealth: vi.fn(),
    selectPolicy: vi.fn(),
    localRouting: vi.fn(),
    setLocalRouting: vi.fn(),
  },
}))

import { api } from '../api'
import { PoliciesPage } from './PoliciesPage'

const health: ProxyHealthSnapshot = {
  schema_version: 1,
  test_url: 'https://www.gstatic.com/generate_204',
  proxies: [
    { name: 'Proxy-A', type: 'Hysteria2', selected: '', provider: 'home', udp: true, status: 'reachable', delay_ms: 86, tested_at: '2026-07-16T08:00:00Z', probeable: true },
    { name: 'Proxy-B', type: 'Trojan', selected: '', provider: 'home', udp: false, status: 'timeout', tested_at: '2026-07-16T08:00:00Z', probeable: true },
    { name: 'DIRECT', type: 'Direct', selected: '', provider: '', udp: true, status: 'not_applicable', probeable: false },
  ],
}

const overview = {
  status: { gateway: 'running', mihomo: 'running' },
  policies: [
    { name: 'Main', type: 'Selector', selected: 'Proxy-A', options: ['Proxy-A', 'Proxy-B', 'DIRECT'] },
    { name: 'device/alice/default', type: 'Selector', selected: 'Proxy-B', options: ['Proxy-A', 'Proxy-B'] },
  ],
} as unknown as Overview

function localRouting(mode: LocalRouting['mode'] = 'rule', selected = 'Proxy-A'): LocalRouting {
  return {
    schema_version: 1,
    mode,
    available_modes: ['rule', 'direct', 'global'],
    global_group: { name: 'open-surge/mac-global', type: 'Selector', selected, options: ['Proxy-A', 'Proxy-B'] },
    udp_behavior: mode === 'global' ? 'proxy' : mode === 'direct' ? 'direct' : 'rules',
    transports: ['tun', 'loopback_explicit_proxy'],
    new_connections_only: true,
    consistent: true,
  }
}

describe('PoliciesPage', () => {
  beforeEach(() => {
    vi.mocked(api.proxyHealth).mockResolvedValue(health)
    vi.mocked(api.testProxyHealth).mockResolvedValue({ schema_version: 1, test_url: health.test_url, results: [] })
    vi.mocked(api.selectPolicy).mockResolvedValue({} as never)
    vi.mocked(api.localRouting).mockResolvedValue(localRouting())
    vi.mocked(api.setLocalRouting).mockImplementation(async (mode, policy) => localRouting(mode, policy ?? 'Proxy-A'))
  })

  afterEach(() => { cleanup(); vi.clearAllMocks() })

  it('shows global node health, filters device groups, and switches selector nodes', async () => {
    const onChanged = vi.fn(async () => {})
    render(<PoliciesPage overview={overview} onChanged={onChanged} />)

    expect(await screen.findByRole('heading', { name: 'Main' })).toBeTruthy()
    expect(screen.queryByRole('heading', { name: 'device/alice/default' })).toBeNull()
    expect(screen.getAllByText('86 ms').length).toBeGreaterThan(0)
    expect(screen.getByText('超时')).toBeTruthy()

    await userEvent.click(screen.getByRole('button', { name: 'Main 选择 Proxy-B' }))
    await waitFor(() => expect(api.selectPolicy).toHaveBeenCalledWith('Main', 'Proxy-B'))
    expect(onChanged).toHaveBeenCalledOnce()

    await userEvent.click(screen.getByRole('button', { name: '设备策略' }))
    expect(screen.getByRole('heading', { name: 'device/alice/default' })).toBeTruthy()
    expect(screen.queryByRole('heading', { name: 'Main' })).toBeNull()
  })

  it('tests the probeable nodes in the current view', async () => {
    render(<PoliciesPage overview={overview} onChanged={vi.fn(async () => {})} />)
    await screen.findAllByText('86 ms')
    await userEvent.click(screen.getByRole('button', { name: '检测当前视图' }))
    await waitFor(() => expect(api.testProxyHealth).toHaveBeenCalledWith(['Proxy-A', 'Proxy-B']))
  })

  it('keeps the Mac global policy group first and switches it through the local-routing API', async () => {
    const onChanged = vi.fn(async () => {})
    render(<PoliciesPage overview={overview} onChanged={onChanged} />)

    const localGroup = await screen.findByRole('heading', { name: '本机全局策略组' })
    const mainGroup = await screen.findByRole('heading', { name: 'Main' })
    expect(localGroup.compareDocumentPosition(mainGroup) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

    await userEvent.click(screen.getByLabelText('本机全局策略组 当前策略 Proxy-A'))
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /Proxy-B/ }))
    await waitFor(() => expect(api.setLocalRouting).toHaveBeenCalledWith('rule', 'Proxy-B'))
    expect(onChanged).toHaveBeenCalledOnce()
  })
})
