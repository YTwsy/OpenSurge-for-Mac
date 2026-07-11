package controlapi

import (
	"time"

	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/doctor"
	"open-mihomo-gateway/internal/gateway"
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
	SchemaVersion int                      `json:"schema_version"`
	Revision      string                   `json:"revision"`
	DesiredDigest string                   `json:"desired_digest,omitempty"`
	AppliedDigest string                   `json:"applied_digest,omitempty"`
	Warnings      []string                 `json:"warnings"`
	Status        gateway.Status           `json:"status"`
	StatusError   string                   `json:"status_error,omitempty"`
	Doctor        []doctor.Check           `json:"doctor"`
	DoctorHealthy bool                     `json:"doctor_healthy"`
	Leases        []device.Client          `json:"leases"`
	Policies      []mihomo.ProxyGroup      `json:"policies"`
	Providers     mihomo.ProvidersSnapshot `json:"providers"`
	Recovery      RecoveryState            `json:"recovery"`
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
	SchemaVersion  int       `json:"schema_version"`
	Stage          string    `json:"stage"`
	Topology       string    `json:"topology,omitempty"`
	NetworkService string    `json:"network_service,omitempty"`
	OriginalIPv4   string    `json:"original_ipv4,omitempty"`
	OriginalRouter string    `json:"original_router,omitempty"`
	RecoveryNotes  string    `json:"recovery_notes,omitempty"`
	Required       bool      `json:"required"`
	UpdatedAt      time.Time `json:"updated_at"`
}

const (
	RecoveryIdle                        = "idle"
	RecoveryPrepared                    = "prepared"
	RecoveryMacStatic                   = "mac_static"
	RecoveryRouterDHCPDisabledConfirmed = "router_dhcp_disabled_confirmed"
	RecoveryGatewayActive               = "gateway_active"
	RecoveryGatewayStopped              = "gateway_stopped_waiting_router_dhcp"
	RecoveryRouterDHCPRestored          = "router_dhcp_restored"
	RecoveryComplete                    = "complete"
)

type RecoveryUpdate struct {
	Stage          string `json:"stage"`
	NetworkService string `json:"network_service,omitempty"`
	OriginalIPv4   string `json:"original_ipv4,omitempty"`
	OriginalRouter string `json:"original_router,omitempty"`
	RecoveryNotes  string `json:"recovery_notes,omitempty"`
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
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Kind          string    `json:"kind"`
	Origin        string    `json:"origin"`
	FetchURL      string    `json:"fetch_url,omitempty"`
	SnapshotPath  string    `json:"snapshot_path,omitempty"`
	Digest        string    `json:"digest"`
	Size          int64     `json:"size"`
	Valid         bool      `json:"valid"`
	Validation    string    `json:"validation,omitempty"`
	Inventory     Inventory `json:"inventory"`
	ImportedAt    time.Time `json:"imported_at"`
	Applied       bool      `json:"applied"`
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
	SchemaVersion int                     `json:"schema_version"`
	DesiredDigest string                  `json:"desired_digest,omitempty"`
	AppliedDigest string                  `json:"applied_digest,omitempty"`
	Drift         bool                    `json:"drift"`
	Applied       bool                    `json:"applied"`
	Devices       []device.CompiledDevice `json:"devices"`
	Leases        []device.Client         `json:"leases"`
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
