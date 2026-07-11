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
)

type ActionRunner interface {
	Run(context.Context, string, string) error
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

type HelperClient struct {
	SocketPath string
}

type HelperRequest struct {
	Action     string `json:"action"`
	ConfigPath string `json:"config_path"`
}

type HelperResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (c HelperClient) Run(ctx context.Context, action, configPath string) error {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
	if err := json.NewEncoder(conn).Encode(HelperRequest{Action: action, ConfigPath: configPath}); err != nil {
		return err
	}
	var response HelperResponse
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&response); err != nil {
		return err
	}
	if !response.OK {
		return fmt.Errorf("%s", response.Error)
	}
	return nil
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
	if request.Action != "start" && request.Action != "stop" {
		_ = json.NewEncoder(conn).Encode(HelperResponse{Error: "action is not allowed"})
		return
	}
	configPath, err := filepath.Abs(request.ConfigPath)
	if err == nil {
		root, rootErr := filepath.Abs(allowedRoot)
		if rootErr != nil || (configPath != root && !strings.HasPrefix(configPath, root+string(os.PathSeparator))) {
			err = fmt.Errorf("config path is outside allowed root")
		}
	}
	if err == nil {
		err = requireRootOwnedConfig(configPath)
	}
	if err == nil {
		err = (DirectRunner{}).Run(ctx, request.Action, configPath)
	}
	response := HelperResponse{OK: err == nil}
	if err != nil {
		response.Error = err.Error()
	}
	_ = json.NewEncoder(conn).Encode(response)
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
