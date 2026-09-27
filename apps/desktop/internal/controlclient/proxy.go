package controlclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type session struct {
	discovery discovery
	cookie    *http.Cookie
}

// authenticatedSession keeps credentials and cookies in native memory. Every
// request rechecks discovery so a service restart/port change needs no UI reload.
func (c *Client) authenticatedSession(ctx context.Context) (*session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Before(c.retryAt) {
		return nil, c.lastError
	}
	d, err := c.discover()
	if err == nil && c.session != nil && c.session.discovery.identity == d.identity && c.session.cookie.Expires.After(time.Now().Add(time.Minute)) {
		return c.session, nil
	}
	if err == nil {
		c.session, err = c.openSession(ctx, d)
		if err == nil {
			return c.session, nil
		}
	}
	// Do not reveal bootstrap URLs or credentials from transport errors.
	c.session = nil
	c.lastError = ErrUnavailable
	c.retryAt = time.Now().Add(time.Second)
	return nil, c.lastError
}

func (c *Client) openSession(ctx context.Context, d discovery) (*session, error) {
	location, err := c.bootstrapURL(ctx, d, "dashboard")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusSeeOther || response.StatusCode == http.StatusFound {
		for _, cookie := range response.Cookies() {
			if cookie.Name == "opensurge_session" && cookie.Value != "" && cookie.HttpOnly && cookie.Expires.After(time.Now()) {
				return &session{discovery: d, cookie: cookie}, nil
			}
		}
	}
	return nil, ErrResponse
}

func (c *Client) invalidate(old *session) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == old {
		c.session = nil
	}
}

// ServeHTTP relays only the existing Control API. It never retries a mutation,
// even after a confirmed authentication failure; the user decides whether to retry.
func (c *Client) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/api/v1/") || strings.HasPrefix(r.URL.Path, "/api/v1/session/") {
		http.NotFound(w, r)
		return
	}
	current, err := c.authenticatedSession(r.Context())
	if err != nil {
		unavailable(w)
		return
	}
	response, err := c.forward(r, current)
	readOnly := r.Method == http.MethodGet || r.Method == http.MethodHead
	if err == nil && response.StatusCode == http.StatusUnauthorized {
		response.Body.Close()
		c.invalidate(current)
		if readOnly {
			current, err = c.authenticatedSession(r.Context())
			if err == nil {
				response, err = c.forward(r, current)
			}
		} else {
			// Authentication rejected the action; renew lazily for the next
			// explicit attempt, without replaying this request or losing drafts.
			unavailable(w)
			return
		}
	}
	if err != nil {
		unavailable(w)
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		c.invalidate(current)
		unavailable(w)
		return
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 && response.StatusCode != http.StatusNotModified {
		// An API redirect must never turn into WebView navigation.
		unavailable(w)
		return
	}
	for _, name := range []string{"Content-Type", "Content-Disposition", "ETag", "Cache-Control"} {
		if value := response.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	if strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		_ = http.NewResponseController(w).Flush()
		buffer := make([]byte, 4096)
		for {
			n, readErr := response.Body.Read(buffer)
			if n > 0 {
				if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
					return
				}
				if http.NewResponseController(w).Flush() != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}
	_, _ = io.Copy(w, response.Body)
}

func (c *Client) forward(in *http.Request, current *session) (*http.Response, error) {
	target := *current.discovery.base
	target.Path, target.RawPath = in.URL.Path, in.URL.RawPath
	query := in.URL.Query()
	query.Del("desktop_session")
	target.RawQuery = query.Encode()
	out := in.Clone(in.Context())
	out.URL, out.Host, out.RequestURI = &target, target.Host, ""
	out.GetBody = nil
	out.Header = in.Header.Clone()
	for _, name := range []string{"Authorization", "Cookie", "Origin", "Referer", "X-OpenSurge-Desktop", "Connection", "Proxy-Authorization", "Proxy-Connection", "Transfer-Encoding", "Upgrade"} {
		out.Header.Del(name)
	}
	out.AddCookie(current.cookie)
	out.Header.Set("Origin", (&url.URL{Scheme: target.Scheme, Host: target.Host}).String())
	if in.Method != http.MethodGet && in.Method != http.MethodHead {
		return c.mutations.Do(out)
	}
	return c.api.Do(out)
}

func unavailable(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "desktop_service_unavailable", "message": "OpenSurge Control Service is reconnecting. Your current page is preserved; retry the action once connected."}})
}
