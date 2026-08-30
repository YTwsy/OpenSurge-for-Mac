// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useRef, useState } from 'react'
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
import { PoliciesPage, type PoliciesViewState } from './PoliciesPage'

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

function PoliciesPageHarness({ data = overview, onChanged = async () => {} }: { data?: Overview; onChanged?: () => Promise<void> }) {
  const [viewState, setViewState] = useState<PoliciesViewState>({ search: '', scope: 'global', activeGroup: null })
  return <PoliciesPage overview={data} onChanged={onChanged} viewState={viewState} onViewStateChange={patch => setViewState(current => ({ ...current, ...patch }))} restoreScrollY={null} onScrollPositionChange={() => {}} />
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
    vi.mocked(api.localRouting).mockResolvedValue(localRouting())
    vi.mocked(api.setLocalRouting).mockImplementation(async (mode, policy) => localRouting(mode, policy ?? 'Proxy-A'))
  })

  afterEach(() => { cleanup(); vi.clearAllMocks(); vi.restoreAllMocks() })

  it('shows global node health, filters device groups, and switches selector nodes', async () => {
    const onChanged = vi.fn(async () => {})
    render(<PoliciesPageHarness onChanged={onChanged} />)

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
    render(<PoliciesPageHarness />)
    await screen.findAllByText('86 ms')
    await userEvent.click(screen.getByRole('button', { name: '检测当前视图' }))
    await waitFor(() => expect(api.testProxyHealth).toHaveBeenCalledWith(['Proxy-A', 'Proxy-B']))
  })

  it('shows the dedicated Tailscale Exit Node group with a friendly name', async () => {
    vi.mocked(api.proxyHealth).mockResolvedValue({
      ...health,
      proxies: [...health.proxies,
        { name: 'open-surge/tailscale-exit', display_name: 'Home Tailnet · Exit Node', type: 'Selector', selected: 'open-surge/tailscale', provider: '', udp: true, role: 'exit_node', status: 'reachable', probeable: true },
        { name: 'open-surge/tailscale', display_name: 'Home Tailnet', type: 'Tailscale', selected: '', provider: '', udp: true, role: 'exit_node', status: 'reachable', probeable: true },
      ],
    })
    const data = {
      ...overview,
      policies: [...overview.policies, { name: 'open-surge/tailscale-exit', type: 'Selector', selected: 'open-surge/tailscale', options: ['open-surge/tailscale'] }],
    } as Overview
    render(<PoliciesPageHarness data={data} />)

    expect(await screen.findByRole('heading', { name: 'Home Tailnet · Exit Node' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Home Tailnet · Exit Node' })).toBeTruthy()
    expect(screen.getAllByText('Home Tailnet').length).toBeGreaterThan(0)
  })

  it('keeps the Mac global policy group first and switches it through the local-routing API', async () => {
    const onChanged = vi.fn(async () => {})
    render(<PoliciesPageHarness onChanged={onChanged} />)

    const localGroup = await screen.findByRole('heading', { name: '本机全局策略组' })
    const mainGroup = await screen.findByRole('heading', { name: 'Main' })
    expect(localGroup.compareDocumentPosition(mainGroup) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

    await userEvent.click(screen.getByLabelText('本机全局策略组 当前策略 Proxy-A'))
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /Proxy-B/ }))
    await waitFor(() => expect(api.setLocalRouting).toHaveBeenCalledWith('rule', 'Proxy-B'))
    expect(onChanged).toHaveBeenCalledOnce()
  })

  it('keeps the search and active group when the page is left and mounted again', async () => {
    render(<PoliciesSessionHarness />)

    const search = await screen.findByRole('searchbox', { name: '搜索策略组或节点' })
    await userEvent.click(screen.getByRole('button', { name: '全部' }))
    await userEvent.type(search, 'Streaming')
    await userEvent.click(screen.getByRole('button', { name: 'Streaming' }))
    expect(screen.getByRole('button', { name: 'Streaming' }).getAttribute('aria-current')).toBe('location')

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
    render(<PoliciesSessionHarness />)
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
