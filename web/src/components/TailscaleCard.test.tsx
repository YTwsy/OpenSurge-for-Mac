// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { TailscaleResponse } from '../types'

vi.mock('../api', () => ({
  api: {
    tailscale: vi.fn(),
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

beforeEach(() => {
  vi.mocked(api.tailscale).mockResolvedValue(base)
  vi.mocked(api.devices).mockResolvedValue({ desired_digest: '', applied_digest: '', drift: false, applied: false, devices: [], desired_devices: [{ id: 'apple-tv', mac: 'aa:bb:cc:dd:ee:01', ipv4: '192.168.50.110', profile: 'home', groups: {} }], applied_devices: [], changed_devices: [], out_of_lan_devices: [], leases: [], observed_devices: [] })
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('TailscaleCard', () => {
  it('collects a write-only key, explicit targets, device scope, and exit node', async () => {
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
    await userEvent.type(within(dialog).getByLabelText('Tailscale Auth Key'), 'tskey-auth-secret')
    await userEvent.type(within(dialog).getByLabelText('MagicDNS 后缀'), 'home-name.ts.net')
    await userEvent.click(within(dialog).getAllByRole('button', { name: '添加' })[0])
    await userEvent.type(within(dialog).getByLabelText('Tailnet 节点 IP / CIDR'), '100.82.10.7')
    await userEvent.click(within(dialog).getAllByRole('button', { name: '添加' })[1])
    await userEvent.click(within(dialog).getByRole('button', { name: '指定设备' }))
    await waitFor(() => expect(within(dialog).getByRole('button', { name: '指定设备' }).getAttribute('aria-pressed')).toBe('true'))
    await userEvent.click(await within(dialog).findByText('apple-tv'))
    await userEvent.type(within(dialog).getByLabelText('Tailscale Exit Node'), '100.90.3.4')
    await userEvent.click(within(dialog).getByText('保存后启用'))
    await userEvent.click(within(dialog).getByRole('button', { name: '保存 Tailscale 配置' }))

    await waitFor(() => expect(api.saveTailscale).toHaveBeenCalledTimes(1))
    expect(api.saveTailscale).toHaveBeenCalledWith('rev-1', expect.objectContaining({
      enabled: true,
      auth_key: 'tskey-auth-secret',
      magic_dns_suffixes: ['home-name.ts.net'],
      peer_cidrs: ['100.82.10.7'],
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
})
