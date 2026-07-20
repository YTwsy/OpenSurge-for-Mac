// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { authenticationRequiredEvent, request } from './api'

describe('Control API requests', () => {
  afterEach(() => vi.unstubAllGlobals())

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
})
