package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/doctor"
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
)

const defaultConfigPath = "examples/config.example.yaml"

var (
	fetchProxyGroups = mihomo.FetchProxyGroups
	selectProxyGroup = mihomo.SelectProxyGroup
	fetchConnections = mihomo.FetchConnections
)

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
	logTail := fs.Int("tail", 0, "number of recent log lines to include")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	jsonOutput, err := parseOutputFormat(*outputFormat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "format: %v\n", err)
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}

	ctx := context.Background()
	manager := gateway.New(cfg)

	switch command {
	case "start":
		if err := manager.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "start: %v\n", err)
			return 1
		}
	case "stop":
		if err := manager.Stop(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "stop: %v\n", err)
			return 1
		}
	case "status":
		status, err := manager.Status(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "status: %v\n", err)
			return 1
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
			fmt.Fprintf(os.Stderr, "leases: %v\n", err)
			return 1
		}
		if jsonOutput {
			return writeJSONExit(leasesJSON{Clients: clients})
		}
		fmt.Print(device.FormatClients(clients))
	case "logs":
		if *logTail < 0 {
			fmt.Fprintf(os.Stderr, "logs: tail must be greater than or equal to 0\n")
			return 2
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
	case "policies":
		groups, err := fetchProxyGroups(ctx, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "policies: %v\n", err)
			return 1
		}
		if jsonOutput {
			return writeJSONExit(policiesJSON{Groups: groups})
		}
		fmt.Print(formatProxyGroups(groups))
	case "policy-select":
		groups, err := fetchProxyGroups(ctx, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "policy-select: %v\n", err)
			return 1
		}
		if err := validatePolicySelection(groups, *policyGroup, *policyName); err != nil {
			fmt.Fprintf(os.Stderr, "policy-select: %v\n", err)
			return 1
		}
		if err := selectProxyGroup(ctx, cfg, *policyGroup, *policyName); err != nil {
			fmt.Fprintf(os.Stderr, "policy-select: %v\n", err)
			return 1
		}
		if jsonOutput {
			return writeJSONExit(policySelectJSON{Group: *policyGroup, Selected: *policyName})
		}
		fmt.Printf("Policy group %q selected %q\n", *policyGroup, *policyName)
	case "connections":
		connections, err := fetchConnections(ctx, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "connections: %v\n", err)
			return 1
		}
		if jsonOutput {
			return writeJSONExit(connections)
		}
		fmt.Print(formatConnections(connections))
	case "render-mihomo":
		rendered, err := mihomo.RenderConfig(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "render-mihomo: %v\n", err)
			return 1
		}
		fmt.Print(rendered)
	case "validate-mihomo":
		paths := runtime.NewPaths(cfg)
		if err := mihomo.New(cfg, paths).ValidateConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "validate-mihomo: %v\n", err)
			return 1
		}
		if jsonOutput {
			return writeJSONExit(validateMihomoJSON{Valid: true, MihomoConfig: paths.MihomoConfig})
		}
		fmt.Printf("mihomo config valid: %s\n", paths.MihomoConfig)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", command)
		printUsage(os.Stderr)
		return 2
	}

	return 0
}

type doctorJSON struct {
	Healthy bool           `json:"healthy"`
	Checks  []doctor.Check `json:"checks"`
}

type leasesJSON struct {
	Clients []device.Client `json:"clients"`
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

type policiesJSON struct {
	Groups []mihomo.ProxyGroup `json:"groups"`
}

type policySelectJSON struct {
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

func policyGroupNames(groups []mihomo.ProxyGroup) []string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Name)
	}
	return names
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
  status   print gateway status
  doctor   run environment checks
  leases   print DHCP leases
  logs     print runtime log location or recent log lines with --tail
  policies
           list mihomo policy groups from the external-controller API
  policy-select --group <name> --policy <name>
           switch the selected policy in a mihomo policy group
  connections
           print current mihomo connections from the external-controller API
  render-mihomo
           print the final mihomo config without starting services
  validate-mihomo
           render and validate the final mihomo config without starting services

Default config: %s
`, defaultConfigPath)
}
