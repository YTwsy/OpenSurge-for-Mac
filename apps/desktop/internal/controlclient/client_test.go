package controlclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func writeDiscovery(t *testing.T, directory, endpoint, token string) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"schema_version": 1, "url": endpoint, "pid": 1})
	for name, contents := range map[string][]byte{"control-endpoint.json": data, "control-token": []byte(token)} {
		if err := os.WriteFile(filepath.Join(directory, name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func writeGrant(w http.ResponseWriter, endpoint string) {
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"schema_version": 1, "url": endpoint + "/bootstrap?code=one-time-grant",
		"expires_at": time.Now().Add(30 * time.Second),
	})
}

func TestBootstrapUsesNativeCredentialAndRereadsDiscovery(t *testing.T) {
	directory := t.TempDir()
	client := New(directory)
	t.Cleanup(client.http.CloseIdleConnections)
	for _, token := range []string{"first-native-token", "rotated-native-token"} {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/session/bootstrap" ||
				r.Header.Get("Authorization") != "Bearer "+token || r.Header.Get("Content-Type") != "application/json" {
				t.Error("unexpected bootstrap request or credential")
			}
			var body struct{ Path string }
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Path != "network" {
				t.Error("requested page was not preserved")
			}
			writeGrant(w, "http://"+r.Host)
		}))
		writeDiscovery(t, directory, server.URL, token+"\n")
		location, err := client.BootstrapURL(context.Background(), "network")
		if err != nil || location != server.URL+"/bootstrap?code=one-time-grant" || calls.Load() != 1 {
			t.Fatalf("bootstrap did not return exactly one local grant: %v", err)
		}
		if strings.Contains(location, token) {
			t.Fatal("native credential exposed in navigation URL")
		}
		server.Close()
	}
}

func TestOpenBrowserUsesFreshUnconsumedDashboardGrant(t *testing.T) {
	directory := t.TempDir()
	client := New(directory)
	t.Cleanup(client.http.CloseIdleConnections)
	for _, token := range []string{"first-token", "rotated-token"} {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			var request struct{ Path string }
			if r.Method != "POST" || r.URL.Path != "/api/v1/session/bootstrap" ||
				r.Header.Get("Authorization") != "Bearer "+token ||
				json.NewDecoder(r.Body).Decode(&request) != nil || request.Path != "dashboard" {
				t.Error("browser grant must be authenticated, unconsumed and scoped to Dashboard")
			}
			writeGrant(w, "http://"+r.Host)
		}))
		writeDiscovery(t, directory, server.URL, token)
		opened := ""
		err := client.OpenBrowser(context.Background(), func(value string) error { opened = value; return nil })
		if err != nil || opened != server.URL+"/bootstrap?code=one-time-grant" || calls.Load() != 1 {
			t.Fatalf("unexpected browser launch: %v, requests=%d", err, calls.Load())
		}
		server.Close()
	}
}

func TestOpenBrowserRejectsInvalidGrantAndRedactsOpenerErrors(t *testing.T) {
	var valid atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if valid.Load() {
			writeGrant(w, "http://"+r.Host)
		} else {
			writeGrant(w, "https://external.example")
		}
	}))
	defer server.Close()
	directory := t.TempDir()
	writeDiscovery(t, directory, server.URL, "private-token")
	client := New(directory)
	t.Cleanup(client.http.CloseIdleConnections)
	opened := 0
	opener := func(value string) error { opened++; return errors.New(value) }
	if err := client.OpenBrowser(context.Background(), opener); err == nil || opened != 0 {
		t.Fatal("unvalidated grant reached the browser")
	}
	valid.Store(true)
	if err := client.OpenBrowser(context.Background(), opener); err == nil || strings.Contains(err.Error(), "one-time-grant") || opened != 1 {
		t.Fatal("opener errors must not expose the grant")
	}
}

func TestBootstrapRejectsUntrustedDescriptorBeforeSendingCredential(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	for _, endpoint := range []string{
		"https://127.0.0.1:8080", "http://example.com:8080", "http://127.0.0.2:8080",
		"http://127.0.0.1", "http://127.0.0.1:0", "http://127.0.0.1:65536",
		"http://user@127.0.0.1:8080", server.URL + "/other", server.URL + "?next=elsewhere",
		server.URL + "?", server.URL + "#fragment", ":invalid",
	} {
		t.Run(endpoint, func(t *testing.T) {
			directory := t.TempDir()
			writeDiscovery(t, directory, endpoint, "private-token")
			_, err := New(directory).BootstrapURL(context.Background(), "dashboard")
			if !errors.Is(err, ErrDescriptor) {
				t.Fatalf("got %v, want invalid descriptor", err)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatal("invalid descriptor caused a network request")
	}
}

func TestBootstrapDoesNotFollowRedirectOrRetryMutation(t *testing.T) {
	var calls, redirected atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/capture" {
			redirected.Add(1)
			return
		}
		calls.Add(1)
		http.Redirect(w, r, "/capture", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	directory := t.TempDir()
	writeDiscovery(t, directory, server.URL, "private-token")
	client := New(directory)
	t.Cleanup(client.http.CloseIdleConnections)
	if _, err := client.BootstrapURL(context.Background(), "dashboard"); err == nil || strings.Contains(err.Error(), "private-token") {
		t.Fatal("redirect must fail without exposing the credential")
	}
	if calls.Load() != 1 || redirected.Load() != 0 {
		t.Fatal("bootstrap followed a redirect or retried the POST")
	}
}

func TestBootstrapRejectsInvalidGrant(t *testing.T) {
	for _, scenario := range []string{"remote", "other-port", "credentials", "path", "fragment", "missing-code", "duplicate-code", "extra-query", "expired", "schema", "oversized", "malformed", "unauthorized"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if scenario == "unauthorized" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.WriteHeader(http.StatusCreated)
				endpoint := "http://" + r.Host
				grant := map[string]any{"schema_version": 1, "url": endpoint + "/bootstrap?code=grant", "expires_at": time.Now().Add(30 * time.Second)}
				switch scenario {
				case "remote":
					grant["url"] = "https://example.com/bootstrap?code=grant"
				case "other-port":
					grant["url"] = "http://127.0.0.1:1/bootstrap?code=grant"
				case "credentials":
					grant["url"] = "http://user@" + r.Host + "/bootstrap?code=grant"
				case "path":
					grant["url"] = endpoint + "/api/v1/gateway/start?code=grant"
				case "fragment":
					grant["url"] = endpoint + "/bootstrap?code=grant#other"
				case "missing-code":
					grant["url"] = endpoint + "/bootstrap"
				case "duplicate-code":
					grant["url"] = endpoint + "/bootstrap?code=grant&code=other"
				case "extra-query":
					grant["url"] = endpoint + "/bootstrap?code=grant&next=elsewhere"
				case "expired":
					grant["expires_at"] = time.Now().Add(-time.Second)
				case "schema":
					grant["schema_version"] = 2
				case "oversized":
					grant["padding"] = strings.Repeat("x", 17<<10)
				case "malformed":
					_, _ = w.Write([]byte("not JSON"))
					return
				}
				_ = json.NewEncoder(w).Encode(grant)
			}))
			defer server.Close()
			directory := t.TempDir()
			writeDiscovery(t, directory, server.URL, "private-token")
			client := New(directory)
			t.Cleanup(client.http.CloseIdleConnections)
			if location, err := client.BootstrapURL(context.Background(), "dashboard"); location != "" || err == nil {
				t.Fatalf("invalid grant accepted: %q", location)
			}
		})
	}
}

func TestBootstrapMissingCredentialAndCancellation(t *testing.T) {
	directory := t.TempDir()
	client := New(directory)
	t.Cleanup(client.http.CloseIdleConnections)
	if _, err := client.BootstrapURL(context.Background(), "dashboard"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing descriptor: %v", err)
	}
	for _, token := range []string{"", "embedded\nheader", strings.Repeat("x", (4<<10)+1)} {
		writeDiscovery(t, directory, "http://127.0.0.1:1", token)
		if _, err := client.BootstrapURL(context.Background(), "dashboard"); !errors.Is(err, ErrCredential) {
			t.Fatalf("invalid credential: %v", err)
		}
	}
	writeDiscovery(t, directory, "http://127.0.0.1:1", "private-token")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.BootstrapURL(ctx, "dashboard"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
