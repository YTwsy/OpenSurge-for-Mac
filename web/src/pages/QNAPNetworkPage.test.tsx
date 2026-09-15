// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ConfigFile, ControlConfig, NetworkDefaults, Overview } from '../types'

vi.mock('../api', () => ({
  api: {
    config: vi.fn(),
    configFile: vi.fn(),
    networkDefaults: vi.fn(),
    gateway: vi.fn(),
    saveConfigFile: vi.fn(),
  },
  request: vi.fn(),
  waitForOperation: vi.fn(async () => ({ id: 'operation', kind: 'start', state: 'succeeded' })),
}))

import { api, request, waitForOperation } from '../api'
import { QNAPNetworkPage } from './QNAPNetworkPage'

const config: ControlConfig = {
  schema_version: 1,
  revision: 'config-revision',
  gateway: { mode: 'same_lan', interface: 'eth0', lan_ip: '192.168.2.241', lan_prefix_len: 24, upstream_interface: 'eth0' },
  dhcp: { enabled: false, range_start: '', range_end: '', lease_time: '12h', domain: 'lan', bypass_gateway: '', bypass_dns: [] },
  dns: { listen: '192.168.2.241', upstream: '127.0.0.1#1053', ipv6: false },
  mihomo: { store_fake_ip: true },
  transparent: { mode: 'tun', strict_route: false, tun_ipv6: 'off' },
  local_system_proxy: { enabled: false },
  device_policy: { enabled: true, protected_ipv4: [] },
}

const networkDefaults: NetworkDefaults = {
  schema_version: 1,
  mode: 'same_lan',
  snapshot: { network_service: 'QNET', interface: 'eth0', ipv4: '192.168.2.241', subnet_mask: '255.255.255.0', router: '192.168.2.1', dns: ['192.168.2.1'], ipv6_default: false },
  gateway_ipv4: '192.168.2.241',
  lan_prefix_len: 24,
  bypass_dns: [],
  warnings: [],
  blockers: [],
}

const overview: Overview = {
  schema_version: 1,
  revision: 'config-revision',
  topology: 'same_lan',
  drift: false,
  warnings: [],
  status: {
    gateway: 'stopped', runtime_state: 'none', interface: 'eth0', lan_ip: '192.168.2.241', data_plane: 'stopped', routing: 'not_applied',
    dhcp: 'stopped', dhcp_enabled: false, mihomo: 'stopped', tun: 'stopped', forwarding: 'disabled', dns_ipv6: false,
    tun_ipv6_requested: 'off', ipv6_packet: 'disabled', native_ipv6_available: false, client_count: 0,
  },
  doctor: [], doctor_healthy: true, leases: [], policies: [], providers: { proxy_providers: [], rule_providers: [] },
  recovery: { stage: 'idle', required: false },
  sleep_prevention: { enabled: false, active: false },
}

const hostRouting = {
  schema_version: 1, supported: true, desired: false, enabled: false, gateway_ready: false,
  host_ipv4: '192.168.2.240', gateway_ipv4: '192.168.2.241', fallback_gateway: '192.168.2.1', dns_redirect: false, checked_at: '2026-09-16T00:00:00Z',
}

let currentFile: ConfigFile

describe('QNAPNetworkPage configuration file editor', () => {
  afterEach(() => { cleanup(); vi.clearAllMocks(); vi.unstubAllGlobals() })

  beforeEach(() => {
    currentFile = {
      schema_version: 1,
      path: '/data/config/opensurge.yaml',
      revision: 'file-revision',
      content: 'gateway:\n  upstream_gateway: "192.168.2.1"\n\nmihomo:\n  secret: "<redacted>"\n\nupstream_proxy:\n  password: "<redacted>"\n',
      protected_fields: ['mihomo.secret', 'upstream_proxy.password'],
    }
    vi.mocked(api.config).mockResolvedValue(config)
    vi.mocked(api.configFile).mockImplementation(async () => currentFile)
    vi.mocked(api.networkDefaults).mockResolvedValue(networkDefaults)
    vi.mocked(api.gateway).mockImplementation(async action => ({ id: action, kind: action, state: 'running' }))
    vi.mocked(api.saveConfigFile).mockImplementation(async (content, revision) => {
      currentFile = { ...currentFile, content, revision: `${revision}-saved` }
      return currentFile
    })
    vi.mocked(request).mockResolvedValue(hostRouting)
    vi.mocked(waitForOperation).mockResolvedValue({ id: 'operation', kind: 'start', state: 'succeeded' })
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, status: 200, json: async () => ({ enabled: false, api_base: '/api/remote/v1', capabilities: '/api/remote/v1/capabilities' }) })))
  })

  it('loads the redacted file and saves an edited upstream gateway with revision proof', async () => {
    const onChanged = vi.fn(async () => {})
    const onNotify = vi.fn()
    render(<QNAPNetworkPage overview={overview} onChanged={onChanged} onNavigate={() => {}} onNotify={onNotify} />)

    const editor = await screen.findByRole('textbox', { name: 'OpenSurge 配置文件内容' }) as HTMLTextAreaElement
    expect(editor.value).toContain('upstream_gateway: "192.168.2.1"')
    expect(editor.value).toContain('secret: "<redacted>"')
    expect(editor.value).not.toContain('mihomo-secret-value')

    await userEvent.clear(editor)
    await userEvent.type(editor, 'gateway:\n  upstream_gateway: "192.168.2.254"\n\nmihomo:\n  secret: "<redacted>"\n\nupstream_proxy:\n  password: "<redacted>"\n')
    await userEvent.click(screen.getByRole('button', { name: '保存配置文件' }))

    await waitFor(() => expect(api.saveConfigFile).toHaveBeenCalledWith(expect.stringContaining('192.168.2.254'), 'file-revision'))
    expect(api.gateway).not.toHaveBeenCalled()
    expect(await screen.findByText('配置文件已保存；敏感字段保持不变。')).toBeTruthy()
    expect(onChanged).toHaveBeenCalled()
    expect(onNotify).toHaveBeenCalledWith(expect.objectContaining({ tone: 'success', title: '配置文件已保存' }))
  })

  it('stops and restarts a running gateway around a file save', async () => {
    vi.stubGlobal('confirm', vi.fn(() => true))
    const onChanged = vi.fn(async () => {})
    const onNotify = vi.fn()
    const runningOverview = { ...overview, status: { ...overview.status, gateway: 'running' as const } }
    render(<QNAPNetworkPage overview={runningOverview} onChanged={onChanged} onNavigate={() => {}} onNotify={onNotify} />)

    const editor = await screen.findByRole('textbox', { name: 'OpenSurge 配置文件内容' }) as HTMLTextAreaElement
    await userEvent.clear(editor)
    await userEvent.type(editor, 'gateway:\n  upstream_gateway: "192.168.2.254"\n\nmihomo:\n  secret: "<redacted>"\n\nupstream_proxy:\n  password: "<redacted>"\n')
    await userEvent.click(screen.getByRole('button', { name: '保存配置文件并重启' }))

    await waitFor(() => expect(api.gateway).toHaveBeenNthCalledWith(1, 'stop'))
    await waitFor(() => expect(api.gateway).toHaveBeenNthCalledWith(2, 'start'))
    expect(await screen.findByText('配置文件已保存并重新启动网关。')).toBeTruthy()
  })
})
