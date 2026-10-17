// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AppShell } from './AppShell'

vi.mock('../desktop', () => ({ isDesktop: () => false }))
vi.mock('../i18n', () => ({ t: (text: string) => text }))

const nav = [{ id: 'dashboard', label: '总览', icon: '◈' }]
let compact = true
let resize: () => void

describe('mobile app shell', () => {
  beforeEach(() => {
    compact = true
    vi.stubGlobal('matchMedia', () => ({
      get matches() { return compact },
      addEventListener: (_event: string, callback: () => void) => { resize = callback },
      removeEventListener: vi.fn(),
    }))
  })
  afterEach(() => { cleanup(); document.body.style.overflow = ''; vi.unstubAllGlobals() })

  it('opens and closes navigation without rendering or remounting the active page', () => {
    const Page = vi.fn(() => <input aria-label="Draft" defaultValue="original" />)
    const Overlay = vi.fn(() => <span>Background notification</span>)
    render(<AppShell page="dashboard" nav={nav} status="running" statusLabel="正在运行" onNavigate={() => true} footer={null} overlays={<Overlay />}><Page /></AppShell>)
    const draft = screen.getByRole('textbox', { name: 'Draft' }) as HTMLInputElement
    fireEvent.change(draft, { target: { value: 'unsaved draft' } })

    for (let cycle = 0; cycle < 3; cycle++) {
      fireEvent.click(screen.getByRole('button', { name: '打开导航' }))
      expect(document.body.style.overflow).toBe('hidden')
      fireEvent.click(screen.getByRole('button', { name: '关闭导航' }))
      expect(document.body.style.overflow).toBe('')
    }

    expect(Page).toHaveBeenCalledTimes(1)
    expect(Overlay).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('textbox', { name: 'Draft' })).toBe(draft)
    expect(draft.value).toBe('unsaved draft')
  })

  it('restores scrolling when an open drawer leaves compact layout or unmounts', () => {
    document.body.style.overflow = 'auto'
    const { unmount } = render(<AppShell page="dashboard" nav={nav} status="running" statusLabel="正在运行" onNavigate={() => true} footer={null} overlays={null}><p>Page content</p></AppShell>)
    fireEvent.click(screen.getByRole('button', { name: '打开导航' }))
    expect(document.body.style.overflow).toBe('hidden')

    act(() => { compact = false; resize() })
    expect(document.body.style.overflow).toBe('auto')
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(screen.getByRole('navigation')).toBeTruthy()

    act(() => { compact = true; resize() })
    expect(screen.queryByRole('dialog')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '打开导航' }))
    unmount()
    expect(document.body.style.overflow).toBe('auto')
  })
})
