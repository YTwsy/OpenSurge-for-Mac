export type GatewayStatus = {
  gateway: string
  interface: string
  lan_ip: string
  dhcp: string
  dhcp_enabled: boolean
  mihomo: string
  pf_anchor: string
  forwarding: string
  client_count: number
}

export type DoctorCheck = { name: string; ok: boolean; message?: string }
export type Lease = { ip: string; mac: string; hostname?: string; expires_at: string; online: boolean }
export type ProxyGroup = { name: string; type: string; selected: string; options: string[] }
export type ProviderProxy = { name: string; type: string; alive: boolean }
export type ProxyProvider = { name: string; type: string; vehicle_type: string; updated_at?: string; proxy_count: number; proxies: ProviderProxy[] }
export type RuleProvider = { name: string; type: string; vehicle_type: string; behavior?: string; updated_at?: string; rule_count: number }
export type NetworkSnapshot = { network_service: string; interface: string; hardware_address?: string; ipv4?: string; subnet_mask?: string; router?: string; dns: string[]; ipv6_default: boolean }
export type Recovery = { stage: string; topology?: string; required: boolean; updated_at?: string; recovery_notes?: string; network_snapshot?: NetworkSnapshot; client_validation_skipped?: boolean }
export type GatewayPlan = { schema_version: number; revision: string; topology: string; snapshot: NetworkSnapshot; protected_ipv4: string[]; dhcp_servers: string[]; warnings: string[]; blockers: string[] }
export type Operation = { id: string; kind: string; state: string; error?: string }
export type ControlConfig = {
  schema_version: number; revision: string
  gateway: { mode: 'same_lan' | 'same_wifi_dhcp' | 'isolated_lan'; interface: string; lan_ip: string; upstream_interface: string }
  dhcp: { enabled: boolean; range_start: string; range_end: string; lease_time: string; domain: string }
  dns: { listen: string; upstream: string }
  transparent: { mode: 'off' | 'tun'; strict_route: boolean }
  device_policy: { enabled: boolean; protected_ipv4: string[] }
}

export type Overview = {
  schema_version: number
  revision: string
  desired_digest?: string
  applied_digest?: string
  desired_profile_digest?: string
  applied_profile_digest?: string
  drift: boolean
  warnings: string[]
  status: GatewayStatus
  status_error?: string
  doctor: DoctorCheck[]
  doctor_healthy: boolean
  leases: Lease[]
  policies: ProxyGroup[]
  providers: { proxy_providers: ProxyProvider[]; rule_providers: RuleProvider[] }
  recovery: Recovery
}

export type Source = {
  id: string
  name: string
  kind: string
  origin: string
  digest: string
  size: number
  valid: boolean
  validation?: string
  desired: boolean
  applied: boolean
  versions: Array<{ digest: string; size: number; valid: boolean; validation?: string; imported_at: string; desired: boolean; applied: boolean }>
  diff: { previous_digest?: string; proxies_added: string[]; proxies_removed: string[]; groups_added: string[]; groups_removed: string[]; proxy_providers_added: string[]; proxy_providers_removed: string[]; rule_providers_added: string[]; rule_providers_removed: string[]; rule_count_delta: number }
  imported_at: string
  inventory: {
    proxies: string[]
    proxy_providers: string[]
    proxy_groups: string[]
    rule_providers: string[]
    rule_count: number
    terminal_match: boolean
    warnings: string[]
  }
}

export type DeviceEgressMode = 'inherit_global' | 'dedicated'
export type AppliedDeviceEgressMode = DeviceEgressMode | 'legacy_fallback'
export type CompiledDevice = { id: string; mac: string; ipv4: string; profile: string; egress_mode?: AppliedDeviceEgressMode | ''; groups: Record<string, string> }
export type DevicesResponse = {
  desired_digest?: string
  applied_digest?: string
  drift: boolean
  applied: boolean
  devices: CompiledDevice[]
  desired_devices?: CompiledDevice[]
  applied_devices?: CompiledDevice[]
  changed_devices?: string[]
  leases: Lease[]
}

export type DeviceTrafficRow = {
  hostname?: string
  ip: string
  mac: string
  online: boolean
  active_connections: number
  upload: number
  download: number
  primary_egress?: string
}

export type DeviceTraffic = {
  schema_version: number
  revision: string
  sampled_at: string
  scope: 'active_sessions'
  devices: DeviceTrafficRow[]
  totals: { devices: number; active_connections: number; upload: number; download: number }
  unmatched_connections: number
  connection_error?: string
}

export type PolicyRule = {
  id: string
  match: { domains?: string[]; ip_cidrs?: string[]; protocols?: string[]; ports?: string[]; rule_sets?: string[] }
  action?: string
  policies?: string[]
  on_unsupported?: string
}
export type PolicyProfile = { id: string; template?: string; default_policies: string[]; on_unsupported?: string; rules?: PolicyRule[] }
export type PolicyDevice = { id: string; mac: string; ipv4: string; profile: string; egress_mode?: DeviceEgressMode }
export type PolicyTemplate = { id: string; default_policies: string[]; on_unsupported?: string; rules?: PolicyRule[] }
export type PolicyRuleSet = { id: string; type?: 'inline' | 'http'; behavior: 'domain' | 'ipcidr' | 'classical'; format?: string; url?: string; interval?: number; payload?: string[] }
export type PolicySet = { devices: PolicyDevice[]; profiles: PolicyProfile[]; templates: PolicyTemplate[]; rule_sets: PolicyRuleSet[] }
export type DevicePolicyDocument = { schema_version: number; revision: string; policy: PolicySet }

export type APIError = { error?: { code?: string; message?: string } }
export type Diagnostics = { schema_version: number; revision: string; connections: { upload_total: number; download_total: number; connections: Array<{ id: string; upload: number; download: number; rule?: string; chains?: string[]; metadata?: Record<string, unknown> }> }; connection_error?: string; logs: Record<string, string[]>; operations: Array<{ id: string; kind: string; state: string; error?: string; created_at: string; updated_at: string }>; recovery: Recovery }
