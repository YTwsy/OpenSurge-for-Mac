// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Overview } from './types'

vi.mock('./api', () => ({
  RequestError: class RequestError extends Error { status = 500 },
  api: {
    overview: vi.fn(),
    config: vi.fn(async () => ({
      schema_version: 1, revision: 'config-revision',
      gateway: { mode: 'same_wifi_dhcp', interface: 'en0', lan_ip: '192.168.1.20', upstream_interface: 'en0' },
      dhcp: { enabled: true, range_start: '192.168.1.120', range_end: '192.168.1.199', lease_time: '12h', domain: 'lan' },
      dns: { listen: '192.168.1.20', upstream: '1.1.1.1' }, transparent: { mode: 'tun', strict_route: false },
      device_policy: { enabled: false, protected_ipv4: [] },
    })),
    saveConfig: vi.fn(),
    gateway: vi.fn(),
    operation: vi.fn(),
    gatewayPlan: vi.fn(async () => ({
      schema_version: 1,
      revision: 'config-revision',
      topology: 'same_wifi_dhcp',
      snapshot: {
        network_service: 'Wi-Fi', interface: 'en0', ipv4: '192.168.1.20',
        subnet_mask: '255.255.255.0', router: '192.168.1.1', dns: ['192.168.1.1'], ipv6_default: false,
      },
      protected_ipv4: ['192.168.1.1', '192.168.1.20'],
      dhcp_servers: [], warnings: [], blockers: [],
    })),
    recovery: vi.fn(),
    prepareRecovery: vi.fn(),
    applyStatic: vi.fn(),
    probeDHCP: vi.fn(),
    confirmRouterRestored: vi.fn(),
    restoreMacDHCP: vi.fn(),
    validateClient: vi.fn(),
    sources: vi.fn(async () => ({ revision: 'config-revision', sources: [] })),
    importURL: vi.fn(),
    importFile: vi.fn(),
    refreshSource: vi.fn(),
    applySource: vi.fn(),
    devices: vi.fn(async () => ({ devices: [], leases: [], drift: false, applied: false })),
    policies: vi.fn(async () => ({ groups: [] })),
    devicePolicy: vi.fn(async () => null),
    refreshProvider: vi.fn(),
    diagnostics: vi.fn(async () => ({ revision: 'r', connections: { upload_total: 0, download_total: 0, connections: [] }, logs: {}, operations: [], recovery: { stage: 'idle', required: false } })),
  },
}))

import { api } from './api'
import { App } from './App'

const overview: Overview = {
  schema_version: 1,
  revision: 'config-revision',
  warnings: [],
  status: {
    gateway: 'stopped', interface: 'en0', lan_ip: '192.168.1.20', dhcp: 'stopped',
    dhcp_enabled: true, mihomo: 'stopped', pf_anchor: 'unloaded', forwarding: 'disabled', client_count: 0,
  },
  doctor: [], doctor_healthy: true, leases: [], policies: [],
  providers: { proxy_providers: [], rule_providers: [] },
  recovery: { stage: 'prepared', topology: 'same_wifi_dhcp', required: true },
}

describe('OpenSurge app shell', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/dashboard')
    vi.mocked(api.overview).mockResolvedValue(overview)
  })
  afterEach(() => { cleanup(); vi.clearAllMocks() })

  it('shows a persisted recovery warning on the dashboard', async () => {
    render(<App />)
    expect(await screen.findByRole('heading', { name: '全屋网关，一眼可见' })).toBeTruthy()
    expect(screen.getByRole('alert').textContent).toContain('网络恢复尚未完成')
    expect(screen.getByRole('button', { name: '启动网关' }).hasAttribute('disabled')).toBe(false)
  })

  it('navigates to the cooperative same-WiFi recovery flow', async () => {
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '网络设置' }))
    expect(screen.getByRole('heading', { name: '网络与 DHCP 接管' })).toBeTruthy()
    expect(screen.getByText('合作式 IPv4 模式')).toBeTruthy()
    expect(window.location.pathname).toBe('/network')
  })

  it('selects an isolated topology in the revisioned network editor', async () => {
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '网络设置' }))
    const isolated = await screen.findByRole('button', { name: /独立下游 LAN/ })
    await userEvent.click(isolated)
    expect(isolated.getAttribute('aria-pressed')).toBe('true')
    expect(screen.getByRole('button', { name: '保存网络配置' })).toBeTruthy()
  })

  it('imports an HTTPS source as a draft', async () => {
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '代理与规则源' }))
    await userEvent.type(screen.getByLabelText('来源名称'), 'Home')
    await userEvent.type(screen.getByLabelText('HTTPS 订阅 URL'), 'https://example.com/profile')
    await userEvent.click(screen.getByRole('button', { name: '导入为草稿' }))
    expect(api.importURL).toHaveBeenCalledWith('Home', 'https://example.com/profile')
  })

  it('edits templates in the structured device policy editor', async () => {
    vi.mocked(api.devicePolicy).mockResolvedValue({ schema_version: 1, revision: 'policy-r', policy: { devices: [], profiles: [], templates: [], rule_sets: [] } })
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '设备' }))
    await userEvent.type(await screen.findByLabelText('Template ID'), 'home')
    await userEvent.click(screen.getByRole('button', { name: '添加模板' }))
    expect(screen.getByText('template: home')).toBeTruthy()
  })
})
