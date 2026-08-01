// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ConnectivityResponse, Overview } from '../types'

vi.mock('../api', () => ({ api: { connectivity: vi.fn(), testConnectivity: vi.fn(), gateway: vi.fn() }, waitForOperation: vi.fn() }))

import { api, waitForOperation } from '../api'
import { ConnectivityPage } from './ConnectivityPage'

const catalog: ConnectivityResponse = {
  schema_version: 1,
  source: 'gateway_mihomo',
  scope: 'applied_global_rules',
  rounds: 3,
  targets: [
    { id: 'baidu', name: '百度', category: 'china', symbol: 'BD', url: 'https://www.baidu.com/favicon.ico', expected_route: 'direct' },
    { id: 'github', name: 'GitHub', category: 'developer', symbol: 'GH', url: 'https://github.com/favicon.ico', expected_route: 'proxy' },
  ],
  results: [],
}

const running = {
  drift: false,
  status: { gateway: 'running', mihomo: 'running' },
} as unknown as Overview

describe('ConnectivityPage', () => {
  beforeEach(() => {
    window.localStorage.clear()
    vi.mocked(api.connectivity).mockResolvedValue(catalog)
    vi.mocked(api.testConnectivity).mockResolvedValue({
      ...catalog,
      started_at: '2026-07-16T08:00:00Z',
      completed_at: '2026-07-16T08:00:02Z',
      results: [
        { target_id: 'baidu', status: 'reachable', grade: 'excellent', median_ms: 28, http_status: 200, chain: ['DIRECT'], rule: 'DomainSuffix', rule_payload: 'baidu.com', route: 'direct', route_match: true, tested_at: '2026-07-16T08:00:01Z', samples: [{ status: 'reachable', delay_ms: 27, http_status: 200 }, { status: 'reachable', delay_ms: 28, http_status: 200 }, { status: 'reachable', delay_ms: 31, http_status: 200 }] },
        { target_id: 'github', status: 'reachable', grade: 'good', median_ms: 188, http_status: 200, chain: ['DIRECT'], rule: 'MATCH', rule_payload: 'DIRECT', route: 'direct', route_match: false, tested_at: '2026-07-16T08:00:02Z', samples: [{ status: 'reachable', delay_ms: 188, http_status: 200 }] },
      ],
    })
    vi.mocked(api.gateway).mockResolvedValue({ id: 'restart-1', kind: 'restart-mihomo', state: 'running' } as never)
    vi.mocked(waitForOperation).mockResolvedValue({ id: 'restart-1', kind: 'restart-mihomo', state: 'succeeded' } as never)
  })

  afterEach(() => { cleanup(); vi.clearAllMocks() })

  it('runs the applied-path catalog and exposes reachability plus route evidence', async () => {
    render(<ConnectivityPage overview={running} />)
    expect(await screen.findByText('百度')).toBeTruthy()
    expect(api.testConnectivity).not.toHaveBeenCalled()
    expect(document.querySelector('.mismatch-badge')).toBeNull()

    await userEvent.click(screen.getByRole('button', { name: '检测全部' }))
    await waitFor(() => expect(api.testConnectivity).toHaveBeenCalledWith(['baidu', 'github']))
    expect(await screen.findByText('2/2')).toBeTruthy()
    expect(screen.getByText('1 项路径需要关注')).toBeTruthy()
    expect(screen.getAllByText('路径不符').length).toBe(2)

    const github = screen.getByText('GitHub').closest('article')!
    expect(within(github).getAllByText('DIRECT').length).toBeGreaterThan(0)
    await userEvent.click(within(github).getByText('查看检测证据'))
    expect(within(github).getByText('MATCH · DIRECT')).toBeTruthy()
  })

  it('permits probes when the running mihomo status includes its live version', async () => {
    render(<ConnectivityPage overview={{ ...running, status: { ...running.status, mihomo: 'running (v1.19.27)' } }} />)
    await screen.findByText('百度')

    expect((screen.getByRole('button', { name: '检测全部' }) as HTMLButtonElement).disabled).toBe(false)
    expect(screen.queryByText(/启动网关和 mihomo/)).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: '检测全部' }))
    await waitFor(() => expect(api.testConnectivity).toHaveBeenCalledWith(['baidu', 'github']))
  })

  it('keeps external browser testing available while the gateway is stopped', async () => {
    render(<ConnectivityPage overview={{ ...running, status: { ...running.status, gateway: 'stopped', mihomo: 'stopped' } }} />)
    await screen.findByText('百度')
    expect((screen.getByRole('button', { name: '检测全部' }) as HTMLButtonElement).disabled).toBe(true)
    expect(screen.getByRole('link', { name: /本机浏览器线路/ }).getAttribute('href')).toBe('https://ip.net.coffee/link/')
    expect(screen.getByText(/启动网关和 mihomo/)).toBeTruthy()
  })

  it('restarts only mihomo and reruns applied-path probes after recovery', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(<ConnectivityPage overview={running} />)
    await screen.findByText('百度')
    await userEvent.click(screen.getByRole('button', { name: '仅重启 Mihomo' }))
    await waitFor(() => expect(api.gateway).toHaveBeenCalledWith('restart-mihomo'))
    expect(waitForOperation).toHaveBeenCalledWith('restart-1')
    await waitFor(() => expect(api.testConnectivity).toHaveBeenCalledWith(['baidu', 'github']))
  })
})
