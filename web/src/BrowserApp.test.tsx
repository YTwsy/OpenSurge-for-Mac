// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { BrowserApp } from './BrowserApp'
import { authenticationRequiredEvent, RequestError } from './api'
import { isLANBrowser, isRemoteLANBrowser, lanAPI, setBrowserSession } from './lanAccess'

vi.mock('./App', () => ({ App: () => <div>Authenticated control page</div> }))
vi.mock('./lanAccess', async importOriginal => ({ ...await importOriginal<typeof import('./lanAccess')>(), lanAPI: { session: vi.fn(), login: vi.fn() } }))
beforeEach(() => { window.history.replaceState({}, '', '/policies'); window.localStorage.clear() })
afterEach(() => { cleanup(); vi.resetAllMocks(); setBrowserSession({ mode: 'local' }) })

it('shows only the password form before authentication and preserves the selected page after re-login', async () => {
  vi.mocked(lanAPI.session).mockResolvedValue({ mode: 'lan', authenticated: false })
  vi.mocked(lanAPI.login).mockResolvedValue({ mode: 'lan', authenticated: true })
  render(<BrowserApp />)
  await screen.findByLabelText('管理密码')
  expect(screen.queryByText('Authenticated control page')).toBeNull()
  fireEvent.change(screen.getByLabelText('管理密码'), { target: { value: 'correct horse lantern' } })
  fireEvent.click(screen.getByRole('button', { name: '登录' }))
  await screen.findByText('Authenticated control page')
  expect(lanAPI.login).toHaveBeenCalledExactlyOnceWith('correct horse lantern')
  expect(window.location.pathname).toBe('/policies')
  await act(async () => window.dispatchEvent(new Event(authenticationRequiredEvent)))
  expect(screen.queryByText('Authenticated control page')).toBeNull()
  expect(screen.getByLabelText('管理密码')).toHaveProperty('value', '')
  expect(screen.queryByText(/菜单栏/)).toBeNull()
  expect(window.localStorage.getItem('password')).toBeNull()
})

it('reports incorrect passwords and throttling without mounting the control page', async () => {
  vi.mocked(lanAPI.session).mockResolvedValue({ mode: 'lan', authenticated: false })
  vi.mocked(lanAPI.login).mockRejectedValueOnce(new RequestError(401, 'password_incorrect', '')).mockRejectedValueOnce(new RequestError(429, 'login_limited', ''))
  render(<BrowserApp />)
  const field = await screen.findByLabelText('管理密码')
  for (const message of ['管理密码不正确。', '尝试次数过多，请稍后再试。']) {
    fireEvent.change(field, { target: { value: 'wrong long password' } })
    fireEvent.click(screen.getByRole('button', { name: '登录' }))
    await screen.findByText(message)
    expect(field).toHaveProperty('value', '')
    expect(screen.queryByText('Authenticated control page')).toBeNull()
  }
})

it('preserves the original local browser bootstrap flow', async () => {
  vi.mocked(lanAPI.session).mockResolvedValue({ mode: 'local' })
  render(<BrowserApp />)
  await screen.findByText('Authenticated control page')
  expect(screen.queryByLabelText('管理密码')).toBeNull()
  expect(lanAPI.login).not.toHaveBeenCalled()
  expect(isRemoteLANBrowser()).toBe(false)
})

it.each([true, false])('uses the server client location on page load (local: %s)', async local => {
  vi.mocked(lanAPI.session).mockResolvedValue({ mode: 'lan', authenticated: true, local_client: local })
  render(<BrowserApp />)
  await screen.findByText('Authenticated control page')
  expect(isLANBrowser()).toBe(true)
  expect(isRemoteLANBrowser()).toBe(!local)
})

it('refreshes client location after login without treating the LAN session as a local entry', async () => {
  vi.mocked(lanAPI.session).mockResolvedValue({ mode: 'lan', authenticated: false, local_client: false })
  vi.mocked(lanAPI.login).mockResolvedValue({ mode: 'lan', authenticated: true, local_client: true })
  render(<BrowserApp />)
  fireEvent.change(await screen.findByLabelText('管理密码'), { target: { value: 'correct horse lantern' } })
  fireEvent.click(screen.getByRole('button', { name: '登录' }))
  await screen.findByText('Authenticated control page')
  expect(isLANBrowser()).toBe(true)
  expect(isRemoteLANBrowser()).toBe(false)
})

it('keeps service errors separate from password rejection and allows a connection retry', async () => {
  vi.mocked(lanAPI.session).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ mode: 'lan', authenticated: false })
  render(<BrowserApp />)
  await screen.findByRole('alert')
  expect(screen.queryByText('Authenticated control page')).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: '重试' }))
  await screen.findByLabelText('管理密码')
})
