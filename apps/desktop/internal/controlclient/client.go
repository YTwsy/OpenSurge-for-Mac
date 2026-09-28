// Package controlclient discovers the existing per-user Control Service and
// authenticates the desktop's native API transport. It contains no gateway rules.
package controlclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnavailable = errors.New("OpenSurge Control Service is not ready")
	ErrDescriptor  = errors.New("invalid local Control Service descriptor")
	ErrCredential  = errors.New("local Control Service credential is unavailable")
	ErrResponse    = errors.New("invalid Control Service bootstrap response")
)

type Client struct {
	directory string
	http      *http.Client
	api       *http.Client
	mutations *http.Client
	mu        sync.Mutex
	session   *session
	retryAt   time.Time
	lastError error
}

func DefaultDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "OpenSurge"), nil
}

func New(directory string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// The credential must never travel through a configured HTTP proxy or DNS.
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || (host != "127.0.0.1" && host != "localhost") {
			return nil, ErrDescriptor
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp4", net.JoinHostPort("127.0.0.1", port))
	}
	newHTTP := func(t *http.Transport, timeout time.Duration) *http.Client {
		return &http.Client{
			Transport: t, Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	apiTransport := transport.Clone()
	apiTransport.ResponseHeaderTimeout = 2 * time.Minute
	mutationTransport := apiTransport.Clone()
	// A fresh connection prevents net/http's automatic retry on a reused
	// connection, including bodyless POSTs carrying an Idempotency-Key.
	mutationTransport.DisableKeepAlives = true
	return &Client{directory: directory, api: newHTTP(apiTransport, 0), mutations: newHTTP(mutationTransport, 0), http: &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
		// Never follow a redirect carrying the native credential.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// BootstrapURL rereads discovery and credentials for every connection attempt,
// including after a service restart. The desktop exchanges this grant natively.
func (c *Client) BootstrapURL(ctx context.Context, path string) (string, error) {
	discovery, err := c.discover()
	if err != nil {
		return "", err
	}
	return c.bootstrapURL(ctx, discovery, path)
}

// OpenBrowser mints a separate dashboard grant on each explicit request. The
// validated URL goes straight to the native opener, never to the renderer or
// through the desktop session exchange (which would consume the browser grant).
func (c *Client) OpenBrowser(ctx context.Context, openURL func(string) error) error {
	location, err := c.BootstrapURL(ctx, "dashboard")
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := openURL(location); err != nil {
		// Native opener errors can include the one-time URL.
		return errors.New("could not open browser")
	}
	return nil
}

type discovery struct {
	base     *url.URL
	token    string
	identity [32]byte
}

func (c *Client) discover() (discovery, error) {
	data, err := readLimited(filepath.Join(c.directory, "control-endpoint.json"), 16<<10)
	if err != nil {
		return discovery{}, ErrUnavailable
	}
	var descriptor struct {
		SchemaVersion int    `json:"schema_version"`
		URL           string `json:"url"`
	}
	if json.Unmarshal(data, &descriptor) != nil || descriptor.SchemaVersion != 1 {
		return discovery{}, ErrDescriptor
	}
	base, err := url.Parse(descriptor.URL)
	if err != nil || !validEndpoint(base) {
		return discovery{}, ErrDescriptor
	}
	tokenBytes, err := readLimited(filepath.Join(c.directory, "control-token"), 4<<10)
	token := strings.TrimSpace(string(tokenBytes))
	if err != nil || token == "" || strings.ContainsAny(token, "\r\n\t ") {
		return discovery{}, ErrCredential
	}
	return discovery{base: base, token: token, identity: sha256.Sum256(append(data, tokenBytes...))}, nil
}

func (c *Client) bootstrapURL(ctx context.Context, discovery discovery, path string) (string, error) {
	base, token := discovery.base, discovery.token
	body, _ := json.Marshal(struct {
		Path string `json:"path"`
	}{Path: path})
	endpoint := *base
	endpoint.Path = "/api/v1/session/bootstrap"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return "", ErrDescriptor
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("Control Service bootstrap failed (HTTP %d)", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (16<<10)+1))
	var grant struct {
		SchemaVersion int       `json:"schema_version"`
		URL           string    `json:"url"`
		ExpiresAt     time.Time `json:"expires_at"`
	}
	if err != nil || len(data) > 16<<10 || json.Unmarshal(data, &grant) != nil ||
		grant.SchemaVersion != 1 || !grant.ExpiresAt.After(time.Now()) {
		return "", ErrResponse
	}
	location, err := url.Parse(grant.URL)
	if err != nil || location.Scheme != base.Scheme || location.Host != base.Host ||
		location.User != nil || location.Opaque != "" || location.Path != "/bootstrap" ||
		location.RawPath != "" || location.Fragment != "" {
		return "", ErrResponse
	}
	query, err := url.ParseQuery(location.RawQuery)
	if err != nil || len(query) != 1 || len(query["code"]) != 1 || query.Get("code") == "" {
		return "", ErrResponse
	}
	return location.String(), nil
}

func validEndpoint(value *url.URL) bool {
	if value.Scheme != "http" || value.User != nil || value.Opaque != "" ||
		(value.Hostname() != "127.0.0.1" && value.Hostname() != "localhost") ||
		(value.Path != "" && value.Path != "/") || value.RawPath != "" ||
		value.RawQuery != "" || value.ForceQuery || value.Fragment != "" {
		return false
	}
	port, err := strconv.Atoi(value.Port())
	return err == nil && port > 0 && port <= 65535
}

func readLimited(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("expected a regular credential file")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("credential file cannot be read within limit")
	}
	return data, nil
}
