package controlapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/macosnetwork"
)

type ActionRunner interface {
	Run(context.Context, string, string) error
}

type NetworkRunner interface {
	SetManual(context.Context, string, macosnetwork.ManualConfig) error
	SetDHCP(context.Context, string, string) error
	ProbeDHCP(context.Context, string, string, time.Duration) ([]string, error)
}

type DirectRunner struct{}

func (DirectRunner) Run(ctx context.Context, action, configPath string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("privileged helper is not installed or reachable")
	}
	var (
		cfg config.Config
		err error
	)
	if action == "start" {
		cfg, err = config.Load(configPath)
	} else {
		cfg, err = config.LoadRuntime(configPath)
	}
	if err != nil {
		return err
	}
	manager := gateway.New(cfg)
	switch action {
	case "start":
		return manager.Start(ctx)
	case "stop":
		return manager.Stop(ctx)
	default:
		return fmt.Errorf("unsupported privileged action %q", action)
	}
}

func (DirectRunner) SetManual(ctx context.Context, _ string, cfg macosnetwork.ManualConfig) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("privileged helper is required")
	}
	return macosnetwork.SetManual(ctx, cfg)
}

func (DirectRunner) SetDHCP(ctx context.Context, _ string, service string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("privileged helper is required")
	}
	return macosnetwork.SetDHCP(ctx, service)
}

func (DirectRunner) ProbeDHCP(ctx context.Context, _ string, interfaceName string, timeout time.Duration) ([]string, error) {
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("privileged helper is required")
	}
	return macosnetwork.ProbeDHCPServers(ctx, interfaceName, timeout)
}

type HelperClient struct {
	SocketPath string
}

type HelperRequest struct {
	Action         string                     `json:"action"`
	ConfigPath     string                     `json:"config_path"`
	Manual         *macosnetwork.ManualConfig `json:"manual,omitempty"`
	NetworkService string                     `json:"network_service,omitempty"`
	Interface      string                     `json:"interface,omitempty"`
	TimeoutMillis  int                        `json:"timeout_millis,omitempty"`
}

type HelperResponse struct {
	OK          bool     `json:"ok"`
	Error       string   `json:"error,omitempty"`
	DHCPServers []string `json:"dhcp_servers,omitempty"`
}

func (c HelperClient) Run(ctx context.Context, action, configPath string) error {
	_, err := c.call(ctx, HelperRequest{Action: action, ConfigPath: configPath})
	return err
}

func (c HelperClient) SetManual(ctx context.Context, configPath string, cfg macosnetwork.ManualConfig) error {
	_, err := c.call(ctx, HelperRequest{Action: "network-set-manual", ConfigPath: configPath, Manual: &cfg})
	return err
}

func (c HelperClient) SetDHCP(ctx context.Context, configPath, service string) error {
	_, err := c.call(ctx, HelperRequest{Action: "network-set-dhcp", ConfigPath: configPath, NetworkService: service})
	return err
}

func (c HelperClient) ProbeDHCP(ctx context.Context, configPath, interfaceName string, timeout time.Duration) ([]string, error) {
	response, err := c.call(ctx, HelperRequest{Action: "dhcp-probe", ConfigPath: configPath, Interface: interfaceName, TimeoutMillis: int(timeout / time.Millisecond)})
	return response.DHCPServers, err
}

func (c HelperClient) call(ctx context.Context, request HelperRequest) (HelperResponse, error) {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return HelperResponse{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return HelperResponse{}, err
	}
	var response HelperResponse
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&response); err != nil {
		return HelperResponse{}, err
	}
	if !response.OK {
		return HelperResponse{}, fmt.Errorf("%s", response.Error)
	}
	return response, nil
}

func ServeHelper(ctx context.Context, socketPath, allowedRoot, socketGroup string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("opensurge-helper must run as root")
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return err
	}
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(socketPath)
	if socketGroup != "" {
		group, err := user.LookupGroup(socketGroup)
		if err != nil {
			return fmt.Errorf("lookup helper socket group: %w", err)
		}
		gid, err := strconv.Atoi(group.Gid)
		if err != nil {
			return fmt.Errorf("parse helper socket group: %w", err)
		}
		if err := os.Chown(socketPath, 0, gid); err != nil {
			return fmt.Errorf("set helper socket group: %w", err)
		}
	}
	if err := os.Chmod(socketPath, 0o660); err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go handleHelperConn(ctx, conn, allowedRoot)
	}
}

func handleHelperConn(ctx context.Context, conn net.Conn, allowedRoot string) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
	var request HelperRequest
	if err := json.NewDecoder(ioLimitReader(conn, 64<<10)).Decode(&request); err != nil {
		_ = json.NewEncoder(conn).Encode(HelperResponse{Error: err.Error()})
		return
	}
	if request.Action != "start" && request.Action != "stop" && request.Action != "network-set-manual" && request.Action != "network-set-dhcp" && request.Action != "dhcp-probe" {
		_ = json.NewEncoder(conn).Encode(HelperResponse{Error: "action is not allowed"})
		return
	}
	var err error
	configPath := ""
	if request.Action == "start" || request.Action == "stop" || request.Action == "network-set-manual" || request.Action == "network-set-dhcp" || request.Action == "dhcp-probe" {
		configPath, err = filepath.Abs(request.ConfigPath)
	}
	if err == nil && configPath != "" {
		root, rootErr := filepath.Abs(allowedRoot)
		if rootErr != nil || (configPath != root && !strings.HasPrefix(configPath, root+string(os.PathSeparator))) {
			err = fmt.Errorf("config path is outside allowed root")
		}
	}
	if err == nil && configPath != "" {
		err = requireRootOwnedConfig(configPath)
	}
	var cfg config.Config
	if err == nil {
		cfg, err = config.Load(configPath)
	}
	response := HelperResponse{}
	if err == nil {
		runner := DirectRunner{}
		switch request.Action {
		case "start", "stop":
			err = runner.Run(ctx, request.Action, configPath)
		case "network-set-manual":
			if request.Manual == nil {
				err = fmt.Errorf("manual network configuration is required")
			} else if err = validateHelperManualNetwork(ctx, cfg, *request.Manual); err == nil {
				err = runner.SetManual(ctx, configPath, *request.Manual)
			}
		case "network-set-dhcp":
			if err = validateHelperNetworkTarget(ctx, cfg, request.NetworkService, cfg.Gateway.Interface); err == nil {
				err = runner.SetDHCP(ctx, configPath, request.NetworkService)
			}
		case "dhcp-probe":
			if request.Interface != cfg.Gateway.Interface {
				err = fmt.Errorf("DHCP probe interface does not match configured gateway interface")
			} else {
				timeout := time.Duration(request.TimeoutMillis) * time.Millisecond
				if timeout < time.Second || timeout > 10*time.Second {
					timeout = 3 * time.Second
				}
				response.DHCPServers, err = runner.ProbeDHCP(ctx, configPath, request.Interface, timeout)
			}
		}
	}
	response.OK = err == nil
	if err != nil {
		response.Error = err.Error()
	}
	_ = json.NewEncoder(conn).Encode(response)
}

func validateHelperNetworkTarget(ctx context.Context, cfg config.Config, service, interfaceName string) error {
	if interfaceName != cfg.Gateway.Interface {
		return fmt.Errorf("network interface does not match configured gateway interface")
	}
	actual, err := macosnetwork.ServiceInterface(ctx, service)
	if err != nil {
		return err
	}
	if actual != interfaceName {
		return fmt.Errorf("network service %q uses %s, not configured interface %s", service, actual, interfaceName)
	}
	return nil
}

func validateHelperManualNetwork(ctx context.Context, cfg config.Config, manual macosnetwork.ManualConfig) error {
	if err := validateHelperNetworkTarget(ctx, cfg, manual.NetworkService, manual.Interface); err != nil {
		return err
	}
	if manual.IPv4 != cfg.Gateway.LANIP {
		return fmt.Errorf("manual IPv4 does not match configured gateway LAN IP")
	}
	return macosnetwork.ValidateManual(manual)
}

func requireRootOwnedConfig(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("config path is not a regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 {
		return fmt.Errorf("helper config must be owned by root")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("helper config must not be writable by group or other")
	}
	return nil
}

type limitedReader struct {
	r net.Conn
	n int64
}

func ioLimitReader(conn net.Conn, n int64) *limitedReader { return &limitedReader{r: conn, n: n} }

func (r *limitedReader) Read(p []byte) (int, error) {
	if r.n <= 0 {
		return 0, fmt.Errorf("helper request too large")
	}
	if int64(len(p)) > r.n {
		p = p[:r.n]
	}
	n, err := r.r.Read(p)
	r.n -= int64(n)
	return n, err
}
