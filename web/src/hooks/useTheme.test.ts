// @vitest-environment jsdom
import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { useTheme } from './useTheme'

afterEach(() => { cleanup(); vi.unstubAllGlobals(); window.localStorage.clear() })
it('tracks system changes only while System is selected and removes the listener on unmount', () => {
 let light = true
 const media = new EventTarget()
 const remove = vi.spyOn(media, 'removeEventListener')
 Object.defineProperty(media, 'matches', { get: () => light })
 vi.stubGlobal('matchMedia', () => media)
 const hook = renderHook(useTheme)
 expect(hook.result.current[0]).toBe('light')
 expect(hook.result.current[2]).toBe('system')
 act(() => { light = false; media.dispatchEvent(new Event('change')) })
 expect(hook.result.current[0]).toBe('dark')
 act(() => hook.result.current[3]('light'))
 act(() => { media.dispatchEvent(new Event('change')) })
 expect(hook.result.current[0]).toBe('light')
 act(() => hook.result.current[3]('system'))
 expect(hook.result.current[0]).toBe('dark')
 expect(window.localStorage.getItem('opensurge-theme')).toBe('system')
 hook.unmount()
 expect(remove).toHaveBeenCalledWith('change', expect.any(Function))
})
