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
	Leases               []device.Client          `json:"leases"`
	Policies             []mihomo.ProxyGroup      `json:"policies"`
	Providers            mihomo.ProvidersSnapshot `json:"providers"`
	Recovery             RecoveryState            `json:"recovery"`
}

type MenuBarStatus struct {
	SchemaVersion int      `json:"schema_version"`
	Revision      string   `json:"revision"`
	Gateway       string   `json:"gateway"`
	Topology      string   `json:"topology"`
	LANIP         string   `json:"lan_ip"`
	DHCP          string   `json:"dhcp"`
	Mihomo        string   `json:"mihomo"`
	PFAnchor      string   `json:"pf_anchor"`
	Forwarding    string   `json:"forwarding"`
	ClientCount   int      `json:"client_count"`
	Drift         bool     `json:"drift"`
	DoctorHealthy bool     `json:"doctor_healthy"`
	Recovery      bool     `json:"recovery_required"`
	RecoveryStage string   `json:"recovery_stage,omitempty"`
	Warnings      []string `json:"warnings"`
	ErrorCode     string   `json:"error_code,omitempty"`
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
	SchemaVersion int                     `json:"schema_version"`
	Revision      string                  `json:"revision"`
	Gateway       GatewayConfigInput      `json:"gateway"`
	DHCP          DHCPConfigInput         `json:"dhcp"`
	DNS           DNSConfigInput          `json:"dns"`
	Transparent   TransparentConfigInput  `json:"transparent"`
	DevicePolicy  DevicePolicyConfigInput `json:"device_policy"`
}

type GatewayConfigInput struct {
	Mode              string `json:"mode"`
	Interface         string `json:"interface"`
	LANIP             string `json:"lan_ip"`
	UpstreamInterface string `json:"upstream_interface"`
}

type DHCPConfigInput struct {
	Enabled    bool   `json:"enabled"`
	RangeStart string `json:"range_start"`
	RangeEnd   string `json:"range_end"`
	LeaseTime  string `json:"lease_time"`
	Domain     string `json:"domain"`
}

type DNSConfigInput struct {
	Listen   string `json:"listen"`
	Upstream string `json:"upstream"`
}

type TransparentConfigInput struct {
	Mode        string `json:"mode"`
	StrictRoute bool   `json:"strict_route"`
}

type DevicePolicyConfigInput struct {
	Enabled       bool     `json:"enabled"`
	ProtectedIPv4 []string `json:"protected_ipv4"`
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
	SchemaVersion int             `json:"schema_version"`
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Kind          string          `json:"kind"`
	Origin        string          `json:"origin"`
	FetchURL      string          `json:"fetch_url,omitempty"`
	SnapshotPath  string          `json:"snapshot_path,omitempty"`
	Digest        string          `json:"digest"`
	Size          int64           `json:"size"`
	Valid         bool            `json:"valid"`
	Validation    string          `json:"validation,omitempty"`
	Inventory     Inventory       `json:"inventory"`
	ImportedAt    time.Time       `json:"imported_at"`
	Desired       bool            `json:"desired"`
	Applied       bool            `json:"applied"`
	Versions      []SourceVersion `json:"versions"`
	Diff          SourceDiff      `json:"diff"`
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

type SelectionRequest struct {
	Policy string `json:"policy"`
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
	Leases         []device.Client         `json:"leases"`
}

// DeviceTrafficResponse is a point-in-time aggregation of the currently
// active mihomo sessions that can be attributed to an OpenSurge DHCP lease.
// Counters are session-lifetime counters from mihomo, not persisted history.
type DeviceTrafficResponse struct {
	SchemaVersion        int                 `json:"schema_version"`
	Revision             string              `json:"revision"`
	SampledAt            time.Time           `json:"sampled_at"`
	Scope                string              `json:"scope"`
	Devices              []DeviceTraffic     `json:"devices"`
	Totals               DeviceTrafficTotals `json:"totals"`
	GatewayRates         TrafficRates        `json:"gateway_rates"`
	UnmatchedConnections int                 `json:"unmatched_connections"`
	ConnectionError      string              `json:"connection_error,omitempty"`
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
	SchemaVersion        int           `json:"schema_version"`
	Revision             string        `json:"revision"`
	Gateway              string        `json:"gateway"`
	DesiredDigest        string        `json:"desired_digest,omitempty"`
	AppliedDigest        string        `json:"applied_digest,omitempty"`
	DesiredProfileDigest string        `json:"desired_profile_digest,omitempty"`
	AppliedProfileDigest string        `json:"applied_profile_digest,omitempty"`
	Drift                bool          `json:"drift"`
	Recovery             RecoveryState `json:"recovery"`
	At                   time.Time     `json:"at"`
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
