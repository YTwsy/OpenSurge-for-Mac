// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { LANSessionControls } from './LANSessionControls'
import { lanAPI } from '../lanAccess'
import { authenticationRequiredEvent, RequestError } from '../api'

vi.mock('../lanAccess', async importOriginal => ({ ...await importOriginal<typeof import('../lanAccess')>(), lanAPI: { logout: vi.fn(), changePassword: vi.fn() } }))
afterEach(() => { cleanup(); vi.resetAllMocks() })

it('returns to login after a successful logout', async () => {
  vi.mocked(lanAPI.logout).mockResolvedValue({ ok: true })
  const expire = vi.fn()
  window.addEventListener(authenticationRequiredEvent, expire)
  render(<LANSessionControls />)
  fireEvent.click(screen.getByRole('button', { name: '退出登录' }))
  await waitFor(() => expect(expire).toHaveBeenCalledOnce())
  window.removeEventListener(authenticationRequiredEvent, expire)
})

it('validates confirmation, reports a wrong old password, and clears submitted secrets', async () => {
  vi.mocked(lanAPI.changePassword).mockRejectedValue(new RequestError(400, 'password_incorrect', ''))
  render(<LANSessionControls />)
  fireEvent.click(screen.getByRole('button', { name: '修改管理密码' }))
  fireEvent.change(screen.getByLabelText('当前密码'), { target: { value: 'old management password' } })
  fireEvent.change(screen.getByLabelText('新管理密码'), { target: { value: 'new management password' } })
  fireEvent.change(screen.getByLabelText('确认新密码'), { target: { value: 'not the same password' } })
  fireEvent.click(screen.getByRole('button', { name: '保存密码' }))
  await screen.findByText('两次输入的密码不一致。')
  expect(lanAPI.changePassword).not.toHaveBeenCalled()
  fireEvent.change(screen.getByLabelText('确认新密码'), { target: { value: 'new management password' } })
  fireEvent.click(screen.getByRole('button', { name: '保存密码' }))
  await screen.findByText('管理密码不正确。')
  expect(lanAPI.changePassword).toHaveBeenCalledExactlyOnceWith('old management password', 'new management password')
  for (const label of ['当前密码', '新管理密码', '确认新密码']) expect(screen.getByLabelText(label)).toHaveProperty('value', '')
})

it('requires reauthentication after changing the password', async () => {
  vi.mocked(lanAPI.changePassword).mockResolvedValue({ ok: true })
  const expire = vi.fn()
  window.addEventListener(authenticationRequiredEvent, expire)
  render(<LANSessionControls />)
  fireEvent.click(screen.getByRole('button', { name: '修改管理密码' }))
  for (const label of ['当前密码', '新管理密码', '确认新密码']) fireEvent.change(screen.getByLabelText(label), { target: { value: 'new management password' } })
  fireEvent.click(screen.getByRole('button', { name: '保存密码' }))
  await waitFor(() => expect(expire).toHaveBeenCalledOnce())
  window.removeEventListener(authenticationRequiredEvent, expire)
})
