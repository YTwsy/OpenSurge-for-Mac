import type { APIError, DevicePolicyDocument, DevicesResponse, GatewayPlan, Operation, Overview, PolicySet, ProxyGroup, Source } from './types'

export class RequestError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message)
  }
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: 'same-origin',
    ...init,
    headers: init?.body instanceof FormData ? init.headers : { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!response.ok) {
    let payload: APIError = {}
    try { payload = await response.json() as APIError } catch { /* response was not JSON */ }
    throw new RequestError(response.status, payload.error?.code ?? 'request_failed', payload.error?.message ?? response.statusText)
  }
  return response.json() as Promise<T>
}

export const api = {
  overview: () => request<Overview>('/api/v1/overview'),
  gateway: (action: 'start' | 'stop') => request<Operation>(`/api/v1/gateway/${action}`, { method: 'POST', headers: { 'Idempotency-Key': crypto.randomUUID() } }),
  operation: (id: string) => request<Operation>(`/api/v1/operations/${encodeURIComponent(id)}`),
  gatewayPlan: (routerDHCPDisabled = false) => request<GatewayPlan>('/api/v1/gateway/plan', { method: 'POST', body: JSON.stringify({ network_service: 'Wi-Fi', router_dhcp_disabled: routerDHCPDisabled }) }),
  recovery: (stage: string) => request('/api/v1/recovery', { method: 'POST', body: JSON.stringify({ stage }) }),
  prepareRecovery: () => request('/api/v1/recovery/prepare', { method: 'POST', body: JSON.stringify({ network_service: 'Wi-Fi' }) }),
  applyStatic: () => request('/api/v1/network/apply-static', { method: 'POST' }),
  probeDHCP: () => request('/api/v1/network/dhcp-probe', { method: 'POST' }),
  confirmRouterRestored: () => request('/api/v1/recovery/router-restored', { method: 'POST' }),
  restoreMacDHCP: () => request('/api/v1/network/restore-dhcp', { method: 'POST' }),
  sources: () => request<{ revision: string; sources: Source[] }>('/api/v1/sources'),
  importURL: (name: string, url: string) => request<Source>('/api/v1/sources', { method: 'POST', body: JSON.stringify({ name, kind: 'mihomo_profile', url }) }),
  importFile: (file: File) => {
    const data = new FormData()
    data.set('file', file)
    data.set('name', file.name)
    data.set('kind', 'mihomo_profile')
    return request<Source>('/api/v1/sources', { method: 'POST', body: data })
  },
  refreshSource: (id: string) => request<Source>(`/api/v1/sources/${id}/refresh`, { method: 'POST' }),
  applySource: (id: string, revision: string) => request<Source>(`/api/v1/sources/${id}/apply`, { method: 'POST', headers: { 'If-Match': `"${revision}"` } }),
  devices: () => request<DevicesResponse>('/api/v1/devices'),
  devicePolicy: () => request<DevicePolicyDocument>('/api/v1/device-policy'),
  saveDevicePolicy: (policy: PolicySet, revision: string) => request<DevicePolicyDocument>('/api/v1/device-policy', { method: 'PUT', headers: { 'If-Match': `"${revision}"` }, body: JSON.stringify(policy) }),
  policies: () => request<{ groups: ProxyGroup[] }>('/api/v1/policies'),
  selectPolicy: (group: string, policy: string) => request(`/api/v1/policies/${encodeURIComponent(group)}/selection`, { method: 'POST', body: JSON.stringify({ policy }) }),
  selectDevicePolicy: (device: string, slot: string, policy: string) => request(`/api/v1/devices/${encodeURIComponent(device)}/selectors/${encodeURIComponent(slot)}`, { method: 'POST', body: JSON.stringify({ policy }) }),
  refreshProvider: (name: string) => request(`/api/v1/providers/${encodeURIComponent(name)}/refresh`, { method: 'POST' }),
}

export async function waitForOperation(id: string, timeoutMs = 180_000): Promise<Operation> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const operation = await api.operation(id)
    if (operation.state === 'succeeded') return operation
    if (operation.state === 'failed') throw new Error(operation.error || `${operation.kind} failed`)
    await new Promise(resolve => window.setTimeout(resolve, 500))
  }
  throw new Error('Gateway operation timed out')
}
