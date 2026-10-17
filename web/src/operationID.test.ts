import { afterEach, expect, it, vi } from 'vitest'
import { operationID } from './operationID'

afterEach(() => vi.unstubAllGlobals())
it('creates valid UUIDs on LAN HTTP when randomUUID is unavailable', () => {
  const getRandomValues = vi.fn((bytes: Uint8Array) => { for (let i = 0; i < bytes.length; i++) bytes[i] = i + 128; return bytes })
  vi.stubGlobal('crypto', { getRandomValues })
  expect(operationID()).toBe('80818283-8485-4687-8889-8a8b8c8d8e8f')
  expect(getRandomValues).toHaveBeenCalledOnce()
})
it('uses the native implementation where available', () => {
  const randomUUID = vi.fn(() => 'native-id')
  vi.stubGlobal('crypto', { randomUUID })
  expect(operationID()).toBe('native-id')
})
