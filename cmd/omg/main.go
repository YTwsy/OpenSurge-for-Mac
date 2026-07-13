package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/doctor"
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
)

const defaultConfigPath = "examples/config.example.yaml"

var (
	fetchProxyGroups    = mihomo.FetchProxyGroups
	selectProxyGroup    = mihomo.SelectProxyGroup
	fetchConnections    = mihomo.FetchConnections
	fetchProviders      = mihomo.FetchProviders
	updateProxyProvider = mihomo.UpdateProxyProvider
	newGatewayManager   = func(cfg config.Config) gatewayManager {
		return gateway.New(cfg)
	}
)

type gatewayManager interface {
	Start(context.Context) error
	Stop(context.Context) error
	Reload(context.Context) error
	Status(context.Context) (gateway.Status, error)
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return 2
	}

	command := args[0]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "path to gateway config")
	outputFormat := fs.String("format", "text", "output format: text or json")
	policyGroup := fs.String("group", "", "mihomo policy group name")
	policyName := fs.String("policy", "", "mihomo policy name to select")
	deviceID := fs.String("device", "", "configured device id")
	deviceSlot := fs.String("slot", "default", "device policy slot: default or a rule id")
	providerName := fs.String("provider", "", "mihomo proxy provider name")
	logTail := fs.Int("tail", 0, "number of recent log lines to include")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	jsonOutput, err := parseOutputFormat(*outputFormat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "format: %v\n", err)
		return 2
	}

	loadConfig := config.LoadRuntime
	if commandRequiresDesiredPolicy(command) {
		loadConfig = config.Load
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		return writeErrorExit(command, jsonOutput, 1, "config", err)
	}

	ctx := context.Background()
	manager := newGatewayManager(cfg)

	switch command {
	case "start":
		if err := manager.Start(ctx); err != nil {
			return writeErrorExit(command, jsonOutput, 1, "start", err)
		}
		if jsonOutput {
			return writeJSONExit(commandResultJSON{Command: "start", OK: true, ConfigPath: *configPath})
		}
	case "stop":
		if err := manager.Stop(ctx); err != nil {
			return writeErrorExit(command, jsonOutput, 1, "stop", err)
		}
		if jsonOutput {
			return writeJSONExit(commandResultJSON{Command: "stop", OK: true, ConfigPath: *configPath})
		}
	case "reload":
		if err := manager.Reload(ctx); err != nil {
			return writeErrorExit(command, jsonOutput, 1, "reload", err)
		}
		if jsonOutput {
			return writeJSONExit(commandResultJSON{Command: "reload", OK: true, ConfigPath: *configPath})
		}
	case "status":
		status, err := manager.Status(ctx)
		if err != nil {
			return writeErrorExit(command, jsonOutput, 1, "status", err)
		}
		if jsonOutput {
			return writeJSONExit(status)
		}
		fmt.Print(status.Format())
	case "doctor":
		report := doctor.Run(cfg)
		if jsonOutput {
			return writeJSONExit(doctorJSON{
				Healthy: report.Healthy(),
				Checks:  report.Checks,
			})
		}
		fmt.Print(report.Format())
	case "leases":
		clients, err := device.LoadLeases(runtime.NewPaths(cfg).LeaseFile)
		if err != nil {
			return writeErrorExit(command, jsonOutput, 1, "leases", err)
		}
		if jsonOutput {
			return writeJSONExit(leasesJSON{Clients: clients})
		}
		fmt.Print(device.FormatClients(clients))
	case "devices":
		devices, err := configuredDevices(cfg)
		if err != nil {
			return writeErrorExit(command, jsonOutput, 1, "devices", err)
		}
		if jsonOutput {
			return writeJSONExit(devicePoliciesJSON{Devices: devices})
		}
		fmt.Print(formatConfiguredDevices(devices))
	case "logs":
		if *logTail < 0 {
			return writeErrorMessageExit(command, jsonOutput, 2, "logs: tail must be greater than or equal to 0")
		}
		paths := runtime.NewPaths(cfg)
		recent := recentLogFiles(paths, *logTail)
		if jsonOutput {
			return writeJSONExit(logsJSON{
				LogsDir:      paths.LogDir,
				DNSMasqLog:   paths.DNSMasqLog,
				MihomoLog:    paths.MihomoLog,
				StateFile:    paths.StateFile,
				MihomoConfig: paths.MihomoConfig,
				Recent:       recent,
			})
		}
		if *logTail > 0 {
			fmt.Print(formatRecentLogs(recent))
			return 0
		}
		fmt.Printf("Logs directory: %s\n", paths.LogDir)
	case "snapshot":
		if *logTail < 0 {
			return writeErrorMessageExit(command, jsonOutput, 2, "snapshot: tail must be greater than or equal to 0")
		}
		snapshot := buildSnapshot(ctx, cfg, manager, *logTail)
		if jsonOutput {
			return writeJSONExit(snapshot)
		}
		fmt.Print(formatSnapshot(snapshot))
	case "policies":
		groups, err := fetchProxyGroups(ctx, cfg)
		if err != nil {
			return writeErrorExit(command, jsonOutput, 1, "policies", err)
		}
		if jsonOutput {
			return writeJSONExit(policiesJSON{Groups: groups})
		}
		fmt.Print(formatProxyGroups(groups))
	case "policy-select":
		groups, err := fetchProxyGroups(ctx, cfg)
		if err != nil {
			return writeErrorExit(command, jsonOutput, 1, "policy-select", err)
		}
		if err := validatePolicySelection(groups, *policyGroup, *policyName); err != nil {
			return writeErrorExit(command, jsonOutput, 1, "policy-select", err)
		}
		if err := selectProxyGroup(ctx, cfg, *policyGroup, *policyName); err != nil {
			return writeErrorExit(command, jsonOutput, 1, "policy-select", err)
		}
		if jsonOutput {
			return writeJSONExit(policySelectJSON{Group: *policyGroup, Selected: *policyName})
		}
		fmt.Printf("Policy group %q selected %q\n", *policyGroup, *policyName)
	case "device-policy-select":
		bundle, err := loadAppliedPolicyBundle(cfg)
		if err != nil {
			return writeErrorExit(command, jsonOutput, 1, "device-policy-select", err)
		}
		group, err := device.DeviceGroup(bundle.Policy, *deviceID, *deviceSlot)
		if err != nil {
			return writeErrorExit(command, jsonOutput, 1, "device-policy-select", err)
		}
		groups, err := fetchProxyGroups(ctx, cfg)
		if err != nil {
			return writeErrorExit(command, jsonOutput, 1, "device-policy-select", err)
		}
		if err := validatePolicySelection(groups, group, *policyName); err != nil {
			return writeErrorExit(command, jsonOutput, 1, "device-policy-select", err)
		}
		if err := selectProxyGroup(ctx, cfg, group, *policyName); err != nil {
			return writeErrorExit(command, jsonOutput, 1, "device-policy-select", err)
		}
		result := devicePolicySelectJSON{Device: *deviceID, Slot: *deviceSlot, Group: group, Selected: *policyName}
		if jsonOutput {
			return writeJSONExit(result)
		}
		fmt.Printf("Device %q slot %q selected %q in policy group %q\n", result.Device, result.Slot, result.Selected, result.Group)
	case "connections":
		connections, err := fetchConnections(ctx, cfg)
		if err != nil {
			return writeErrorExit(command, jsonOutput, 1, "connections", err)
		}
		if jsonOutput {
			return writeJSONExit(connections)
		}
		fmt.Print(formatConnections(connections))
	case "providers":
		providers, err := fetchProviders(ctx, cfg)
		if err != nil {
			return writeErrorExit(command, jsonOutput, 1, "providers", err)
		}
		if jsonOutput {
			return writeJSONExit(providers)
		}
		fmt.Print(formatProviders(providers))
	case "provider-update":
		provider, err := updateProxyProvider(ctx, cfg, *providerName)
		if err != nil {
			return writeErrorExit(command, jsonOutput, 1, "provider-update", err)
		}
		if jsonOutput {
			return writeJSONExit(providerUpdateJSON{
				Provider:      provider.Name,
				Updated:       true,
				ProxyProvider: provider,
			})
		}
		fmt.Print(formatProviderUpdate(provider))
	case "render-mihomo":
		rendered, err := mihomo.RenderConfig(cfg)
		if err != nil {
			return writeErrorExit(command, jsonOutput, 1, "render-mihomo", err)
		}
		fmt.Print(rendered)
	case "validate-mihomo":
		paths := runtime.NewPaths(cfg)
		if err := mihomo.New(cfg, paths).ValidateConfig(); err != nil {
			return writeErrorExit(command, jsonOutput, 1, "validate-mihomo", err)
		}
		if jsonOutput {
			return writeJSONExit(validateMihomoJSON{Valid: true, MihomoConfig: paths.MihomoConfig})
		}
		fmt.Printf("mihomo config valid: %s\n", paths.MihomoConfig)
	default:
		if jsonOutput {
			return writeErrorMessageExit(command, jsonOutput, 2, fmt.Sprintf("unknown command %q", command))
		}
		fmt.Fprintf(os.Stderr, "unknown command %q\n", command)
		printUsage(os.Stderr)
		return 2
	}

	return 0
}

func commandRequiresDesiredPolicy(command string) bool {
	switch command {
	case "start", "reload", "doctor", "render-mihomo", "validate-mihomo":
		return true
	default:
		return false
	}
}

type doctorJSON struct {
	Healthy bool           `json:"healthy"`
	Checks  []doctor.Check `json:"checks"`
}

type leasesJSON struct {
	Clients []device.Client `json:"clients"`
}

type commandResultJSON struct {
	Command    string `json:"command"`
	OK         bool   `json:"ok"`
	ConfigPath string `json:"config_path"`
}

type commandErrorJSON struct {
	Command string `json:"command"`
	OK      bool   `json:"ok"`
	Error   string `json:"error"`
}

type logsJSON struct {
	LogsDir      string        `json:"logs_dir"`
	DNSMasqLog   string        `json:"dnsmasq_log"`
	MihomoLog    string        `json:"mihomo_log"`
	StateFile    string        `json:"state_file"`
	MihomoConfig string        `json:"mihomo_config"`
	Recent       []logFileJSON `json:"recent,omitempty"`
}

type logFileJSON struct {
	Name   string   `json:"name"`
	Path   string   `json:"path"`
	Exists bool     `json:"exists"`
	Lines  []string `json:"lines"`
	Error  string   `json:"error"`
}

type snapshotJSON struct {
	Status statusSnapshotJSON `json:"status"`
	Doctor doctorJSON         `json:"doctor"`
	Leases leasesSnapshotJSON `json:"leases"`
	Logs   logsJSON           `json:"logs"`
	Mihomo mihomoSnapshotJSON `json:"mihomo"`
}

type statusSnapshotJSON struct {
	gateway.Status
	Error string `json:"error"`
}

type leasesSnapshotJSON struct {
	Clients []device.Client `json:"clients"`
	Error   string          `json:"error"`
}

type mihomoSnapshotJSON struct {
	APIAddr     string                  `json:"api_addr"`
	Policies    policiesSnapshotJSON    `json:"policies"`
	Connections connectionsSnapshotJSON `json:"connections"`
	Providers   providersSnapshotJSON   `json:"providers"`
}

type policiesSnapshotJSON struct {
	Available bool                `json:"available"`
	Groups    []mihomo.ProxyGroup `json:"groups"`
	Error     string              `json:"error"`
}

type connectionsSnapshotJSON struct {
	Available     bool                `json:"available"`
	UploadTotal   int64               `json:"upload_total"`
	DownloadTotal int64               `json:"download_total"`
	Connections   []mihomo.Connection `json:"connections"`
	Error         string              `json:"error"`
}

type providersSnapshotJSON struct {
	Available      bool                   `json:"available"`
	ProxyProviders []mihomo.ProxyProvider `json:"proxy_providers"`
	RuleProviders  []mihomo.RuleProvider  `json:"rule_providers"`
	Error          string                 `json:"error"`
}

type policiesJSON struct {
	Groups []mihomo.ProxyGroup `json:"groups"`
}

type policySelectJSON struct {
	Group    string `json:"group"`
	Selected string `json:"selected"`
}

type providerUpdateJSON struct {
	Provider      string               `json:"provider"`
	Updated       bool                 `json:"updated"`
	ProxyProvider mihomo.ProxyProvider `json:"proxy_provider"`
}

type configuredDeviceJSON struct {
	ID                       string            `json:"id"`
	MAC                      string            `json:"mac"`
	IPv4                     string            `json:"ipv4"`
	ExpectedIP               string            `json:"expected_ip"`
	Profile                  string            `json:"profile"`
	Groups                   map[string]string `json:"groups"`
	PolicySource             string            `json:"policy_source"`
	DesiredDigest            string            `json:"desired_digest"`
	DesiredError             string            `json:"desired_error,omitempty"`
	AppliedDigest            string            `json:"applied_digest,omitempty"`
	Drift                    bool              `json:"drift"`
	Applied                  bool              `json:"applied"`
	ReservationInDynamicPool bool              `json:"reservation_in_dynamic_pool"`
	Hostname                 string            `json:"hostname,omitempty"`
	LeaseMACMatch            bool              `json:"lease_mac_match"`
	LeaseIP                  string            `json:"lease_ip,omitempty"`
	LeaseIPMatch             bool              `json:"lease_ip_match"`
	LeaseExpiresAt           *time.Time        `json:"lease_expires_at,omitempty"`
	LeaseActive              bool              `json:"lease_active"`
	LeaseMatch               bool              `json:"lease_match"`
	PolicyIdentityReady      bool              `json:"policy_identity_ready"`
}

type devicePoliciesJSON struct {
	Devices []configuredDeviceJSON `json:"devices"`
}

type devicePolicySelectJSON struct {
	Device   string `json:"device"`
	Slot     string `json:"slot"`
	Group    string `json:"group"`
	Selected string `json:"selected"`
}

type validateMihomoJSON struct {
	Valid        bool   `json:"valid"`
	MihomoConfig string `json:"mihomo_config"`
}

func parseOutputFormat(format string) (bool, error) {
	switch format {
	case "text":
		return false, nil
	case "json":
		return true, nil
	default:
		return false, fmt.Errorf("must be text or json")
	}
}

func writeJSONExit(value any) int {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(os.Stderr, "json: %v\n", err)
		return 1
	}
	return 0
}

func writeErrorExit(command string, jsonOutput bool, code int, label string, err error) int {
	return writeErrorMessageExit(command, jsonOutput, code, fmt.Sprintf("%s: %v", label, err))
}

func writeErrorMessageExit(command string, jsonOutput bool, code int, message string) int {
	if jsonOutput {
		encoder := json.NewEncoder(os.Stderr)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(commandErrorJSON{Command: command, OK: false, Error: message}); err != nil {
			fmt.Fprintf(os.Stderr, "json: %v\n", err)
		}
		return code
	}
	fmt.Fprintln(os.Stderr, message)
	return code
}

func validatePolicySelection(groups []mihomo.ProxyGroup, groupName, selected string) error {
	if strings.TrimSpace(groupName) == "" {
		return fmt.Errorf("policy group is required")
	}
	if strings.TrimSpace(selected) == "" {
		return fmt.Errorf("selected policy is required")
	}

	for _, group := range groups {
		if group.Name != groupName {
			continue
		}
		for _, option := range group.Options {
			if option == selected {
				return nil
			}
		}
		return fmt.Errorf("policy %q is not a member of group %q (available: %s)", selected, groupName, strings.Join(group.Options, ", "))
	}

	return fmt.Errorf("policy group %q not found (available: %s)", groupName, strings.Join(policyGroupNames(groups), ", "))
}

func loadConfiguredPolicyBundle(cfg config.Config) (device.PolicyBundle, error) {
	if strings.TrimSpace(cfg.DevicePolicy.File) == "" {
		return device.PolicyBundle{}, fmt.Errorf("device_policy.file is not configured")
	}
	if cfg.DevicePolicy.Bundle != nil {
		return *cfg.DevicePolicy.Bundle, nil
	}
	return device.LoadPolicyBundle(cfg.DevicePolicy.File)
}

func loadAppliedPolicyBundle(cfg config.Config) (device.PolicyBundle, error) {
	paths := runtime.NewPaths(cfg)
	state, exists, err := runtime.LoadState(paths.StateFile)
	if err != nil {
		return device.PolicyBundle{}, err
	}
	if !exists || state.DevicePolicyDigest == "" {
		return device.PolicyBundle{}, fmt.Errorf("no applied device policy is active; start the gateway before selecting a device policy")
	}
	bundle, err := device.LoadPolicyBundleSnapshot(paths.DevicePolicyApplied)
	if err != nil {
		return device.PolicyBundle{}, fmt.Errorf("load applied device policy: %w", err)
	}
	if bundle.Digest != state.DevicePolicyDigest {
		return device.PolicyBundle{}, fmt.Errorf("applied device policy digest does not match runtime state")
	}
	return bundle, nil
}

func configuredDevices(cfg config.Config) ([]configuredDeviceJSON, error) {
	desired, desiredErr := loadConfiguredPolicyBundle(cfg)
	paths := runtime.NewPaths(cfg)
	bundle := desired
	policySource := "desired"
	applied := false
	appliedDigest := ""
	state, exists, err := runtime.LoadState(paths.StateFile)
	if err != nil {
		return nil, err
	}
	if exists && state.DevicePolicyDigest != "" {
		bundle, err = loadAppliedPolicyBundle(cfg)
		if err != nil {
			return nil, err
		}
		policySource = "applied"
		applied = true
		appliedDigest = bundle.Digest
	} else if desiredErr != nil {
		return nil, desiredErr
	}
	leases, err := device.LoadLeases(paths.LeaseFile)
	if err != nil {
		return nil, err
	}
	leasesByMAC := make(map[string][]device.Client, len(leases))
	for _, lease := range leases {
		key := strings.ToLower(lease.MAC)
		leasesByMAC[key] = append(leasesByMAC[key], lease)
	}
	desiredDigest := desired.Digest
	desiredError := ""
	if desiredErr != nil {
		desiredError = desiredErr.Error()
	}
	devices := make([]configuredDeviceJSON, 0, len(bundle.Compiled.Devices))
	for _, managed := range bundle.Compiled.Devices {
		entry := configuredDeviceJSON{
			ID:                       managed.ID,
			MAC:                      managed.MAC,
			IPv4:                     managed.IPv4,
			ExpectedIP:               managed.IPv4,
			Profile:                  managed.Profile,
			Groups:                   managed.Groups,
			PolicySource:             policySource,
			DesiredDigest:            desiredDigest,
			DesiredError:             desiredError,
			AppliedDigest:            appliedDigest,
			Drift:                    applied && (desiredErr != nil || desiredDigest != appliedDigest),
			Applied:                  applied,
			ReservationInDynamicPool: ipv4InDHCPRange(managed.IPv4, cfg.DHCP.RangeStart, cfg.DHCP.RangeEnd),
		}
		if lease, exists := selectLease(leasesByMAC[managed.MAC], managed.IPv4); exists {
			entry.Hostname = lease.Hostname
			entry.LeaseMACMatch = true
			entry.LeaseIP = lease.IP
			entry.LeaseIPMatch = lease.IP == managed.IPv4
			entry.LeaseExpiresAt = &lease.ExpiresAt
			entry.LeaseActive = lease.Online
			entry.LeaseMatch = entry.LeaseMACMatch && entry.LeaseIPMatch && entry.LeaseActive
			entry.PolicyIdentityReady = entry.Applied && entry.LeaseMatch
		}
		devices = append(devices, entry)
	}
	return devices, nil
}

func selectLease(leases []device.Client, expectedIP string) (device.Client, bool) {
	if len(leases) == 0 {
		return device.Client{}, false
	}
	best := leases[0]
	for _, lease := range leases {
		if lease.IP == expectedIP && best.IP != expectedIP {
			best = lease
			continue
		}
		if lease.IP == best.IP && lease.ExpiresAt.After(best.ExpiresAt) {
			best = lease
		}
	}
	return best, true
}

func ipv4InDHCPRange(value, start, end string) bool {
	ip := parseIPv4Uint32(value)
	first := parseIPv4Uint32(start)
	last := parseIPv4Uint32(end)
	return ip != 0 && first != 0 && last != 0 && first <= last && ip >= first && ip <= last
}

func parseIPv4Uint32(value string) uint32 {
	parts := strings.Split(value, ".")
	if len(parts) != 4 {
		return 0
	}
	var result uint32
	for _, part := range parts {
		value, err := strconv.ParseUint(part, 10, 8)
		if err != nil {
			return 0
		}
		result = result<<8 | uint32(value)
	}
	return result
}

func policyGroupNames(groups []mihomo.ProxyGroup) []string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Name)
	}
	return names
}

func buildSnapshot(ctx context.Context, cfg config.Config, manager gatewayManager, tail int) snapshotJSON {
	status, statusErr := manager.Status(ctx)
	report := doctor.Run(cfg)
	paths := runtime.NewPaths(cfg)
	leases := leasesSnapshot(paths.LeaseFile)
	logs := logsJSON{
		LogsDir:      paths.LogDir,
		DNSMasqLog:   paths.DNSMasqLog,
		MihomoLog:    paths.MihomoLog,
		StateFile:    paths.StateFile,
		MihomoConfig: paths.MihomoConfig,
		Recent:       recentLogFiles(paths, tail),
	}

	snapshotStatus := statusSnapshotJSON{Status: status}
	if statusErr != nil {
		snapshotStatus.Error = statusErr.Error()
	}

	return snapshotJSON{
		Status: snapshotStatus,
		Doctor: doctorJSON{
			Healthy: report.Healthy(),
			Checks:  report.Checks,
		},
		Leases: leases,
		Logs:   logs,
		Mihomo: mihomoSnapshot(ctx, cfg),
	}
}

func leasesSnapshot(path string) leasesSnapshotJSON {
	clients, err := device.LoadLeases(path)
	if clients == nil {
		clients = []device.Client{}
	}
	snapshot := leasesSnapshotJSON{Clients: clients}
	if err != nil {
		snapshot.Error = err.Error()
	}
	return snapshot
}

func mihomoSnapshot(ctx context.Context, cfg config.Config) mihomoSnapshotJSON {
	return mihomoSnapshotJSON{
		APIAddr:     cfg.Mihomo.APIAddr,
		Policies:    policiesSnapshot(ctx, cfg),
		Connections: connectionsSnapshot(ctx, cfg),
		Providers:   providersSnapshot(ctx, cfg),
	}
}

func policiesSnapshot(ctx context.Context, cfg config.Config) policiesSnapshotJSON {
	groups, err := fetchProxyGroups(ctx, cfg)
	if groups == nil {
		groups = []mihomo.ProxyGroup{}
	}
	snapshot := policiesSnapshotJSON{Available: err == nil, Groups: groups}
	if err != nil {
		snapshot.Error = err.Error()
	}
	return snapshot
}

func connectionsSnapshot(ctx context.Context, cfg config.Config) connectionsSnapshotJSON {
	connections, err := fetchConnections(ctx, cfg)
	if connections.Connections == nil {
		connections.Connections = []mihomo.Connection{}
	}
	snapshot := connectionsSnapshotJSON{
		Available:     err == nil,
		UploadTotal:   connections.UploadTotal,
		DownloadTotal: connections.DownloadTotal,
		Connections:   connections.Connections,
	}
	if err != nil {
		snapshot.Error = err.Error()
	}
	return snapshot
}

func providersSnapshot(ctx context.Context, cfg config.Config) providersSnapshotJSON {
	providers, err := fetchProviders(ctx, cfg)
	if providers.ProxyProviders == nil {
		providers.ProxyProviders = []mihomo.ProxyProvider{}
	}
	if providers.RuleProviders == nil {
		providers.RuleProviders = []mihomo.RuleProvider{}
	}
	snapshot := providersSnapshotJSON{
		Available:      err == nil,
		ProxyProviders: providers.ProxyProviders,
		RuleProviders:  providers.RuleProviders,
	}
	if err != nil {
		snapshot.Error = err.Error()
	}
	return snapshot
}

func recentLogFiles(paths runtime.Paths, tail int) []logFileJSON {
	if tail <= 0 {
		return nil
	}

	files := []struct {
		name string
		path string
	}{
		{name: "dnsmasq", path: paths.DNSMasqLog},
		{name: "mihomo", path: paths.MihomoLog},
	}

	recent := make([]logFileJSON, 0, len(files))
	for _, file := range files {
		lines, exists, err := tailFile(file.path, tail)
		if lines == nil {
			lines = []string{}
		}
		entry := logFileJSON{
			Name:   file.name,
			Path:   file.path,
			Exists: exists,
			Lines:  lines,
		}
		if err != nil {
			entry.Error = err.Error()
		}
		recent = append(recent, entry)
	}
	return recent
}

func tailFile(path string, limit int) ([]string, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if info.IsDir() {
		return nil, true, fmt.Errorf("is a directory")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, true, err
	}
	defer file.Close()

	lines := make([]string, 0, limit)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if len(lines) == limit {
			copy(lines, lines[1:])
			lines[len(lines)-1] = scanner.Text()
			continue
		}
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return lines, true, err
	}
	return lines, true, nil
}

func formatProxyGroups(groups []mihomo.ProxyGroup) string {
	if len(groups) == 0 {
		return "No mihomo policy groups found.\n"
	}

	var out strings.Builder
	for _, group := range groups {
		fmt.Fprintf(&out, "%s (%s): %s\n", group.Name, group.Type, group.Selected)
		for _, option := range group.Options {
			marker := " "
			if option == group.Selected {
				marker = "*"
			}
			fmt.Fprintf(&out, "  %s %s\n", marker, option)
		}
	}
	return out.String()
}

func formatConfiguredDevices(devices []configuredDeviceJSON) string {
	if len(devices) == 0 {
		return "No configured device policies found.\n"
	}
	var out strings.Builder
	for _, device := range devices {
		status := "lease inactive"
		if device.LeaseActive {
			status = "lease active"
		}
		if device.PolicyIdentityReady {
			status = "policy identity ready"
		}
		fmt.Fprintf(&out, "%s %s %s (%s, %s, %s)\n", device.ID, device.IPv4, device.MAC, device.Profile, device.PolicySource, status)
		if device.Hostname != "" {
			fmt.Fprintf(&out, "  hostname: %s\n", device.Hostname)
		}
		for _, slot := range sortedDeviceSlots(device.Groups) {
			fmt.Fprintf(&out, "  %s: %s\n", slot, device.Groups[slot])
		}
	}
	return out.String()
}

func sortedDeviceSlots(groups map[string]string) []string {
	slots := make([]string, 0, len(groups))
	for slot := range groups {
		slots = append(slots, slot)
	}
	sort.Strings(slots)
	return slots
}

func formatRecentLogs(files []logFileJSON) string {
	var out strings.Builder
	for _, file := range files {
		fmt.Fprintf(&out, "== %s (%s) ==\n", file.Name, file.Path)
		switch {
		case file.Error != "":
			fmt.Fprintf(&out, "error: %s\n", file.Error)
		case !file.Exists:
			fmt.Fprintln(&out, "missing")
		case len(file.Lines) == 0:
			fmt.Fprintln(&out, "empty")
		default:
			for _, line := range file.Lines {
				fmt.Fprintln(&out, line)
			}
		}
	}
	return out.String()
}

func formatSnapshot(snapshot snapshotJSON) string {
	var out strings.Builder
	out.WriteString(snapshot.Status.Format())
	if snapshot.Status.Error != "" {
		fmt.Fprintf(&out, "Status error: %s\n", snapshot.Status.Error)
	}
	fmt.Fprintf(&out, "Doctor healthy: %t\n", snapshot.Doctor.Healthy)
	fmt.Fprintf(&out, "Leases: %d\n", len(snapshot.Leases.Clients))
	if snapshot.Leases.Error != "" {
		fmt.Fprintf(&out, "Leases error: %s\n", snapshot.Leases.Error)
	}
	if snapshot.Mihomo.Policies.Available {
		fmt.Fprintf(&out, "Policy groups: %d\n", len(snapshot.Mihomo.Policies.Groups))
	} else {
		fmt.Fprintf(&out, "Policy groups unavailable: %s\n", snapshot.Mihomo.Policies.Error)
	}
	if snapshot.Mihomo.Connections.Available {
		fmt.Fprintf(&out, "Connections: %d\n", len(snapshot.Mihomo.Connections.Connections))
	} else {
		fmt.Fprintf(&out, "Connections unavailable: %s\n", snapshot.Mihomo.Connections.Error)
	}
	if snapshot.Mihomo.Providers.Available {
		fmt.Fprintf(&out, "Proxy providers: %d\n", len(snapshot.Mihomo.Providers.ProxyProviders))
	} else {
		fmt.Fprintf(&out, "Providers unavailable: %s\n", snapshot.Mihomo.Providers.Error)
	}
	return out.String()
}

func formatProviders(snapshot mihomo.ProvidersSnapshot) string {
	var out strings.Builder
	fmt.Fprintf(&out, "Proxy providers: %d\n", len(snapshot.ProxyProviders))
	for _, provider := range snapshot.ProxyProviders {
		fmt.Fprintf(&out, "%s (%s/%s): %d proxies", provider.Name, provider.Type, provider.VehicleType, provider.ProxyCount)
		if provider.UpdatedAt != "" && !strings.HasPrefix(provider.UpdatedAt, "0001-01-01") {
			fmt.Fprintf(&out, " updated %s", provider.UpdatedAt)
		}
		out.WriteByte('\n')
		for _, proxy := range provider.Proxies {
			status := "dead"
			if proxy.Alive {
				status = "alive"
			}
			fmt.Fprintf(&out, "  %s (%s): %s\n", proxy.Name, proxy.Type, status)
		}
	}
	fmt.Fprintf(&out, "Rule providers: %d\n", len(snapshot.RuleProviders))
	for _, provider := range snapshot.RuleProviders {
		fmt.Fprintf(&out, "%s (%s/%s): %d rules\n", provider.Name, provider.Type, provider.VehicleType, provider.RuleCount)
	}
	return out.String()
}

func formatProviderUpdate(provider mihomo.ProxyProvider) string {
	return fmt.Sprintf("Provider %q updated (%d proxies)\n", provider.Name, provider.ProxyCount)
}

func formatConnections(snapshot mihomo.ConnectionsSnapshot) string {
	var out strings.Builder
	fmt.Fprintf(&out, "Connections: %d\n", len(snapshot.Connections))
	fmt.Fprintf(&out, "Upload total: %d bytes\n", snapshot.UploadTotal)
	fmt.Fprintf(&out, "Download total: %d bytes\n", snapshot.DownloadTotal)
	for _, connection := range snapshot.Connections {
		target := connectionTarget(connection.Metadata)
		chain := strings.Join(connection.Chains, " -> ")
		if chain == "" {
			chain = "-"
		}
		rule := connection.Rule
		if connection.RulePayload != "" {
			rule += "(" + connection.RulePayload + ")"
		}
		if rule == "" {
			rule = "-"
		}
		fmt.Fprintf(&out, "%s %s %s %s\n", connection.ID, target, chain, rule)
	}
	return out.String()
}

func connectionTarget(metadata map[string]any) string {
	if len(metadata) == 0 {
		return "-"
	}
	host := stringMetadata(metadata, "host")
	if host == "" {
		host = stringMetadata(metadata, "destinationIP")
	}
	port := stringMetadata(metadata, "destinationPort")
	if host == "" {
		return "-"
	}
	if port == "" {
		return host
	}
	return host + ":" + port
}

func stringMetadata(metadata map[string]any, key string) string {
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return fmt.Sprint(typed)
	}
}

func printUsage(out *os.File) {
	fmt.Fprintf(out, `OpenSurge for Mac

Usage:
  omg <command> --config <path>

Commands:
  start    prepare runtime state and start gateway services
  stop     stop gateway services and clean runtime state
  reload   validate desired configuration, then stop and restart gateway services
  status   print gateway status
  doctor   run environment checks
  leases   print DHCP leases
  devices  print configured device identities, DHCP state, and policy slots
  logs     print runtime log location or recent log lines with --tail
  snapshot collect status, doctor, leases, logs, policies, providers, and connections
  policies
           list mihomo policy groups from the external-controller API
  policy-select --group <name> --policy <name>
           switch the selected policy in a mihomo policy group
  device-policy-select --device <id> --slot <default|rule-id> --policy <name>
           switch one configured device's independent policy selector
  connections
           print current mihomo connections from the external-controller API
  providers
           print mihomo proxy/rule providers from the external-controller API
  provider-update --provider <name>
           refresh a mihomo proxy provider through the external-controller API
  render-mihomo
           print the final mihomo config without starting services
  validate-mihomo
           render and validate the final mihomo config without starting services

Default config: %s
`, defaultConfigPath)
}
