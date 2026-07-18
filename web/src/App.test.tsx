// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Overview, Source } from './types'

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
    finishRecoveryManually: vi.fn(),
    finishRecoveryKeepingStatic: vi.fn(),
    restoreMacDHCP: vi.fn(),
    validateClient: vi.fn(),
    skipClientValidation: vi.fn(),
    sources: vi.fn(async () => ({ revision: 'config-revision', sources: [] })),
    importURL: vi.fn(),
    importFile: vi.fn(),
    refreshSource: vi.fn(),
    applySource: vi.fn(),
    devices: vi.fn(async () => ({ devices: [], leases: [], drift: false, applied: false })),
    deviceTraffic: vi.fn(async () => ({ schema_version: 1, revision: 'r', sampled_at: '2026-07-13T00:00:00Z', scope: 'active_sessions', devices: [], totals: { devices: 0, active_connections: 0, upload: 0, download: 0, upload_rate: 0, download_rate: 0 }, gateway_rates: { upload: 0, download: 0 }, unmatched_connections: 0 })),
    policies: vi.fn(async () => ({ groups: [] })),
    selectPolicy: vi.fn(),
    devicePolicy: vi.fn(async () => null),
    saveDevicePolicy: vi.fn(),
    selectDevicePolicy: vi.fn(),
    proxyHealth: vi.fn(async () => ({ schema_version: 1, test_url: 'https://www.gstatic.com/generate_204', proxies: [] })),
    testProxyHealth: vi.fn(async () => ({ schema_version: 1, test_url: 'https://www.gstatic.com/generate_204', results: [] })),
    connectivity: vi.fn(async () => ({ schema_version: 1, source: 'gateway_mihomo', scope: 'applied_global_rules', rounds: 3, targets: [], results: [] })),
    testConnectivity: vi.fn(async () => ({ schema_version: 1, source: 'gateway_mihomo', scope: 'applied_global_rules', rounds: 3, targets: [], results: [] })),
    refreshProvider: vi.fn(),
    diagnostics: vi.fn(async () => ({ revision: 'r', connections: { upload_total: 0, download_total: 0, connections: [] }, logs: {}, operations: [], recovery: { stage: 'idle', required: false } })),
  },
}))

import { api } from './api'
import { App } from './App'

const overview: Overview = {
  schema_version: 1,
  revision: 'config-revision',
  topology: 'same_wifi_dhcp',
  drift: false,
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
    vi.mocked(api.deviceTraffic).mockResolvedValue({ schema_version: 1, revision: 'r', sampled_at: '2026-07-13T00:00:00Z', scope: 'active_sessions', devices: [], totals: { devices: 0, active_connections: 0, upload: 0, download: 0, upload_rate: 0, download_rate: 0 }, gateway_rates: { upload: 0, download: 0 }, unmatched_connections: 0 })
  })
  afterEach(() => { cleanup(); vi.clearAllMocks() })

  it('does not present a saved recovery card as an unfinished network recovery', async () => {
    render(<App />)
    const brandIcon = document.querySelector<HTMLImageElement>('img.brand-mark')
    expect(brandIcon?.getAttribute('src')).toBe('/opensurge-icon.png')
    expect(await screen.findByRole('heading', { name: '全屋网关，一眼可见' })).toBeTruthy()
    const gateway = screen.getByRole('article', { name: '网关状态' })
    expect(within(gateway).getByText('en0 · 192.168.1.20')).toBeTruthy()
    expect(within(gateway).getByText('接管模式')).toBeTruthy()
    expect(within(gateway).getByText('配置状态')).toBeTruthy()
    expect(screen.getByRole('img', { name: '上传最近 60 秒趋势' }).querySelector('.rate-line')?.getAttribute('d')).toContain(' C ')
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.getByRole('button', { name: '启动网关' }).hasAttribute('disabled')).toBe(false)
  })

  it('routes the dashboard start button to network settings without starting the gateway', async () => {
    render(<App />)
    const start = await screen.findByRole('button', { name: '启动网关' })
    await waitFor(() => expect(start.hasAttribute('disabled')).toBe(false))
    await userEvent.click(start)
    expect(await screen.findByRole('heading', { name: '网络与 DHCP 接管' })).toBeTruthy()
    expect(window.location.pathname).toBe('/network')
    expect(api.gateway).not.toHaveBeenCalled()
  })

  it('routes the dashboard stop button to network settings without stopping the gateway', async () => {
    vi.mocked(api.overview).mockResolvedValue({ ...overview, status: { ...overview.status, gateway: 'running' } })
    render(<App />)
    await userEvent.click(await screen.findByRole('button', { name: '停止网关' }))
    expect(await screen.findByRole('heading', { name: '网络与 DHCP 接管' })).toBeTruthy()
    expect(window.location.pathname).toBe('/network')
    expect(api.gateway).not.toHaveBeenCalled()
  })

  it('joins managed DHCP devices with active mihomo session traffic', async () => {
    vi.mocked(api.deviceTraffic).mockResolvedValue({
      schema_version: 1, revision: 'r', sampled_at: '2026-07-13T00:00:00Z', scope: 'active_sessions', unmatched_connections: 1,
      devices: [
        { hostname: 'Apple-TV', ip: '192.168.1.88', mac: 'aa:bb:cc:dd:ee:88', online: true, active_connections: 3, upload: 96 * 1024, download: 412 * 1024 * 1024, upload_rate: 123_000, download_rate: 2_400_000, primary_egress: '流媒体组 → 美国-02' },
        { ip: '192.168.1.110', mac: 'a4:5e:60:00:00:01', online: false, active_connections: 0, upload: 0, download: 0, upload_rate: 0, download_rate: 0 },
      ],
      totals: { devices: 2, active_connections: 3, upload: 96 * 1024, download: 412 * 1024 * 1024, upload_rate: 123_000, download_rate: 2_400_000 },
      gateway_rates: { upload: 125_000, download: 2_500_000 },
    })
    render(<App />)
    expect(await screen.findByRole('heading', { name: '活跃设备' })).toBeTruthy()
    expect(await screen.findByText('Apple-TV')).toBeTruthy()
    expect(screen.getAllByText('流媒体组 → 美国-02').length).toBeGreaterThan(0)
    expect(screen.getByText('未知设备 a4:5e:60:…')).toBeTruthy()
    expect(screen.getByText('累计 96 KB')).toBeTruthy()
    expect(screen.getByText('累计 412 MB')).toBeTruthy()
    expect(screen.getAllByText('123 kB/s').length).toBeGreaterThan(0)
    expect(screen.getByText(/合计 2 台 · 3 个设备连接/)).toBeTruthy()
    expect(screen.getByText(/1 个连接无法匹配 DHCP 租约/)).toBeTruthy()
    expect(screen.getByText('192.168.1.88')).toBeTruthy()

    const deviceButton = screen.getByRole('button', { name: '查看 Apple-TV 192.168.1.88 流量趋势' })
    await userEvent.click(deviceButton)
    expect(deviceButton.getAttribute('aria-expanded')).toBe('true')
    expect(screen.getByRole('heading', { name: 'Apple-TV 流量趋势' })).toBeTruthy()
  })

  it('prefers registered device names in traffic and recent lease summaries', async () => {
    vi.mocked(api.overview).mockResolvedValue({
      ...overview,
      leases: [{ ip: '192.168.1.190', mac: '90:47:48:c8:f9:1b', registered_name: 'PlayStation 5', expires_at: '2099-01-01T00:00:00Z', online: true }],
    })
    vi.mocked(api.deviceTraffic).mockResolvedValue({
      schema_version: 1, revision: 'r', sampled_at: '2026-07-13T00:00:00Z', scope: 'active_sessions', unmatched_connections: 0,
      devices: [{ name: 'PlayStation 5', ip: '192.168.1.190', mac: '90:47:48:c8:f9:1b', online: true, active_connections: 1, upload: 1, download: 2, upload_rate: 0, download_rate: 0 }],
      totals: { devices: 1, active_connections: 1, upload: 1, download: 2, upload_rate: 0, download_rate: 0 },
      gateway_rates: { upload: 0, download: 0 },
    })
    render(<App />)
    expect((await screen.findAllByText('PlayStation 5')).length).toBe(1)
    expect(screen.queryByText(/未知设备 90:47:48/)).toBeNull()
    expect(screen.queryByText('未命名设备')).toBeNull()
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

  it('allows the client acceptance checkpoint to be explicitly skipped', async () => {
    vi.mocked(api.overview).mockResolvedValue({ ...overview, status: { ...overview.status, gateway: 'running' }, recovery: { ...overview.recovery, stage: 'gateway_active' } })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(<App />)
    await userEvent.click(await screen.findByRole('button', { name: '网络设置' }))
    await userEvent.click(await screen.findByRole('button', { name: '跳过客户端验收' }))
    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining('不能把本次运行称为已验收'))
    expect(api.skipClientValidation).toHaveBeenCalledOnce()
  })

  it('navigates to the cooperative same-LAN DHCP recovery flow', async () => {
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '网络设置' }))
    expect(screen.getByRole('heading', { name: '网络与 DHCP 接管' })).toBeTruthy()
    expect(screen.getByText('合作式 IPv4 模式')).toBeTruthy()
    expect(window.location.pathname).toBe('/network')
  })

  it('navigates to the native applied-path connectivity page', async () => {
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '连通性' }))
    expect(screen.getByRole('heading', { name: '分流与网络连通性' })).toBeTruthy()
    expect(window.location.pathname).toBe('/connectivity')
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

  it('does not let takeover plan blockers lock post-stop recovery actions', async () => {
    vi.mocked(api.overview).mockResolvedValue({ ...overview, recovery: { ...overview.recovery, stage: 'gateway_stopped_waiting_router_dhcp' } })
    vi.mocked(api.gatewayPlan).mockResolvedValue({
      schema_version: 1, revision: 'config-revision', topology: 'same_wifi_dhcp',
      snapshot: { network_service: 'Wi-Fi', interface: 'en0', ipv4: '192.168.1.103', subnet_mask: '255.255.255.0', router: '192.168.1.1', dns: [], ipv6_default: false },
      protected_ipv4: [], dhcp_servers: [], warnings: [], blockers: ['Mac IPv4 192.168.1.103 differs from configured gateway.lan_ip 192.168.1.20'],
    })
    render(<App />)
    await userEvent.click(await screen.findByRole('button', { name: '网络设置' }))
    expect((await screen.findByRole('button', { name: '路由器 DHCP 已恢复，执行 OFFER 探测' })).hasAttribute('disabled')).toBe(false)
    expect(screen.getByRole('button', { name: '跳过 OFFER 探测并恢复 Mac 自动 DHCP' }).hasAttribute('disabled')).toBe(false)
    expect(screen.getByRole('button', { name: '保留静态 IP 并结束' }).hasAttribute('disabled')).toBe(false)
    await userEvent.click(screen.getByRole('button', { name: '路由器 DHCP 已恢复，执行 OFFER 探测' }))
    expect(api.confirmRouterRestored).toHaveBeenCalledOnce()
  })

  it('manually finishes post-stop recovery only after explicit confirmation', async () => {
    vi.mocked(api.overview).mockResolvedValue({ ...overview, recovery: { ...overview.recovery, stage: 'gateway_stopped_waiting_router_dhcp' } })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(<App />)
    await userEvent.click(await screen.findByRole('button', { name: '网络设置' }))
    await userEvent.click(await screen.findByRole('button', { name: '跳过 OFFER 探测并恢复 Mac 自动 DHCP' }))
    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining('如果路由器 DHCP 实际未恢复，Mac 可能断网'))
    expect(api.finishRecoveryManually).toHaveBeenCalledOnce()
  })

  it('can finish the post-stop flow while keeping the Mac static', async () => {
    vi.mocked(api.overview).mockResolvedValue({ ...overview, recovery: { ...overview.recovery, stage: 'gateway_stopped_waiting_router_dhcp' } })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(<App />)
    await userEvent.click(await screen.findByRole('button', { name: '网络设置' }))
    await userEvent.click(await screen.findByRole('button', { name: '保留静态 IP 并结束' }))
    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining('不会探测路由器 DHCP，也不会把 Mac 切回自动 DHCP'))
    expect(api.finishRecoveryKeepingStatic).toHaveBeenCalledOnce()
    expect(api.restoreMacDHCP).not.toHaveBeenCalled()
    expect(api.confirmRouterRestored).not.toHaveBeenCalled()
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
    expect(screen.getByText(/保存不会立即改动网络/)).toBeTruthy()
    expect(screen.getByText('有未保存的修改')).toBeTruthy()
    expect(screen.getByRole('button', { name: '保存网络配置' })).toBeTruthy()
  })

  it('imports an HTTPS source as a draft', async () => {
    vi.mocked(api.importURL).mockImplementationOnce(() => new Promise<Source>(() => {}))
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '代理与规则源' }))
    await userEvent.type(screen.getByLabelText('来源名称'), 'Home')
    await userEvent.type(screen.getByLabelText('HTTPS 订阅 URL'), 'https://example.com/profile')
    await userEvent.click(screen.getByRole('button', { name: '导入为草稿' }))
    expect(api.importURL).toHaveBeenCalledWith('Home', 'https://example.com/profile')
    expect(await screen.findByRole('button', { name: '正在导入并校验…' })).toBeTruthy()
  })

  it('shows explicit feedback after refreshing a source draft', async () => {
    const source: Source = {
      id: 'remote', name: 'Home', kind: 'mihomo_profile', origin: 'https://example.com/profile', digest: 'next', size: 100,
      valid: true, validation: 'valid', desired: false, applied: false, versions: [], imported_at: '2026-07-15T00:00:00Z',
      diff: { proxies_added: [], proxies_removed: [], groups_added: [], groups_removed: [], proxy_providers_added: [], proxy_providers_removed: [], rule_providers_added: [], rule_providers_removed: [], rule_count_delta: 0 },
      inventory: { proxies: ['edge'], proxy_providers: [], proxy_groups: ['Main'], rule_providers: [], rule_count: 1, terminal_match: true, warnings: [] },
    }
    vi.mocked(api.sources).mockResolvedValue({ revision: 'config-revision', sources: [source] })
    vi.mocked(api.refreshSource).mockResolvedValue(source)
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '代理与规则源' }))
    await userEvent.click(await screen.findByRole('button', { name: '刷新草稿' }))
    expect(await screen.findByText('Home 已刷新；新内容已保存为草稿。')).toBeTruthy()
  })

  it('confirms and applies a source through a running gateway reload', async () => {
    vi.mocked(api.overview).mockResolvedValue({ ...overview, drift: true, status: { ...overview.status, gateway: 'running', dhcp: 'running', mihomo: 'running', pf_anchor: 'loaded', forwarding: 'enabled' } })
    const source: Source = {
      id: 'home', name: 'Home', kind: 'mihomo_profile', origin: 'file:home.yaml', digest: 'next', size: 100,
      valid: true, validation: 'valid', desired: false, applied: false, versions: [], imported_at: '2026-07-15T00:00:00Z',
      diff: { proxies_added: [], proxies_removed: [], groups_added: [], groups_removed: [], proxy_providers_added: [], proxy_providers_removed: [], rule_providers_added: [], rule_providers_removed: [], rule_count_delta: 0 },
      inventory: { proxies: ['edge'], proxy_providers: [], proxy_groups: ['Main'], rule_providers: [], rule_count: 1, terminal_match: true, warnings: [] },
    }
    vi.mocked(api.sources).mockResolvedValue({ revision: 'config-revision', sources: [source] })
    let resolveApply!: (value: Source) => void
    vi.mocked(api.applySource).mockImplementationOnce(() => new Promise<Source>(resolve => { resolveApply = resolve }))
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '代理与规则源' }))
    await userEvent.click(await screen.findByRole('button', { name: '校验、应用并重载' }))
    const dialog = screen.getByRole('dialog', { name: '应用订阅并重载网关？' })
    expect(within(dialog).getByText(/只有重载成功后才会标记为运行版本/)).toBeTruthy()
    await userEvent.click(within(dialog).getByRole('button', { name: '确认应用并重载' }))
    await waitFor(() => expect(api.applySource).toHaveBeenCalledWith('home', 'config-revision'))
    expect(within(dialog).getByRole('button', { name: '正在验证并应用…' })).toBeTruthy()
    resolveApply({ ...source, desired: true, applied: true })
    expect(await screen.findByText('订阅已应用，网关已使用新的运行配置。')).toBeTruthy()
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
    await userEvent.click(await screen.findByRole('button', { name: /高级 \/ 复用机制/ }))
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
    expect((screen.getByLabelText('设备名称') as HTMLInputElement).value).toBe('Pixel-10')
    expect((screen.getByLabelText('设备 MAC') as HTMLInputElement).value).toBe(lease.mac)
    expect((screen.getByLabelText('固定 IPv4') as HTMLInputElement).value).toBe(lease.ip)
    await userEvent.click(screen.getByRole('button', { name: '登记或更新设备' }))
    await userEvent.click(screen.getByRole('button', { name: '保存设备配置' }))
    expect(api.saveDevicePolicy).toHaveBeenCalledWith(expect.objectContaining({
      devices: [{ id: 'pixel-10', name: 'Pixel-10', mac: lease.mac.toLowerCase(), ipv4: lease.ip, profile: 'pixel-10-policy', egress_mode: 'inherit_global' }],
      profiles: expect.arrayContaining([expect.objectContaining({ id: 'pixel-10-policy', default_policies: ['DIRECT'] })]),
    }), 'policy-r')
  })

  it('protects unsaved device edits before sidebar navigation', async () => {
    vi.mocked(api.config).mockResolvedValue({
      schema_version: 1, revision: 'config-revision',
      gateway: { mode: 'same_wifi_dhcp', interface: 'en0', lan_ip: '192.168.1.20', upstream_interface: 'en0' },
      dhcp: { enabled: true, range_start: '192.168.1.120', range_end: '192.168.1.199', lease_time: '12h', domain: 'lan' },
      dns: { listen: '192.168.1.20', upstream: '1.1.1.1' }, transparent: { mode: 'tun', strict_route: false },
      device_policy: { enabled: true, protected_ipv4: [] },
    })
    vi.mocked(api.devicePolicy).mockResolvedValue({ schema_version: 1, revision: 'policy-r', policy: { devices: [], profiles: [], templates: [], rule_sets: [] } })
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    render(<App />)
    await screen.findByRole('heading', { name: '全屋网关，一眼可见' })
    await userEvent.click(screen.getByRole('button', { name: '设备' }))
    await userEvent.click(await screen.findByRole('button', { name: /高级 \/ 复用机制/ }))
    await userEvent.type(await screen.findByLabelText('Template ID'), 'draft-template')
    await userEvent.click(screen.getByRole('button', { name: '添加模板' }))
    await userEvent.click(screen.getByRole('button', { name: '策略' }))
    expect(window.location.pathname).toBe('/devices')
    expect(screen.getByText('template: draft-template')).toBeTruthy()
    confirm.mockReturnValue(true)
    await userEvent.click(screen.getByRole('button', { name: '策略' }))
    expect(window.location.pathname).toBe('/policies')
  })
})
