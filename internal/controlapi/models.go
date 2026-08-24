package controlapi

import (
	"time"

	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/doctor"
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/macosnetwork"
	"open-mihomo-gateway/internal/mihomo"
)

const SchemaVersion = 1

type APIError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type ErrorResponse struct {
	SchemaVersion int      `json:"schema_version"`
	Error         APIError `json:"error"`
}

type Overview struct {
	SchemaVersion        int                      `json:"schema_version"`
	Revision             string                   `json:"revision"`
	Topology             string                   `json:"topology"`
	DesiredDigest        string                   `json:"desired_digest,omitempty"`
	AppliedDigest        string                   `json:"applied_digest,omitempty"`
	DesiredProfileDigest string                   `json:"desired_profile_digest,omitempty"`
	AppliedProfileDigest string                   `json:"applied_profile_digest,omitempty"`
	Drift                bool                     `json:"drift"`
	Warnings             []string                 `json:"warnings"`
	Status               gateway.Status           `json:"status"`
	StatusError          string                   `json:"status_error,omitempty"`
	Doctor               []doctor.Check           `json:"doctor"`
	DoctorHealthy        bool                     `json:"doctor_healthy"`
	DoctorStatus         DoctorRunStatus          `json:"doctor_status"`
	Leases               []device.Client          `json:"leases"`
	Policies             []mihomo.ProxyGroup      `json:"policies"`
	Providers            mihomo.ProvidersSnapshot `json:"providers"`
	Recovery             RecoveryState            `json:"recovery"`
	MihomoRecovery       MihomoRecoveryStatus     `json:"mihomo_recovery"`
	SleepPrevention      SleepPreventionStatus    `json:"sleep_prevention"`
	UIPreferences        UIPreferences            `json:"ui_preferences"`
}

type DoctorRunStatus struct {
	SchemaVersion int            `json:"schema_version"`
	State         string         `json:"state"`
	Revision      string         `json:"revision,omitempty"`
	Current       bool           `json:"current"`
	Checks        []doctor.Check `json:"checks"`
	Healthy       bool           `json:"healthy"`
	Error         string         `json:"error,omitempty"`
	StartedAt     *time.Time     `json:"started_at,omitempty"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
}

type MenuBarStatus struct {
	SchemaVersion   int                   `json:"schema_version"`
	Revision        string                `json:"revision"`
	Gateway         string                `json:"gateway"`
	Topology        string                `json:"topology"`
	LANIP           string                `json:"lan_ip"`
	DHCP            string                `json:"dhcp"`
	Mihomo          string                `json:"mihomo"`
	MihomoError     string                `json:"mihomo_error,omitempty"`
	TUN             string                `json:"tun"`
	TUNInterface    string                `json:"tun_interface,omitempty"`
	TUNError        string                `json:"tun_error,omitempty"`
	PFAnchor        string                `json:"pf_anchor"`
	Forwarding      string                `json:"forwarding"`
	ClientCount     int                   `json:"client_count"`
	Drift           bool                  `json:"drift"`
	DoctorHealthy   bool                  `json:"doctor_healthy"`
	Recovery        bool                  `json:"recovery_required"`
	RecoveryStage   string                `json:"recovery_stage,omitempty"`
	Warnings        []string              `json:"warnings"`
	ErrorCode       string                `json:"error_code,omitempty"`
	MihomoRecovery  MihomoRecoveryStatus  `json:"mihomo_recovery"`
	SleepPrevention SleepPreventionStatus `json:"sleep_prevention"`
	UIPreferences   UIPreferences         `json:"ui_preferences"`
}

const (
	UILanguageSystem = "system"
	UILanguageZHCHS  = "zh-Hans"
	UILanguageEN     = "en"
)

type UIPreferences struct {
	SchemaVersion int    `json:"schema_version"`
	Language      string `json:"language"`
}

type MihomoRecoveryStatus struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

type SleepPreventionStatus struct {
	Enabled bool   `json:"enabled"`
	Active  bool   `json:"active"`
	Error   string `json:"error,omitempty"`
}

type RecoveryState struct {
	SchemaVersion           int                    `json:"schema_version"`
	Stage                   string                 `json:"stage"`
	Topology                string                 `json:"topology,omitempty"`
	NetworkService          string                 `json:"network_service,omitempty"`
	OriginalIPv4            string                 `json:"original_ipv4,omitempty"`
	OriginalRouter          string                 `json:"original_router,omitempty"`
	RecoveryNotes           string                 `json:"recovery_notes,omitempty"`
	NetworkSnapshot         *macosnetwork.Snapshot `json:"network_snapshot,omitempty"`
	ClientValidationSkipped bool                   `json:"client_validation_skipped,omitempty"`
	Required                bool                   `json:"required"`
	UpdatedAt               time.Time              `json:"updated_at"`
}

type GatewayPlanRequest struct {
	NetworkService     string `json:"network_service,omitempty"`
	RouterDHCPDisabled bool   `json:"router_dhcp_disabled,omitempty"`
}

type GatewayPlan struct {
	SchemaVersion int                   `json:"schema_version"`
	Revision      string                `json:"revision"`
	Topology      string                `json:"topology"`
	Snapshot      macosnetwork.Snapshot `json:"snapshot"`
	ProtectedIPv4 []string              `json:"protected_ipv4"`
	DHCPServers   []string              `json:"dhcp_servers"`
	Warnings      []string              `json:"warnings"`
	Blockers      []string              `json:"blockers"`
}

type NetworkActionResponse struct {
	SchemaVersion int           `json:"schema_version"`
	Recovery      RecoveryState `json:"recovery"`
	DHCPServers   []string      `json:"dhcp_servers,omitempty"`
}

type NetworkInterfacesResponse struct {
	SchemaVersion int                            `json:"schema_version"`
	Interfaces    []macosnetwork.InterfaceOption `json:"interfaces"`
}

type NetworkDefaultsResponse struct {
	SchemaVersion  int                   `json:"schema_version"`
	Mode           string                `json:"mode"`
	Snapshot       macosnetwork.Snapshot `json:"snapshot"`
	GatewayIPv4    string                `json:"gateway_ipv4"`
	LANPrefixLen   int                   `json:"lan_prefix_len,omitempty"`
	DHCPRangeStart string                `json:"dhcp_range_start,omitempty"`
	DHCPRangeEnd   string                `json:"dhcp_range_end,omitempty"`
	BypassGateway  string                `json:"bypass_gateway,omitempty"`
	BypassDNS      []string              `json:"bypass_dns"`
	Warnings       []string              `json:"warnings"`
	Blockers       []string              `json:"blockers"`
}

type ManualRecoveryFinishRequest struct {
	RouterDHCPRestoredConfirmed bool `json:"router_dhcp_restored_confirmed"`
}

type ClientValidationSkipRequest struct {
	SkipConfirmed bool `json:"skip_confirmed"`
}

type KeepStaticFinishRequest struct {
	KeepStaticConfirmed bool `json:"keep_static_confirmed"`
}

type ControlConfig struct {
	SchemaVersion    int                         `json:"schema_version"`
	Revision         string                      `json:"revision"`
	Gateway          GatewayConfigInput          `json:"gateway"`
	DHCP             DHCPConfigInput             `json:"dhcp"`
	DNS              DNSConfigInput              `json:"dns"`
	Mihomo           MihomoConfigInput           `json:"mihomo"`
	Transparent      TransparentConfigInput      `json:"transparent"`
	LocalSystemProxy LocalSystemProxyConfigInput `json:"local_system_proxy"`
	DevicePolicy     DevicePolicyConfigInput     `json:"device_policy"`
}

type GatewayConfigInput struct {
	Mode              string `json:"mode"`
	Interface         string `json:"interface"`
	LANIP             string `json:"lan_ip"`
	LANPrefixLen      int    `json:"lan_prefix_len"`
	UpstreamInterface string `json:"upstream_interface"`
}

type DHCPConfigInput struct {
	Enabled       bool     `json:"enabled"`
	RangeStart    string   `json:"range_start"`
	RangeEnd      string   `json:"range_end"`
	LeaseTime     string   `json:"lease_time"`
	Domain        string   `json:"domain"`
	BypassGateway string   `json:"bypass_gateway"`
	BypassDNS     []string `json:"bypass_dns"`
}

type DNSConfigInput struct {
	Listen   string `json:"listen"`
	Upstream string `json:"upstream"`
	IPv6     bool   `json:"ipv6"`
}

type MihomoConfigInput struct {
	// Pointer preserves the current value when an older schema-v1 client omits
	// the field. GET responses always include an explicit boolean.
	StoreFakeIP *bool `json:"store_fake_ip,omitempty"`
}

type TransparentConfigInput struct {
	Mode              string `json:"mode"`
	StrictRoute       bool   `json:"strict_route"`
	TUNIPv6           string `json:"tun_ipv6"`
	IPv6SharedL2Ready bool   `json:"ipv6_shared_l2_ready"`
}

type LocalSystemProxyConfigInput struct {
	Enabled bool `json:"enabled"`
}

type DevicePolicyConfigInput struct {
	Enabled       bool     `json:"enabled"`
	ProtectedIPv4 []string `json:"protected_ipv4"`
}

type TailscaleSettings struct {
	Enabled                bool     `json:"enabled"`
	DisplayName            string   `json:"display_name"`
	Hostname               string   `json:"hostname"`
	ControlURL             string   `json:"control_url"`
	AcceptRoutes           bool     `json:"accept_routes"`
	MagicDNSSuffixes       []string `json:"magic_dns_suffixes"`
	PeerCIDRs              []string `json:"peer_cidrs"`
	SubnetRoutes           []string `json:"subnet_routes"`
	AllowMac               bool     `json:"allow_mac"`
	AllowAllDevices        bool     `json:"allow_all_devices"`
	AllowedDevices         []string `json:"allowed_devices"`
	ExitNode               string   `json:"exit_node"`
	ExitNodeAllowLANAccess bool     `json:"exit_node_allow_lan_access"`
}

type TailscaleUpdateRequest struct {
	TailscaleSettings
	// AuthKey is write-only. It is accepted only when replacing the stored key
	// and is never included in GET or PUT responses.
	AuthKey string `json:"auth_key,omitempty"`
}

type TailscaleResponse struct {
	SchemaVersion   int               `json:"schema_version"`
	Revision        string            `json:"revision"`
	Settings        TailscaleSettings `json:"settings"`
	AuthKeyPresent  bool              `json:"auth_key_present"`
	IdentityPresent bool              `json:"identity_present"`
	GatewayActive   bool              `json:"gateway_active"`
	RuntimeState    string            `json:"runtime_state"`
	SelectableExit  bool              `json:"selectable_exit"`
	Warnings        []string          `json:"warnings"`
}

const (
	RecoveryIdle                        = "idle"
	RecoveryPrepared                    = "prepared"
	RecoveryMacStatic                   = "mac_static"
	RecoveryRouterDHCPDisabledConfirmed = "router_dhcp_disabled_confirmed"
	RecoveryGatewayActive               = "gateway_active"
	RecoveryClientValidated             = "client_validated"
	RecoveryClientValidationSkipped     = "client_validation_skipped"
	RecoveryGatewayStopped              = "gateway_stopped_waiting_router_dhcp"
	RecoveryRouterDHCPRestored          = "router_dhcp_restored"
	RecoveryComplete                    = "complete"
	RecoveryCompleteStatic              = "complete_static"
)

type RecoveryUpdate struct {
	Stage          string `json:"stage"`
	NetworkService string `json:"network_service,omitempty"`
	OriginalIPv4   string `json:"original_ipv4,omitempty"`
	OriginalRouter string `json:"original_router,omitempty"`
	RecoveryNotes  string `json:"recovery_notes,omitempty"`
}

type ClientAcceptanceRequest struct {
	ClientIPv4                 string `json:"client_ipv4"`
	GatewayDNSConfirmed        bool   `json:"gateway_dns_confirmed"`
	NoExplicitProxyConfirmed   bool   `json:"no_explicit_proxy_confirmed"`
	IPv6BypassWarningConfirmed bool   `json:"ipv6_bypass_warning_confirmed"`
}

type Operation struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	State         string    `json:"state"`
	Error         string    `json:"error,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Source struct {
	SchemaVersion       int             `json:"schema_version"`
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	Kind                string          `json:"kind"`
	Origin              string          `json:"origin"`
	FetchURL            string          `json:"fetch_url,omitempty"`
	SnapshotPath        string          `json:"snapshot_path,omitempty"`
	SnapshotDisplayPath string          `json:"snapshot_display_path,omitempty"`
	Digest              string          `json:"digest"`
	Size                int64           `json:"size"`
	Valid               bool            `json:"valid"`
	Validation          string          `json:"validation,omitempty"`
	Inventory           Inventory       `json:"inventory"`
	ImportedAt          time.Time       `json:"imported_at"`
	Desired             bool            `json:"desired"`
	Applied             bool            `json:"applied"`
	Versions            []SourceVersion `json:"versions"`
	Diff                SourceDiff      `json:"diff"`
	EffectiveDigest     string          `json:"effective_digest,omitempty"`
	EffectiveInventory  Inventory       `json:"effective_inventory"`
	OverlayCompatible   bool            `json:"overlay_compatible"`
	OverlayValidation   string          `json:"overlay_validation,omitempty"`
}

type SourceSnapshotFile struct {
	SchemaVersion int    `json:"schema_version"`
	SourceID      string `json:"source_id"`
	Kind          string `json:"kind"`
	Path          string `json:"path"`
	DisplayPath   string `json:"display_path"`
}

type SourceVersion struct {
	Digest       string    `json:"digest"`
	Size         int64     `json:"size"`
	Valid        bool      `json:"valid"`
	Validation   string    `json:"validation,omitempty"`
	Inventory    Inventory `json:"inventory"`
	ImportedAt   time.Time `json:"imported_at"`
	Desired      bool      `json:"desired"`
	Applied      bool      `json:"applied"`
	SnapshotPath string    `json:"snapshot_path,omitempty"`
}

type SourceDiff struct {
	PreviousDigest        string   `json:"previous_digest,omitempty"`
	ProxiesAdded          []string `json:"proxies_added"`
	ProxiesRemoved        []string `json:"proxies_removed"`
	GroupsAdded           []string `json:"groups_added"`
	GroupsRemoved         []string `json:"groups_removed"`
	ProxyProvidersAdded   []string `json:"proxy_providers_added"`
	ProxyProvidersRemoved []string `json:"proxy_providers_removed"`
	RuleProvidersAdded    []string `json:"rule_providers_added"`
	RuleProvidersRemoved  []string `json:"rule_providers_removed"`
	RuleCountDelta        int      `json:"rule_count_delta"`
}

type Inventory struct {
	Proxies        []string `json:"proxies"`
	ProxyProviders []string `json:"proxy_providers"`
	ProxyGroups    []string `json:"proxy_groups"`
	RuleProviders  []string `json:"rule_providers"`
	RuleCount      int      `json:"rule_count"`
	TerminalMatch  bool     `json:"terminal_match"`
	Warnings       []string `json:"warnings"`
}

type SourceImportRequest struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

type ProfileOverlayResponse struct {
	SchemaVersion int                           `json:"schema_version"`
	Revision      string                        `json:"revision"`
	YAML          string                        `json:"yaml"`
	Document      mihomo.ProfileOverlayDocument `json:"document"`
	Desired       bool                          `json:"desired"`
	Applied       bool                          `json:"applied"`
	Validation    string                        `json:"validation"`
}

type ProfileOverlaySaveRequest struct {
	YAML     *string                        `json:"yaml,omitempty"`
	Document *mihomo.ProfileOverlayDocument `json:"document,omitempty"`
}

type ProfileOverlayPreview struct {
	SchemaVersion        int        `json:"schema_version"`
	SourceID             string     `json:"source_id"`
	SourceYAML           string     `json:"source_yaml"`
	OverlayYAML          string     `json:"overlay_yaml"`
	EffectiveProfileYAML string     `json:"effective_profile_yaml"`
	FinalMihomoYAML      string     `json:"final_mihomo_yaml"`
	OriginalInventory    Inventory  `json:"original_inventory"`
	EffectiveInventory   Inventory  `json:"effective_inventory"`
	Diff                 SourceDiff `json:"diff"`
	Validation           string     `json:"validation"`
}

type SelectionRequest struct {
	Policy string `json:"policy"`
}

type LocalRoutingRequest struct {
	Mode         string `json:"mode"`
	GlobalPolicy string `json:"global_policy,omitempty"`
}

type LocalRoutingResponse struct {
	SchemaVersion int `json:"schema_version"`
	mihomo.LocalRoutingSnapshot
}

type ConnectionRefreshResponse struct {
	SchemaVersion      int    `json:"schema_version"`
	Scope              string `json:"scope"`
	DeviceID           string `json:"device_id,omitempty"`
	MatchedConnections int    `json:"matched_connections"`
	ClosedConnections  int    `json:"closed_connections"`
}

type DevicesResponse struct {
	SchemaVersion  int                     `json:"schema_version"`
	DesiredDigest  string                  `json:"desired_digest,omitempty"`
	AppliedDigest  string                  `json:"applied_digest,omitempty"`
	Drift          bool                    `json:"drift"`
	Applied        bool                    `json:"applied"`
	Devices        []device.CompiledDevice `json:"devices"` // legacy running view
	DesiredDevices []device.CompiledDevice `json:"desired_devices"`
	AppliedDevices []device.CompiledDevice `json:"applied_devices"`
	ChangedDevices []string                `json:"changed_devices"`
	// OutOfLANDevices are registrations whose IPv4 is not on the configured LAN.
	// They stay in the policy document but cannot be served, so the UI offers
	// re-registration or removal instead of the gateway refusing to start.
	OutOfLANDevices  []string         `json:"out_of_lan_devices"`
	LANPrefix        string           `json:"lan_prefix,omitempty"`
	Leases           []device.Client  `json:"leases"`
	ObservedDevices  []ObservedDevice `json:"observed_devices"`
	ObservationError string           `json:"observation_error,omitempty"`
}

// ObservedDevice is a currently active same-LAN source seen by mihomo. A MAC
// is included only when the macOS ARP cache contains a matching neighbor; this
// remains observation evidence rather than DHCP-backed identity proof.
type ObservedDevice struct {
	IP                string `json:"ip"`
	MAC               string `json:"mac,omitempty"`
	ActiveConnections int    `json:"active_connections"`
	NeighborObserved  bool   `json:"neighbor_observed"`
}

// DeviceTrafficResponse is a point-in-time aggregation of the currently
// active mihomo sessions. GatewayLocal is kept separate from Devices so the
// Mac can be shown alongside downstream traffic without becoming part of the
// LAN device inventory or policy identity model. Counters are session-lifetime
// counters from mihomo, not persisted history.
type DeviceTrafficResponse struct {
	SchemaVersion                 int                 `json:"schema_version"`
	Revision                      string              `json:"revision"`
	SampledAt                     time.Time           `json:"sampled_at"`
	Scope                         string              `json:"scope"`
	GatewayLocal                  DeviceTraffic       `json:"gateway_local"`
	Devices                       []DeviceTraffic     `json:"devices"`
	Totals                        DeviceTrafficTotals `json:"totals"`
	GatewayRates                  TrafficRates        `json:"gateway_rates"`
	UnidentifiedDeviceConnections int                 `json:"unidentified_device_connections"`
	UnclassifiedConnections       int                 `json:"unclassified_connections"`
	UnmatchedConnections          int                 `json:"unmatched_connections"`
	ConnectionError               string              `json:"connection_error,omitempty"`
}

type DeviceTraffic struct {
	Name              string `json:"name,omitempty"`
	Hostname          string `json:"hostname,omitempty"`
	IP                string `json:"ip"`
	MAC               string `json:"mac"`
	Online            bool   `json:"online"`
	ActiveConnections int    `json:"active_connections"`
	Upload            int64  `json:"upload"`
	Download          int64  `json:"download"`
	UploadRate        int64  `json:"upload_rate"`
	DownloadRate      int64  `json:"download_rate"`
	PrimaryEgress     string `json:"primary_egress,omitempty"`
	IdentitySource    string `json:"identity_source"`
	Transport         string `json:"transport,omitempty"`
	GatewayTarget     string `json:"gateway_target,omitempty"`
	IPv6Blocked       bool   `json:"ipv6_blocked,omitempty"`
}

type DeviceTrafficTotals struct {
	Devices           int   `json:"devices"`
	ActiveConnections int   `json:"active_connections"`
	Upload            int64 `json:"upload"`
	Download          int64 `json:"download"`
	UploadRate        int64 `json:"upload_rate"`
	DownloadRate      int64 `json:"download_rate"`
}

type TrafficRates struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
}

type DevicePolicyResponse struct {
	SchemaVersion int              `json:"schema_version"`
	Revision      string           `json:"revision"`
	Policy        device.PolicySet `json:"policy"`
}

type BootstrapResponse struct {
	SchemaVersion int       `json:"schema_version"`
	URL           string    `json:"url"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type StateEvent struct {
	SchemaVersion        int                   `json:"schema_version"`
	Revision             string                `json:"revision"`
	Gateway              string                `json:"gateway"`
	DesiredDigest        string                `json:"desired_digest,omitempty"`
	AppliedDigest        string                `json:"applied_digest,omitempty"`
	DesiredProfileDigest string                `json:"desired_profile_digest,omitempty"`
	AppliedProfileDigest string                `json:"applied_profile_digest,omitempty"`
	Drift                bool                  `json:"drift"`
	Recovery             RecoveryState         `json:"recovery"`
	SleepPrevention      SleepPreventionStatus `json:"sleep_prevention"`
	UIPreferences        UIPreferences         `json:"ui_preferences"`
	At                   time.Time             `json:"at"`
}

type DiagnosticsResponse struct {
	SchemaVersion   int                        `json:"schema_version"`
	Revision        string                     `json:"revision"`
	Connections     mihomo.ConnectionsSnapshot `json:"connections"`
	ConnectionError string                     `json:"connection_error,omitempty"`
	Logs            map[string][]string        `json:"logs"`
	Operations      []Operation                `json:"operations"`
	Recovery        RecoveryState              `json:"recovery"`
}
