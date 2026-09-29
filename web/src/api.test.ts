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

  it('delivers split UTF-8 result frames before the final workspace and keeps desktop authentication', async () => {
    const marker = document.createElement('meta')
    marker.name = 'opensurge-desktop-session'
    marker.content = 'local-capability'
    document.head.append(marker)
    let stream!: ReadableStreamDefaultController<Uint8Array>
    const body = new ReadableStream<Uint8Array>({ start(controller) { stream = controller } })
    const fetcher = vi.fn(async () => new Response(body, { headers: { 'Content-Type': 'text/event-stream' } }))
    vi.stubGlobal('fetch', fetcher)
    const onResult = vi.fn()
    const controller = new AbortController()
    let completed = false
    const request = api.policyWorkspace({ action: 'test', names: ['东京'] }, { onResult, signal: controller.signal })
    void request.then(() => { completed = true })
    const result = { name: '东京', status: 'reachable', delay_ms: 42, tested_at: '2026-09-29T00:00:00Z', test_url: 'https://example.invalid/' }
    const bytes = new TextEncoder().encode(`: keepalive\r\n\r\ndata: ${JSON.stringify({ type: 'result', result })}\r\n\r\n`)
    // Includes splits within both multibyte text and CRLF frame boundaries.
    for (const byte of bytes) stream.enqueue(new Uint8Array([byte]))
    await vi.waitFor(() => expect(onResult).toHaveBeenCalledExactlyOnceWith(result))
    expect(completed).toBe(false)
    const workspace = { schema_version: 1, mode: 'running', revision: 'r', groups: [], health: { schema_version: 1, test_url: '', proxies: [] } }
    stream.enqueue(new TextEncoder().encode(`data: ${JSON.stringify({ type: 'complete', workspace })}\n\n`))
    expect(await request).toEqual(workspace)
    expect(fetcher).toHaveBeenCalledExactlyOnceWith('/api/v1/policy-workspace', expect.objectContaining({ headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream', 'X-OpenSurge-Desktop': 'local-capability' }, signal: controller.signal }))
  })

  it.each(['error', 'disconnect'])('keeps delivered results and rejects an incomplete %s stream without replaying', async failure => {
    const result = { name: 'A', status: 'reachable', delay_ms: 42, tested_at: '', test_url: '' }
    const frames = `data: ${JSON.stringify({ type: 'result', result })}\n\n${failure === 'error' ? 'data: {"type":"error","error":"controller unavailable"}\n\n' : ''}`
    const fetcher = vi.fn(async () => new Response(frames, { headers: { 'Content-Type': 'text/event-stream' } }))
    vi.stubGlobal('fetch', fetcher)
    const onResult = vi.fn()
    await expect(api.policyWorkspace({ action: 'test', names: ['A'] }, { onResult, signal: new AbortController().signal })).rejects.toThrow(failure === 'error' ? 'controller unavailable' : '节点检测连接已中断')
    expect(onResult).toHaveBeenCalledExactlyOnceWith(result)
    expect(fetcher).toHaveBeenCalledOnce()
  })
})
