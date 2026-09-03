import type { APIError, ConnectionRefreshResult, ConnectivityResponse, ControlConfig, DevicePolicyDocument, DevicesResponse, DeviceTraffic, Diagnostics, DoctorRunStatus, GatewayPlan, LocalRouting, LocalRoutingMode, NetworkDefaults, NetworkInterfacesResponse, Operation, Overview, PolicySet, ProfileOverlay, ProfileOverlayDocument, ProfileOverlayPreview, ProxyGroup, ProxyHealthSnapshot, ProxyHealthTestResponse, SleepPreventionStatus, Source, SourceSnapshotFile, TailscaleDiscoveryResponse, TailscaleResponse, TailscaleUpdate, UIPreferences } from './types'
import { getOperation, markOperationConnection, operationStatusUnknownMessage, recordOperation } from './operations'

export class RequestError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message)
  }
}

export const authenticationRequiredEvent = 'opensurge:authentication-required'

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: 'same-origin',
    ...init,
    headers: init?.body instanceof FormData ? init.headers : { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!response.ok) {
    if (response.status === 401) window.dispatchEvent(new Event(authenticationRequiredEvent))
    let payload: APIError = {}
    try { payload = await response.json() as APIError } catch { /* response was not JSON */ }
    throw new RequestError(response.status, payload.error?.code ?? 'request_failed', payload.error?.message ?? response.statusText)
  }
  return response.json() as Promise<T>
}

async function operationStatusRequest<T>(path: string): Promise<T> {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), 5000)
  try { return await request<T>(path, { signal: controller.signal }) }
  finally { window.clearTimeout(timer) }
}

export const api = {
  overview: () => request<Overview>('/api/v1/overview'),
  config: () => request<ControlConfig>('/api/v1/config'),
  networkInterfaces: () => request<NetworkInterfacesResponse>('/api/v1/network/interfaces'),
  networkDefaults: (mode: NetworkDefaults['mode']) => request<NetworkDefaults>(`/api/v1/network/defaults?mode=${encodeURIComponent(mode)}`),
  saveConfig: (config: ControlConfig) => request<ControlConfig>('/api/v1/config', { method: 'PUT', headers: { 'If-Match': `"${config.revision}"` }, body: JSON.stringify(config) }),
  gateway: (action: 'start' | 'stop' | 'reload' | 'restart-mihomo') => trackedRequest<Operation>(action, `/api/v1/gateway/${action}`, { method: 'POST' }, true),
  setSleepPrevention: (enabled: boolean) => request<SleepPreventionStatus>('/api/v1/sleep-prevention', { method: 'PUT', body: JSON.stringify({ enabled }) }),
  setUIPreferences: (preferences: Pick<UIPreferences, 'language'>) => request<UIPreferences>('/api/v1/ui-preferences', { method: 'PUT', body: JSON.stringify(preferences) }),
  operation: (id: string) => operationStatusRequest<Operation>(`/api/v1/operations/${encodeURIComponent(id)}`),
  operations: () => operationStatusRequest<{ operations: Operation[] }>('/api/v1/operations'),
  gatewayPlan: (routerDHCPDisabled = false) => request<GatewayPlan>('/api/v1/gateway/plan', { method: 'POST', body: JSON.stringify({ router_dhcp_disabled: routerDHCPDisabled }) }),
  recovery: (stage: string) => request('/api/v1/recovery', { method: 'POST', body: JSON.stringify({ stage }) }),
  prepareRecovery: () => request('/api/v1/recovery/prepare', { method: 'POST', body: JSON.stringify({}) }),
  discardRecovery: () => request('/api/v1/recovery/discard', { method: 'POST' }),
  abandonTakeover: () => request('/api/v1/recovery/abandon-takeover', { method: 'POST' }),
  applyStatic: () => request('/api/v1/network/apply-static', { method: 'POST' }),
  probeDHCP: () => request('/api/v1/network/dhcp-probe', { method: 'POST' }),
  confirmRouterRestored: () => request('/api/v1/recovery/router-restored', { method: 'POST' }),
  finishRecoveryManually: () => request('/api/v1/recovery/manual-finish', { method: 'POST', body: JSON.stringify({ router_dhcp_restored_confirmed: true }) }),
  finishRecoveryKeepingStatic: () => request('/api/v1/recovery/keep-static', { method: 'POST', body: JSON.stringify({ keep_static_confirmed: true }) }),
  restoreMacDHCP: () => request('/api/v1/network/restore-dhcp', { method: 'POST' }),
  validateClient: (clientIPv4: string, ipv6Acknowledged: boolean) => request('/api/v1/recovery/client-validated', { method: 'POST', body: JSON.stringify({ client_ipv4: clientIPv4, gateway_dns_confirmed: true, no_explicit_proxy_confirmed: true, ipv6_bypass_warning_confirmed: ipv6Acknowledged }) }),
  skipClientValidation: () => request('/api/v1/recovery/client-validation-skip', { method: 'POST', body: JSON.stringify({ skip_confirmed: true }) }),
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
  applySource: (id: string, revision: string) => trackedRequest<Source>('apply-profile', `/api/v1/sources/${id}/apply`, { method: 'POST', headers: { 'If-Match': `"${revision}"` } }),
  sourceSnapshotLocation: (id: string) => request<SourceSnapshotFile>(`/api/v1/sources/${encodeURIComponent(id)}/snapshot-location`),
  revealSourceSnapshot: (id: string) => request<SourceSnapshotFile>(`/api/v1/sources/${encodeURIComponent(id)}/reveal`, { method: 'POST' }),
  exportSourceSnapshot: (id: string) => request<SourceSnapshotFile>(`/api/v1/sources/${encodeURIComponent(id)}/export`, { method: 'POST' }),
	tailscale: () => request<TailscaleResponse>('/api/v1/tailscale'),
	tailscaleDiscovery: () => request<TailscaleDiscoveryResponse>('/api/v1/tailscale/discovery'),
	saveTailscale: (revision: string, settings: TailscaleUpdate) => trackedRequest<TailscaleResponse>('apply-tailscale', '/api/v1/tailscale', { method: 'PUT', headers: { 'If-Match': `"${revision}"` }, body: JSON.stringify(settings) }),
	forgetTailscaleIdentity: (revision: string) => request<TailscaleResponse>('/api/v1/tailscale/forget-identity', { method: 'POST', headers: { 'If-Match': `"${revision}"` } }),
	profileOverlay: () => request<ProfileOverlay>('/api/v1/profile-overlay'),
	saveProfileOverlayDocument: (document: ProfileOverlayDocument, revision: string) => request<ProfileOverlay>('/api/v1/profile-overlay', { method: 'PUT', headers: { 'If-Match': `"${revision}"` }, body: JSON.stringify({ document }) }),
	saveProfileOverlayYAML: (yaml: string, revision: string) => request<ProfileOverlay>('/api/v1/profile-overlay', { method: 'PUT', headers: { 'If-Match': `"${revision}"` }, body: JSON.stringify({ yaml }) }),
	sourcePreview: (id: string) => request<ProfileOverlayPreview>(`/api/v1/sources/${encodeURIComponent(id)}/preview`),
  devices: () => request<DevicesResponse>('/api/v1/devices'),
  deviceTraffic: () => request<DeviceTraffic>('/api/v1/device-traffic'),
  devicePolicy: () => request<DevicePolicyDocument>('/api/v1/device-policy'),
  saveDevicePolicy: (policy: PolicySet, revision: string) => trackedRequest<DevicePolicyDocument>('save-device-policy', '/api/v1/device-policy', { method: 'PUT', headers: { 'If-Match': `"${revision}"` }, body: JSON.stringify(policy) }),
  policies: () => request<{ groups: ProxyGroup[] }>('/api/v1/policies'),
  selectPolicy: (group: string, policy: string) => request(`/api/v1/policies/${encodeURIComponent(group)}/selection`, { method: 'POST', body: JSON.stringify({ policy }) }),
  localRouting: () => request<LocalRouting>('/api/v1/local-routing'),
  setLocalRouting: (mode: LocalRoutingMode, globalPolicy?: string) => request<LocalRouting>('/api/v1/local-routing', { method: 'POST', body: JSON.stringify({ mode, global_policy: globalPolicy }) }),
  refreshLocalConnections: () => request<ConnectionRefreshResult>('/api/v1/local-routing/connections/refresh', { method: 'POST' }),
  refreshDeviceConnections: (device: string) => request<ConnectionRefreshResult>(`/api/v1/devices/${encodeURIComponent(device)}/connections/refresh`, { method: 'POST' }),
  selectDevicePolicy: (device: string, slot: string, policy: string) => request(`/api/v1/devices/${encodeURIComponent(device)}/selectors/${encodeURIComponent(slot)}`, { method: 'POST', body: JSON.stringify({ policy }) }),
  proxyHealth: () => request<ProxyHealthSnapshot>('/api/v1/proxy-health'),
  testProxyHealth: (names: string[]) => request<ProxyHealthTestResponse>('/api/v1/proxy-health/tests', { method: 'POST', body: JSON.stringify({ names }) }),
  connectivity: () => request<ConnectivityResponse>('/api/v1/connectivity'),
  testConnectivity: (targetIDs: string[] = []) => request<ConnectivityResponse>('/api/v1/connectivity/tests', { method: 'POST', body: JSON.stringify({ target_ids: targetIDs }) }),
  refreshProvider: (name: string) => request(`/api/v1/providers/${encodeURIComponent(name)}/refresh`, { method: 'POST' }),
  doctorStatus: () => request<DoctorRunStatus>('/api/v1/doctor'),
  runDoctor: () => request<DoctorRunStatus>('/api/v1/doctor', { method: 'POST' }),
  diagnostics: () => request<Diagnostics>('/api/v1/diagnostics'),
}

class OperationStatusUnknownError extends Error {
  constructor() { super(operationStatusUnknownMessage); this.name = 'OperationStatusUnknownError' }
}

// Tracking starts before the mutation is acknowledged. The same observer is
// shared by the page awaiting completion and the global, navigation-safe card.
async function trackedRequest<T>(kind: string, path: string, init: RequestInit, asynchronous = false): Promise<T> {
  const id = crypto.randomUUID()
  const now = new Date().toISOString()
  recordOperation({ id, kind, state: 'running', phase: 'submitting', created_at: now, updated_at: now, phase_started_at: now })
  const response = request<T>(path, { ...init, headers: { ...init.headers, [asynchronous ? 'Idempotency-Key' : 'X-OpenSurge-Operation-ID']: id } })
  void waitForOperation(id).catch(() => { /* the card and original caller own feedback */ })
  try {
    const result = await response
    if (asynchronous) {
      recordOperation(result as Operation)
    } else {
      recordOperation({ id, kind, state: 'succeeded' })
      // The final request can win the polling race; retain its last phase and
      // best-effort notices without delaying the next user workflow step.
      void api.operation(id).then(recordOperation).catch(() => {})
    }
    return result
  } catch (cause) {
    if (cause instanceof RequestError) {
      recordOperation({ id, kind, state: 'failed', error: cause.message })
      throw cause
    }
    markOperationConnection(id, 'unknown')
    // Losing the HTTP response is not evidence that a privileged action failed.
    // Keep querying its ID, and never replay the POST/PUT automatically.
    throw new OperationStatusUnknownError()
  }
}

const operationMonitors = new Map<string, Promise<Operation>>()

export function waitForOperation(id: string, timeoutMs = 180_000): Promise<Operation> {
  const existing = operationMonitors.get(id)
  if (existing) return existing
  const monitor = pollOperation(id, timeoutMs)
  operationMonitors.set(id, monitor)
  const finished = () => { operationMonitors.delete(id) }
  void monitor.then(finished, finished)
  return monitor
}

async function pollOperation(id: string, timeoutMs: number): Promise<Operation> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const known = getOperation(id)
    if (known?.state === 'succeeded') return known
    if (known?.state === 'failed') throw new Error(known.error || `${known.kind} failed`)
    try {
      const operation = await api.operation(id)
      recordOperation(operation)
      if (operation.state === 'succeeded') return operation
      if (operation.state === 'failed') throw new RequestError(409, 'operation_failed', operation.error || `${operation.kind} failed`)
    } catch (cause) {
      if (cause instanceof RequestError && cause.code === 'operation_failed') throw cause
      if (cause instanceof RequestError && cause.status === 401) {
        markOperationConnection(id, 'unknown')
        throw cause
      }
      // A correlated configuration request may not have created its operation
      // yet, so its initial 404 is just the handoff window.
      const handingOff = cause instanceof RequestError && cause.status === 404 && known?.phase === 'submitting' && Date.now() - Date.parse(known.created_at || '') < 3000
      if (!handingOff) markOperationConnection(id, 'reconnecting')
    }
    await new Promise(resolve => window.setTimeout(resolve, 500))
  }
  markOperationConnection(id, 'unknown')
  throw new OperationStatusUnknownError()
}

// Resume unfinished operations after a refresh and discover actions initiated
// in another page/tab or by automatic mihomo recovery. Completed history stays
// in diagnostics; it is not replayed as a fresh notification on every mount.
export function watchOperations() {
  let stopped = false
  let fetching = false
  const discover = async () => {
    if (stopped || fetching) return
    fetching = true
    try {
      const { operations } = await api.operations()
      if (stopped) return
      for (const operation of operations ?? []) {
        if (operation.state !== 'running' && !getOperation(operation.id)) continue
        recordOperation(operation)
        const updatedAt = Date.parse(operation.updated_at || operation.created_at || '')
        if (operation.state === 'running' && Date.now() - updatedAt < 180_000) {
          void waitForOperation(operation.id).catch(() => {})
        } else if (operation.state === 'running') {
          markOperationConnection(operation.id, 'unknown')
        }
      }
    } catch { /* normal auth and connection banners handle discovery errors */ }
    finally { fetching = false }
  }
  void discover()
  const timer = window.setInterval(() => void discover(), 4000)
  return () => { stopped = true; window.clearInterval(timer) }
}
