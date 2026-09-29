// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { api } from '../api'
import { desktopAction } from '../desktop'
import { activateLanguage, languageCacheKey, prepareLanguage } from '../i18n'
import { selectOption } from '../test/select'
import { SettingsApp } from './SettingsApp'

vi.mock('../api', () => ({ api: { uiPreferences: vi.fn(), setUIPreferences: vi.fn() } }))
vi.mock('../desktop', () => ({ isDesktop: () => true, desktopAction: vi.fn() }))
const initial = { login: { state: 'disabled', sequence: 1, failed: false }, update: { current: 'v0.2.4-next', checking: false, checked: false, failed: false, sequence: 1 } }
beforeEach(async () => {
 window.localStorage.clear(); window.localStorage.setItem(languageCacheKey, 'en')
 await prepareLanguage('en'); activateLanguage('en')
 vi.mocked(api.uiPreferences).mockResolvedValue({ schema_version: 1, language: 'en' })
 vi.mocked(api.setUIPreferences).mockImplementation(async value => ({ schema_version: 1, ...value }))
 vi.mocked(desktopAction).mockImplementation(async action => action === 'utilities' ? initial : {})
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
  if (action === 'login-item') return { state: 'approval', sequence: 2, failed: false }
  if (action === 'check-updates') return { ...initial.update, sequence: 2, version: 'v0.2.5', url, checked: true }
  return {}
 })
 render(<SettingsApp />)
 await waitFor(() => expect((screen.getByRole('switch') as HTMLInputElement).disabled).toBe(false))
 fireEvent.click(screen.getByRole('switch', { name: 'Show at login' }))
 expect(await screen.findByText('Waiting for macOS approval of the login item.')).toBeTruthy()
 await userEvent.click(screen.getByRole('button', { name: 'Open Login Items settings' }))
 expect(desktopAction).toHaveBeenCalledWith('login-settings', {})
 await userEvent.click(screen.getByRole('button', { name: 'Check for updates' }))
 await userEvent.click(await screen.findByRole('button', { name: 'Open stable release v0.2.5 download page' }))
 expect(desktopAction).toHaveBeenCalledWith('open-external', { url })
 await act(async () => window.dispatchEvent(new Event('opensurge:refresh')))
 expect((screen.getByRole('switch') as HTMLInputElement).checked).toBe(true)
 expect(screen.getByRole('button', { name: /download page/ })).toBeTruthy()
})
