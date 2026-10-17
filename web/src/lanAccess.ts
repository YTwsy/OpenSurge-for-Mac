import { RequestError, request } from './api'
import { t } from './i18n'

export type LANAddress = { interface: string; ipv4: string; prefix_len: number }
export type LANSettings = LANAddress & { enabled: boolean; port: number }
export type LANAccess = LANSettings & {
  schema_version: number; revision: string; password_set: boolean
  state: 'disabled' | 'listening' | 'unavailable'; url?: string; error?: string
  recommended_interface?: string; addresses: LANAddress[]
}
export type BrowserSession = { mode: 'local' | 'lan'; authenticated?: boolean; local_client?: boolean }

let browserSession: BrowserSession = { mode: 'local' }
export function setBrowserSession(session: BrowserSession) { browserSession = session }
export function isLANBrowser() { return browserSession.mode === 'lan' }
export function isRemoteLANBrowser() { return isLANBrowser() && !browserSession.local_client }

async function authRequest<T>(path: string, body?: object): Promise<T> {
  const response = await fetch(`/api/v1/auth/${path}`, {
    method: body ? 'POST' : 'GET', credentials: 'same-origin', cache: 'no-store',
    headers: { 'Content-Type': 'application/json' }, ...(body ? { body: JSON.stringify(body) } : {}),
  })
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}))
    throw new RequestError(response.status, payload.error?.code ?? '', payload.error?.message ?? '')
  }
  return response.json()
}

export const lanAPI = {
  settings: () => request<LANAccess>('/api/v1/lan-access'),
  save: (settings: LANSettings, revision: string, password?: string) => request<LANAccess>('/api/v1/lan-access', {
    method: 'PUT', headers: { 'If-Match': `"${revision}"` }, body: JSON.stringify({ ...settings, ...(password ? { password } : {}) }),
  }),
  resetPassword: (password: string, revision: string) => request<LANAccess>('/api/v1/lan-access/password', {
    method: 'POST', headers: { 'If-Match': `"${revision}"` }, body: JSON.stringify({ password }),
  }),
  session: () => authRequest<BrowserSession>('session'),
  login: (password: string) => authRequest<BrowserSession>('login', { password }),
  logout: () => authRequest<{ ok: boolean }>('logout', {}),
  changePassword: (old_password: string, password: string) => request<{ ok: boolean }>('/api/v1/auth/password', {
    method: 'POST', body: JSON.stringify({ old_password, password }),
  }),
}

export function lanError(cause: unknown): string {
  const code = typeof cause === 'string' ? cause : cause instanceof RequestError ? cause.code : ''
  const messages: Record<string, string> = {
    configuration_invalid: '局域网管理配置无法读取，请在本机重新设置。',
    address_unavailable: '原局域网地址已不可用，请重新选择地址并保存。',
    lan_address_unavailable: '请选择当前接口上可用的局域网 IPv4 地址。',
    listen_failed: '无法监听管理端口，请检查端口占用或更换端口。',
    lan_unavailable: '局域网管理暂不可用，请在 Mac 上检查设置。',
    password_required: '请先设置管理密码。',
    password_invalid: '密码需要 12–128 个字符，支持空格和长密码。',
    password_incorrect: '管理密码不正确。',
    authentication_required: '登录已过期，请重新输入管理密码。',
    login_limited: '尝试次数过多，请稍后再试。',
    revision_conflict: '设置已在其他窗口修改，请刷新后重试。',
    lan_save_failed: '无法保存局域网管理设置，请重试。',
  }
  return t(messages[code] ?? '无法连接管理服务，请检查连接后重试。')
}

export function validManagementPassword(value: string) { return [...value].length >= 12 && [...value].length <= 128 }
