package controlclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type serviceFixture struct {
	server       *httptest.Server
	generation   atomic.Int32
	grants       atomic.Int32
	reads        atomic.Int32
	writes       atomic.Int32
	rejectWrites atomic.Bool
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	f := &serviceFixture{}
	f.generation.Store(1)
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		if r.URL.Path == "/api/v1/session/bootstrap" {
			if r.Header.Get("Authorization") != "Bearer native-token" {
				t.Error("missing native bootstrap credential")
			}
			f.grants.Add(1)
			writeGrant(w, base)
			return
		}
		if r.URL.Path == "/bootstrap" {
			http.SetCookie(w, &http.Cookie{Name: "opensurge_session", Value: fmt.Sprint(f.generation.Load()), HttpOnly: true, Path: "/", Expires: time.Now().Add(time.Hour)})
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}
		if r.URL.Path == "/dashboard" {
			t.Error("native client followed bootstrap redirect")
		}
		cookie, err := r.Cookie("opensurge_session")
		if r.Method == http.MethodGet {
			f.reads.Add(1)
		} else {
			f.writes.Add(1)
		}
		if err != nil || cookie.Value != fmt.Sprint(f.generation.Load()) || (r.Method != http.MethodGet && f.rejectWrites.Load()) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-OpenSurge-Desktop") != "" || r.URL.Query().Has("desktop_session") {
			t.Error("host capability or bearer was forwarded to an API route")
		}
		if r.Header.Get("Origin") != base {
			t.Error("incorrect API origin")
		}
		if r.URL.Path == "/api/v1/events" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: state\ndata: {}\n\n")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		http.SetCookie(w, &http.Cookie{Name: "opensurge_session", Value: "do-not-expose"})
		body, _ := io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]string{"body": string(body), "revision": r.Header.Get("If-Match"), "operation": r.Header.Get("Idempotency-Key")})
	}))
	t.Cleanup(f.server.Close)
	return f
}

func proxyRequest(c *Client, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer frontend-must-not-select-credentials")
	r.Header.Set("Cookie", "frontend=not-trusted")
	r.Header.Set("X-OpenSurge-Desktop", "host-capability")
	r.Header.Set("If-Match", `"revision"`)
	r.Header.Set("Idempotency-Key", "operation-id")
	w := httptest.NewRecorder()
	c.ServeHTTP(w, r)
	return w
}

func TestProxyRenewsReadSessionWithoutExposingCredentials(t *testing.T) {
	f := newServiceFixture(t)
	directory := t.TempDir()
	writeDiscovery(t, directory, f.server.URL, "native-token")
	c := New(directory)
	first := proxyRequest(c, http.MethodGet, "/api/v1/overview", "")
	if first.Code != 200 || first.Header().Get("Set-Cookie") != "" {
		t.Fatal("session not isolated from frontend")
	}
	f.generation.Add(1)
	second := proxyRequest(c, http.MethodGet, "/api/v1/overview", "")
	if second.Code != 200 || f.grants.Load() != 2 || f.reads.Load() != 3 {
		t.Fatal("read did not recover from expired session")
	}
	if strings.Contains(second.Body.String(), "native-token") || strings.Contains(second.Body.String(), "do-not-expose") {
		t.Fatal("credential leaked")
	}
	// Changing the endpoint does not require changing a renderer URL or state.
	replacement := newServiceFixture(t)
	writeDiscovery(t, directory, replacement.server.URL, "native-token")
	if proxyRequest(c, http.MethodGet, "/api/v1/overview", "").Code != 200 || replacement.reads.Load() != 1 {
		t.Fatal("endpoint change not followed")
	}
}

func TestProxyNeverReplaysMutation(t *testing.T) {
	f := newServiceFixture(t)
	directory := t.TempDir()
	writeDiscovery(t, directory, f.server.URL, "native-token")
	c := New(directory)
	f.rejectWrites.Store(true)
	failed := proxyRequest(c, http.MethodPost, "/api/v1/gateway/start", `{"intent":"once"}`)
	if failed.Code != http.StatusServiceUnavailable || f.writes.Load() != 1 {
		t.Fatal("rejected mutation was replayed")
	}
	f.rejectWrites.Store(false)
	response := proxyRequest(c, http.MethodPut, "/api/v1/config", `{"edited":true}`)
	if response.Code != 200 || f.writes.Load() != 2 {
		t.Fatal("explicit next mutation failed")
	}
	var values map[string]string
	_ = json.Unmarshal(response.Body.Bytes(), &values)
	if values["body"] != `{"edited":true}` || values["revision"] != `"revision"` || values["operation"] != "operation-id" {
		t.Fatal("mutation body or concurrency/operation headers were lost")
	}
}

func TestProxyStreamsEventsAndCancelsUpstream(t *testing.T) {
	f := newServiceFixture(t)
	directory := t.TempDir()
	writeDiscovery(t, directory, f.server.URL, "native-token")
	proxy := httptest.NewServer(New(directory))
	defer proxy.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, proxy.URL+"/api/v1/events?desktop_session=local-only", nil)
	response, err := proxy.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	if err != nil || line != "event: state\n" {
		t.Fatal("SSE buffered until response completion")
	}
	cancel()
}

func TestProxyMissingServicePreservesFrontendSession(t *testing.T) {
	c := New(t.TempDir())
	response := proxyRequest(c, http.MethodGet, "/api/v1/overview", "")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "desktop_service_unavailable") {
		t.Fatal("unavailable service must not trigger browser session-expired unmount")
	}
	if proxyRequest(c, http.MethodPost, "/api/v1/session/bootstrap", "").Code != 404 {
		t.Fatal("frontend can create a native bootstrap")
	}
}
