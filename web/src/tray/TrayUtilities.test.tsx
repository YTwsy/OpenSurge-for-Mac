// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { desktopAction } from '../desktop'
import { activateLanguage, prepareLanguage } from '../i18n'
import { TrayUtilities } from './TrayUtilities'
vi.mock('../desktop', () => ({ desktopAction: vi.fn() }))
const initial = { login: { state: 'disabled', sequence: 1, failed: false }, update: { current: 'v0.2.4-next', checking: false, checked: false, failed: false, sequence: 1 } }
beforeEach(async () => { await prepareLanguage('en'); activateLanguage('en'); vi.mocked(desktopAction).mockResolvedValue(initial) })
afterEach(() => { cleanup(); vi.resetAllMocks(); activateLanguage('zh-Hans') })
it('shows pending approval and preserves the OS state after a failed change', async () => {
 let login = initial.login
 vi.mocked(desktopAction).mockImplementation(async (action, body) => {
  if (action === 'login-item') { login = body?.enabled ? { state: 'approval', sequence: 2, failed: false } : { ...login, sequence: 3, failed: true }; return login }
  return initial // deliberately late status must not erase the acknowledged result
 })
 render(<TrayUtilities />)
 await userEvent.click(await screen.findByText('App settings and updates'))
 await userEvent.click(screen.getByRole('checkbox', { name: 'Show at login' }))
 expect(await screen.findByText('Waiting for macOS approval of the login item.')).toBeTruthy()
 await userEvent.click(screen.getByRole('checkbox', { name: 'Show at login' }))
 expect(await screen.findByRole('alert')).toHaveProperty('textContent', 'Could not update the login item. The actual macOS state is preserved.')
 expect((screen.getByRole('checkbox') as HTMLInputElement).checked).toBe(true)
 await userEvent.click(screen.getByRole('button', { name: 'Open Login Items settings' }))
 expect(desktopAction).toHaveBeenCalledWith('login-settings', {})
})
it('opens the validated stable release and clears it when a later check fails', async () => {
 const url = 'https://github.com/YTwsy/OpenSurge-for-Mac/releases/tag/v0.2.5'
 let update = { ...initial.update, version: 'v0.2.5', url, checked: true }
 vi.mocked(desktopAction).mockImplementation(async action => {
  if (action === 'check-updates') { update = { ...update, failed: true, version: '', url: '', sequence: 2 }; return update }
  return { ...initial, update }
 })
 render(<TrayUtilities />)
 await userEvent.click(await screen.findByText('App settings and updates'))
 await userEvent.click(screen.getByRole('button', { name: 'Open stable release v0.2.5 download page' }))
 expect(desktopAction).toHaveBeenCalledWith('open-external', { url })
 await userEvent.click(screen.getByRole('button', { name: 'Check for updates' }))
 await waitFor(() => expect(screen.queryByRole('button', { name: /download page/ })).toBeNull())
 expect(await screen.findByRole('status')).toHaveProperty('textContent', 'Could not check for updates. Please try again later.')
})

it('keeps preview uninstall disabled and routes eligible installed actions to native confirmation', async () => {
 vi.mocked(desktopAction).mockResolvedValue({ ...initial, uninstall: 'preview' })
 const view = render(<TrayUtilities canUninstall />)
 await userEvent.click(await screen.findByText('App settings and updates'))
 expect((screen.getByRole('button', { name: 'Uninstall OpenSurge…' }) as HTMLButtonElement).disabled).toBe(true)
 expect(screen.getByText('The preview cannot uninstall your installed OpenSurge application.')).toBeTruthy()
 view.unmount()
 vi.mocked(desktopAction).mockResolvedValue({ ...initial, uninstall: 'available' })
 render(<TrayUtilities canUninstall />)
 await userEvent.click(await screen.findByText('App settings and updates'))
 await userEvent.click(screen.getByRole('button', { name: 'Uninstall OpenSurge…' }))
 expect(desktopAction).toHaveBeenCalledWith('uninstall')
})
