// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { watchVisibleRefresh } from './visibility'

afterEach(() => { delete window.__opensurgeWindowVisible; vi.useRealTimers() })

it('pauses native hidden-window polls, refreshes on reopen and never overlaps requests', async () => {
  vi.useFakeTimers()
  let finish!: () => void
  const refresh = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
  const stop = watchVisibleRefresh(refresh, 2000)
  expect(refresh).toHaveBeenCalledOnce()
  await vi.advanceTimersByTimeAsync(6000)
  expect(refresh).toHaveBeenCalledOnce()
  window.__opensurgeWindowVisible = false
  document.dispatchEvent(new Event('visibilitychange'))
  finish()
  await vi.advanceTimersByTimeAsync(6000)
  expect(refresh).toHaveBeenCalledOnce()
  window.__opensurgeWindowVisible = true
  document.dispatchEvent(new Event('visibilitychange'))
  expect(refresh).toHaveBeenCalledTimes(2)
  finish()
  await Promise.resolve()
  window.dispatchEvent(new Event('opensurge:refresh'))
  expect(refresh).toHaveBeenCalledTimes(3)
  stop()
  finish()
  await vi.advanceTimersByTimeAsync(6000)
  expect(refresh).toHaveBeenCalledTimes(3)
})
