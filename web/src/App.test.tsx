// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
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
    discardRecovery: vi.fn(),
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
    saveDevicePolicy: vi.fn(),
    selectDevicePolicy: vi.fn(),
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
  recovery: {
    stage: 'prepared', topology: 'same_wifi_dhcp', required: true,
    network_snapshot: {
      network_service: 'Wi-Fi', interface: 'en0', ipv4: '192.168.1.10', subnet_mask: '255.255.255.0',
      router: '192.168.1.1', dns: ['192.168.1.1', '1.1.1.1'], ipv6_default: false,
    },
  },
}

describe('OpenSurge app shell', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/dashboard')
    window.localStorage.clear()
    delete document.documentElement.dataset.theme
    vi.mocked(api.overview).mockResolvedValue(overview)
  })
  afterEach(() => { cleanup(); vi.clearAllMocks() })

  it('does not present a saved recovery card as an unfinished network recovery', async () => {
    render(<App />)
    expect(await screen.findByRole('heading', { name: '全屋网关，一眼可见' })).toBeTruthy()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.getByRole('button', { name: '启动网关' }).hasAttribute('disabled')).toBe(false)
  })

  it('warns on every page only after the recovery flow changes network state', async () => {
    vi.mocked(api.overview).mockResolvedValue({ ...overview, recovery: { ...overview.recovery, stage: 'mac_static' } })
    render(<App />)
    expect(await screen.findByRole('heading', { name: '全屋网关，一眼可见' })).toBeTruthy()
    expect(screen.getByRole('alert').textContent).toContain('网络恢复尚未完成')
  })

  it('does not label an active takeover as unfinished network recovery', async () => {
    vi.mocked(api.overview).mockResolvedValue({ ...overview, status: { ...overview.status, gateway: 'running' }, recovery: { ...overview.recovery, stage: 'gateway_active' } })
    render(<App />)
    expect(await screen.findByRole('heading', { name: '全屋网关，一眼可见' })).toBeTruthy()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.getAllByText('正在运行').length).toBeGreaterThan(0)
  })

  it('navigates to the cooperative same-LAN DHCP recovery flow', async () => {
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '网络设置' }))
    expect(screen.getByRole('heading', { name: '网络与 DHCP 接管' })).toBeTruthy()
    expect(screen.getByText('合作式 IPv4 模式')).toBeTruthy()
    expect(window.location.pathname).toBe('/network')
  })

  it('shows, links, downloads, and can discard the prepared recovery card', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '网络设置' }))
    const card = (await screen.findByText('已保存的恢复资料')).closest('section')!
    expect(within(card).getByText('192.168.1.10')).toBeTruthy()
    expect(within(card).getByText('192.168.1.1, 1.1.1.1')).toBeTruthy()
    expect(within(card).getByText('Wi-Fi')).toBeTruthy()
    expect(within(card).getByText('en0')).toBeTruthy()
    const routerLinks = screen.getAllByRole('link', { name: '192.168.1.1' })
    expect(routerLinks.some(link => link.getAttribute('href') === 'http://192.168.1.1')).toBe(true)
    expect(screen.getAllByText('打不开?试试 https 或路由器专属域名').length).toBeGreaterThan(0)
    expect(screen.getByRole('link', { name: '查看恢复卡' }).getAttribute('href')).toBe('/api/v1/recovery/card')
    expect(screen.getByRole('link', { name: '下载恢复卡' }).getAttribute('href')).toBe('/api/v1/recovery/card?download=1')
    await userEvent.click(screen.getByRole('button', { name: '放弃恢复并销毁资料' }))
    expect(api.discardRecovery).toHaveBeenCalledOnce()
  })

  it('shows router shutdown guidance with the detected administration link', async () => {
    vi.mocked(api.overview).mockResolvedValue({ ...overview, recovery: { ...overview.recovery, stage: 'mac_static' } })
    render(<App />)
    await userEvent.click(await screen.findByRole('button', { name: '网络设置' }))
    expect(await screen.findByText('关闭路由器 DHCP')).toBeTruthy()
    expect(screen.getByText('关闭 DHCP → 保存；保留路由器 LAN IP 不变')).toBeTruthy()
    expect(screen.getAllByRole('link', { name: '192.168.1.1' }).some(link => link.getAttribute('href') === 'http://192.168.1.1')).toBe(true)
  })

  it('shows fallback router discovery guidance when no IPv4 router was found', async () => {
    vi.mocked(api.overview).mockResolvedValue({ ...overview, recovery: { ...overview.recovery, stage: 'gateway_stopped_waiting_router_dhcp', network_snapshot: { ...overview.recovery.network_snapshot!, router: '' } } })
    vi.mocked(api.gatewayPlan).mockResolvedValue({
      schema_version: 1, revision: 'config-revision', topology: 'same_wifi_dhcp',
      snapshot: { network_service: 'Wi-Fi', interface: 'en0', ipv4: '192.168.1.20', subnet_mask: '255.255.255.0', router: '', dns: [], ipv6_default: false },
      protected_ipv4: [], dhcp_servers: [], warnings: [], blockers: [],
    })
    render(<App />)
    await userEvent.click(await screen.findByRole('button', { name: '网络设置' }))
    expect(await screen.findByText('恢复路由器 DHCP')).toBeTruthy()
    expect(screen.getByText(/未能自动获取路由器地址/).textContent).toContain("networksetup -getinfo 'Wi-Fi'")
  })

  it('does not immediately re-run IPv4 discovery after restoring Mac DHCP', async () => {
    vi.mocked(api.overview).mockResolvedValue({ ...overview, recovery: { ...overview.recovery, stage: 'router_dhcp_restored' } })
    render(<App />)
    await userEvent.click(await screen.findByRole('button', { name: '网络设置' }))
    await screen.findByRole('button', { name: '将 Mac 恢复为自动 DHCP' })
    await waitFor(() => expect(api.gatewayPlan).toHaveBeenCalled())
    vi.mocked(api.gatewayPlan).mockClear()
    await userEvent.click(screen.getByRole('button', { name: '将 Mac 恢复为自动 DHCP' }))
    await waitFor(() => expect(api.restoreMacDHCP).toHaveBeenCalled())
    expect(api.gatewayPlan).not.toHaveBeenCalled()
    expect(screen.queryByText(/does not expose a complete IPv4 configuration/)).toBeNull()
  })

  it('switches between dark and light backgrounds and remembers the choice', async () => {
    render(<App />)
    const toggle = await screen.findByRole('button', { name: '切换为浅色模式' })
    await userEvent.click(toggle)
    expect(document.documentElement.dataset.theme).toBe('light')
    expect(window.localStorage.getItem('opensurge-theme')).toBe('light')
    expect(screen.getByRole('button', { name: '切换为深色模式' })).toBeTruthy()
  })

  it('requires saving corrected configuration before the prepared recovery can advance', async () => {
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '网络设置' }))
    const save = await screen.findByRole('button', { name: '保存网络配置' })
    expect(save.hasAttribute('disabled')).toBe(false)
    await userEvent.clear(screen.getByLabelText('Mac 网关 IPv4'))
    await userEvent.type(screen.getByLabelText('Mac 网关 IPv4'), '192.168.1.21')
    expect(screen.getByText('网络配置有未保存的修改。先保存配置，再保存恢复资料或继续第 2 步。')).toBeTruthy()
    expect(screen.getByRole('button', { name: '将 Mac 切换为固定 IPv4' }).hasAttribute('disabled')).toBe(true)
  })

  it('selects an isolated topology in the revisioned network editor', async () => {
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '网络设置' }))
    expect(screen.getByRole('button', { name: /同一 LAN DHCP 接管/ })).toBeTruthy()
    const isolated = await screen.findByRole('button', { name: /独立下游 LAN/ })
    await userEvent.click(isolated)
    expect(isolated.getAttribute('aria-pressed')).toBe('true')
    expect(screen.getByLabelText('下游 LAN 接口')).toBeTruthy()
    expect(screen.getByLabelText('上游 DNS')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'mihomo DNS（推荐）' }))
    expect((screen.getByLabelText('上游 DNS') as HTMLInputElement).value).toBe('127.0.0.1#1053')
    await userEvent.click(screen.getByRole('button', { name: '公共 DNS（调试）' }))
    expect((screen.getByLabelText('上游 DNS') as HTMLInputElement).value).toBe('1.1.1.1')
    expect(screen.getByText('填写顺序')).toBeTruthy()
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
    vi.mocked(api.config).mockResolvedValue({
      schema_version: 1, revision: 'config-revision',
      gateway: { mode: 'same_wifi_dhcp', interface: 'en0', lan_ip: '192.168.1.20', upstream_interface: 'en0' },
      dhcp: { enabled: true, range_start: '192.168.1.120', range_end: '192.168.1.199', lease_time: '12h', domain: 'lan' },
      dns: { listen: '192.168.1.20', upstream: '1.1.1.1' }, transparent: { mode: 'tun', strict_route: false },
      device_policy: { enabled: true, protected_ipv4: [] },
    })
    vi.mocked(api.devicePolicy).mockResolvedValue({ schema_version: 1, revision: 'policy-r', policy: { devices: [], profiles: [], templates: [], rule_sets: [] } })
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '设备' }))
    await userEvent.type(await screen.findByLabelText('Template ID'), 'home')
    await userEvent.click(screen.getByRole('button', { name: '添加模板' }))
    expect(screen.getByText('template: home')).toBeTruthy()
  })

  it('prefills a device policy registration from a current DHCP lease', async () => {
    const lease = { ip: '192.168.1.123', mac: 'AA:BB:CC:DD:EE:12', hostname: 'Pixel-10', expires_at: '2026-07-13T12:00:00Z', online: true }
    vi.mocked(api.overview).mockResolvedValue({ ...overview, leases: [lease] })
    vi.mocked(api.config).mockResolvedValue({
      schema_version: 1, revision: 'config-revision',
      gateway: { mode: 'same_wifi_dhcp', interface: 'en0', lan_ip: '192.168.1.20', upstream_interface: 'en0' },
      dhcp: { enabled: true, range_start: '192.168.1.120', range_end: '192.168.1.199', lease_time: '12h', domain: 'lan' },
      dns: { listen: '192.168.1.20', upstream: '1.1.1.1' }, transparent: { mode: 'tun', strict_route: false },
      device_policy: { enabled: true, protected_ipv4: [] },
    })
    vi.mocked(api.devicePolicy).mockResolvedValue({ schema_version: 1, revision: 'policy-r', policy: { devices: [], profiles: [{ id: 'home', default_policies: ['DIRECT'], rules: [] }], templates: [], rule_sets: [] } })
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '设备' }))
    expect(await screen.findByText('当前已接管设备')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: '配置此设备' }))
    expect((screen.getByLabelText('Device ID') as HTMLInputElement).value).toBe('pixel-10')
    expect((screen.getByLabelText('设备 MAC') as HTMLInputElement).value).toBe(lease.mac)
    expect((screen.getByLabelText('固定 IPv4') as HTMLInputElement).value).toBe(lease.ip)
    expect((screen.getByLabelText('设备 Profile') as HTMLSelectElement).value).toBe('home')
    await userEvent.click(screen.getByRole('button', { name: '登记或更新设备' }))
    await userEvent.click(screen.getByRole('button', { name: '保存 Desired Policy' }))
    expect(api.saveDevicePolicy).toHaveBeenCalledWith(expect.objectContaining({ devices: [{ id: 'pixel-10', mac: lease.mac, ipv4: lease.ip, profile: 'home' }] }), 'policy-r')
  })
})
