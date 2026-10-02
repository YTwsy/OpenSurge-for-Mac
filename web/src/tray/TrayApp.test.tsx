// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { TrayApp, diagnosticSummary } from './TrayApp'
import { desktopAction, copyText } from '../desktop'
import { activateLanguage, prepareLanguage } from '../i18n'
import type { TraySnapshot } from './types'

vi.mock('../desktop', () => ({ desktopAction: vi.fn(), copyText: vi.fn() }))
const fixture: TraySnapshot = { indicator: 'stopped', sequence: 1, can_quit: true, status: {
 schema_version: 1, revision: 'test', gateway: 'stopped', topology: 'same_lan', lan_ip: '192.0.2.10', dhcp: 'stopped', mihomo: 'stopped', tun: 'stopped', pf_anchor: 'unloaded', forwarding: 'disabled', ipv4_takeover: 'stopped', ipv6_takeover: 'disabled', client_count: 0, drift: false, doctor_healthy: false, recovery_required: false, warnings: [], sleep_prevention: { enabled: false, active: false }, ui_preferences: { schema_version: 1, language: 'en' },
} }
beforeEach(async () => {
 await prepareLanguage('en'); activateLanguage('en')
 vi.mocked(desktopAction).mockImplementation(async () => fixture)
})
afterEach(() => { cleanup(); vi.clearAllMocks(); activateLanguage('zh-Hans'); localStorage.clear(); delete window.__opensurgeWindowVisible })

it('presents pending configuration and cached diagnostics without a runtime alarm', async () => {
 vi.mocked(desktopAction).mockResolvedValue({ ...fixture, indicator: 'running', status: { ...fixture.status!, gateway: 'running', drift: true,
  presentation: { state: 'running', busy: false, config_pending: true, diagnosis_warning: true },
 } })
 render(<TrayApp />)
 await screen.findByText('Configuration changes are pending; the previous configuration is still in use')
 expect(screen.queryByRole('alert')).toBeNull()
 expect(screen.getByText('The last diagnostic run found items to review')).toBeTruthy()
 expect(document.querySelector('.tray-dot.running')).toBeTruthy()
})

it('shows startup progress instead of a fault for incomplete runtime components', async () => {
 vi.mocked(desktopAction).mockResolvedValue({ ...fixture, indicator: 'starting', can_quit: false, status: { ...fixture.status!, gateway: 'degraded',
  presentation: { state: 'starting', phase: 'starting_mihomo', busy: true, config_pending: false, diagnosis_warning: false },
 } })
 render(<TrayApp />)
 await screen.findByText('Starting Mihomo and waiting for readiness')
 expect(screen.queryByRole('alert')).toBeNull()
 expect(document.querySelector('.tray-dot.transition')).toBeTruthy()
 expect(screen.getByRole('main').textContent).not.toMatch(/[\u3400-\u9fff]/)
})

it('uses the concrete runtime failure reason rather than an unrelated warning', async () => {
 vi.mocked(desktopAction).mockResolvedValue({ ...fixture, indicator: 'degraded', can_quit: false, status: { ...fixture.status!, gateway: 'degraded', warnings: ['unrelated policy observation'],
  presentation: { state: 'degraded', reason: 'dns_stopped', busy: false, config_pending: false, diagnosis_warning: false },
 } })
 render(<TrayApp />)
 await screen.findByText('DHCP / DNS service has stopped')
 expect(screen.getByRole('alert').textContent).not.toContain('unrelated policy observation')
})

it('focuses the panel on reopen without stealing existing keyboard focus', async () => {
 render(<TrayApp />)
 const panel = screen.getByRole('main')
 expect(document.activeElement).toBe(panel)
 screen.getByLabelText('More actions').focus()
 expect(document.activeElement).toBe(screen.getByLabelText('More actions'))
 document.dispatchEvent(new Event('visibilitychange'))
 expect(document.activeElement).toBe(screen.getByLabelText('More actions'))
 window.__opensurgeWindowVisible = false
 document.dispatchEvent(new Event('visibilitychange'))
 window.__opensurgeWindowVisible = true
 document.dispatchEvent(new Event('visibilitychange'))
 expect(document.activeElement).toBe(panel)
})

it('opens the browser through a native grant action and makes failures retryable', async () => {
 let finish: () => void = () => {}
 vi.mocked(desktopAction).mockImplementation(async action => {
  if (action === 'open-browser') return new Promise<void>(resolve => { finish = resolve })
  return fixture
 })
 render(<TrayApp />)
 await userEvent.click(screen.getByLabelText('More actions'))
 await userEvent.click(screen.getByRole('button', { name: 'Open in browser' }))
 expect(desktopAction).toHaveBeenCalledWith('open-browser')
 expect((screen.getByRole('button', { name: 'Opening browser…' }) as HTMLButtonElement).disabled).toBe(true)
 finish()
 await waitFor(() => expect((screen.getByRole('button', { name: 'Open in browser' }) as HTMLButtonElement).disabled).toBe(false))
 vi.mocked(desktopAction).mockRejectedValueOnce(new Error('Browser could not be opened'))
 await userEvent.click(screen.getByRole('button', { name: 'Open in browser' }))
 expect(await screen.findByRole('alert')).toHaveProperty('textContent', 'Browser could not be opened')
 expect((screen.getByRole('button', { name: 'Open in browser' }) as HTMLButtonElement).disabled).toBe(false)
 expect(vi.mocked(desktopAction).mock.calls.some(([action]) => action === 'open-external')).toBe(false)
})

it('opens the existing main window and copies a credential-free diagnostic summary', async () => {
 render(<TrayApp />)
 await screen.findByText('192.0.2.10', { selector: '.tray-subtitle span' })
 await userEvent.click(screen.getByRole('button', { name: /Open OpenSurge/ }))
 expect(desktopAction).toHaveBeenCalledWith('show-main', { page: 'dashboard' })
 await userEvent.click(screen.getByRole('button', { name: /View all/ }))
 expect(desktopAction).toHaveBeenCalledWith('show-main', { page: 'dashboard', section: 'active-devices' })
 await userEvent.click(screen.getByLabelText('More actions'))
 await userEvent.click(screen.getByRole('button', { name: 'Copy diagnostic summary' }))
 expect(copyText).toHaveBeenCalledWith(diagnosticSummary(fixture.status))
 expect(diagnosticSummary(fixture.status)).toContain('PF: unloaded')
})

it('keeps network details out of keyboard navigation until the disclosure opens', async () => {
 render(<TrayApp />)
 await screen.findByText('192.0.2.10', { selector: '.tray-subtitle span' })
 const disclosure = screen.getByRole('button', { name: /Network status/ })
 expect(disclosure.getAttribute('aria-expanded')).toBe('false')
 expect(screen.queryByRole('button', { name: /View network settings/i })).toBeNull()
 disclosure.focus()
 await userEvent.keyboard('{Enter}')
 expect(disclosure.getAttribute('aria-expanded')).toBe('true')
 expect(screen.getByRole('button', { name: /View network settings/i })).toBeTruthy()
 expect(document.getElementById('tray-network-details')?.hasAttribute('inert')).toBe(false)
 await userEvent.keyboard(' ')
 expect(disclosure.getAttribute('aria-expanded')).toBe('false')
 expect(document.getElementById('tray-network-details')?.hasAttribute('inert')).toBe(true)
})

it('presents recovery before runtime state and clears actionable controls on disconnection', async () => {
 vi.mocked(desktopAction).mockResolvedValue({ ...fixture, indicator: 'recovery', can_quit: false, status: { ...fixture.status!, recovery_required: true, recovery_stage: 'gateway_stopped_waiting_router_dhcp' } })
 render(<TrayApp />)
 await screen.findByRole('button', { name: 'Continue recovery' })
 expect(screen.getByRole('alert').textContent).toContain('Network recovery')
 await userEvent.click(screen.getByLabelText('More actions'))
 expect((screen.getByRole('button', { name: 'Quit OpenSurge…' }) as HTMLButtonElement).disabled).toBe(true)
 expect((screen.getByRole('button', { name: 'Quit Desktop App Only…' }) as HTMLButtonElement).disabled).toBe(false)
 vi.mocked(desktopAction).mockResolvedValue({ status: null, sequence: 2, indicator: 'unreachable', can_quit: false })
 await userEvent.click(screen.getByRole('button', { name: 'Refresh status' }))
 await waitFor(() => expect(screen.queryByText('192.0.2.10')).toBeNull())
 expect((screen.getByRole('switch') as HTMLInputElement).disabled).toBe(true)
 expect(screen.queryByRole('button', { name: 'Continue recovery' })).toBeNull()
})

it('routes full exit to native confirmation only when the service snapshot permits it', async () => {
 vi.mocked(desktopAction).mockResolvedValue({ ...fixture, service_actions: true })
 render(<TrayApp />)
 await screen.findByText('192.0.2.10', { selector: '.tray-subtitle span' })
 await userEvent.click(screen.getByLabelText('More actions'))
 await userEvent.click(screen.getByRole('button', { name: 'Quit OpenSurge…' }))
 expect(desktopAction).toHaveBeenCalledWith('quit', { full: true })
})

it('acknowledges sleep changes once and shows the heat warning', async () => {
 let enabled = false
 vi.mocked(desktopAction).mockImplementation(async (action) => {
  if (action === 'sleep-prevention') { enabled = true; return { enabled: true, active: true } }
  return { ...fixture, sequence: enabled ? 2 : 1, status: { ...fixture.status!, sleep_prevention: { enabled, active: enabled } } }
 })
 render(<TrayApp />)
 await screen.findByText('192.0.2.10', { selector: '.tray-subtitle span' })
 await userEvent.click(screen.getByRole('switch'))
 await waitFor(() => expect((screen.getByRole('switch') as HTMLInputElement).checked).toBe(true))
 expect(vi.mocked(desktopAction).mock.calls.filter(([action]) => action === 'sleep-prevention')).toEqual([['sleep-prevention', { enabled: true }]])
 expect(await screen.findByText(/unventilated bag/)).toBeTruthy()
})
