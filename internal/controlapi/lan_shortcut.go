package controlapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"open-mihomo-gateway/internal/config"
)

type LANShortcutLease interface {
	Done() <-chan error
	Close() error
}

type LANShortcutRunner interface {
	AcquireLANShortcut(context.Context, string, lanSettings) (LANShortcutLease, error)
}

// Called with the controller lock held, only after the authenticated IP listener
// has opened. A shortcut failure must never disable that recovery entry point.
func (c *lanAccessController) startShortcutLocked() {
	if c.shortcutRunner == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	lease, err := c.shortcutRunner.AcquireLANShortcut(ctx, c.owner.configPath, c.config.lanSettings)
	if err != nil {
		c.shortcutError = "shortcut_unavailable"
		log.Printf("OpenSurge LAN shortcut unavailable; IP management remains active: %v", err)
		return
	}
	c.shortcut, c.shortcutError = lease, ""
	go func() {
		<-lease.Done()
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.shortcut == lease {
			c.shortcut = nil
			c.shortcutError = "shortcut_unavailable"
		}
	}()
}

func (c *lanAccessController) stopShortcutLocked() {
	if c.shortcut != nil {
		_ = c.shortcut.Close()
		c.shortcut = nil
	}
	c.shortcutError = ""
}

func (c HelperClient) AcquireLANShortcut(ctx context.Context, configPath string, binding lanSettings) (LANShortcutLease, error) {
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = conn.Close()
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	if err = json.NewEncoder(conn).Encode(HelperRequest{Action: "lan-shortcut-hold", ConfigPath: configPath, LANShortcut: &binding}); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	var response HelperResponse
	if err = json.NewDecoder(reader).Decode(&response); err != nil {
		return nil, err
	}
	if !response.OK {
		return nil, fmt.Errorf("%s", response.Error)
	}
	_ = conn.SetDeadline(time.Time{})
	lease := &helperLANShortcutLease{conn: conn, done: make(chan error)}
	go func() {
		_, _ = io.Copy(io.Discard, reader)
		_ = conn.Close()
		close(lease.done)
	}()
	success = true
	return lease, nil
}

type helperLANShortcutLease struct {
	conn net.Conn
	done chan error
	once sync.Once
}

func (l *helperLANShortcutLease) Done() <-chan error { return l.done }
func (l *helperLANShortcutLease) Close() error {
	l.once.Do(func() {
		// Wait for the helper to release its sockets before a settings save rebinds.
		if unix, ok := l.conn.(*net.UnixConn); ok {
			_ = unix.CloseWrite()
		} else {
			_ = l.conn.Close()
		}
		select {
		case <-l.done:
		case <-time.After(3 * time.Second):
			_ = l.conn.Close()
		}
	})
	return nil
}

func (DirectRunner) AcquireLANShortcut(ctx context.Context, _ string, binding lanSettings) (LANShortcutLease, error) {
	return startLANShortcut(context.Background(), binding, net.Listen, net.ListenPacket, localLANAddresses)
}

func serveLANShortcutLease(ctx context.Context, conn net.Conn, binding *lanSettings, validationErr error) {
	serveLANShortcutLeaseWithStarter(ctx, conn, binding, validationErr, func(ctx context.Context, b lanSettings) (LANShortcutLease, error) {
		return startLANShortcut(ctx, b, net.Listen, net.ListenPacket, localLANAddresses)
	})
}

func serveLANShortcutLeaseWithStarter(ctx context.Context, conn net.Conn, binding *lanSettings, validationErr error, start func(context.Context, lanSettings) (LANShortcutLease, error)) {
	var lease LANShortcutLease
	err := validationErr
	if err == nil && binding == nil {
		err = fmt.Errorf("LAN binding is required")
	}
	if err == nil {
		lease, err = start(ctx, *binding)
	}
	response := HelperResponse{OK: err == nil}
	if err != nil {
		response.Error = err.Error()
	}
	if err == nil && lease != nil {
		defer lease.Close()
	}
	if encodeErr := json.NewEncoder(conn).Encode(response); err != nil || encodeErr != nil {
		return
	}
	_ = conn.SetDeadline(time.Time{})
	disconnected := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, conn); close(disconnected) }()
	select {
	case <-ctx.Done():
	case <-disconnected:
	case <-lease.Done():
	}
}

type lanShortcutService struct {
	server *http.Server
	tcp    net.Listener
	udp    net.PacketConn
	done   chan error
	once   sync.Once
}

func (s *lanShortcutService) Done() <-chan error { return s.done }
func (s *lanShortcutService) Close() error {
	s.once.Do(func() {
		// Mark unavailable before releasing listeners or admitting another request.
		close(s.done)
		_ = s.server.Close()
		_ = s.tcp.Close()
		_ = s.udp.Close()
	})
	return nil
}

func startLANShortcut(ctx context.Context, binding lanSettings, listen func(string, string) (net.Listener, error), packet func(string, string) (net.PacketConn, error), addresses func() ([]lanAddress, error)) (*lanShortcutService, error) {
	available := func() bool { return binding.Enabled && lanBindingAvailable(binding, addresses) }
	if !available() {
		return nil, fmt.Errorf("LAN binding is unavailable")
	}
	httpListener, err := listen("tcp4", net.JoinHostPort(binding.IPv4, "80"))
	if err != nil {
		return nil, fmt.Errorf("shortcut port 80: %w", err)
	}
	tcp, err := listen("tcp4", config.LANShortcutDNS)
	if err != nil {
		_ = httpListener.Close()
		return nil, fmt.Errorf("shortcut DNS: %w", err)
	}
	udp, err := packet("udp4", config.LANShortcutDNS)
	if err != nil {
		_ = httpListener.Close()
		_ = tcp.Close()
		return nil, fmt.Errorf("shortcut DNS: %w", err)
	}
	service := &lanShortcutService{tcp: tcp, udp: udp, done: make(chan error)}
	live := func() bool {
		select {
		case <-service.done:
			return false
		default:
			return available()
		}
	}
	service.server = &http.Server{Handler: lanShortcutHandler(binding, live), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, IdleTimeout: 3 * time.Second, MaxHeaderBytes: 8 << 10}
	go func() { _ = service.server.Serve(httpListener); _ = service.Close() }()
	go service.serveDNSUDP(binding, live)
	go service.serveDNSTCP(binding, live)
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-service.done:
				return
			case <-ctx.Done():
				_ = service.Close()
				return
			case <-ticker.C:
				if !available() {
					_ = service.Close()
					return
				}
			}
		}
	}()
	return service, nil
}

func lanShortcutHandler(binding lanSettings, available func() bool) http.Handler {
	target := "http://" + net.JoinHostPort(binding.IPv4, strconv.Itoa(binding.Port)) + "/"
	prefix := netip.PrefixFrom(netip.MustParseAddr(binding.IPv4), binding.PrefixLen).Masked()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		setControlSecurityHeaders(w)
		if !available() {
			http.Error(w, "LAN access is unavailable", 503)
			return
		}
		peer, err := netip.ParseAddrPort(r.RemoteAddr)
		if err != nil || !peer.Addr().Is4() || !prefix.Contains(peer.Addr()) {
			http.Error(w, "client is outside the configured LAN", 403)
			return
		}
		host := strings.ToLower(r.Host)
		if host != config.LANShortcutHost && host != config.LANShortcutHost+":80" {
			http.Error(w, "request host is not allowed", 403)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method is not allowed", 405)
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		// Never forward a password, bootstrap code, path or untrusted redirect target.
		http.Redirect(w, r, target, http.StatusFound)
	})
}
