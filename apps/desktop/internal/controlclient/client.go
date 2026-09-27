// Package controlclient discovers the existing per-user Control Service and
// exchanges the native credential for a short-lived WebView bootstrap grant.
// It intentionally exposes no gateway operations.
package controlclient

import (
	"bytes"
	"context"
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
	return &Client{directory: directory, http: &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
		// Never follow a redirect carrying the native credential.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// BootstrapURL rereads discovery and credentials for every connection attempt,
// including after a service restart. Only the one-time grant reaches the WebView.
func (c *Client) BootstrapURL(ctx context.Context, path string) (string, error) {
	data, err := readLimited(filepath.Join(c.directory, "control-endpoint.json"), 16<<10)
	if err != nil {
		return "", ErrUnavailable
	}
	var descriptor struct {
		SchemaVersion int    `json:"schema_version"`
		URL           string `json:"url"`
	}
	if json.Unmarshal(data, &descriptor) != nil || descriptor.SchemaVersion != 1 {
		return "", ErrDescriptor
	}
	base, err := url.Parse(descriptor.URL)
	if err != nil || !validEndpoint(base) {
		return "", ErrDescriptor
	}
	tokenBytes, err := readLimited(filepath.Join(c.directory, "control-token"), 4<<10)
	token := strings.TrimSpace(string(tokenBytes))
	if err != nil || token == "" || strings.ContainsAny(token, "\r\n\t ") {
		return "", ErrCredential
	}
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
	data, err = io.ReadAll(io.LimitReader(response.Body, (16<<10)+1))
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
