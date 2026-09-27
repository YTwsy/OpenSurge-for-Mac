// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, authenticationRequiredEvent, request } from './api'
import { controlEventsURL } from './desktop'

describe('Control API requests', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    document.querySelector('meta[name="opensurge-desktop-session"]')?.remove()
  })

  it('keeps browser transport unchanged and authenticates desktop JSON, file and event requests', async () => {
    const fetcher = vi.fn(async () => ({ ok: true, json: async () => ({}) }))
    vi.stubGlobal('fetch', fetcher)
    await api.overview()
    expect(fetcher).toHaveBeenLastCalledWith('/api/v1/overview', expect.objectContaining({ headers: { 'Content-Type': 'application/json' } }))
    expect(controlEventsURL()).toBe('/api/v1/events')
    const marker = document.createElement('meta')
    marker.name = 'opensurge-desktop-session'
    marker.content = 'local-capability'
    document.head.append(marker)
    await api.setUIPreferences({ language: 'en' })
    expect(fetcher).toHaveBeenLastCalledWith('/api/v1/ui-preferences', expect.objectContaining({
      headers: { 'Content-Type': 'application/json', 'X-OpenSurge-Desktop': 'local-capability' },
      body: JSON.stringify({ language: 'en' }),
    }))
    await api.importFile(new File(['rules: []'], 'example.yaml', { type: 'text/yaml' }))
    expect(fetcher).toHaveBeenLastCalledWith('/api/v1/sources', expect.objectContaining({
      headers: { 'X-OpenSurge-Desktop': 'local-capability' }, body: expect.any(FormData),
    }))
    expect(controlEventsURL()).toBe('/api/v1/events?desktop_session=local-capability')
  })

  it('does not expire the mounted UI or replay actions while the desktop is reconnecting', async () => {
    const listener = vi.fn()
    window.addEventListener(authenticationRequiredEvent, listener)
    const fetcher = vi.fn(async () => ({ ok: false, status: 503, json: async () => ({ error: { code: 'desktop_service_unavailable' } }) }))
    vi.stubGlobal('fetch', fetcher)
    await expect(api.setSleepPrevention(true)).rejects.toMatchObject({ status: 503, code: 'desktop_service_unavailable' })
    expect(fetcher).toHaveBeenCalledOnce()
    expect(listener).not.toHaveBeenCalled()
    window.removeEventListener(authenticationRequiredEvent, listener)
  })

  it('announces authentication expiry for any API request that receives 401', async () => {
    const listener = vi.fn()
    window.addEventListener(authenticationRequiredEvent, listener, { once: true })
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false,
      status: 401,
      statusText: 'Unauthorized',
      json: async () => ({ error: { code: 'authentication_required', message: 'expired' } }),
    })))

    await expect(request('/api/v1/overview')).rejects.toMatchObject({
      status: 401,
      code: 'authentication_required',
    })
    expect(listener).toHaveBeenCalledOnce()
  })

  it('uses the policy workspace endpoint for read, selection, and delay tests', async () => {
    const snapshot = { schema_version: 1, mode: 'prepared', revision: 'r', groups: [], health: { schema_version: 1, test_url: '', proxies: [] } }
    const fetcher = vi.fn(async () => ({ ok: true, json: async () => snapshot }))
    vi.stubGlobal('fetch', fetcher)

    for (const action of [{ action: 'read' }, { action: 'select', group: 'AI / Home', policy: 'Exit Node' }, { action: 'test', names: ['Exit Node'] }] as const) {
      expect(await api.policyWorkspace(action.action === 'test' ? { ...action, names: [...action.names] } : action)).toEqual(snapshot)
      expect(fetcher).toHaveBeenLastCalledWith('/api/v1/policy-workspace', expect.objectContaining({ method: 'POST', credentials: 'same-origin', body: JSON.stringify(action) }))
    }
  })

  it('encodes the complete policy group name for a scoped connection refresh', async () => {
    const fetcher = vi.fn(async () => ({ ok: true, json: async () => ({ scope: 'policy_group', closed_connections: 0 }) }))
    vi.stubGlobal('fetch', fetcher)
    await api.refreshPolicyConnections('共享/香港 策略')
    expect(fetcher).toHaveBeenCalledExactlyOnceWith(`/api/v1/policies/${encodeURIComponent('共享/香港 策略')}/connections/refresh`, expect.objectContaining({ method: 'POST', credentials: 'same-origin' }))
  })
})
