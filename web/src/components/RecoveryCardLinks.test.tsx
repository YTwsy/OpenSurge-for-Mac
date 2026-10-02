// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { RecoveryCardLinks } from './RecoveryCardLinks'
import { api } from '../api'
import { desktopAction, isDesktop } from '../desktop'
import { activateLanguage, prepareLanguage } from '../i18n'

vi.mock('../api', () => ({ api: { recoveryCard: vi.fn() } }))
vi.mock('../desktop', () => ({ desktopAction: vi.fn(), isDesktop: vi.fn(() => true) }))
const originalShowModal = Object.getOwnPropertyDescriptor(HTMLDialogElement.prototype, 'showModal')
const originalClose = Object.getOwnPropertyDescriptor(HTMLDialogElement.prototype, 'close')
beforeEach(async () => {
  await prepareLanguage('en')
  activateLanguage('en')
  vi.mocked(isDesktop).mockReturnValue(true)
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', { configurable: true, value: function(this: HTMLDialogElement) { this.open = true } })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', { configurable: true, value: function(this: HTMLDialogElement) { this.open = false; this.dispatchEvent(new Event('close')) } })
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  for (const [name, descriptor] of [['showModal', originalShowModal], ['close', originalClose]] as const) {
    if (descriptor) Object.defineProperty(HTMLDialogElement.prototype, name, descriptor)
    else Reflect.deleteProperty(HTMLDialogElement.prototype, name)
  }
  activateLanguage('zh-Hans')
})

it('opens the authenticated card as text inside the desktop and treats cancelled save as cancellation', async () => {
  vi.mocked(api.recoveryCard).mockResolvedValue('Smoke card <img src="untrusted">')
  vi.mocked(desktopAction).mockResolvedValue({ saved: false })
  render(<RecoveryCardLinks />)
  await userEvent.click(screen.getByRole('button', { name: 'View recovery card' }))
  const dialog = await screen.findByRole('dialog')
  expect(dialog.textContent).toContain('Smoke card <img src="untrusted">')
  expect(dialog.querySelector('img')).toBeNull()
  await userEvent.click(screen.getByRole('button', { name: 'Close preview' }))
  await userEvent.click(screen.getByRole('button', { name: 'Download recovery card' }))
  expect(desktopAction).toHaveBeenCalledWith('save-recovery-card')
  expect(screen.queryByText('Recovery card saved.')).toBeNull()
})

it('retains ordinary browser view and download links', () => {
  vi.mocked(isDesktop).mockReturnValue(false)
  render(<RecoveryCardLinks />)
  expect(screen.getByRole('link', { name: 'View recovery card' }).getAttribute('href')).toBe('/api/v1/recovery/card')
  expect(screen.getByRole('link', { name: 'Download recovery card' }).hasAttribute('download')).toBe(true)
})
