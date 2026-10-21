package controlapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const testLANPassword = "correct horse lantern"

func testLANController(t *testing.T) (*Server, *lanAccessController) {
	t.Helper()
	s := newTestServer(t)
	c := s.lan
	c.addresses = func() ([]lanAddress, error) {
		return []lanAddress{{Interface: "en0", IPv4: "192.168.50.1", PrefixLen: 22}}, nil
	}
	c.listen = func(network, address string) (net.Listener, error) { return net.Listen("tcp4", "127.0.0.1:0") }
	t.Cleanup(c.close)
	return s, c
}

func configureLAN(t *testing.T, s *Server, enabled bool, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"enabled": enabled, "interface": "en0", "ipv4": "192.168.50.1", "prefix_len": 22, "port": defaultLANPort, "password": password})
	req := httptest.NewRequest("PUT", s.baseURL+"/api/v1/lan-access", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("If-Match", `"`+s.lan.response().Revision+`"`)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	return rr
}

func performLAN(c *lanAccessController, method, path, body string, cookie *http.Cookie, edit func(*http.Request)) *httptest.ResponseRecorder {
	c.mu.Lock()
	binding, generation := c.config.lanSettings, c.generation
	c.mu.Unlock()
	req := httptest.NewRequest(method, "http://192.168.50.1:61768"+path, strings.NewReader(body))
	req.RemoteAddr = "192.168.51.102:34000"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://192.168.50.1:61768")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if edit != nil {
		edit(req)
	}
	rr := httptest.NewRecorder()
	c.handler(binding, generation).ServeHTTP(rr, req)
	return rr
}

func loginLAN(t *testing.T, c *lanAccessController) *http.Cookie {
	t.Helper()
	r := performLAN(c, "POST", "/api/v1/auth/login", `{"password":"`+testLANPassword+`"}`, nil, nil)
	if r.Code != 200 {
		t.Fatalf("login: %d %s", r.Code, r.Body.String())
	}
	for _, cookie := range r.Result().Cookies() {
		if cookie.Name == lanSessionCookie {
			return cookie
		}
	}
	t.Fatal("no LAN session cookie")
	return nil
}

func TestLANAccessDefaultPersistenceAndFailureIsolation(t *testing.T) {
	s, c := testLANController(t)
	if state := c.response(); state.Enabled || state.State != "disabled" || state.PasswordSet || state.URL != "" {
		t.Fatalf("default: %+v", state)
	}
	if r := configureLAN(t, s, true, ""); r.Code != 400 {
		t.Fatalf("password required: %d", r.Code)
	}
	if r := configureLAN(t, s, true, "short"); r.Code != 400 {
		t.Fatalf("short password: %d", r.Code)
	}
	if r := configureLAN(t, s, true, testLANPassword); r.Code != 200 {
		t.Fatalf("enable: %d %s", r.Code, r.Body.String())
	}
	state := c.response()
	if state.State != "listening" || state.URL != "http://192.168.50.1:61768/" || !state.PasswordSet {
		t.Fatalf("active: %+v", state)
	}
	data, _ := os.ReadFile(c.path())
	info, _ := os.Stat(c.path())
	if bytes.Contains(data, []byte(testLANPassword)) || !bytes.Contains(data, []byte("argon2id")) || info.Mode().Perm() != 0o600 {
		t.Fatal("invalid credential storage")
	}
	public, _ := json.Marshal(state)
	if bytes.Contains(public, []byte("argon2")) || bytes.Contains(public, []byte("password_hash")) {
		t.Fatal("credential exposed")
	}
	loginLAN(t, c)
	reloaded := newLANAccessController(s)
	if !reloaded.config.Enabled || !verifyLANPassword(reloaded.config.PasswordHash, testLANPassword) || len(reloaded.sessions) != 0 {
		t.Fatal("restart persistence/session boundary")
	}
	c.listen = func(_, _ string) (net.Listener, error) { return nil, errors.New("in use") }
	if r := configureLAN(t, s, true, ""); r.Code != 200 {
		t.Fatalf("binding status: %d", r.Code)
	}
	if state := c.response(); state.State != "unavailable" || state.Error != "listen_failed" || state.URL != "" {
		t.Fatalf("conflict: %+v", state)
	}
	if r := performAuthorized(s, "GET", "/api/v1/ui-preferences", nil); r.Code != 200 {
		t.Fatal("LAN failure broke local service")
	}
	if r := configureLAN(t, s, false, ""); r.Code != 200 || c.response().State != "disabled" {
		t.Fatalf("disable: %d", r.Code)
	}
}

func TestLANSessionClientLocation(t *testing.T) {
	s, c := testLANController(t)
	c.addresses = func() ([]lanAddress, error) {
		return []lanAddress{
			{Interface: "en0", IPv4: "192.168.50.1", PrefixLen: 22},
			{Interface: "en7", IPv4: "192.168.50.2", PrefixLen: 22},
		}, nil
	}
	if r := configureLAN(t, s, true, testLANPassword); r.Code != 200 {
		t.Fatalf("enable: %d", r.Code)
	}
	for _, test := range []struct {
		name, peer, forwarded string
		local                 bool
	}{
		{name: "remote device", peer: "192.168.51.102:34000"},
		{name: "gateway bound address", peer: "192.168.50.1:34000", local: true},
		{name: "gateway other address", peer: "192.168.50.2:34000", local: true},
		{name: "forwarded header ignored", peer: "192.168.51.102:34000", forwarded: "192.168.50.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			edit := func(r *http.Request) {
				r.RemoteAddr = test.peer
				r.Header.Set("X-Forwarded-For", test.forwarded)
			}
			if r := performLAN(c, "GET", "/api/v1/ui-preferences", "", nil, edit); r.Code != 401 {
				t.Fatalf("client location must not bypass authentication: %d", r.Code)
			}
			for _, method := range []string{"GET", "POST"} {
				path, body := "/api/v1/auth/session", ""
				if method == "POST" {
					path, body = "/api/v1/auth/login", `{"password":"`+testLANPassword+`"}`
				}
				r := performLAN(c, method, path, body, nil, edit)
				var result struct {
					Mode          string `json:"mode"`
					Authenticated bool   `json:"authenticated"`
					LocalClient   *bool  `json:"local_client"`
				}
				if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &result) != nil || result.Mode != "lan" || result.Authenticated != (method == "POST") || result.LocalClient == nil || *result.LocalClient != test.local {
					t.Fatalf("%s client location: %d %s", method, r.Code, r.Body.String())
				}
			}
		})
	}
}

func TestLANAuthenticationBoundaryAndCSRF(t *testing.T) {
	s, c := testLANController(t)
	configureLAN(t, s, true, testLANPassword)
	if r := performLAN(c, "GET", "/api/v1/ui-preferences", "", nil, nil); r.Code != 401 {
		t.Fatalf("unauthenticated: %d", r.Code)
	}
	if r := performLAN(c, "GET", "/api/v1/ui-preferences", "", &http.Cookie{Name: "opensurge_session", Value: "local"}, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+s.token) }); r.Code != 401 {
		t.Fatal("local credential accepted remotely")
	}
	cookie := loginLAN(t, c)
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Secure || cookie.Domain != "" || cookie.Path != "/" {
		t.Fatalf("cookie: %+v", cookie)
	}
	if r := performLAN(c, "GET", "/api/v1/ui-preferences", "", cookie, nil); r.Code != 200 {
		t.Fatalf("authorized /22: %d", r.Code)
	}
	for name, edit := range map[string]func(*http.Request){
		"other subnet": func(r *http.Request) {
			r.RemoteAddr = "192.168.55.2:5000"
			r.Header.Set("X-Forwarded-For", "192.168.50.5")
		},
		"VPN":            func(r *http.Request) { r.RemoteAddr = "100.64.0.2:5000" },
		"IPv6":           func(r *http.Request) { r.RemoteAddr = "[fd00::2]:5000" },
		"mapped IPv4":    func(r *http.Request) { r.RemoteAddr = "[::ffff:192.168.50.5]:5000" },
		"DNS rebinding":  func(r *http.Request) { r.Host = "attacker.example:61768" },
		"wrong port":     func(r *http.Request) { r.Host = "192.168.50.1:61767" },
		"cross origin":   func(r *http.Request) { r.Header.Set("Origin", "https://attacker.example") },
		"missing origin": func(r *http.Request) { r.Header.Del("Origin") },
	} {
		t.Run(name, func(t *testing.T) {
			if r := performLAN(c, "POST", "/api/v1/auth/login", `{"password":"`+testLANPassword+`"}`, cookie, edit); r.Code != 403 {
				t.Fatalf("status %d", r.Code)
			}
		})
	}
	for _, path := range []string{"/bootstrap?code=abc", "/api/v1/session/bootstrap", "/api/v1/lan-access", "/api/v1/lan-access/password", "/api/v1/sources/a/snapshot-location", "/api/v1/sources/a/reveal", "/api/v1/sources/a/export", "/desktop/v1/uninstall", "/desktop-settings"} {
		if r := performLAN(c, "POST", path, "{}", cookie, nil); r.Code != 404 {
			t.Fatalf("local route %s: %d", path, r.Code)
		}
	}
	r := httptest.NewRequest("GET", s.baseURL+"/api/v1/ui-preferences", nil)
	r.AddCookie(cookie)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, r)
	if rr.Code != 401 {
		t.Fatal("LAN session accepted locally")
	}
}

func TestLANSessionRenewalRevocationAndPasswordChange(t *testing.T) {
	s, c := testLANController(t)
	configureLAN(t, s, true, testLANPassword)
	now := time.Now()
	c.now = func() time.Time { return now }
	cookie := loginLAN(t, c)
	session := c.sessions[cookie.Value]
	now = now.Add(11 * time.Hour)
	if r := performLAN(c, "GET", "/api/v1/auth/session", "", cookie, nil); r.Code != 200 || !strings.Contains(r.Body.String(), `"authenticated":true`) {
		t.Fatal("active session not renewed")
	}
	if session.expires != now.Add(webSessionIdleTimeout) {
		t.Fatal("wrong sliding expiry")
	}
	other := loginLAN(t, c)
	if r := performLAN(c, "POST", "/api/v1/auth/password", `{"old_password":"wrong password","password":"new correct password"}`, cookie, nil); r.Code != 400 {
		t.Fatalf("wrong old password: %d", r.Code)
	}
	if r := performLAN(c, "POST", "/api/v1/auth/password", `{"old_password":"`+testLANPassword+`","password":"new correct password"}`, cookie, nil); r.Code != 200 {
		t.Fatalf("change: %d %s", r.Code, r.Body.String())
	}
	if session.ctx.Err() == nil {
		t.Fatal("open stream not revoked")
	}
	for _, old := range []*http.Cookie{cookie, other} {
		if r := performLAN(c, "GET", "/api/v1/ui-preferences", "", old, nil); r.Code != 401 {
			t.Fatal("old session survived password change")
		}
	}
	if !verifyLANPassword(c.config.PasswordHash, "new correct password") || verifyLANPassword(c.config.PasswordHash, testLANPassword) {
		t.Fatal("wrong persisted password")
	}
	if r := performAuthorized(s, "GET", "/api/v1/ui-preferences", nil); r.Code != 200 {
		t.Fatal("local session broken by password change")
	}
}

func TestLANExpiryLogoutDisableAndAddressChange(t *testing.T) {
	for _, action := range []string{"expire", "logout", "disable", "address", "prefix", "interface"} {
		t.Run(action, func(t *testing.T) {
			s, c := testLANController(t)
			configureLAN(t, s, true, testLANPassword)
			cookie := loginLAN(t, c)
			session := c.sessions[cookie.Value]
			switch action {
			case "expire":
				c.mu.Lock()
				session.expires = time.Now().Add(-time.Second)
				c.mu.Unlock()
				c.expireSessions()
			case "logout":
				performLAN(c, "POST", "/api/v1/auth/logout", "{}", cookie, nil)
			case "disable":
				configureLAN(t, s, false, "")
			default:
				address := lanAddress{Interface: "en0", IPv4: "192.168.50.1", PrefixLen: 22}
				if action == "address" {
					address.IPv4 = "192.168.50.2"
				}
				if action == "prefix" {
					address.PrefixLen = 24
				}
				if action == "interface" {
					address.Interface = "en7"
				}
				c.addresses = func() ([]lanAddress, error) { return []lanAddress{address}, nil }
				if r := performLAN(c, "GET", "/api/v1/ui-preferences", "", cookie, nil); r.Code != 503 {
					t.Fatal("drift leaked data before monitor")
				}
				c.opMu.Lock()
				c.reconcile()
				c.opMu.Unlock()
				c.addresses = func() ([]lanAddress, error) {
					return []lanAddress{{Interface: "en0", IPv4: "192.168.50.1", PrefixLen: 22}}, nil
				}
				c.opMu.Lock()
				c.reconcile()
				c.opMu.Unlock()
				if c.response().State != "unavailable" {
					t.Fatal("silently reopened after network change")
				}
				reloaded := newLANAccessController(s)
				reloaded.addresses = c.addresses
				reloaded.reconcile()
				if reloaded.response().State != "unavailable" || reloaded.server != nil {
					t.Fatal("service restart cleared the network-change pause")
				}
				if r := configureLAN(t, s, true, ""); r.Code != 200 || c.response().State != "listening" {
					t.Fatal("local confirmation did not resume access")
				}
			}
			if session.ctx.Err() == nil {
				t.Fatal("revocation did not cancel stream")
			}
			if r := performLAN(c, "GET", "/api/v1/ui-preferences", "", cookie, nil); r.Code == 200 {
				t.Fatal("revoked session still works")
			}
		})
	}
}

func TestLANLimitsCorruptConfigAndRevision(t *testing.T) {
	s, c := testLANController(t)
	configureLAN(t, s, true, testLANPassword)
	for i := 0; i < 5; i++ {
		if r := performLAN(c, "POST", "/api/v1/auth/login", `{"password":"wrong long password"}`, nil, nil); r.Code != 401 {
			t.Fatalf("attempt %d: %d", i, r.Code)
		}
	}
	if r := performLAN(c, "POST", "/api/v1/auth/login", `{"password":"`+testLANPassword+`"}`, nil, nil); r.Code != 429 || r.Header().Get("Retry-After") == "" {
		t.Fatal("missing login throttle")
	}
	c.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	loginLAN(t, c)
	req := httptest.NewRequest("PUT", s.baseURL+"/api/v1/lan-access", strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("If-Match", `"old"`)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 409 || c.response().State != "listening" {
		t.Fatal("stale write changed binding")
	}
	if err := os.WriteFile(c.path(), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	broken := newLANAccessController(s)
	if broken.response().State != "unavailable" || broken.response().Error != "configuration_invalid" {
		t.Fatal("invalid state not surfaced")
	}
	broken.opMu.Lock()
	broken.reconcile()
	broken.opMu.Unlock()
	if broken.server != nil {
		t.Fatal("invalid credentials opened listener")
	}
}

func TestLANOpenStreamStopsOnLogout(t *testing.T) {
	s, c := testLANController(t)
	configureLAN(t, s, true, testLANPassword)
	cookie := loginLAN(t, c)
	started, stopped := make(chan struct{}), make(chan struct{})
	handler := c.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(stopped) }))
	req := httptest.NewRequest("GET", "http://192.168.50.1:61768/api/v1/events", nil)
	req.AddCookie(cookie)
	done := make(chan struct{})
	go func() { defer close(done); handler.ServeHTTP(httptest.NewRecorder(), req) }()
	<-started
	performLAN(c, "POST", "/api/v1/auth/logout", "{}", cookie, nil)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stream did not close")
	}
	<-done
}

func TestLANListenerIsReleasedWithoutStoppingLocalService(t *testing.T) {
	s, c := testLANController(t)
	var address string
	c.listen = func(_, _ string) (net.Listener, error) {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err == nil {
			address = listener.Addr().String()
		}
		return listener, err
	}
	configureLAN(t, s, true, testLANPassword)
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + address + "/api/v1/auth/session")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("loopback test transport unexpectedly bypassed LAN peer check")
	}
	configureLAN(t, s, false, "")
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatalf("port not released: %v", err)
	}
	listener.Close()
	if r := performAuthorized(s, "GET", "/api/v1/ui-preferences", nil); r.Code != 200 {
		t.Fatal("local service unavailable")
	}
}

func TestLANConcurrentLoginAndReset(t *testing.T) {
	s, c := testLANController(t)
	configureLAN(t, s, true, testLANPassword)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			performLAN(c, "POST", "/api/v1/auth/login", `{"password":"`+testLANPassword+`"}`, nil, nil)
		}()
	}
	req := httptest.NewRequest("POST", s.baseURL+"/api/v1/lan-access/password", strings.NewReader(`{"password":"new management password"}`))
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("If-Match", c.response().Revision)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	wg.Wait()
	if rr.Code != 200 {
		t.Fatalf("reset: %d", rr.Code)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sessions) != 0 {
		t.Fatal("in-flight old login survived reset")
	}
}

func TestLANLoginAdmittedBeforeRebindCannotCreateSession(t *testing.T) {
	s, c := testLANController(t)
	configureLAN(t, s, true, testLANPassword)
	oldGeneration := c.generation
	configureLAN(t, s, true, "")
	req := httptest.NewRequest("POST", "http://192.168.50.1:61768/api/v1/auth/login", strings.NewReader(`{"password":"`+testLANPassword+`"}`))
	req.RemoteAddr = "192.168.50.5:34000"
	req = req.WithContext(context.WithValue(req.Context(), lanGenerationKey{}, oldGeneration))
	rr := httptest.NewRecorder()
	c.handleLogin(rr, req)
	if rr.Code != 401 || len(c.sessions) != 0 {
		t.Fatal("previous listener request created a new session")
	}
}
