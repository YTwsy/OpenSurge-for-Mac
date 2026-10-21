package controlapi

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"time"
)

const lanSessionCookie = "opensurge_lan_session"

type lanGenerationKey struct{}

type lanSession struct {
	expires time.Time
	created time.Time
	ctx     context.Context
	cancel  context.CancelFunc
}

type lanAttemptWindow struct {
	start time.Time
	count int
}

func (c *lanAccessController) revokeLocked() {
	for key, session := range c.sessions {
		session.cancel()
		delete(c.sessions, key)
	}
}

func (c *lanAccessController) pruneLocked(now time.Time) {
	for key, session := range c.sessions {
		if !now.Before(session.expires) {
			session.cancel()
			delete(c.sessions, key)
		}
	}
}

// Bound both per-peer guessing and total memory/CPU use. Limits apply before
// expensive password hashing and include password-change attempts.
func (c *lanAccessController) beginPasswordCheck(w http.ResponseWriter, r *http.Request) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		writeError(w, 403, "lan_peer_rejected", "invalid client address")
		return false
	}
	now := c.now()
	c.mu.Lock()
	for peer, window := range c.attempts {
		if now.Sub(window.start) >= time.Minute {
			delete(c.attempts, peer)
		}
	}
	window := c.attempts[ip]
	if now.Sub(window.start) >= time.Minute {
		window = lanAttemptWindow{start: now}
	}
	if now.Sub(c.globalAttempts.start) >= time.Minute {
		c.globalAttempts = lanAttemptWindow{start: now}
	}
	limited := window.count >= 5 || c.globalAttempts.count >= 60 || len(c.attempts) >= 1024
	if !limited {
		window.count++
		c.attempts[ip] = window
		c.globalAttempts.count++
	}
	c.mu.Unlock()
	if !limited {
		select {
		case c.passwordSlots <- struct{}{}:
			return true
		default:
		}
	}
	w.Header().Set("Retry-After", "60")
	writeError(w, http.StatusTooManyRequests, "login_limited", "too many attempts; try again shortly")
	return false
}

func setLANSessionCookie(w http.ResponseWriter, token string, now time.Time) {
	// Secure must remain false for the explicitly selected HTTP transport.
	http.SetCookie(w, &http.Cookie{Name: lanSessionCookie, Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Expires: now.Add(webSessionIdleTimeout), MaxAge: int(webSessionIdleTimeout / time.Second)})
}

func clearLANSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: lanSessionCookie, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}

func (c *lanAccessController) authenticated(r *http.Request, renew bool) (*lanSession, string) {
	cookie, err := r.Cookie(lanSessionCookie)
	if err != nil {
		return nil, ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(c.now())
	if c.closed || !c.config.Enabled || c.problem != "" {
		return nil, ""
	}
	if generation, ok := r.Context().Value(lanGenerationKey{}).(uint64); ok && generation != c.generation {
		return nil, ""
	}
	session := c.sessions[cookie.Value]
	if session != nil && renew {
		session.expires = c.now().Add(webSessionIdleTimeout)
	}
	return session, cookie.Value
}

func (c *lanAccessController) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, token := c.authenticated(r, true)
		if session == nil {
			writeError(w, 401, "authentication_required", "enter the management password")
			return
		}
		setLANSessionCookie(w, token, c.now())
		// Revoke open reads, including SSE, without canceling already-authorized
		// network mutations. Those retain the existing operation lifecycle.
		if r.Method == http.MethodGet || r.Header.Get("Accept") == "text/event-stream" {
			ctx, cancel := context.WithCancel(r.Context())
			stop := context.AfterFunc(session.ctx, cancel)
			defer stop()
			defer cancel()
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

func (c *lanAccessController) handleSession(w http.ResponseWriter, r *http.Request) {
	session, token := c.authenticated(r, true)
	if session != nil {
		setLANSessionCookie(w, token, c.now())
	}
	writeJSON(w, 200, map[string]any{"mode": "lan", "authenticated": session != nil, "local_client": c.isLocalClient(r)})
}

// This is only a presentation hint. Local clients using the LAN listener still
// need its password and never inherit the local control service's privileges.
func (c *lanAccessController) isLocalClient(r *http.Request) bool {
	peer, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	addresses, err := c.addresses()
	if err != nil {
		return false
	}
	for _, address := range addresses {
		if peer.Addr().String() == address.IPv4 {
			return true
		}
	}
	return false
}

func (c *lanAccessController) handleLogin(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &request, 1024); err != nil {
		writeError(w, 400, "invalid_request", "invalid login request")
		return
	}
	if !c.beginPasswordCheck(w, r) {
		return
	}
	defer func() { <-c.passwordSlots }()
	c.mu.Lock()
	hash := c.config.PasswordHash
	generation, admitted := r.Context().Value(lanGenerationKey{}).(uint64)
	c.mu.Unlock()
	valid := verifyLANPassword(hash, request.Password)
	c.mu.Lock()
	defer c.mu.Unlock()
	if !valid || !admitted || c.config.PasswordHash != hash || c.generation != generation || !c.config.Enabled || c.closed || c.problem != "" {
		writeError(w, 401, "password_incorrect", "incorrect management password")
		return
	}
	now := c.now()
	c.pruneLocked(now)
	if old, err := r.Cookie(lanSessionCookie); err == nil {
		if session := c.sessions[old.Value]; session != nil {
			session.cancel()
			delete(c.sessions, old.Value)
		}
	}
	// A household-sized bounded session set. A new login replaces the oldest
	// session when full rather than growing the control service indefinitely.
	if len(c.sessions) >= 64 {
		oldestKey := ""
		var oldest time.Time
		for key, session := range c.sessions {
			if oldestKey == "" || session.created.Before(oldest) {
				oldestKey, oldest = key, session.created
			}
		}
		c.sessions[oldestKey].cancel()
		delete(c.sessions, oldestKey)
	}
	token := randomToken(32)
	ctx, cancel := context.WithCancel(context.Background())
	c.sessions[token] = &lanSession{expires: now.Add(webSessionIdleTimeout), created: now, ctx: ctx, cancel: cancel}
	setLANSessionCookie(w, token, now)
	writeJSON(w, 200, map[string]any{"mode": "lan", "authenticated": true, "local_client": c.isLocalClient(r)})
}

func (c *lanAccessController) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(lanSessionCookie); err == nil {
		c.mu.Lock()
		if session := c.sessions[cookie.Value]; session != nil {
			session.cancel()
			delete(c.sessions, cookie.Value)
		}
		c.mu.Unlock()
	}
	clearLANSessionCookie(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (c *lanAccessController) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var request struct {
		OldPassword string `json:"old_password"`
		Password    string `json:"password"`
	}
	if err := decodeJSON(r, &request, 2048); err != nil {
		writeError(w, 400, "invalid_request", "invalid password request")
		return
	}
	if !validLANPassword(request.Password) {
		writeError(w, 400, "password_invalid", "password must contain 12 to 128 characters")
		return
	}
	if !c.beginPasswordCheck(w, r) {
		return
	}
	defer func() { <-c.passwordSlots }()
	c.opMu.Lock()
	defer c.opMu.Unlock()
	// Recheck after acquiring the mutation lock: another reset may have revoked
	// the request's session while it was waiting.
	if session, _ := c.authenticated(r, false); session == nil {
		writeError(w, 401, "authentication_required", "enter the management password")
		return
	}
	c.mu.Lock()
	cfg := c.config
	c.mu.Unlock()
	if !verifyLANPassword(cfg.PasswordHash, request.OldPassword) {
		writeError(w, 400, "password_incorrect", "incorrect management password")
		return
	}
	hash, err := hashLANPassword(request.Password)
	if err != nil {
		writeError(w, 500, "lan_save_failed", "could not change password")
		return
	}
	cfg.PasswordHash = hash
	if err := c.save(cfg, false); err != nil {
		writeError(w, 500, "lan_save_failed", "could not save password")
		return
	}
	clearLANSessionCookie(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// sessionExpired reports expiration even for long-lived streams with no new
// HTTP requests. Heartbeats must not keep an abandoned browser authenticated.
func (c *lanAccessController) expireSessions() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(c.now())
}
