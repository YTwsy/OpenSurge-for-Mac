package desktopserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedUIAndPrivateAPITransport(t *testing.T) {
	calls := 0
	server, err := New(fstest.MapFS{"index.html": {Data: []byte("<head></head><body>shared React</body>")}}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }), nil)
	if err != nil {
		t.Fatal(err)
	}
	page := httptest.NewRecorder()
	server.ServeHTTP(page, httptest.NewRequest("GET", "wails://localhost/devices", nil))
	if !strings.Contains(page.Body.String(), server.secret) || !strings.Contains(page.Body.String(), "shared React") || page.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("SPA route missing per-process capability")
	}
	for _, scenario := range []struct {
		origin, token string
		status        int
	}{
		{"", "", 403}, {"wails://localhost", "wrong", 403}, {"https://external.example", server.secret, 403},
		{"null", server.secret, 204}, {"wails://localhost", server.secret, 204},
	} {
		r := httptest.NewRequest("POST", "wails://localhost/api/v1/config", nil)
		r.Header.Set("Origin", scenario.origin)
		r.Header.Set("X-OpenSurge-Desktop", scenario.token)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if w.Code != scenario.status {
			t.Fatalf("origin %q: %d", scenario.origin, w.Code)
		}
	}
	if calls != 2 {
		t.Fatal("untrusted renderer reached Control Service")
	}
}

func TestEventCapabilityCannotAuthorizeOtherRoutes(t *testing.T) {
	server, _ := New(fstest.MapFS{"index.html": {Data: []byte("<head></head>")}}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), nil)
	for _, endpoint := range []string{"/api/v1/events", "/api/v1/overview"} {
		request := httptest.NewRequest("GET", "wails://localhost"+endpoint+"?desktop_session="+server.secret, nil)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		want := 403
		if endpoint == "/api/v1/events" {
			want = 204
		}
		if response.Code != want {
			t.Fatalf("%s: %d", endpoint, response.Code)
		}
	}
}

func TestNativeActionsRequireRendererCapability(t *testing.T) {
	calls := 0
	action := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) })
	server, _ := New(fstest.MapFS{"index.html": {Data: []byte("<head></head>")}}, nil, action)
	for _, scenario := range []struct {
		proof, origin string
		status        int
	}{{"", "", 403}, {server.secret, "https://external.example", 403}, {server.secret, "wails://localhost", 204}} {
		r := httptest.NewRequest("POST", "wails://localhost/desktop/v1/copy-text?desktop_session="+server.secret, strings.NewReader(`{"text":"hello"}`))
		r.Header.Set("X-OpenSurge-Desktop", scenario.proof)
		r.Header.Set("Origin", scenario.origin)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if w.Code != scenario.status {
			t.Fatalf("native action status = %d, want %d", w.Code, scenario.status)
		}
	}
	if calls != 1 {
		t.Fatalf("untrusted native action reached host: %d calls", calls)
	}
}
