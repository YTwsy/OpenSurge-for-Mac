// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { watchControlEvents } from './desktop'

afterEach(() => {
  document.querySelector('meta[name="opensurge-desktop-session"]')?.remove()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

it('reopens a failed native event stream with bounded backoff and cancels on teardown', () => {
  vi.useFakeTimers()
  const marker = document.createElement('meta')
  marker.name = 'opensurge-desktop-session'
  marker.content = 'local-capability'
  document.head.append(marker)
  const streams: FakeSource[] = []
  class FakeSource extends EventTarget {
    close = vi.fn()
    constructor(public url: string) { super(); streams.push(this) }
  }
  vi.stubGlobal('EventSource', FakeSource)
  const changed = vi.fn()
  const stop = watchControlEvents(changed)
  streams[0].dispatchEvent(new Event('error'))
  expect(streams[0].close).toHaveBeenCalledOnce()
  vi.advanceTimersByTime(1000)
  expect(streams).toHaveLength(2)
  streams[0].dispatchEvent(new Event('error'))
  expect(streams[1].close).not.toHaveBeenCalled()
  streams[1].dispatchEvent(new Event('error'))
  vi.advanceTimersByTime(1000)
  expect(streams).toHaveLength(2)
  vi.advanceTimersByTime(1000)
  expect(streams).toHaveLength(3)
  streams[2].dispatchEvent(new Event('open'))
  streams[2].dispatchEvent(new Event('state'))
  expect(changed).toHaveBeenCalledOnce()
  streams[2].dispatchEvent(new Event('error'))
  vi.advanceTimersByTime(1000)
  expect(streams).toHaveLength(4)
  expect(streams[3].url).toBe('/api/v1/events?desktop_session=local-capability')
  streams[3].dispatchEvent(new Event('error'))
  stop()
  vi.advanceTimersByTime(60_000)
  expect(streams).toHaveLength(4)
})
