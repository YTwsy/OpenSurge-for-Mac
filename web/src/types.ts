export type GatewayStatus = {
  gateway: string
  runtime_state?: 'none' | 'active' | 'interrupted'
  interface: string
  lan_ip: string
  dhcp: string
  dhcp_enabled: boolean
  mihomo: string
  mihomo_error?: string
  tun?: string
  tun_interface?: string
  tun_error?: string
  pf_anchor: string
  forwarding: string
  dns_ipv6: boolean
  tun_ipv6_requested: 'off' | 'auto' | 'always'
  ipv6_packet: 'disabled' | 'stopped' | 'ready' | 'failed'
  native_ipv6_available: boolean
  ipv6_reason?: string
  client_count: number
}

export type DoctorCheck = { name: string; ok: boolean; message?: string }
export type DoctorRunStatus = {
  schema_version: number
  state: 'idle' | 'running' | 'succeeded' | 'failed'
  revision?: string
  current: boolean
  checks: DoctorCheck[]
  healthy: boolean
  error?: string
  started_at?: string
  completed_at?: string
}
export type Lease = { ip: string; mac: string; hostname?: string; registered_name?: string; expires_at: string; online: boolean }
export type ProxyGroup = { name: string; type: string; selected: string; options: string[] }
export type LocalRoutingMode = 'rule' | 'global' | 'direct'
export type LocalRouting = {
  schema_version: number
  mode: LocalRoutingMode
  available_modes: LocalRoutingMode[]
  global_group?: ProxyGroup
  udp_behavior: 'rules' | 'proxy' | 'direct' | 'reject'
  transports: Array<'tun' | 'loopback_explicit_proxy'>
  new_connections_only: boolean
  consistent: boolean
  warning?: string
}
export type ProxyHealthEntry = {
  name: string
  display_name?: string
  role?: 'tailnet' | 'exit_node'
  type: string
  selected?: string
  provider?: string
  udp: boolean
  status: 'untested' | 'reachable' | 'unreachable' | 'timeout' | 'error' | 'not_applicable' | 'available_on_demand'
  delay_ms?: number
  tested_at?: string
  probeable: boolean
  error?: string
}
export type ProxyHealthSnapshot = { schema_version: number; test_url: string; proxies: ProxyHealthEntry[] }
export type ProxyHealthTestResponse = {
  schema_version: number
  test_url: string
  results: Array<{ name: string; status: ProxyHealthEntry['status']; delay_ms?: number; tested_at: string; test_url: string; error?: string }>
}
export type ProviderProxy = { name: string; type: string; alive: boolean }
export type ProxyProvider = { name: string; type: string; vehicle_type: string; updated_at?: string; proxy_count: number; proxies: ProviderProxy[] }
export type RuleProvider = { name: string; type: string; vehicle_type: string; behavior?: string; updated_at?: string; rule_count: number }
export type NetworkSnapshot = { network_service: string; interface: string; hardware_address?: string; ipv4?: string; subnet_mask?: string; router?: string; dns: string[]; ipv6_default: boolean; ipv6_default_self_only?: boolean }
export type NetworkInterfaceOption = { interface: string; network_service: string; ipv6_link_local?: string }
export type NetworkInterfacesResponse = { schema_version: number; interfaces: NetworkInterfaceOption[] }
export type NetworkDefaults = { schema_version: number; mode: 'same_lan' | 'same_wifi_dhcp'; snapshot: NetworkSnapshot; gateway_ipv4: string; lan_prefix_len?: number; dhcp_range_start?: string; dhcp_range_end?: string; bypass_gateway?: string; bypass_dns: string[]; warnings: string[]; blockers: string[] }
export type Recovery = { stage: string; topology?: string; required: boolean; updated_at?: string; recovery_notes?: string; network_snapshot?: NetworkSnapshot; client_validation_skipped?: boolean }
export type GatewayPlan = { schema_version: number; revision: string; topology: string; snapshot: NetworkSnapshot; protected_ipv4: string[]; dhcp_servers: string[]; warnings: string[]; blockers: string[] }
export type Operation = { id: string; kind: string; state: string; error?: string }
export type MihomoRecoveryStatus = { state: 'idle' | 'observing' | 'recovering' | 'failed'; reason?: 'process_missing' | 'controller_refused'; error?: string }
export type SleepPreventionStatus = { enabled: boolean; active: boolean; error?: string }
export type ControlConfig = {
  schema_version: number; revision: string
  gateway: { mode: 'same_lan' | 'same_wifi_dhcp' | 'isolated_lan'; interface: string; lan_ip: string; lan_prefix_len: number; upstream_interface: string }
  dhcp: { enabled: boolean; range_start: string; range_end: string; lease_time: string; domain: string; bypass_gateway: string; bypass_dns: string[] }
  dns: { listen: string; upstream: string; ipv6: boolean }
  mihomo: { store_fake_ip: boolean }
  transparent: { mode: 'off' | 'tun'; strict_route: boolean; tun_ipv6: 'off' | 'auto' | 'always'; ipv6_shared_l2_ready?: boolean }
  local_system_proxy: { enabled: boolean }
  device_policy: { enabled: boolean; protected_ipv4: string[] }
}

export type TailscaleSettings = {
  enabled: boolean
  display_name: string
  hostname: string
  control_url: string
  accept_routes: boolean
  magic_dns_suffixes: string[]
  peer_cidrs: string[]
  subnet_routes: string[]
  allow_mac: boolean
  allow_all_devices: boolean
  allowed_devices: string[]
  exit_node: string
  exit_node_allow_lan_access: boolean
}

export type TailscaleUpdate = TailscaleSettings & { auth_key?: string }

export type TailscaleResponse = {
  schema_version: number
  revision: string
  settings: TailscaleSettings
  auth_key_present: boolean
  identity_present: boolean
  gateway_active: boolean
  runtime_state: 'disabled' | 'pending_gateway_start' | 'available_on_demand'
  selectable_exit: boolean
  warnings: string[]
}

export type TailscaleDiscoveredNode = {
  id: string
  name: string
  dns_name?: string
  tailscale_ips: string[]
  online: boolean
  exit_node: boolean
  exit_node_option: boolean
  subnet_routes: string[]
}

export type TailscaleDiscoveryResponse = {
  schema_version: number
  available: boolean
  backend_state?: string
  tailnet_name?: string
  magic_dns: boolean
  magic_dns_suffix?: string
  self?: TailscaleDiscoveredNode
  peers: TailscaleDiscoveredNode[]
  error?: string
}

export type Overview = {
  schema_version: number
  revision: string
  topology: string
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
  doctor_status?: DoctorRunStatus
  leases: Lease[]
  policies: ProxyGroup[]
  providers: { proxy_providers: ProxyProvider[]; rule_providers: RuleProvider[] }
  recovery: Recovery
  mihomo_recovery?: MihomoRecoveryStatus
  sleep_prevention?: SleepPreventionStatus
  ui_preferences?: UIPreferences
}

export type RequestedLanguage = 'system' | 'zh-Hans' | 'en'
export type UIPreferences = { schema_version: number; language: RequestedLanguage }

export type Source = {
  id: string
  name: string
  kind: string
  origin: string
  snapshot_display_path?: string
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
  effective_digest?: string
  effective_inventory?: SourceInventory
  overlay_compatible?: boolean
  overlay_validation?: string
}

export type SourceInventory = {
  proxies: string[]
  proxy_providers: string[]
  proxy_groups: string[]
  rule_providers: string[]
  rule_count: number
  terminal_match: boolean
  warnings: string[]
}

export type ProfileOverlayRuleOps = { prepend: string[]; append_before_match: string[] }
export type ProfileOverlaySequenceOps = { add: Array<Record<string, unknown>>; replace: Array<Record<string, unknown>> }
export type ProfileOverlayMappingOps = { add: Record<string, Record<string, unknown>>; replace: Record<string, Record<string, unknown>> }
export type ProfileOverlayGroupPatch = { name: string; append_proxies: string[]; append_use: string[] }
export type ProfileOverlayDocument = {
  schema_version: number
  enabled: boolean
  rules: ProfileOverlayRuleOps
  proxies: ProfileOverlaySequenceOps
  proxy_providers: ProfileOverlayMappingOps
  proxy_groups: ProfileOverlaySequenceOps & { patch: ProfileOverlayGroupPatch[] }
  rule_providers: ProfileOverlayMappingOps
  dns: { merge: Record<string, unknown>; append: Record<string, unknown[]> }
}
export type ProfileOverlay = {
  schema_version: number
  revision: string
  yaml: string
  document: ProfileOverlayDocument
  desired: boolean
  applied: boolean
  validation: string
}
export type ProfileOverlayPreview = {
  schema_version: number
  source_id: string
  source_yaml: string
  overlay_yaml: string
  effective_profile_yaml: string
  final_mihomo_yaml: string
  original_inventory: SourceInventory
  effective_inventory: SourceInventory
  diff: Source['diff']
  validation: string
}

export type SourceSnapshotFile = {
  schema_version: number
  source_id: string
  kind: 'managed_snapshot' | 'editable_export'
  path: string
  display_path: string
}

export type DeviceEgressMode = 'inherit_global' | 'dedicated'
export type AppliedDeviceEgressMode = DeviceEgressMode | 'legacy_fallback'
export type DeviceGatewayTarget = 'opensurge' | 'upstream_router'
export type CompiledDevice = { id: string; mac: string; ipv4: string; profile: string; gateway_target?: DeviceGatewayTarget | ''; egress_mode?: AppliedDeviceEgressMode | ''; ipv6_blocked?: boolean; groups: Record<string, string> }
export type ObservedDevice = { ip: string; mac?: string; active_connections: number; neighbor_observed: boolean }
export type DevicesResponse = {
  desired_digest?: string
  applied_digest?: string
  drift: boolean
  applied: boolean
  devices: CompiledDevice[]
  desired_devices?: CompiledDevice[]
  applied_devices?: CompiledDevice[]
  changed_devices?: string[]
  out_of_lan_devices?: string[]
  lan_prefix?: string
  leases: Lease[]
  observed_devices: ObservedDevice[]
  observation_error?: string
}

export type DeviceTrafficRow = {
  name?: string
  hostname?: string
  ip: string
  mac: string
  online: boolean
  active_connections: number
  upload: number
  download: number
  upload_rate: number
  download_rate: number
  primary_egress?: string
  identity_source?: 'dhcp_lease' | 'registered_static' | 'observed_traffic' | 'gateway_local'
  transport?: 'none' | 'tun' | 'explicit_proxy' | 'tun_and_explicit_proxy' | 'other'
  gateway_target?: DeviceGatewayTarget
  ipv6_blocked?: boolean
}

export type TrafficRates = { upload: number; download: number }

export type DeviceTraffic = {
  schema_version: number
  revision: string
  sampled_at: string
  scope: 'active_sessions'
  gateway_local: DeviceTrafficRow
  devices: DeviceTrafficRow[]
  totals: { devices: number; active_connections: number; upload: number; download: number; upload_rate: number; download_rate: number }
  gateway_rates: TrafficRates
  unidentified_device_connections: number
  unclassified_connections: number
  /** Legacy count of gateway-local plus unclassified connections. */
  unmatched_connections: number
  connection_error?: string
}

export type TrafficHistoryPoint = {
  sampled_at: string
  upload: number
  download: number
  devices: Record<string, TrafficRates>
}

export type PolicyRule = {
  id: string
  match: { domains?: string[]; ip_cidrs?: string[]; protocols?: string[]; ports?: string[]; rule_sets?: string[]; template?: string }
  action?: string
  policies?: string[]
  on_unsupported?: string
}
export type PolicyProfile = { id: string; template?: string; default_policies: string[]; on_unsupported?: string; rules?: PolicyRule[] }
export type PolicyDevice = { id: string; name?: string; mac: string; ipv4: string; profile: string; gateway_target?: DeviceGatewayTarget; egress_mode?: DeviceEgressMode }
export type PolicyTemplate = { id: string; rule_sets?: string[]; default_policies?: string[]; on_unsupported?: string; rules?: PolicyRule[] }
export type PolicyRuleSet = { id: string; type?: 'inline' | 'http'; behavior: 'domain' | 'ipcidr' | 'classical'; format?: string; url?: string; interval?: number; payload?: string[] }
export type PolicySet = { devices: PolicyDevice[]; profiles: PolicyProfile[]; templates: PolicyTemplate[]; rule_sets: PolicyRuleSet[] }
export type DevicePolicyDocument = { schema_version: number; revision: string; policy: PolicySet }

export type APIError = { error?: { code?: string; message?: string } }
export type Diagnostics = { schema_version: number; revision: string; connections: { upload_total: number; download_total: number; connections: Array<{ id: string; upload: number; download: number; rule?: string; chains?: string[]; metadata?: Record<string, unknown> }> }; connection_error?: string; logs: Record<string, string[]>; operations: Array<{ id: string; kind: string; state: string; error?: string; created_at: string; updated_at: string }>; recovery: Recovery }

export type ConnectionRefreshResult = {
  schema_version: number
  scope: 'gateway_local' | 'device'
  device_id?: string
  matched_connections: number
  closed_connections: number
}

export type ConnectivityTarget = {
  id: string
  name: string
  category: 'china' | 'global' | 'ai' | 'developer'
  symbol: string
  url: string
  expected_route: 'direct' | 'proxy' | 'reject' | 'any'
}
export type ConnectivitySample = { status: string; delay_ms?: number; http_status?: number; error?: string }
export type ConnectivityResult = {
  target_id: string
  status: string
  grade: 'excellent' | 'good' | 'slow' | 'very_slow' | 'timeout'
  median_ms?: number
  http_status?: number
  chain: string[]
  rule?: string
  rule_payload?: string
  route: 'direct' | 'proxy' | 'reject' | 'unknown'
  route_match?: boolean
  samples: ConnectivitySample[]
  tested_at: string
}
export type ConnectivityResponse = {
  schema_version: number
  source: 'gateway_mihomo'
  scope: 'local_mac_runtime'
  rounds: number
  targets: ConnectivityTarget[]
  results: ConnectivityResult[]
  started_at?: string
  completed_at?: string
}
