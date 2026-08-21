// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PolicyGroupNav } from './PolicyGroupNav'

describe('PolicyGroupNav', () => {
  afterEach(() => cleanup())

  it('lays out groups in one scrollable navigation and exposes edge controls', async () => {
    const onNavigate = vi.fn()
    render(<PolicyGroupNav groups={['Main', 'Streaming', 'Gaming', 'Fallback']} activeGroup={null} onNavigate={onNavigate} />)

    const navigation = screen.getByRole('navigation', { name: '策略组快速导航' })
    Object.defineProperties(navigation, {
      clientWidth: { configurable: true, value: 220 },
      scrollWidth: { configurable: true, value: 720 },
      scrollLeft: { configurable: true, writable: true, value: 0 },
      scrollBy: { configurable: true, value: vi.fn() },
    })
    fireEvent(window, new Event('resize'))

    const left = screen.getByRole('button', { name: '向左浏览策略组' })
    const right = screen.getByRole('button', { name: '向右浏览策略组' })
    await waitFor(() => expect(right.hasAttribute('disabled')).toBe(false))
    expect(left.hasAttribute('disabled')).toBe(true)
    expect(navigation.parentElement?.classList.contains('can-scroll-right')).toBe(true)

    await userEvent.click(right)
    expect(navigation.scrollBy).toHaveBeenCalledWith({ left: 180, behavior: 'smooth' })

    Object.defineProperty(navigation, 'scrollLeft', { configurable: true, writable: true, value: 500 })
    fireEvent.scroll(navigation)
    await waitFor(() => expect(left.hasAttribute('disabled')).toBe(false))
    expect(right.hasAttribute('disabled')).toBe(true)
    expect(navigation.parentElement?.classList.contains('can-scroll-left')).toBe(true)
  })

  it('supports horizontal arrow-key navigation', async () => {
    const onNavigate = vi.fn()
    render(<PolicyGroupNav groups={['Main', 'Streaming']} activeGroup="Main" onNavigate={onNavigate} />)

    const main = screen.getByRole('button', { name: 'Main' })
    main.focus()
    await userEvent.keyboard('{ArrowRight}')

    expect(onNavigate).toHaveBeenCalledWith('Streaming')
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Streaming' }))
  })

  it('uses a vertical wheel to browse overflowing groups and releases it at the edge', () => {
    const { rerender } = render(<PolicyGroupNav groups={[]} activeGroup={null} onNavigate={() => {}} />)
    rerender(<PolicyGroupNav groups={['Main', 'Streaming', 'Gaming']} activeGroup={null} onNavigate={() => {}} />)
    const navigation = screen.getByRole('navigation', { name: '策略组快速导航' })
    Object.defineProperties(navigation, {
      clientWidth: { configurable: true, value: 200 },
      scrollWidth: { configurable: true, value: 600 },
      scrollLeft: { configurable: true, writable: true, value: 0 },
    })

    const scrollRight = new WheelEvent('wheel', { bubbles: true, cancelable: true, deltaY: 80 })
    navigation.dispatchEvent(scrollRight)
    expect(scrollRight.defaultPrevented).toBe(true)
    expect(navigation.scrollLeft).toBe(80)

    Object.defineProperty(navigation, 'scrollLeft', { configurable: true, writable: true, value: 400 })
    const atRightEdge = new WheelEvent('wheel', { bubbles: true, cancelable: true, deltaY: 80 })
    navigation.dispatchEvent(atRightEdge)
    expect(atRightEdge.defaultPrevented).toBe(false)
    expect(navigation.scrollLeft).toBe(400)
  })
})
