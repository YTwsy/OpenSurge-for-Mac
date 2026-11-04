// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { OperationNotifications } from './OperationNotifications'

describe('OperationNotifications', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => { cleanup(); vi.useRealTimers(); vi.restoreAllMocks() })

  it('keeps failures available until explicitly dismissed', () => {
    const dismiss = vi.fn()
    render(<OperationNotifications notifications={[{ id: 1, tone: 'error', title: '保存失败', message: 'Invalid gateway address' }]} onDismiss={dismiss} />)
    act(() => vi.advanceTimersByTime(60000))
    expect(dismiss).not.toHaveBeenCalled()
    expect(screen.getByRole('alert').textContent).toContain('Invalid gateway address')
    fireEvent.click(screen.getByRole('button', { name: '关闭通知：保存失败' }))
    expect(dismiss).toHaveBeenCalledWith(1)
  })

  it('pauses a success while hovered or focused, then uses the remaining time', () => {
    const dismiss = vi.fn()
    render(<OperationNotifications notifications={[{ id: 2, tone: 'success', title: '草稿已刷新', message: 'Home updated' }]} onDismiss={dismiss} />)
    act(() => vi.advanceTimersByTime(2000))
    fireEvent.mouseEnter(screen.getByRole('status'))
    act(() => vi.advanceTimersByTime(20000))
    fireEvent.focus(screen.getByRole('button'))
    fireEvent.mouseLeave(screen.getByRole('status'))
    act(() => vi.advanceTimersByTime(20000))
    expect(dismiss).not.toHaveBeenCalled()
    fireEvent.blur(screen.getByRole('button'))
    act(() => vi.advanceTimersByTime(3999))
    expect(dismiss).not.toHaveBeenCalled()
    act(() => vi.advanceTimersByTime(1))
    expect(dismiss).toHaveBeenCalledWith(2)
  })

  it('does not expire a success while the page is hidden', () => {
    const dismiss = vi.fn()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    render(<OperationNotifications notifications={[{ id: 3, tone: 'success', title: '草稿已刷新', message: 'Home updated' }]} onDismiss={dismiss} />)
    act(() => vi.advanceTimersByTime(60000))
    expect(dismiss).not.toHaveBeenCalled()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    fireEvent(document, new Event('visibilitychange'))
    act(() => vi.advanceTimersByTime(6000))
    expect(dismiss).toHaveBeenCalledWith(3)
  })
})
