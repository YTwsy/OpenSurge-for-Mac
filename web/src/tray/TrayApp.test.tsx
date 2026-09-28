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
afterEach(() => { cleanup(); vi.clearAllMocks(); activateLanguage('zh-Hans'); localStorage.clear() })

it('opens the existing main window and copies a credential-free diagnostic summary', async () => {
 render(<TrayApp />)
 await screen.findByText('192.0.2.10', { selector: '.tray-subtitle span' })
 await userEvent.click(screen.getByRole('button', { name: /Open OpenSurge/ }))
 expect(desktopAction).toHaveBeenCalledWith('show-main', { page: 'dashboard' })
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
