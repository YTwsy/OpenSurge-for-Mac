// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { api } from '../api'
import { desktopAction } from '../desktop'
import { activateLanguage, languageCacheKey, prepareLanguage } from '../i18n'
import { selectOption } from '../test/select'
import { SettingsApp } from './SettingsApp'
import type { TraySnapshot } from '../tray/types'

vi.mock('../api', () => ({ api: { uiPreferences: vi.fn(), setUIPreferences: vi.fn() } }))
vi.mock('../desktop', () => ({ isDesktop: () => true, desktopAction: vi.fn() }))
const initial = { login: { state: 'disabled', sequence: 1, failed: false }, update: { current: 'v0.3.0-rc.1', checking: false, checked: false, failed: false, sequence: 1 }, uninstall: 'available' }
const runtime: TraySnapshot = { indicator: 'stopped', sequence: 1, can_quit: true, can_uninstall: true, service_actions: true, status: {
 schema_version: 1, revision: 'test', gateway: 'stopped', topology: 'same_lan', lan_ip: '192.0.2.10', dhcp: 'stopped', mihomo: 'stopped', tun: 'stopped', pf_anchor: 'unloaded', forwarding: 'disabled', ipv4_takeover: 'stopped', ipv6_takeover: 'disabled', client_count: 0, drift: false, doctor_healthy: true, recovery_required: false, warnings: [], sleep_prevention: { enabled: false, active: false }, ui_preferences: { schema_version: 1, language: 'en' },
} }
beforeEach(async () => {
 window.localStorage.clear(); window.localStorage.setItem(languageCacheKey, 'en')
 await prepareLanguage('en'); activateLanguage('en')
 vi.mocked(api.uiPreferences).mockResolvedValue({ schema_version: 1, language: 'en' })
 vi.mocked(api.setUIPreferences).mockImplementation(async value => ({ schema_version: 1, ...value }))
 vi.mocked(desktopAction).mockImplementation(async action => action === 'utilities' ? initial : action === 'menubar-status' ? runtime : {})
})
afterEach(() => { cleanup(); vi.resetAllMocks(); window.localStorage.clear(); activateLanguage('zh-Hans') })

it('publishes language only after saving, fences stale reads, and preserves it on failure', async () => {
 render(<SettingsApp />)
 await screen.findByRole('heading', { name: 'OpenSurge Settings' })
 let save!: (value: { schema_version: number; language: 'zh-Hans' }) => void
 vi.mocked(api.setUIPreferences).mockReturnValueOnce(new Promise(resolve => { save = resolve }))
 await selectOption(screen.getByRole('combobox', { name: 'Interface language' }), 'zh-Hans')
 expect(window.localStorage.getItem(languageCacheKey)).toBe('en')
 expect((screen.getByRole('combobox', { name: 'Interface language' }) as HTMLButtonElement).disabled).toBe(true)
 await act(async () => window.dispatchEvent(new Event('opensurge:refresh')))
 expect(screen.getByRole('heading', { name: 'OpenSurge Settings' })).toBeTruthy()
 await act(async () => save({ schema_version: 1, language: 'zh-Hans' }))
 await screen.findByRole('heading', { name: 'OpenSurge 设置' })
 expect(window.localStorage.getItem(languageCacheKey)).toBe('zh-Hans')
 expect(desktopAction).toHaveBeenCalledWith('language', { language: 'zh-Hans' })
 vi.mocked(api.uiPreferences).mockResolvedValue({ schema_version: 1, language: 'zh-Hans' })
 vi.mocked(api.setUIPreferences).mockRejectedValueOnce(new Error('save unavailable'))
 await selectOption(screen.getByRole('combobox', { name: '界面语言' }), 'en')
 expect(await screen.findByRole('alert')).toHaveProperty('textContent', 'save unavailable')
 expect(window.localStorage.getItem(languageCacheKey)).toBe('zh-Hans')
})

it('shares appearance and language with other windows, including System appearance', async () => {
 render(<SettingsApp />)
 await screen.findByRole('heading', { name: 'OpenSurge Settings' })
 await selectOption(screen.getByRole('combobox', { name: 'Appearance' }), 'light')
 expect(document.documentElement.dataset.theme).toBe('light')
 expect(window.localStorage.getItem('opensurge-theme')).toBe('light')
 expect(desktopAction).toHaveBeenCalledWith('settings-appearance', { theme: 'light' })
 await selectOption(screen.getByRole('combobox', { name: 'Appearance' }), 'system')
 expect(desktopAction).toHaveBeenCalledWith('settings-appearance', { theme: 'system' })
 await act(async () => {
  window.dispatchEvent(new StorageEvent('storage', { key: 'opensurge-theme', newValue: 'dark' }))
  window.dispatchEvent(new StorageEvent('storage', { key: languageCacheKey, newValue: 'zh-Hans' }))
 })
 expect(document.documentElement.dataset.theme).toBe('dark')
 expect(await screen.findByRole('heading', { name: 'OpenSurge 设置' })).toBeTruthy()
})

it('uses native login approval and update results without fabricating success', async () => {
 const url = 'https://github.com/YTwsy/OpenSurge-for-Mac/releases/tag/v0.2.5'
 vi.mocked(desktopAction).mockImplementation(async action => {
  if (action === 'utilities') return initial
  if (action === 'menubar-status') return runtime
  if (action === 'login-item') return { state: 'approval', sequence: 2, failed: false }
  if (action === 'check-updates') return { ...initial.update, sequence: 2, version: 'v0.2.5', url, checked: true }
  return {}
 })
 render(<SettingsApp />)
 await waitFor(() => expect((screen.getByRole('switch', { name: 'Show at login' }) as HTMLInputElement).disabled).toBe(false))
 fireEvent.click(screen.getByRole('switch', { name: 'Show at login' }))
 expect(await screen.findByText('Waiting for macOS approval of the login item.')).toBeTruthy()
 await userEvent.click(screen.getByRole('button', { name: 'Open Login Items settings' }))
 expect(desktopAction).toHaveBeenCalledWith('login-settings', {})
 await userEvent.click(screen.getByRole('button', { name: 'Check for updates' }))
 await userEvent.click(await screen.findByRole('button', { name: 'Open stable release v0.2.5 download page' }))
 expect(desktopAction).toHaveBeenCalledWith('open-external', { url })
 await act(async () => window.dispatchEvent(new Event('opensurge:refresh')))
 expect((screen.getByRole('switch', { name: 'Show at login' }) as HTMLInputElement).checked).toBe(true)
 expect(screen.getByRole('button', { name: /download page/ })).toBeTruthy()
})

it('uses the native quit and uninstall confirmations only on explicit clicks', async () => {
 render(<SettingsApp />)
 const fullQuit = await screen.findByRole('button', { name: 'Quit OpenSurge…' }) as HTMLButtonElement
 const uninstall = screen.getByRole('button', { name: 'Uninstall OpenSurge…' }) as HTMLButtonElement
 await waitFor(() => expect(fullQuit.disabled || uninstall.disabled).toBe(false))
 expect(vi.mocked(desktopAction).mock.calls.some(([action]) => ['quit', 'uninstall', 'sleep-prevention'].includes(action))).toBe(false)
 await userEvent.click(screen.getByRole('button', { name: 'Quit Desktop App Only…' }))
 expect(desktopAction).toHaveBeenCalledWith('quit', { full: false })
 await userEvent.click(fullQuit)
 expect(desktopAction).toHaveBeenCalledWith('quit', { full: true })
 await userEvent.click(uninstall)
 expect(desktopAction).toHaveBeenCalledWith('uninstall')
})

it('lets a missing login item retry registration and shows the native failure without claiming success', async () => {
 let attempt = 0
 vi.mocked(desktopAction).mockImplementation(async action => {
  if (action === 'utilities') return { ...initial, login: { state: 'not_found', sequence: 1, failed: false } }
  if (action === 'menubar-status') return runtime
  if (action === 'login-item') return ++attempt === 1
   ? { state: 'not_found', sequence: 2, failed: true, error: 'SMAppServiceErrorDomain: Permission denied (11)' }
   : { state: 'approval', sequence: 3, failed: false }
  return {}
 })
 render(<SettingsApp />)
 await screen.findByText('macOS could not find a login item for this version. Turn on Show at login to register it again.')
 const toggle = screen.getByRole('switch', { name: 'Show at login' }) as HTMLInputElement
 expect(toggle.disabled || toggle.checked).toBe(false)
 expect(attempt).toBe(0)
 expect(screen.queryByText('This App cannot manage login items. Configure them from the installed application.')).toBeNull()
 await userEvent.click(toggle)
 expect(await screen.findByRole('alert')).toHaveProperty('textContent', 'Could not update the login item. The actual macOS state is preserved. SMAppServiceErrorDomain: Permission denied (11)')
 expect(toggle.checked).toBe(false)
 await userEvent.click(screen.getByRole('button', { name: 'Open Login Items settings' }))
 expect(desktopAction).toHaveBeenCalledWith('login-settings', {})
 await userEvent.click(toggle)
 await screen.findByText('Waiting for macOS approval of the login item.')
 expect(toggle.checked).toBe(true)
 expect(attempt).toBe(2)
 expect(screen.queryByRole('alert')).toBeNull()
})

it('keeps login management disabled when the host does not provide it', async () => {
 vi.mocked(desktopAction).mockImplementation(async action => action === 'utilities'
  ? { ...initial, login: { state: 'unavailable', sequence: 1, failed: false } }
  : action === 'menubar-status' ? runtime : {})
 render(<SettingsApp />)
 await screen.findByText('This App cannot manage login items. Configure them from the installed application.')
 expect((screen.getByRole('switch', { name: 'Show at login' }) as HTMLInputElement).disabled).toBe(true)
 expect(vi.mocked(desktopAction).mock.calls.some(([action]) => action === 'login-item')).toBe(false)
})

it('disables full quit and uninstall when native eligibility is lost or unavailable', async () => {
 let status: TraySnapshot | null = runtime
 vi.mocked(desktopAction).mockImplementation(async action => {
  if (action === 'utilities') return initial
  if (action === 'menubar-status') { if (!status) throw new Error('offline'); return status }
  return {}
 })
 render(<SettingsApp />)
 const fullQuit = screen.getByRole('button', { name: 'Quit OpenSurge…' }) as HTMLButtonElement
 const uninstall = screen.getByRole('button', { name: 'Uninstall OpenSurge…' }) as HTMLButtonElement
 await waitFor(() => expect(fullQuit.disabled || uninstall.disabled).toBe(false))
 status = { ...runtime, sequence: 2, can_quit: false, can_uninstall: false, indicator: 'running', status: { ...runtime.status!, gateway: 'running' } }
 await act(async () => window.dispatchEvent(new Event('opensurge:refresh')))
 await waitFor(() => expect(fullQuit.disabled && uninstall.disabled).toBe(true))
 await userEvent.click(fullQuit)
 await userEvent.click(uninstall)
 expect(vi.mocked(desktopAction).mock.calls.some(([action]) => ['quit', 'uninstall'].includes(action))).toBe(false)
 status = { ...runtime, sequence: 3 }
 await act(async () => window.dispatchEvent(new Event('opensurge:refresh')))
 await waitFor(() => expect(fullQuit.disabled || uninstall.disabled).toBe(false))
 status = null
 await act(async () => window.dispatchEvent(new Event('opensurge:refresh')))
 await waitFor(() => expect(fullQuit.disabled && uninstall.disabled).toBe(true))
 expect((screen.getByRole('switch', { name: 'Keep running with lid closed' }) as HTMLInputElement).disabled).toBe(true)
 expect((screen.getByRole('button', { name: 'Quit Desktop App Only…' }) as HTMLButtonElement).disabled).toBe(false)
})

it('keeps acknowledged sleep state when a read started before the toggle arrives late', async () => {
 let reads = 0
 let stale!: (value: TraySnapshot) => void
 let fresh!: (value: TraySnapshot) => void
 vi.mocked(desktopAction).mockImplementation(async action => {
  if (action === 'utilities') return initial
  if (action === 'menubar-status') {
   reads++
   if (reads === 2) return new Promise<TraySnapshot>(resolve => { stale = resolve })
   if (reads >= 3) return new Promise<TraySnapshot>(resolve => { fresh = resolve })
   return runtime
  }
  if (action === 'sleep-prevention') return { enabled: true, active: true }
  return {}
 })
 render(<SettingsApp />)
 const toggle = screen.getByRole('switch', { name: 'Keep running with lid closed' }) as HTMLInputElement
 await waitFor(() => expect(toggle.disabled).toBe(false))
 await act(async () => window.dispatchEvent(new Event('opensurge:refresh')))
 await userEvent.click(toggle)
 expect(desktopAction).toHaveBeenCalledWith('sleep-prevention', { enabled: true })
 await waitFor(() => expect(toggle.checked).toBe(true))
 await act(async () => stale({ ...runtime, sequence: 2 }))
 expect(toggle.checked).toBe(true)
 await act(async () => fresh({ ...runtime, sequence: 3, status: { ...runtime.status!, sleep_prevention: { enabled: true, active: true } } }))
 expect(toggle.checked).toBe(true)
})

it('reports a failed sleep change and keeps the authoritative switch state', async () => {
 vi.mocked(desktopAction).mockImplementation(async action => {
  if (action === 'utilities') return initial
  if (action === 'menubar-status') return runtime
  if (action === 'sleep-prevention') throw new Error('sleep change failed')
  return {}
 })
 render(<SettingsApp />)
 const toggle = screen.getByRole('switch', { name: 'Keep running with lid closed' }) as HTMLInputElement
 await waitFor(() => expect(toggle.disabled).toBe(false))
 await userEvent.click(toggle)
 expect(await screen.findByRole('alert')).toHaveProperty('textContent', 'sleep change failed')
 expect(toggle.checked).toBe(false)
 await waitFor(() => expect(toggle.disabled).toBe(false))
})
