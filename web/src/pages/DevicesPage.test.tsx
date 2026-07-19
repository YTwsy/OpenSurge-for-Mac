// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { DevicePolicyDocument, DevicesResponse, Overview, PolicySet } from '../types'

vi.mock('../api', () => {
  class RequestError extends Error {
    constructor(public status: number, public code: string, message: string) { super(message) }
  }
  return {
    RequestError,
    waitForOperation: vi.fn(async () => ({ id: 'reload-1', kind: 'reload', state: 'succeeded' })),
    api: {
      devices: vi.fn(), config: vi.fn(), sources: vi.fn(), devicePolicy: vi.fn(), saveDevicePolicy: vi.fn(),
      selectPolicy: vi.fn(), selectDevicePolicy: vi.fn(), gateway: vi.fn(),
      proxyHealth: vi.fn(), testProxyHealth: vi.fn(),
    },
  }
})

import { api, RequestError, waitForOperation } from '../api'
import { DevicesPage } from './DevicesPage'

const basePolicy: PolicySet = {
  devices: [], profiles: [], templates: [], rule_sets: [],
}

const overview = {
  status: { gateway: 'running', interface: 'en0', lan_ip: '192.168.1.20' },
  policies: [
    { name: 'Main', type: 'Selector', selected: 'DIRECT', options: ['DIRECT', 'Proxy-A'] },
    { name: 'device/alice/default', type: 'Selector', selected: 'DIRECT', options: ['DIRECT', 'Proxy-A'] },
  ],
  leases: [],
} as unknown as Overview

function documentFor(policy: PolicySet, revision = 'policy-r1'): DevicePolicyDocument {
  return { schema_version: 1, revision, policy }
}

function devicesResponse(overrides: Partial<DevicesResponse> = {}): DevicesResponse {
  return { drift: false, applied: false, devices: [], desired_devices: [], applied_devices: [], changed_devices: [], leases: [], observed_devices: [], ...overrides }
}

function renderPage(customOverview = overview) {
  const onChanged = vi.fn(async () => {})
  const onNavigate = vi.fn()
  const onDirtyChange = vi.fn()
  render(<DevicesPage overview={customOverview} onChanged={onChanged} onNavigate={onNavigate} onDirtyChange={onDirtyChange} />)
  return { onChanged, onNavigate, onDirtyChange }
}

describe('DevicesPage', () => {
  beforeEach(() => {
    vi.mocked(api.config).mockResolvedValue({ device_policy: { enabled: true } } as never)
    vi.mocked(api.sources).mockResolvedValue({ revision: 'sources-r1', sources: [] })
    vi.mocked(api.devicePolicy).mockResolvedValue(documentFor(basePolicy))
    vi.mocked(api.devices).mockResolvedValue(devicesResponse())
    vi.mocked(api.selectPolicy).mockResolvedValue({} as never)
    vi.mocked(api.selectDevicePolicy).mockResolvedValue({} as never)
    vi.mocked(api.gateway).mockResolvedValue({ id: 'reload-1', kind: 'reload', state: 'running' })
    vi.mocked(api.proxyHealth).mockResolvedValue({ schema_version: 1, test_url: 'https://www.gstatic.com/generate_204', proxies: [
      { name: 'DIRECT', type: 'Direct', selected: '', provider: '', udp: true, status: 'not_applicable', probeable: false },
      { name: 'Proxy-A', type: 'Hysteria2', selected: '', provider: 'demo', udp: true, status: 'reachable', delay_ms: 88, tested_at: '2026-07-15T10:00:00Z', probeable: true },
    ] })
    vi.mocked(api.testProxyHealth).mockResolvedValue({ schema_version: 1, test_url: 'https://www.gstatic.com/generate_204', results: [] })
    vi.mocked(api.saveDevicePolicy).mockImplementation(async (policy, revision) => documentFor(policy, `${revision}-next`))
  })

  afterEach(() => { cleanup(); vi.clearAllMocks() })

  it('shows only global groups on THIS MAC and switches them immediately', async () => {
    renderPage()
    const selector = await screen.findByLabelText('Main 当前出口 DIRECT')
    expect(screen.queryByLabelText('device/alice/default 当前出口 DIRECT')).toBeNull()
    await userEvent.click(selector)
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /Proxy-A/ }))
    await waitFor(() => expect(api.selectPolicy).toHaveBeenCalledWith('Main', 'Proxy-A'))
  })

  it('merges desired and applied devices into four states and separates identity readiness', async () => {
    const policy: PolicySet = {
      ...basePolicy,
      devices: [
        { id: 'ready', mac: 'aa:bb:cc:dd:ee:01', ipv4: '192.168.1.121', profile: 'ready-policy', egress_mode: 'dedicated' },
        { id: 'updated', mac: 'aa:bb:cc:dd:ee:02', ipv4: '192.168.1.122', profile: 'updated-policy', egress_mode: 'dedicated' },
        { id: 'pending', mac: 'aa:bb:cc:dd:ee:03', ipv4: '192.168.1.123', profile: 'pending-policy', egress_mode: 'dedicated' },
      ],
      profiles: ['ready', 'updated', 'pending'].map(id => ({ id: `${id}-policy`, default_policies: ['DIRECT'], rules: [] })),
    }
    vi.mocked(api.devicePolicy).mockResolvedValue(documentFor(policy))
    vi.mocked(api.devices).mockResolvedValue(devicesResponse({
      applied: true,
      applied_devices: [
        { id: 'ready', mac: 'aa:bb:cc:dd:ee:01', ipv4: '192.168.1.121', profile: 'ready-policy', egress_mode: 'dedicated', groups: { default: 'device/ready/default' } },
        { id: 'updated', mac: 'aa:bb:cc:dd:ee:02', ipv4: '192.168.1.122', profile: 'updated-policy', egress_mode: 'dedicated', groups: { default: 'device/updated/default' } },
        { id: 'removing', mac: 'aa:bb:cc:dd:ee:04', ipv4: '192.168.1.124', profile: 'old-policy', egress_mode: 'dedicated', groups: { default: 'device/removing/default' } },
      ],
      changed_devices: ['updated'],
      leases: [{ ip: '192.168.1.121', mac: 'aa:bb:cc:dd:ee:01', hostname: 'Ready', expires_at: '2099-01-01T00:00:00Z', online: true }],
    }))
    renderPage()
    await screen.findByText('ready')
    expect(screen.getByText('已应用')).toBeTruthy()
    expect(screen.getByText('待更新')).toBeTruthy()
    expect(screen.getByText('待应用')).toBeTruthy()
    expect(screen.getByText('待移除')).toBeTruthy()
    expect(screen.getByText('DHCP 身份已验证')).toBeTruthy()
    expect(screen.getAllByText(/身份待确认/).length).toBe(2)
    expect(screen.getByLabelText('ready 独立出口 当前摘要')).toBeTruthy()
    expect(screen.getByText('重载后应用')).toBeTruthy()
  })

  it('keeps the default outlet primary, exposes rule outlets explicitly, and reports switching progress', async () => {
    const policy: PolicySet = {
      ...basePolicy,
      devices: [{ id: 'alice', mac: 'aa:bb:cc:dd:ee:01', ipv4: '192.168.1.121', profile: 'alice-policy', egress_mode: 'dedicated' }],
      profiles: [{ id: 'alice-policy', default_policies: ['DIRECT', 'Proxy-A'], rules: [{ id: 'video', match: { domains: ['video.example'] }, policies: ['DIRECT', 'Proxy-A'] }] }],
    }
    const deviceOverview = {
      ...overview,
      policies: [
        ...overview.policies,
        { name: 'device/alice/rule/video', type: 'Selector', selected: 'DIRECT', options: ['DIRECT', 'Proxy-A'] },
      ],
    } as unknown as Overview
    vi.mocked(api.devicePolicy).mockResolvedValue(documentFor(policy))
    vi.mocked(api.devices).mockResolvedValue(devicesResponse({
      applied: true,
      desired_devices: [{ id: 'alice', mac: 'aa:bb:cc:dd:ee:01', ipv4: '192.168.1.121', profile: 'alice-policy', egress_mode: 'dedicated', groups: { default: 'device/alice/default', 'rule/video': 'device/alice/rule/video' } }],
      applied_devices: [{ id: 'alice', mac: 'aa:bb:cc:dd:ee:01', ipv4: '192.168.1.121', profile: 'alice-policy', egress_mode: 'dedicated', groups: { default: 'device/alice/default', 'rule/video': 'device/alice/rule/video' } }],
    }))
    let finishSwitch!: () => void
    vi.mocked(api.selectDevicePolicy).mockReturnValueOnce(new Promise(resolve => { finishSwitch = () => resolve({} as never) }))
    renderPage(deviceOverview)
    const defaultOutlet = await screen.findByLabelText('alice 独立出口 当前摘要')
    const ruleToggle = screen.getByRole('button', { name: /规则出口（1）/ })
    expect(ruleToggle.getAttribute('aria-expanded')).toBe('false')
    expect(screen.queryByLabelText('alice rule/video 出口当前摘要')).toBeNull()
    await userEvent.click(ruleToggle)
    expect(ruleToggle.getAttribute('aria-expanded')).toBe('true')
    expect(screen.getByLabelText('alice rule/video 出口当前摘要')).toBeTruthy()
    await userEvent.click(defaultOutlet)
    const proxyOption = within(screen.getByRole('dialog')).getByRole('button', { name: /Proxy-A/ })
    await userEvent.click(proxyOption)
    expect(await screen.findByText('正在切换…')).toBeTruthy()
    expect((proxyOption as HTMLButtonElement).disabled).toBe(true)
    finishSwitch()
    await waitFor(() => expect(api.selectDevicePolicy).toHaveBeenCalledWith('alice', 'default', 'Proxy-A'))
  })

  it('separates an applied inherited route from a draft dedicated route', async () => {
    const policy: PolicySet = {
      ...basePolicy,
      devices: [{ id: 'alice', mac: 'aa:bb:cc:dd:ee:01', ipv4: '192.168.1.121', profile: 'alice-policy', egress_mode: 'inherit_global' }],
      profiles: [{ id: 'alice-policy', default_policies: ['DIRECT', 'Proxy-A'], rules: [] }],
    }
    vi.mocked(api.devicePolicy).mockResolvedValue(documentFor(policy))
    vi.mocked(api.devices).mockResolvedValue(devicesResponse({
      applied: true,
      applied_devices: [{ id: 'alice', mac: 'aa:bb:cc:dd:ee:01', ipv4: '192.168.1.121', profile: 'alice-policy', egress_mode: 'inherit_global', groups: {} }],
    }))
    renderPage()
    expect(await screen.findByText('默认出口跟随本机 / 全局规则')).toBeTruthy()
    expect(screen.queryByLabelText('alice 独立出口 当前摘要')).toBeNull()
    await userEvent.click(screen.getByRole('radio', { name: /独立设备出口/ }))
    expect(screen.getByText(/草稿将改为“独立设备出口”/)).toBeTruthy()
    expect(screen.getByLabelText('独立出口候选')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: '保存设备配置' }))
    await waitFor(() => expect(api.saveDevicePolicy).toHaveBeenCalled())
    expect(vi.mocked(api.saveDevicePolicy).mock.calls[0][0].devices[0].egress_mode).toBe('dedicated')
  })

  it('keeps legacy routing readable and requires an explicit migration choice', async () => {
    const policy: PolicySet = {
      ...basePolicy,
      devices: [{ id: 'alice', mac: 'aa:bb:cc:dd:ee:01', ipv4: '192.168.1.121', profile: 'alice-policy' }],
      profiles: [{ id: 'alice-policy', default_policies: ['DIRECT', 'Proxy-A'], rules: [] }],
    }
    vi.mocked(api.devicePolicy).mockResolvedValue(documentFor(policy))
    vi.mocked(api.devices).mockResolvedValue(devicesResponse({
      applied: true,
      applied_devices: [{ id: 'alice', mac: 'aa:bb:cc:dd:ee:01', ipv4: '192.168.1.121', profile: 'alice-policy', egress_mode: '', groups: { default: 'device/alice/default' } }],
    }))
    renderPage()
    expect(await screen.findByText('需要选择新的路由方式')).toBeTruthy()
    expect(screen.getByLabelText('alice 兼容兜底出口 当前摘要')).toBeTruthy()
    expect((screen.getByRole('radio', { name: /跟随本机/ }) as HTMLInputElement).checked).toBe(false)
    expect((screen.getByRole('radio', { name: /独立设备出口/ }) as HTMLInputElement).checked).toBe(false)
    await userEvent.click(screen.getByRole('radio', { name: /跟随本机/ }))
    await userEvent.click(screen.getByRole('button', { name: '保存设备配置' }))
    await waitFor(() => expect(api.saveDevicePolicy).toHaveBeenCalled())
    expect(vi.mocked(api.saveDevicePolicy).mock.calls[0][0].devices[0].egress_mode).toBe('inherit_global')
  })

  it('defaults newly registered devices to following global rules and reveals candidates only for dedicated routing', async () => {
    renderPage()
    const follow = await screen.findByRole('radio', { name: /跟随本机/ })
    expect((follow as HTMLInputElement).checked).toBe(true)
    expect(screen.queryByLabelText('独立出口候选')).toBeNull()
    await userEvent.click(screen.getByRole('radio', { name: /独立设备出口/ }))
    expect(screen.getByLabelText('独立出口候选')).toBeTruthy()
    await userEvent.type(screen.getByLabelText('设备名称'), 'Pixel Living Room')
    await userEvent.type(screen.getByLabelText('设备 MAC'), 'aa:bb:cc:dd:ee:ff')
    await userEvent.type(screen.getByLabelText('固定 IPv4'), '192.168.1.137')
    await userEvent.click(screen.getByRole('button', { name: '登记或更新设备' }))
    await userEvent.click(screen.getByRole('button', { name: '保存设备配置' }))
    await waitFor(() => expect(api.saveDevicePolicy).toHaveBeenCalled())
    expect(vi.mocked(api.saveDevicePolicy).mock.calls[0][0].devices[0]).toEqual(expect.objectContaining({ id: 'pixel-living-room', name: 'Pixel Living Room', egress_mode: 'dedicated' }))
  })

  it('lists a same-LAN source currently passing through Mac and prefills its observed identity', async () => {
    vi.mocked(api.devices).mockResolvedValue(devicesResponse({
      observed_devices: [
        { ip: '192.168.1.137', mac: 'aa:bb:cc:dd:ee:37', active_connections: 3, neighbor_observed: true },
        { ip: '192.168.1.138', active_connections: 1, neighbor_observed: false },
      ],
    }))
    renderPage({ ...overview, topology: 'same_lan' } as unknown as Overview)

    expect(await screen.findByText('当前经过 Mac 的设备')).toBeTruthy()
    expect(screen.getByText('未登记设备 192.168.1.137')).toBeTruthy()
    expect(screen.getByText(/3 个活跃连接/)).toBeTruthy()
    expect(screen.getByText(/MAC 尚未从邻居表解析/)).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: '刷新当前设备' }))
    await waitFor(() => expect(api.devices).toHaveBeenCalledTimes(2))
    expect(api.devicePolicy).toHaveBeenCalledTimes(1)
    await userEvent.click(screen.getByRole('button', { name: '配置设备 192.168.1.137' }))
    expect((screen.getByLabelText('设备 MAC') as HTMLInputElement).value).toBe('aa:bb:cc:dd:ee:37')
    expect((screen.getByLabelText('固定 IPv4') as HTMLInputElement).value).toBe('192.168.1.137')
  })

  it('shows observation evidence instead of requiring a DHCP lease in same-LAN mode', async () => {
    const policy: PolicySet = {
      ...basePolicy,
      devices: [{ id: 'pixel', name: 'Pixel', mac: 'aa:bb:cc:dd:ee:37', ipv4: '192.168.1.137', profile: 'pixel-policy', egress_mode: 'inherit_global' }],
      profiles: [{ id: 'pixel-policy', default_policies: ['DIRECT'], rules: [] }],
    }
    vi.mocked(api.devicePolicy).mockResolvedValue(documentFor(policy))
    vi.mocked(api.devices).mockResolvedValue(devicesResponse({
      applied: true,
      applied_devices: [{ id: 'pixel', mac: 'aa:bb:cc:dd:ee:37', ipv4: '192.168.1.137', profile: 'pixel-policy', egress_mode: 'inherit_global', groups: {} }],
      observed_devices: [{ ip: '192.168.1.137', mac: 'aa:bb:cc:dd:ee:37', active_connections: 1, neighbor_observed: true }],
    }))
    renderPage({ ...overview, topology: 'same_lan' } as unknown as Overview)

    expect(await screen.findByText('流量与邻居已观察：MAC / IPv4 匹配')).toBeTruthy()
    expect(screen.queryByText(/需要在线且未过期的精确/)).toBeNull()
  })

  it('privatizes a shared template profile before adding a validated flat rule', async () => {
    const policy: PolicySet = {
      devices: [
        { id: 'alice', mac: 'aa:bb:cc:dd:ee:01', ipv4: '192.168.1.121', profile: 'shared', egress_mode: 'dedicated' },
        { id: 'bob', mac: 'aa:bb:cc:dd:ee:02', ipv4: '192.168.1.122', profile: 'shared', egress_mode: 'dedicated' },
      ],
      profiles: [{ id: 'shared', template: 'base', default_policies: [], rules: [{ id: 'existing', match: { ports: ['443'] }, action: 'DIRECT' }] }],
      templates: [{ id: 'base', default_policies: ['DIRECT'], rules: [{ id: 'template-rule', match: { protocols: ['udp'] }, action: 'REJECT' }] }],
      rule_sets: [],
    }
    vi.mocked(api.devicePolicy).mockResolvedValue(documentFor(policy))
    renderPage()
    await screen.findByRole('heading', { name: 'alice 的规则' })
    const searchableOutlet = screen.getByLabelText('独立出口候选')
    expect(searchableOutlet.getAttribute('type')).toBe('search')
    await userEvent.type(searchableOutlet, 'REJECT{Enter}')
    expect(screen.getByRole('button', { name: '移除 REJECT' })).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: '＋ 添加设备规则' }))
    await userEvent.click(screen.getByRole('button', { name: '保存到草稿' }))
    expect(screen.getByRole('alert').textContent).toContain('至少添加一个匹配条件')
    await userEvent.type(screen.getByLabelText('域名后缀'), 'youtube.example')
    await userEvent.click(within(screen.getByLabelText('域名后缀').parentElement!).getByRole('button', { name: '添加' }))
    await userEvent.click(screen.getByRole('button', { name: '保存到草稿' }))
    await userEvent.click(screen.getByRole('button', { name: '保存设备配置' }))
    await waitFor(() => expect(api.saveDevicePolicy).toHaveBeenCalled())
    const saved = vi.mocked(api.saveDevicePolicy).mock.calls[0][0]
    expect(saved.devices.find(device => device.id === 'alice')?.profile).toBe('alice-policy')
    expect(saved.devices.find(device => device.id === 'bob')?.profile).toBe('shared')
    expect(saved.profiles.find(profile => profile.id === 'shared')?.template).toBe('base')
    expect(saved.profiles.find(profile => profile.id === 'alice-policy')).toEqual(expect.objectContaining({
      template: undefined,
      default_policies: ['DIRECT', 'REJECT'],
      rules: expect.arrayContaining([expect.objectContaining({ id: 'template-rule' }), expect.objectContaining({ id: 'existing' }), expect.objectContaining({ match: { domains: ['youtube.example'] }, action: 'REJECT' })]),
    }))
  })

  it('supports rule reorder, deletion, and mutually exclusive selector output', async () => {
    const policy: PolicySet = {
      ...basePolicy,
      devices: [{ id: 'alice', mac: 'aa:bb:cc:dd:ee:01', ipv4: '192.168.1.121', profile: 'alice-policy', egress_mode: 'inherit_global' }],
      profiles: [{ id: 'alice-policy', default_policies: ['DIRECT'], rules: [
        { id: 'first', match: { domains: ['first.example'] }, action: 'DIRECT' },
        { id: 'second', match: { domains: ['second.example'] }, action: 'REJECT' },
      ] }],
    }
    vi.mocked(api.devicePolicy).mockResolvedValue(documentFor(policy))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    renderPage()
    await screen.findByText('域名 first.example')
    await userEvent.click(screen.getByRole('button', { name: '上移规则 second' }))
    expect(document.querySelectorAll('.flat-rule')[0].textContent).toContain('second.example')
    await userEvent.click(within(document.querySelectorAll('.flat-rule')[1] as HTMLElement).getByRole('button', { name: '删除' }))
    expect(screen.queryByText('域名 first.example')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: '＋ 添加设备规则' }))
    await userEvent.type(screen.getByLabelText('域名后缀'), 'selector.example{Enter}')
    await userEvent.click(screen.getByLabelText('可即时切换的 Selector'))
    await userEvent.type(screen.getByLabelText('Selector 候选'), 'Main{Enter}')
    await userEvent.click(screen.getByRole('button', { name: '保存到草稿' }))
    await userEvent.click(screen.getByRole('button', { name: '保存设备配置' }))
    await waitFor(() => expect(api.saveDevicePolicy).toHaveBeenCalled())
    const saved = vi.mocked(api.saveDevicePolicy).mock.calls[0][0]
    const added = saved.profiles.find(profile => profile.id === 'alice-policy')?.rules?.find(rule => rule.match.domains?.includes('selector.example'))
    expect(added?.policies).toEqual(['DIRECT', 'Main'])
    expect(added?.action).toBeUndefined()
  })

  it('keeps advanced reuse tools collapsed and disables referenced-object deletion', async () => {
    const policy: PolicySet = {
      devices: [{ id: 'alice', mac: 'aa:bb:cc:dd:ee:01', ipv4: '192.168.1.121', profile: 'home', egress_mode: 'inherit_global' }],
      profiles: [{ id: 'home', template: 'base', default_policies: [], rules: [{ id: 'managed', match: { rule_sets: ['streaming'] }, action: 'DIRECT' }] }],
      templates: [{ id: 'base', default_policies: ['DIRECT'], rules: [] }],
      rule_sets: [{ id: 'streaming', type: 'inline', behavior: 'domain', payload: ['youtube.example'] }],
    }
    vi.mocked(api.devicePolicy).mockResolvedValue(documentFor(policy))
    renderPage()
    const toggle = await screen.findByRole('button', { name: /高级 \/ 复用机制/ })
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    await userEvent.click(toggle)
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    const advanced = toggle.closest('.advanced-policy') as HTMLElement
    for (const label of ['home', 'template: base', 'rule-set: streaming']) {
      const row = within(advanced).getByText(label).closest('.editor-item') as HTMLElement
      expect((within(row).getByRole('button', { name: '移除' }) as HTMLButtonElement).disabled).toBe(true)
    }
  })

  it('uses a custom interruption warning before reload and waits for the operation', async () => {
    vi.mocked(api.devices).mockResolvedValue(devicesResponse({ drift: true, applied: true, desired_digest: 'desired123', applied_digest: 'applied123' }))
    renderPage()
    await userEvent.click(await screen.findByRole('button', { name: '应用并重载网关' }))
    const dialog = screen.getByRole('dialog', { name: '应用设备配置并重载网关？' })
    expect(dialog.textContent).toContain('DHCP/DNS、mihomo、PF 与 IPv4 forwarding')
    expect(dialog.textContent).toContain('当前连接会中断')
    await userEvent.click(within(dialog).getByRole('button', { name: '确认应用并重载' }))
    await waitFor(() => expect(api.gateway).toHaveBeenCalledWith('reload'))
    expect(waitForOperation).toHaveBeenCalledWith('reload-1')
    expect(await screen.findByText(/网关已使用最新设备配置重新启动/)).toBeTruthy()
  })

  it('keeps drift retryable and shows a readable error when reload fails', async () => {
    vi.mocked(api.devices).mockResolvedValue(devicesResponse({ drift: true, applied: true, desired_digest: 'desired123', applied_digest: 'applied123' }))
    vi.mocked(waitForOperation).mockRejectedValueOnce(new Error('候选配置校验失败；现有网关仍在运行'))
    renderPage()
    await userEvent.click(await screen.findByRole('button', { name: '应用并重载网关' }))
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: '确认应用并重载' }))
    expect((await screen.findByRole('alert')).textContent).toContain('候选配置校验失败；现有网关仍在运行')
    expect((screen.getByRole('button', { name: '应用并重载网关' }) as HTMLButtonElement).disabled).toBe(false)
  })

  it('keeps the local form on revision conflict and offers an explicit reload choice', async () => {
    const policy: PolicySet = { ...basePolicy, profiles: [{ id: 'home', default_policies: ['DIRECT'], rules: [] }] }
    vi.mocked(api.devicePolicy).mockResolvedValue(documentFor(policy))
    vi.mocked(api.saveDevicePolicy).mockRejectedValue(new RequestError(409, 'revision_conflict', 'conflict'))
    renderPage()
    await userEvent.click(await screen.findByRole('button', { name: /高级 \/ 复用机制/ }))
    await screen.findByText('home')
    await userEvent.type(screen.getByLabelText('Template ID'), 'new-template')
    await userEvent.click(screen.getByRole('button', { name: '添加模板' }))
    await userEvent.click(screen.getByRole('button', { name: '保存设备配置' }))
    expect(await screen.findByText(/配置已被其他操作更新/)).toBeTruthy()
    expect(screen.getByText('template: new-template')).toBeTruthy()
    expect(screen.getByRole('button', { name: '放弃本地修改并加载最新版本' })).toBeTruthy()
  })
})
