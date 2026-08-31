// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { TailscaleDiscoveryResponse, TailscaleResponse } from '../types'
import { activateLanguage, prepareLanguage } from '../i18n'

vi.mock('../api', () => ({
  api: {
    tailscale: vi.fn(),
    tailscaleDiscovery: vi.fn(),
    devices: vi.fn(),
    saveTailscale: vi.fn(),
    forgetTailscaleIdentity: vi.fn(),
  },
}))

import { api } from '../api'
import { TailscaleCard } from './TailscaleCard'

const base: TailscaleResponse = {
  schema_version: 1,
  revision: 'rev-1',
  settings: {
    enabled: false,
    display_name: 'Tailnet',
    hostname: 'opensurge-mac',
    control_url: 'https://controlplane.tailscale.com',
    accept_routes: false,
    magic_dns_suffixes: [],
    peer_cidrs: [],
    subnet_routes: [],
    allow_mac: true,
    allow_all_devices: false,
    allowed_devices: [],
    exit_node: '',
    exit_node_allow_lan_access: false,
  },
  auth_key_present: false,
  identity_present: false,
  gateway_active: false,
  runtime_state: 'disabled',
  selectable_exit: false,
  warnings: [],
}

const discovery: TailscaleDiscoveryResponse = {
  schema_version: 1,
  available: true,
  backend_state: 'Running',
  tailnet_name: 'Example',
  magic_dns: true,
  magic_dns_suffix: 'example.ts.net',
  peers: [
    { id: 'phone', name: 'Pixel', dns_name: 'pixel.example.ts.net', tailscale_ips: ['100.82.10.7', 'fd7a:115c:a1e0::7'], online: false, exit_node: false, exit_node_option: false, subnet_routes: [] },
    { id: 'router', name: 'Home Router', dns_name: 'home-router.example.ts.net', tailscale_ips: ['100.90.3.4'], online: true, exit_node: false, exit_node_option: true, subnet_routes: ['10.20.0.0/16'] },
  ],
}

beforeEach(() => {
  vi.mocked(api.tailscale).mockResolvedValue(base)
  vi.mocked(api.tailscaleDiscovery).mockResolvedValue(discovery)
  vi.mocked(api.devices).mockResolvedValue({ desired_digest: '', applied_digest: '', drift: false, applied: false, devices: [], desired_devices: [{ id: 'apple-tv', mac: 'aa:bb:cc:dd:ee:01', ipv4: '192.168.50.110', profile: 'home', groups: {} }], applied_devices: [], changed_devices: [], out_of_lan_devices: [], leases: [], observed_devices: [] })
})

afterEach(() => {
  cleanup()
  activateLanguage('zh-Hans')
  vi.clearAllMocks()
})

describe('TailscaleCard', () => {
  it('renders the complete setup surface in English', async () => {
    await prepareLanguage('en')
    activateLanguage('en')
    render(<TailscaleCard onChanged={vi.fn()} onNotify={vi.fn()} />)

    await userEvent.click(await screen.findByRole('button', { name: 'Set up' }))
    const dialog = screen.getByRole('dialog', { name: 'Configure Tailscale outbound' })
    expect(document.body.textContent).toContain('Use the Tailnet as an outbound')
    expect(dialog.textContent).toContain('Who can use it')
    expect(document.body.textContent).not.toMatch(/[\u3400-\u9fff]/)
  })

  it('turns discovered resources into explicit targets and links to Auth Key creation', async () => {
    vi.mocked(api.saveTailscale).mockImplementation(async (_revision, update) => ({
      ...base,
      revision: 'rev-2',
      settings: { ...base.settings, ...update, auth_key: undefined },
      auth_key_present: true,
      runtime_state: 'pending_gateway_start',
      selectable_exit: Boolean(update.exit_node),
    }))
    const notify = vi.fn()
    render(<TailscaleCard onChanged={vi.fn()} onNotify={notify} />)

    await userEvent.click(await screen.findByRole('button', { name: '开始设置' }))
    const dialog = screen.getByRole('dialog', { name: '配置 Tailscale 出站' })
    const keyLink = within(dialog).getByRole('link', { name: /创建 Auth Key/ })
    expect(keyLink.getAttribute('href')).toBe('https://console.tailscale.com/admin/settings/keys')
    expect(keyLink.getAttribute('target')).toBe('_blank')
    await userEvent.type(within(dialog).getByLabelText('Tailscale Auth Key'), 'tskey-auth-secret')
    await userEvent.click(within(dialog).getByLabelText('允许访问 Pixel'))
    await userEvent.click(within(dialog).getByText('允许所有 MagicDNS 名称'))
    await userEvent.click(within(dialog).getByText('10.20.0.0/16'))
    await userEvent.click(within(dialog).getByRole('button', { name: '指定设备' }))
    await waitFor(() => expect(within(dialog).getByRole('button', { name: '指定设备' }).getAttribute('aria-pressed')).toBe('true'))
    await userEvent.click(await within(dialog).findByText('apple-tv'))
    await userEvent.selectOptions(within(dialog).getByLabelText('Tailscale Exit Node'), '100.90.3.4')
    await userEvent.click(within(dialog).getByText('保存后启用'))
    await userEvent.click(within(dialog).getByRole('button', { name: '保存，随网关启动' }))

    await waitFor(() => expect(api.saveTailscale).toHaveBeenCalledTimes(1))
    expect(api.saveTailscale).toHaveBeenCalledWith('rev-1', expect.objectContaining({
      enabled: true,
      auth_key: 'tskey-auth-secret',
      magic_dns_suffixes: ['example.ts.net'],
      peer_cidrs: ['100.82.10.7/32', 'fd7a:115c:a1e0::7/128'],
      subnet_routes: ['10.20.0.0/16'],
      accept_routes: true,
      allowed_devices: ['apple-tv'],
      exit_node: '100.90.3.4',
    }))
    expect(screen.queryByDisplayValue('tskey-auth-secret')).toBeNull()
    expect(await screen.findByText('Tailnet + Exit Node')).toBeTruthy()
    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ tone: 'success' }))
  })

  it('keeps forget identity unavailable until Tailscale and the gateway are stopped', async () => {
    vi.mocked(api.tailscale).mockResolvedValue({ ...base, settings: { ...base.settings, enabled: true }, identity_present: true, gateway_active: true, runtime_state: 'available_on_demand' })
    render(<TailscaleCard onChanged={vi.fn()} onNotify={vi.fn()} />)
    const forget = await screen.findByRole('button', { name: '忘记本地身份' })
    expect((forget as HTMLButtonElement).disabled).toBe(true)
    expect(forget.getAttribute('title')).toBe('请先停止网关')
  })

  it('reuses cached Tailnet suggestions while the native app is disconnected', async () => {
    vi.mocked(api.tailscaleDiscovery).mockResolvedValue({
      ...discovery,
      cached: true,
      cached_at: '2026-08-30T20:00:00Z',
      backend_state: 'Unavailable',
      error: 'Tailscale is stopped',
    })
    render(<TailscaleCard onChanged={vi.fn()} onNotify={vi.fn()} />)

    await userEvent.click(await screen.findByRole('button', { name: '开始设置' }))
    const dialog = screen.getByRole('dialog', { name: '配置 Tailscale 出站' })
    expect(within(dialog).getByText('正在使用上次发现的 Tailnet')).toBeTruthy()
    expect(within(dialog).getByText('example.ts.net · 已缓存 2 台 peer；在线状态可能已变化')).toBeTruthy()
    expect(within(dialog).getByText('上次在线')).toBeTruthy()
    expect(within(dialog).getByLabelText('Tailscale Exit Node').querySelector('option[value="100.90.3.4"]')).toBeTruthy()
  })

  it('migrates a discovered Exit Node name to its stable Tailscale IPv4', async () => {
    vi.mocked(api.tailscale).mockResolvedValue({
      ...base,
      settings: { ...base.settings, exit_node: 'home-router.example.ts.net' },
      auth_key_present: true,
      selectable_exit: true,
    })
    render(<TailscaleCard onChanged={vi.fn()} onNotify={vi.fn()} />)

    await userEvent.click(await screen.findByRole('button', { name: '开始设置' }))
    const select = within(screen.getByRole('dialog', { name: '配置 Tailscale 出站' })).getByLabelText('Tailscale Exit Node') as HTMLSelectElement
    expect(select.value).toBe('100.90.3.4')
  })

  it('blocks a running-gateway reload when the native Tailscale app owns the selected subnet route', async () => {
    vi.mocked(api.tailscale).mockResolvedValue({
      ...base,
      settings: { ...base.settings, enabled: true, accept_routes: true, subnet_routes: ['10.20.0.0/16'] },
      auth_key_present: true,
      identity_present: true,
      gateway_active: true,
      runtime_state: 'available_on_demand',
    })
    vi.mocked(api.tailscaleDiscovery).mockResolvedValue({
      ...discovery,
      subnet_route_conflicts: [{ route: '10.20.0.0/16', interface: 'utun5', peer_id: 'router', peer_name: 'Home Router' }],
    })
    render(<TailscaleCard onChanged={vi.fn()} onNotify={vi.fn()} />)

    await userEvent.click(await screen.findByRole('button', { name: '配置' }))
    const dialog = screen.getByRole('dialog', { name: '配置 Tailscale 出站' })
    expect(within(dialog).getByText('本机 Tailscale App 正在接管所选子网')).toBeTruthy()
    expect(within(dialog).getByText('10.20.0.0/16 → utun5')).toBeTruthy()
    const apply = within(dialog).getByRole('button', { name: '应用并重载' }) as HTMLButtonElement
    expect(apply.disabled).toBe(true)
    expect(apply.title).toBe('请先解除本机 Tailscale 子网路由冲突')
    expect(api.saveTailscale).not.toHaveBeenCalled()
  })
})
