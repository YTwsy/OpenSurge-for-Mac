// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useRef, useState } from 'react'
import type { LocalRouting, Overview, PolicyWorkspaceSnapshot, ProxyHealthSnapshot } from '../types'
import type { ConnectionRefreshSuggestion } from '../components/ConnectionRefreshPrompts'

vi.mock('../api', () => ({
  api: {
    proxyHealth: vi.fn(),
    testProxyHealth: vi.fn(),
    selectPolicy: vi.fn(),
    policyWorkspace: vi.fn(),
    localRouting: vi.fn(),
    setLocalRouting: vi.fn(),
  },
}))

import { api } from '../api'
import { PoliciesPage, type PoliciesViewState } from './PoliciesPage'
import { activateLanguage, prepareLanguage } from '../i18n'

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

let workspace: PolicyWorkspaceSnapshot

function PoliciesPageHarness({ data = overview, onChanged = async () => {}, onSuggestConnectionRefresh }: { data?: Overview | null; onChanged?: () => Promise<void>; onSuggestConnectionRefresh?: (suggestion: ConnectionRefreshSuggestion) => void }) {
  const [viewState, setViewState] = useState<PoliciesViewState>({ search: '', scope: 'global', activeGroup: null })
  return <PoliciesPage overview={data} onChanged={onChanged} onSuggestConnectionRefresh={onSuggestConnectionRefresh} viewState={viewState} onViewStateChange={patch => setViewState(current => ({ ...current, ...patch }))} restoreScrollY={null} onScrollPositionChange={() => {}} />
}

function PoliciesSessionHarness() {
  const [visible, setVisible] = useState(true)
  const [viewState, setViewState] = useState<PoliciesViewState>({ search: '', scope: 'global', activeGroup: null })
  const scrollPosition = useRef<number | null>(null)
  const data = {
    ...overview,
    policies: [
      ...overview.policies,
      { name: 'Streaming', type: 'Selector', selected: 'Proxy-A', options: ['Proxy-A', 'Proxy-B'] },
    ],
  } as Overview
  return <>
    <button type="button" onClick={() => setVisible(current => !current)}>{visible ? '离开策略页' : '返回策略页'}</button>
    {visible ? <PoliciesPage overview={data} onChanged={async () => {}} viewState={viewState} onViewStateChange={patch => setViewState(current => ({ ...current, ...patch }))} restoreScrollY={scrollPosition.current} onScrollPositionChange={scrollY => { scrollPosition.current = scrollY }} /> : null}
  </>
}

function renderSessionPage() {
  workspace.groups.push({ name: 'Streaming', type: 'Selector', selected: 'Proxy-A', options: ['Proxy-A', 'Proxy-B'] })
  return render(<PoliciesSessionHarness />)
}

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
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: vi.fn() })
    Object.defineProperty(window, 'scrollTo', { configurable: true, value: vi.fn() })
    vi.mocked(api.proxyHealth).mockResolvedValue(health)
    vi.mocked(api.testProxyHealth).mockResolvedValue({ schema_version: 1, test_url: health.test_url, results: [] })
    vi.mocked(api.selectPolicy).mockResolvedValue({} as never)
    workspace = { schema_version: 1, mode: 'running', revision: 'workspace-1', groups: structuredClone(overview.policies), health: structuredClone(health) }
    vi.mocked(api.policyWorkspace).mockImplementation(async request => {
      if (request.action === 'select') {
        workspace = { ...workspace, groups: workspace.groups.map(group => group.name === request.group ? { ...group, selected: request.policy } : group) }
      }
      return structuredClone(workspace)
    })
    vi.mocked(api.localRouting).mockResolvedValue(localRouting())
    vi.mocked(api.setLocalRouting).mockImplementation(async (mode, policy) => localRouting(mode, policy ?? 'Proxy-A'))
  })

  afterEach(() => { cleanup(); vi.clearAllMocks(); vi.restoreAllMocks(); vi.useRealTimers(); activateLanguage('zh-Hans') })

  it('shows global node health, filters device groups, and switches selector nodes', async () => {
    const onChanged = vi.fn(async () => {})
    render(<PoliciesPageHarness onChanged={onChanged} />)

    expect(await screen.findByRole('heading', { name: 'Main' })).toBeTruthy()
    expect(screen.queryByRole('heading', { name: 'device/alice/default' })).toBeNull()
    expect(screen.getAllByText('86 ms').length).toBeGreaterThan(0)
    expect(screen.getByText('超时')).toBeTruthy()

    await userEvent.click(screen.getByRole('button', { name: 'Main 选择 Proxy-B' }))
    await waitFor(() => expect(api.policyWorkspace).toHaveBeenCalledWith({ action: 'select', group: 'Main', policy: 'Proxy-B' }))
    expect(screen.getByRole('button', { name: 'Main 选择 Proxy-B' }).getAttribute('aria-pressed')).toBe('true')
    expect(api.selectPolicy).not.toHaveBeenCalled()
    expect(onChanged).toHaveBeenCalledOnce()

    await userEvent.click(screen.getByRole('button', { name: '设备策略' }))
    expect(screen.getByRole('heading', { name: 'device/alice/default' })).toBeTruthy()
    expect(screen.queryByRole('heading', { name: 'Main' })).toBeNull()
  })

  it('tests the probeable nodes in the current view', async () => {
    render(<PoliciesPageHarness />)
    await screen.findAllByText('86 ms')
    await userEvent.click(screen.getByRole('button', { name: '检测当前视图' }))
    await waitFor(() => expect(api.policyWorkspace).toHaveBeenCalledWith({ action: 'test', names: ['Proxy-A', 'Proxy-B'] }))
    expect(api.testProxyHealth).not.toHaveBeenCalled()
  })

  it('prepares and selects nodes before the gateway starts without using overview policies', async () => {
    workspace.mode = 'prepared'
    workspace.groups.push({ name: 'Extension', type: 'Selector', selected: 'Proxy-A', options: ['Proxy-A', 'Proxy-B'] })
    const stopped = { ...overview, policies: [], status: { ...overview.status, gateway: 'stopped', mihomo: 'stopped' } }
    const onSuggestConnectionRefresh = vi.fn()
    render(<PoliciesPageHarness data={stopped} onSuggestConnectionRefresh={onSuggestConnectionRefresh} />)

    expect(await screen.findByText('待启动配置')).toBeTruthy()
    expect(screen.getByRole('heading', { name: 'Extension' })).toBeTruthy()
    expect(api.policyWorkspace).toHaveBeenCalledWith({ action: 'read' })
    expect(api.localRouting).not.toHaveBeenCalled()
    expect(api.proxyHealth).not.toHaveBeenCalled()
    expect(screen.getByText('启动网关后可用')).toBeTruthy()

    await userEvent.click(screen.getByRole('button', { name: 'Extension 选择 Proxy-B' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Extension 选择 Proxy-B' }).getAttribute('aria-pressed')).toBe('true'))
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(onSuggestConnectionRefresh).not.toHaveBeenCalled()
  })

  it('tests nodes in prepared mode and adopts the returned health snapshot', async () => {
    workspace.mode = 'prepared'
    render(<PoliciesPageHarness data={null} />)
    await screen.findByText('待启动配置')
    workspace.health.proxies[0] = { ...workspace.health.proxies[0], delay_ms: 42 }
    await userEvent.click(screen.getByRole('button', { name: '检测当前视图' }))

    expect(await screen.findByText('42 ms')).toBeTruthy()
    expect(api.policyWorkspace).toHaveBeenCalledWith({ action: 'test', names: ['Proxy-A', 'Proxy-B'] })
  })

  it('renders an empty current configuration without a stopped-core error or prompt', async () => {
    workspace = { ...workspace, mode: 'prepared', groups: [], health: { schema_version: 1, test_url: '', proxies: [] } }
    render(<PoliciesPageHarness data={null} />)

    expect(await screen.findByText('当前配置还没有策略组；可以导入 mihomo YAML，也可以只在“全局附加配置”中添加节点与策略组。')).toBeTruthy()
    expect((screen.getByRole('button', { name: '检测当前视图' }) as HTMLButtonElement).disabled).toBe(true)
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(screen.queryByText('mihomo 未运行或没有可选择的策略组')).toBeNull()
  })

  it('clears the previous prepared graph when the new candidate fails and recovers on retry', async () => {
    workspace.mode = 'prepared'
    const page = render(<PoliciesPageHarness data={null} />)
    await screen.findByRole('heading', { name: 'Main' })
    vi.mocked(api.policyWorkspace).mockRejectedValueOnce(new Error('overlay target Missing does not exist'))
    page.rerender(<PoliciesPageHarness data={{ ...overview, revision: 'changed-draft' }} />)
    expect(await screen.findByRole('alert')).toHaveProperty('textContent', expect.stringContaining('overlay target Missing does not exist'))
    expect(screen.queryByRole('heading', { name: 'Main' })).toBeNull()
    expect(screen.queryByText('待启动配置')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByRole('heading', { name: 'Main' })).toBeTruthy()
    expect(screen.getByText('待启动配置')).toBeTruthy()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('accepts null groups and health from an empty backend snapshot', async () => {
    vi.mocked(api.policyWorkspace).mockResolvedValue({ ...workspace, mode: 'prepared', groups: null, health: null } as unknown as PolicyWorkspaceSnapshot)
    render(<PoliciesPageHarness data={null} />)

    expect(await screen.findByText('当前配置还没有策略组；可以导入 mihomo YAML，也可以只在“全局附加配置”中添加节点与策略组。')).toBeTruthy()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('accepts null candidate and health arrays without inventing nodes', async () => {
    vi.mocked(api.policyWorkspace).mockResolvedValue({ ...workspace, groups: [{ ...workspace.groups[0], options: null }], health: { ...health, proxies: null } } as unknown as PolicyWorkspaceSnapshot)
    render(<PoliciesPageHarness data={null} />)

    expect(await screen.findByRole('heading', { name: 'Main' })).toBeTruthy()
    expect(screen.getByText('这个策略组中没有匹配的节点')).toBeTruthy()
    expect((screen.getByRole('button', { name: '检测当前视图' }) as HTMLButtonElement).disabled).toBe(true)
  })

  it('shows a preparation failure and retries only the read action', async () => {
    vi.mocked(api.policyWorkspace).mockRejectedValueOnce(new Error('Configuration rejected'))
    render(<PoliciesPageHarness data={null} />)

    expect(await screen.findByRole('alert')).toHaveProperty('textContent', expect.stringContaining('Configuration rejected'))
    expect(screen.queryByText('当前配置还没有策略组；可以导入 mihomo YAML，也可以只在“全局附加配置”中添加节点与策略组。')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByRole('heading', { name: 'Main' })).toBeTruthy()
    expect(api.policyWorkspace).toHaveBeenNthCalledWith(2, { action: 'read' })
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('discards stale policy data after a failed selection and preserves the engine choice on retry', async () => {
    render(<PoliciesPageHarness data={null} />)
    await screen.findByRole('heading', { name: 'Main' })
    vi.mocked(api.policyWorkspace).mockRejectedValueOnce(new Error('Selection rejected'))
    await userEvent.click(screen.getByRole('button', { name: 'Main 选择 Proxy-B' }))

    expect(await screen.findByRole('alert')).toHaveProperty('textContent', expect.stringContaining('Selection rejected'))
    expect(screen.queryByRole('heading', { name: 'Main' })).toBeNull()
    expect(api.policyWorkspace).toHaveBeenCalledTimes(2)
    await userEvent.click(screen.getByRole('button', { name: '重试' }))
    expect((await screen.findByRole('button', { name: 'Main 选择 Proxy-A' })).getAttribute('aria-pressed')).toBe('true')
    expect(api.policyWorkspace).toHaveBeenLastCalledWith({ action: 'read' })
  })

  it('shows delay-test errors without replaying the test', async () => {
    render(<PoliciesPageHarness data={null} />)
    await screen.findByRole('heading', { name: 'Main' })
    vi.mocked(api.policyWorkspace).mockRejectedValueOnce(new Error('Test unavailable'))
    await userEvent.click(screen.getByRole('button', { name: '检测当前视图' }))

    expect(await screen.findByRole('alert')).toHaveProperty('textContent', expect.stringContaining('Test unavailable'))
    expect((screen.getByRole('button', { name: '检测当前视图' }) as HTMLButtonElement).disabled).toBe(true)
    expect(api.policyWorkspace).toHaveBeenCalledTimes(2)
  })

  it('refreshes on overview revisions and ignores a late read after a selection', async () => {
    const page = render(<PoliciesPageHarness />)
    await screen.findByRole('heading', { name: 'Main' })
    const oldSnapshot = structuredClone(workspace)
    let finishRead!: (snapshot: PolicyWorkspaceSnapshot) => void
    vi.mocked(api.policyWorkspace).mockImplementationOnce(() => new Promise(resolve => { finishRead = resolve }))
    page.rerender(<PoliciesPageHarness data={{ ...overview, revision: 'updated' }} />)
    await waitFor(() => expect(api.policyWorkspace).toHaveBeenCalledTimes(2))

    await userEvent.click(screen.getByRole('button', { name: 'Main 选择 Proxy-B' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Main 选择 Proxy-B' }).getAttribute('aria-pressed')).toBe('true'))
    await act(async () => { finishRead(oldSnapshot) })
    expect(screen.getByRole('button', { name: 'Main 选择 Proxy-B' }).getAttribute('aria-pressed')).toBe('true')
  })

  it('does not let a late failure overwrite a newer successful workspace read', async () => {
    let failFirst!: (cause: Error) => void
    vi.mocked(api.policyWorkspace).mockImplementationOnce(() => new Promise((_, reject) => { failFirst = reject }))
    const page = render(<PoliciesPageHarness />)
    expect(screen.getByRole('status').textContent).toBe('正在准备策略配置…')
    page.rerender(<PoliciesPageHarness data={{ ...overview, revision: 'new' }} />)
    await screen.findByRole('heading', { name: 'Main' })
    await act(async () => { failFirst(new Error('Stale read failed')) })

    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.getByText('运行中配置')).toBeTruthy()
  })

  it('polls delayed providers while the page is visible, skips overlapping reads, and stops on unmount', async () => {
    vi.useFakeTimers()
    const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    workspace.mode = 'prepared'
    const page = render(<PoliciesPageHarness data={null} />)
    await act(async () => {})
    expect(api.policyWorkspace).toHaveBeenCalledTimes(1)
    let finishRead!: (snapshot: PolicyWorkspaceSnapshot) => void
    vi.mocked(api.policyWorkspace).mockImplementationOnce(() => new Promise(resolve => { finishRead = resolve }))
    await act(async () => { vi.advanceTimersByTime(5000) })
    expect(api.policyWorkspace).toHaveBeenCalledTimes(2)
    await act(async () => { vi.advanceTimersByTime(10000) })
    expect(api.policyWorkspace).toHaveBeenCalledTimes(2)

    workspace.groups.push({ name: 'Loaded provider', type: 'Selector', selected: 'Proxy-A', options: ['Proxy-A'] })
    await act(async () => { finishRead(structuredClone(workspace)) })
    expect(screen.getByRole('heading', { name: 'Loaded provider' })).toBeTruthy()
    hidden.mockReturnValue(true)
    await act(async () => { vi.advanceTimersByTime(5000) })
    expect(api.policyWorkspace).toHaveBeenCalledTimes(2)
    hidden.mockReturnValue(false)
    page.unmount()
    await act(async () => { vi.advanceTimersByTime(10000) })
    expect(api.policyWorkspace).toHaveBeenCalledTimes(2)
  })

  it('serializes selections across groups and preserves both confirmed choices', async () => {
    render(<PoliciesPageHarness data={null} />)
    await screen.findByRole('heading', { name: 'Main' })
    await userEvent.click(screen.getByRole('button', { name: '全部' }))
    let finishFirst!: (snapshot: PolicyWorkspaceSnapshot) => void
    vi.mocked(api.policyWorkspace).mockImplementationOnce(() => new Promise(resolve => { finishFirst = resolve }))
    await userEvent.click(screen.getByRole('button', { name: 'Main 选择 Proxy-B' }))
    await userEvent.click(screen.getByRole('button', { name: 'device/alice/default 选择 Proxy-A' }))
    expect(api.policyWorkspace).toHaveBeenCalledTimes(2)

    workspace.groups[0].selected = 'Proxy-B'
    await act(async () => { finishFirst(structuredClone(workspace)) })
    await waitFor(() => expect(api.policyWorkspace).toHaveBeenCalledTimes(3))
    expect(screen.getByRole('button', { name: 'Main 选择 Proxy-B' }).getAttribute('aria-pressed')).toBe('true')
    expect(screen.getByRole('button', { name: 'device/alice/default 选择 Proxy-A' }).getAttribute('aria-pressed')).toBe('true')
  })

  it('suggests a refresh only for the selected running device group', async () => {
    const onSuggestConnectionRefresh = vi.fn()
    render(<PoliciesPageHarness onSuggestConnectionRefresh={onSuggestConnectionRefresh} />)
    await screen.findByRole('heading', { name: 'Main' })
    await userEvent.click(screen.getByRole('button', { name: '设备策略' }))
    await userEvent.click(screen.getByRole('button', { name: 'device/alice/default 选择 Proxy-A' }))

    await waitFor(() => expect(onSuggestConnectionRefresh).toHaveBeenCalledWith({ key: 'device:alice', scope: 'device', deviceID: 'alice', subject: 'alice', selection: 'Proxy-A' }))
  })

  it('suggests the shared group scope after a successful change even if overview refresh fails', async () => {
    const onSuggestConnectionRefresh = vi.fn()
    render(<PoliciesPageHarness onChanged={async () => { throw new Error('overview unavailable') }} onSuggestConnectionRefresh={onSuggestConnectionRefresh} />)
    await screen.findByRole('heading', { name: 'Main' })
    await userEvent.click(screen.getByRole('button', { name: 'Main 选择 Proxy-A' }))
    expect(onSuggestConnectionRefresh).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: 'Main 选择 Proxy-B' }))
    await waitFor(() => expect(onSuggestConnectionRefresh).toHaveBeenCalledExactlyOnceWith({ key: 'policy_group:Main', scope: 'policy_group', group: 'Main', subject: 'Main', selection: 'Proxy-B' }))
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('does not suggest resetting connections when selection fails', async () => {
    const onSuggestConnectionRefresh = vi.fn()
    render(<PoliciesPageHarness onSuggestConnectionRefresh={onSuggestConnectionRefresh} />)
    await screen.findByRole('heading', { name: 'Main' })
    vi.mocked(api.policyWorkspace).mockRejectedValueOnce(new Error('selection failed'))
    await userEvent.click(screen.getByRole('button', { name: 'Main 选择 Proxy-B' }))
    await screen.findByText(/策略配置暂不可用/)
    expect(onSuggestConnectionRefresh).not.toHaveBeenCalled()
  })

  it('uses the acknowledged workspace mode rather than a stale running overview', async () => {
    const onSuggestConnectionRefresh = vi.fn()
    render(<PoliciesPageHarness onSuggestConnectionRefresh={onSuggestConnectionRefresh} />)
    await screen.findByRole('heading', { name: 'Main' })
    workspace.mode = 'prepared'
    await userEvent.click(screen.getByRole('button', { name: 'Main 选择 Proxy-B' }))
    await screen.findByText('待启动配置')
    expect(onSuggestConnectionRefresh).not.toHaveBeenCalled()
  })

  it('does not show a late Mac runtime response after the gateway stops', async () => {
    let finishRouting!: (routing: LocalRouting) => void
    vi.mocked(api.localRouting).mockImplementationOnce(() => new Promise(resolve => { finishRouting = resolve }))
    const page = render(<PoliciesPageHarness />)
    await screen.findByRole('heading', { name: 'Main' })
    workspace.mode = 'prepared'
    page.rerender(<PoliciesPageHarness data={{ ...overview, status: { ...overview.status, gateway: 'stopped', mihomo: 'stopped' } }} />)
    await screen.findByText('待启动配置')
    await act(async () => { finishRouting(localRouting()) })

    expect(screen.getByText('启动网关后可用')).toBeTruthy()
    expect(screen.queryByLabelText('本机全局策略组 当前策略 Proxy-A')).toBeNull()
  })

  it('renders prepared configuration and errors in English', async () => {
    await prepareLanguage('en')
    activateLanguage('en')
    workspace.mode = 'prepared'
    const page = render(<PoliciesPageHarness data={null} />)
    await screen.findByText('Prepared configuration')
    expect(screen.getByRole('heading', { name: 'Main' })).toBeTruthy()
    vi.mocked(api.policyWorkspace).mockRejectedValueOnce(new Error('Test unavailable'))
    await userEvent.click(screen.getByRole('button', { name: 'Test current view' }))
    await screen.findByRole('alert')

    expect(page.container.textContent).not.toMatch(/[\u3400-\u9fff]/)
  })

  it('shows the dedicated Tailscale Exit Node group with a friendly name', async () => {
    workspace.health = {
      ...health,
      proxies: [...health.proxies,
        { name: 'open-surge/tailscale-exit', display_name: 'Home Tailnet · Exit Node', type: 'Selector', selected: 'open-surge/tailscale', provider: '', udp: true, role: 'exit_node', status: 'reachable', probeable: true },
        { name: 'open-surge/tailscale', display_name: 'Home Tailnet', type: 'Tailscale', selected: '', provider: '', udp: true, role: 'exit_node', status: 'reachable', probeable: true },
      ],
    }
    const data = {
      ...overview,
      policies: [...overview.policies, { name: 'open-surge/tailscale-exit', type: 'Selector', selected: 'open-surge/tailscale', options: ['open-surge/tailscale'] }],
    } as Overview
    workspace.groups = data.policies
    render(<PoliciesPageHarness data={data} />)

    expect(await screen.findByRole('heading', { name: 'Home Tailnet · Exit Node' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Home Tailnet · Exit Node' })).toBeTruthy()
    expect(screen.getAllByText('Home Tailnet').length).toBeGreaterThan(0)
  })

  it('shows an injected Exit Node selected by the prepared core and lets the user change it', async () => {
    workspace.mode = 'prepared'
    workspace.groups = [{ name: 'AI', type: 'Selector', selected: 'open-surge/tailscale-exit', options: ['open-surge/tailscale-exit', 'Proxy-A'] }]
    workspace.health.proxies.push({ name: 'open-surge/tailscale-exit', display_name: 'Home Tailnet · Exit Node', type: 'Selector', selected: 'open-surge/tailscale', provider: '', udp: true, role: 'exit_node', status: 'untested', probeable: true })
    render(<PoliciesPageHarness data={null} />)

    const exit = await screen.findByRole('button', { name: 'AI 选择 Home Tailnet · Exit Node' })
    expect(exit.getAttribute('aria-pressed')).toBe('true')
    await userEvent.click(screen.getByRole('button', { name: 'AI 选择 Proxy-A' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'AI 选择 Proxy-A' }).getAttribute('aria-pressed')).toBe('true'))
    expect(exit.getAttribute('aria-pressed')).toBe('false')
    expect(api.policyWorkspace).toHaveBeenCalledWith({ action: 'select', group: 'AI', policy: 'Proxy-A' })
  })

  it('keeps the Mac global policy group first and switches it through the local-routing API', async () => {
    const onChanged = vi.fn(async () => {})
    const onSuggestConnectionRefresh = vi.fn()
    render(<PoliciesPageHarness onChanged={onChanged} onSuggestConnectionRefresh={onSuggestConnectionRefresh} />)

    const localGroup = await screen.findByRole('heading', { name: '本机全局策略组' })
    const mainGroup = await screen.findByRole('heading', { name: 'Main' })
    expect(localGroup.compareDocumentPosition(mainGroup) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

    await userEvent.click(screen.getByLabelText('本机全局策略组 当前策略 Proxy-A'))
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /Proxy-B/ }))
    await waitFor(() => expect(api.setLocalRouting).toHaveBeenCalledWith('rule', 'Proxy-B'))
    expect(onChanged).toHaveBeenCalledOnce()
    expect(onSuggestConnectionRefresh).not.toHaveBeenCalled()
  })

  it('suggests a Mac refresh when the active fixed outlet changes', async () => {
    vi.mocked(api.localRouting).mockResolvedValue(localRouting('global'))
    vi.mocked(api.setLocalRouting).mockImplementation(async (mode, policy) => localRouting(mode, policy ?? 'Proxy-A'))
    const onSuggestConnectionRefresh = vi.fn()
    render(<PoliciesPageHarness onSuggestConnectionRefresh={onSuggestConnectionRefresh} />)

    await userEvent.click(await screen.findByLabelText('本机全局策略组 当前策略 Proxy-A'))
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /Proxy-B/ }))

    await waitFor(() => expect(onSuggestConnectionRefresh).toHaveBeenCalledWith({ key: 'gateway_local', scope: 'gateway_local', subject: 'Mac 本机', selection: 'Proxy-B' }))
  })

  it('keeps the search and active group when the page is left and mounted again', async () => {
    renderSessionPage()

    await screen.findByRole('heading', { name: 'Streaming' })
    const search = screen.getByRole('searchbox', { name: '搜索策略组或节点' })
    await userEvent.click(screen.getByRole('button', { name: '全部' }))
    await userEvent.type(search, 'Streaming')
    await userEvent.click(screen.getByRole('button', { name: 'Streaming' }))
    expect((await screen.findByRole('button', { name: 'Streaming' })).getAttribute('aria-current')).toBe('location')

    await userEvent.click(screen.getByRole('button', { name: '离开策略页' }))
    await userEvent.click(screen.getByRole('button', { name: '返回策略页' }))

    expect((await screen.findByRole('searchbox', { name: '搜索策略组或节点' }) as HTMLInputElement).value).toBe('Streaming')
    expect(screen.getByRole('button', { name: '全部' }).getAttribute('aria-pressed')).toBe('true')
    expect(screen.getByRole('button', { name: 'Streaming' }).getAttribute('aria-current')).toBe('location')
    expect(HTMLElement.prototype.scrollIntoView).toHaveBeenLastCalledWith({ behavior: 'auto', block: 'start' })
  })

  it('restores a saved pre-list scroll position when no group was active', async () => {
    render(<PoliciesPage overview={overview} onChanged={async () => {}} viewState={{ search: '', scope: 'global', activeGroup: null }} onViewStateChange={() => {}} restoreScrollY={240} onScrollPositionChange={() => {}} />)

    await screen.findByRole('navigation', { name: '策略组快速导航' })
    expect(window.scrollTo).toHaveBeenCalledWith({ top: 240, behavior: 'auto' })
  })

  it('keeps search, scope, and group navigation in one sticky control region', async () => {
    render(<PoliciesPageHarness />)

    const search = await screen.findByRole('searchbox', { name: '搜索策略组或节点' })
    const controls = search.closest('.policy-controls-sticky')
    expect(controls).toBeTruthy()
    expect(controls?.contains(screen.getByRole('group', { name: '策略组范围' }))).toBe(true)
    expect(controls?.contains(screen.getByRole('navigation', { name: '策略组快速导航' }))).toBe(true)
  })

  it('keeps the clicked group active while its smooth page scroll is in progress', async () => {
    renderSessionPage()
    const main = await screen.findByRole('button', { name: 'Main' })
    const mainCard = screen.getByRole('heading', { name: 'Main' }).closest('article') as HTMLElement
    const streamingCard = screen.getByRole('heading', { name: 'Streaming' }).closest('article') as HTMLElement
    const controls = main.closest('.policy-controls-sticky') as HTMLElement
    vi.spyOn(mainCard, 'getBoundingClientRect').mockReturnValue({ top: 160 } as DOMRect)
    vi.spyOn(streamingCard, 'getBoundingClientRect').mockReturnValue({ top: 1180 } as DOMRect)
    vi.spyOn(controls, 'getBoundingClientRect').mockReturnValue({ bottom: 120 } as DOMRect)
    await userEvent.click(main)

    fireEvent.scroll(window)

    await waitFor(() => expect(main.getAttribute('aria-current')).toBe('location'))
    expect(screen.getByRole('button', { name: 'Streaming' }).hasAttribute('aria-current')).toBe(false)
  })

  it('does not clear a restored group when a short result page cannot align its card', async () => {
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this.classList.contains('policy-controls-sticky')) return { top: 12, bottom: 123 } as DOMRect
      if (this.classList.contains('policy-health-group')) return { top: 320, bottom: 680 } as DOMRect
      return { top: 0, bottom: 0 } as DOMRect
    })
    const onViewStateChange = vi.fn()
    render(<PoliciesPage overview={overview} onChanged={async () => {}} viewState={{ search: '', scope: 'global', activeGroup: 'Main' }} onViewStateChange={onViewStateChange} restoreScrollY={217} onScrollPositionChange={() => {}} />)

    await screen.findByRole('navigation', { name: '策略组快速导航' })
    await new Promise(resolve => window.setTimeout(resolve, 275))
    fireEvent.scroll(window)
    await new Promise(resolve => window.setTimeout(resolve, 25))

    expect(onViewStateChange).not.toHaveBeenCalledWith({ activeGroup: null })
    rectSpy.mockRestore()
  })
})
