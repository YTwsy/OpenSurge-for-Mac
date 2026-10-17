package controlapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"open-mihomo-gateway/internal/config"
)

const defaultLANPort = 61768

type lanSettings struct {
	Enabled   bool   `json:"enabled"`
	Interface string `json:"interface"`
	IPv4      string `json:"ipv4"`
	PrefixLen int    `json:"prefix_len"`
	Port      int    `json:"port"`
}

type lanAccessConfig struct {
	SchemaVersion int `json:"schema_version"`
	lanSettings
	PasswordHash string `json:"password_hash,omitempty"`
	Paused       bool   `json:"paused,omitempty"`
}

type lanAddress struct {
	Interface string `json:"interface"`
	IPv4      string `json:"ipv4"`
	PrefixLen int    `json:"prefix_len"`
}

type lanAccessResponse struct {
	SchemaVersion int `json:"schema_version"`
	lanSettings
	Revision             string       `json:"revision"`
	PasswordSet          bool         `json:"password_set"`
	State                string       `json:"state"`
	URL                  string       `json:"url,omitempty"`
	Error                string       `json:"error,omitempty"`
	RecommendedInterface string       `json:"recommended_interface,omitempty"`
	Addresses            []lanAddress `json:"addresses"`
}

// A single controller owns the optional listener. It never starts/stops the
// gateway, changes interfaces, or expands a saved binding to a new network.
type lanAccessController struct {
	owner          *Server
	opMu           sync.Mutex
	mu             sync.Mutex
	config         lanAccessConfig
	problem        string
	server         *http.Server
	generation     uint64
	closed         bool
	pauseDirty     bool
	sessions       map[string]*lanSession
	attempts       map[string]lanAttemptWindow
	globalAttempts lanAttemptWindow
	passwordSlots  chan struct{}
	addresses      func() ([]lanAddress, error)
	listen         func(string, string) (net.Listener, error)
	now            func() time.Time
}

func newLANAccessController(owner *Server) *lanAccessController {
	c := &lanAccessController{owner: owner, config: lanAccessConfig{SchemaVersion: 1, lanSettings: lanSettings{Port: defaultLANPort}},
		sessions: map[string]*lanSession{}, attempts: map[string]lanAttemptWindow{}, passwordSlots: make(chan struct{}, 2),
		addresses: localLANAddresses, listen: net.Listen, now: time.Now}
	info, err := os.Lstat(c.path())
	if errors.Is(err, os.ErrNotExist) {
		return c
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 16<<10 {
		c.problem = "configuration_invalid"
		return c
	}
	data, err := os.ReadFile(c.path())
	if err != nil || len(data) > 16<<10 || json.Unmarshal(data, &c.config) != nil || c.config.SchemaVersion != 1 {
		c.config = lanAccessConfig{SchemaVersion: 1, lanSettings: lanSettings{Port: defaultLANPort}}
		c.problem = "configuration_invalid"
		return c
	}
	_, _, hashOK := parseLANPassword(c.config.PasswordHash)
	if (c.config.PasswordHash != "" && !hashOK) || (c.config.Enabled && (!hashOK || !validLANSettings(c.config.lanSettings))) {
		c.config = lanAccessConfig{SchemaVersion: 1, lanSettings: lanSettings{Port: defaultLANPort}}
		c.problem = "configuration_invalid"
	} else if c.config.Paused {
		c.problem = "address_unavailable"
	}
	return c
}

func (c *lanAccessController) path() string {
	return filepath.Join(c.owner.store.Dir(), "lan-access.json")
}

func localLANAddresses() ([]lanAddress, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	result := []lanAddress{}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, value := range addresses {
			prefix, err := netip.ParsePrefix(value.String())
			if err != nil || !prefix.Addr().Is4() || !prefix.Addr().IsPrivate() || prefix.Bits() < 1 || prefix.Bits() > 30 {
				continue
			}
			result = append(result, lanAddress{Interface: iface.Name, IPv4: prefix.Addr().String(), PrefixLen: prefix.Bits()})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Interface == result[j].Interface {
			return result[i].IPv4 < result[j].IPv4
		}
		return result[i].Interface < result[j].Interface
	})
	return result, nil
}

func validLANSettings(value lanSettings) bool {
	ip, err := netip.ParseAddr(value.IPv4)
	return err == nil && ip.Is4() && ip.IsPrivate() && value.Interface != "" &&
		value.PrefixLen >= 1 && value.PrefixLen <= 30 && value.Port >= 1024 && value.Port <= 65535
}

func (c *lanAccessController) bindingAvailable(value lanSettings) bool {
	if !validLANSettings(value) {
		return false
	}
	addresses, err := c.addresses()
	if err != nil {
		return false
	}
	for _, address := range addresses {
		if address.Interface == value.Interface && address.IPv4 == value.IPv4 && address.PrefixLen == value.PrefixLen {
			return true
		}
	}
	return false
}

func lanConfigRevision(value lanAccessConfig) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (c *lanAccessController) response() lanAccessResponse {
	c.mu.Lock()
	cfg, problem, listening := c.config, c.problem, c.server != nil
	c.mu.Unlock()
	addresses, err := c.addresses()
	if err != nil || addresses == nil {
		addresses = []lanAddress{}
	}
	state := "disabled"
	if cfg.Enabled || problem != "" {
		state = "unavailable"
	}
	url := ""
	if listening && c.bindingAvailable(cfg.lanSettings) {
		state = "listening"
		url = "http://" + net.JoinHostPort(cfg.IPv4, strconv.Itoa(cfg.Port)) + "/"
	} else if listening {
		problem = "address_unavailable"
	}
	response := lanAccessResponse{SchemaVersion: 1, lanSettings: cfg.lanSettings, Revision: lanConfigRevision(cfg),
		PasswordSet: cfg.PasswordHash != "", State: state, URL: url, Error: problem, Addresses: addresses}
	if gateway, err := config.Load(c.owner.configPath); err == nil {
		response.RecommendedInterface = gateway.Gateway.Interface
	}
	return response
}

func (c *lanAccessController) start(ctx context.Context) {
	c.opMu.Lock()
	c.reconcile()
	c.opMu.Unlock()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				c.close()
				return
			case <-ticker.C:
				c.expireSessions()
				c.opMu.Lock()
				c.reconcile()
				c.opMu.Unlock()
			}
		}
	}()
}

// Call with opMu held. A vanished address pauses access until a local settings
// save confirms the binding again; returning to a network never silently opens it.
func (c *lanAccessController) reconcile() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if c.pauseDirty && c.persist(c.config) == nil {
		c.pauseDirty = false
	}
	if !c.config.Enabled || c.problem != "" {
		return
	}
	if !c.bindingAvailable(c.config.lanSettings) {
		c.stopLocked()
		c.problem = "address_unavailable"
		c.config.Paused = true
		// Preserve the pause across service restarts. A storage failure still
		// leaves this process closed; retry persistence on the next monitor tick.
		c.pauseDirty = c.persist(c.config) != nil
		return
	}
	if c.server != nil || c.problem != "" {
		return
	}
	listener, err := c.listen("tcp4", net.JoinHostPort(c.config.IPv4, strconv.Itoa(c.config.Port)))
	if err != nil {
		c.problem = "listen_failed"
		return
	}
	c.generation++
	server := &http.Server{Handler: c.handler(c.config.lanSettings, c.generation), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	c.server = server
	go func() {
		err := server.Serve(listener)
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.server == server {
			c.server = nil
			c.revokeLocked()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				c.problem = "listen_failed"
			}
		}
	}()
}

func (c *lanAccessController) stopLocked() {
	c.generation++
	c.revokeLocked()
	if c.server != nil {
		_ = c.server.Close()
		c.server = nil
	}
}

func (c *lanAccessController) close() {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.stopLocked()
}

// persist changes only after validation; a storage failure preserves the running
// listener and its sessions. Password resets do not interrupt unrelated actions.
func (c *lanAccessController) persist(value lanAccessConfig) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(c.path(), append(data, '\n'), 0o600)
}

func (c *lanAccessController) save(value lanAccessConfig, rebind bool) error {
	if err := c.persist(value); err != nil {
		return err
	}
	c.mu.Lock()
	if rebind {
		c.stopLocked()
	} else {
		c.revokeLocked()
	}
	c.config = value
	c.pauseDirty = false
	if rebind || c.problem == "configuration_invalid" {
		c.problem = ""
	}
	c.mu.Unlock()
	if rebind {
		c.reconcile()
	}
	return nil
}

func (c *lanAccessController) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		response := c.response()
		w.Header().Set("ETag", `"`+response.Revision+`"`)
		writeJSON(w, http.StatusOK, response)
		return
	}
	var request struct {
		lanSettings
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &request, 2048); err != nil {
		writeError(w, 400, "invalid_request", "invalid LAN settings")
		return
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.mu.Lock()
	cfg, closed := c.config, c.closed
	c.mu.Unlock()
	if closed {
		writeError(w, 503, "service_stopping", "control service is stopping")
		return
	}
	if strings.Trim(r.Header.Get("If-Match"), `"`) != lanConfigRevision(cfg) {
		writeError(w, 409, "revision_conflict", "reload LAN settings before saving")
		return
	}
	if request.Enabled && (!validLANSettings(request.lanSettings) || !c.bindingAvailable(request.lanSettings)) {
		writeError(w, 400, "lan_address_unavailable", "choose a current private IPv4 address and subnet")
		return
	}
	if request.Password != "" {
		hash, err := hashLANPassword(request.Password)
		if err != nil {
			writeError(w, 400, "password_invalid", "password must contain 12 to 128 characters")
			return
		}
		cfg.PasswordHash = hash
	}
	if request.Enabled && cfg.PasswordHash == "" {
		writeError(w, 400, "password_required", "set a management password before enabling LAN access")
		return
	}
	if request.Enabled {
		cfg.lanSettings = request.lanSettings
	} else {
		cfg.Enabled = false
	}
	cfg.Paused = false
	if err := c.save(cfg, true); err != nil {
		writeError(w, 500, "lan_save_failed", "could not save LAN settings")
		return
	}
	writeJSON(w, http.StatusOK, c.response())
}

func (c *lanAccessController) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &request, 1024); err != nil {
		writeError(w, 400, "invalid_request", "invalid password request")
		return
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.mu.Lock()
	cfg, closed := c.config, c.closed
	c.mu.Unlock()
	if closed {
		writeError(w, 503, "service_stopping", "control service is stopping")
		return
	}
	if strings.Trim(r.Header.Get("If-Match"), `"`) != lanConfigRevision(cfg) {
		writeError(w, 409, "revision_conflict", "reload LAN settings before saving")
		return
	}
	hash, err := hashLANPassword(request.Password)
	if err != nil {
		writeError(w, 400, "password_invalid", "password must contain 12 to 128 characters")
		return
	}
	cfg.PasswordHash = hash
	if err := c.save(cfg, false); err != nil {
		writeError(w, 500, "lan_save_failed", "could not save password")
		return
	}
	writeJSON(w, http.StatusOK, c.response())
}

func (c *lanAccessController) handler(binding lanSettings, generation uint64) http.Handler {
	routes := c.owner.routes(c.auth, false)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		setControlSecurityHeaders(w)
		c.mu.Lock()
		active := c.config.Enabled && c.generation == generation && !c.closed && c.problem == ""
		c.mu.Unlock()
		if !active || !c.bindingAvailable(binding) {
			writeError(w, 503, "lan_unavailable", "LAN access is unavailable")
			return
		}
		peer, err := netip.ParseAddrPort(r.RemoteAddr)
		prefix := netip.PrefixFrom(netip.MustParseAddr(binding.IPv4), binding.PrefixLen).Masked()
		if err != nil || !peer.Addr().Is4() || !prefix.Contains(peer.Addr()) {
			writeError(w, 403, "lan_peer_rejected", "client is outside the configured LAN")
			return
		}
		host := net.JoinHostPort(binding.IPv4, strconv.Itoa(binding.Port))
		if r.Host != host {
			writeError(w, 403, "invalid_host", "request host is not allowed")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get("Origin") != "http://"+host {
				writeError(w, 403, "origin_rejected", "mutation origin is not allowed")
				return
			}
		}
		if r.URL.Path == "/bootstrap" || strings.HasPrefix(r.URL.Path, "/desktop") {
			http.NotFound(w, r)
			return
		}
		routes.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), lanGenerationKey{}, generation)))
	})
}
