// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { LANAccessCard } from './LANAccessCard'
import { lanAPI, type LANAccess } from '../lanAccess'
import { RequestError } from '../api'

vi.mock('../lanAccess', async importOriginal => ({ ...await importOriginal<typeof import('../lanAccess')>(), lanAPI: { settings: vi.fn(), save: vi.fn(), resetPassword: vi.fn() } }))
vi.mock('qrcode.react', () => ({ QRCodeSVG: ({ value }: { value: string }) => <svg data-testid="address-qr"><title>{value}</title></svg> }))
const initial: LANAccess = { schema_version: 1, revision: 'r1', enabled: false, interface: '', ipv4: '', prefix_len: 0, port: 61768, password_set: false, state: 'disabled', recommended_interface: 'en0', addresses: [{ interface: 'en0', ipv4: '192.168.50.1', prefix_len: 22 }] }
const enabled: LANAccess = { ...initial, ...initial.addresses[0], enabled: true, password_set: true, state: 'listening', url: 'http://192.168.50.1:61768/', revision: 'r2' }
beforeEach(() => { vi.mocked(lanAPI.settings).mockResolvedValue(initial) })
afterEach(() => { cleanup(); vi.resetAllMocks() })

it('does not enable on mount, requires matching passwords, and encodes only the saved listening address', async () => {
  vi.mocked(lanAPI.save).mockResolvedValue(enabled)
  render(<LANAccessCard />)
  const toggle = await screen.findByRole('switch', { name: '允许局域网管理' })
  expect(lanAPI.save).not.toHaveBeenCalled()
  fireEvent.click(toggle)
  fireEvent.change(screen.getByLabelText('管理密码'), { target: { value: 'correct horse lantern' } })
  fireEvent.change(screen.getByLabelText('确认管理密码'), { target: { value: 'does not match' } })
  fireEvent.click(screen.getByRole('button', { name: '保存并启用' }))
  await screen.findByText('两次输入的密码不一致。')
  expect(lanAPI.save).not.toHaveBeenCalled()
  fireEvent.change(screen.getByLabelText('确认管理密码'), { target: { value: 'correct horse lantern' } })
  fireEvent.click(screen.getByRole('button', { name: '保存并启用' }))
  await screen.findByTestId('address-qr')
  expect(lanAPI.save).toHaveBeenCalledExactlyOnceWith({ enabled: true, interface: 'en0', ipv4: '192.168.50.1', prefix_len: 22, port: 61768 }, 'r1', 'correct horse lantern')
  expect(screen.getByTestId('address-qr').textContent).toBe(enabled.url)
  expect(screen.queryByLabelText('管理密码')).toBeNull()
  expect(document.body.textContent).not.toContain('correct horse lantern')
})

it('reports a bind failure without showing a working URL or QR', async () => {
  vi.mocked(lanAPI.settings).mockResolvedValue({ ...enabled, state: 'unavailable', url: undefined, error: 'listen_failed' })
  render(<LANAccessCard />)
  await screen.findByText('无法监听管理端口，请检查端口占用或更换端口。')
  expect(screen.queryByTestId('address-qr')).toBeNull()
  expect(screen.queryByText('正在监听')).toBeNull()
})

it('preserves an edited port and its revision while polling another window’s settings', async () => {
  render(<LANAccessCard />)
  fireEvent.click(await screen.findByRole('switch', { name: '允许局域网管理' }))
  fireEvent.change(screen.getByLabelText('管理端口'), { target: { value: '61800' } })
  vi.mocked(lanAPI.settings).mockResolvedValue({ ...enabled, revision: 'r3', port: 61900 })
  await act(async () => window.dispatchEvent(new Event('opensurge:refresh')))
  expect(screen.getByLabelText('管理端口')).toHaveProperty('value', '61800')
  vi.mocked(lanAPI.save).mockRejectedValue(new RequestError(409, 'revision_conflict', ''))
  fireEvent.click(screen.getByRole('button', { name: '保存并启用' }))
  await screen.findByText('设置已在其他窗口修改，请刷新后重试。')
  expect(lanAPI.save).toHaveBeenCalledWith(expect.objectContaining({ port: 61800 }), 'r1', undefined)
})

it('fences an old poll that completes after disabling access', async () => {
  vi.mocked(lanAPI.settings).mockResolvedValue(enabled)
  render(<LANAccessCard />)
  const toggle = await screen.findByRole('switch', { name: '允许局域网管理' })
  let resolve!: (value: LANAccess) => void
  vi.mocked(lanAPI.settings).mockReturnValueOnce(new Promise(done => { resolve = done }))
  await act(async () => window.dispatchEvent(new Event('opensurge:refresh')))
  vi.mocked(lanAPI.save).mockResolvedValue({ ...enabled, enabled: false, state: 'disabled', url: undefined, revision: 'r3' })
  fireEvent.click(toggle); fireEvent.click(screen.getByRole('button', { name: '保存设置' }))
  await screen.findByText('局域网管理已关闭。')
  await act(async () => resolve(enabled))
  expect(screen.queryByTestId('address-qr')).toBeNull()
})

it('resets the password only after explicit submission', async () => {
  vi.mocked(lanAPI.settings).mockResolvedValue(enabled)
  vi.mocked(lanAPI.resetPassword).mockResolvedValue({ ...enabled, revision: 'r3' })
  render(<LANAccessCard />)
  await screen.findByText('重设管理密码')
  expect(lanAPI.resetPassword).not.toHaveBeenCalled()
  fireEvent.change(screen.getByLabelText('新管理密码'), { target: { value: 'new management password' } })
  fireEvent.change(screen.getByLabelText('确认新密码'), { target: { value: 'new management password' } })
  fireEvent.submit(screen.getByLabelText('新管理密码').closest('form')!)
  await waitFor(() => expect(lanAPI.resetPassword).toHaveBeenCalledExactlyOnceWith('new management password', 'r2'))
  await screen.findByText('管理密码已重设，局域网浏览器需要重新登录。')
  expect(screen.getByLabelText('新管理密码')).toHaveProperty('value', '')
})
