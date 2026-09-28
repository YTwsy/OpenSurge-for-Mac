package desktopactions

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserActionIsArgumentFreeAndDoesNotExposeGrant(t *testing.T) {
	calls := 0
	a := &Actions{OpenBrowser: func(context.Context) error { calls++; return nil }}
	for _, body := range []string{`{}`, `{"url":"https://example.com"}`, `{"page":"network"}`, `{"enabled":true}`} {
		w := httptest.NewRecorder()
		a.ServeHTTP(w, httptest.NewRequest("POST", "/desktop/v1/open-browser", strings.NewReader(body)))
		want := 400
		if body == `{}` {
			want = 200
			if strings.TrimSpace(w.Body.String()) != `{"ok":true}` {
				t.Fatal("browser response must contain only acknowledgement")
			}
		}
		if w.Code != want {
			t.Fatalf("%s: got %d", body, w.Code)
		}
	}
	if calls != 1 {
		t.Fatalf("unexpected browser actions: %d", calls)
	}
	a.OpenBrowser = func(context.Context) error { return errors.New("opener failed: /bootstrap?code=private-grant") }
	w := httptest.NewRecorder()
	a.ServeHTTP(w, httptest.NewRequest("POST", "/desktop/v1/open-browser", strings.NewReader(`{}`)))
	if w.Code != 503 || strings.Contains(w.Body.String(), "private-grant") {
		t.Fatal("browser failure must be reported without exposing its grant")
	}
}

func TestClosedNativeCapabilities(t *testing.T) {
	opened := ""
	a := &Actions{OpenExternal: func(value string) error { opened = value; return nil }}
	for _, test := range []struct {
		path, body string
		want       int
	}{
		{"open-external", `{"url":"https://github.com/YTwsy/OpenSurge-for-Mac"}`, 200},
		{"open-external", `{"url":"file:///tmp/private"}`, 400},
		{"open-external", `{"url":"https://token@example.com"}`, 400},
		{"open-external", `{"url":"https://example.com\u0000"}`, 400},
		{"open-external", `{"url":"https://example.com","command":"run"}`, 400},
		{"language", `{"language":"invalid"}`, 400},
		{"exec", `{"text":"echo no"}`, 404},
	} {
		w := httptest.NewRecorder()
		a.ServeHTTP(w, httptest.NewRequest("POST", "/desktop/v1/"+test.path, strings.NewReader(test.body)))
		if w.Code != test.want {
			t.Fatalf("%s: got %d", test.body, w.Code)
		}
	}
	if opened != "https://github.com/YTwsy/OpenSurge-for-Mac" {
		t.Fatal("an untrusted URL reached the native opener")
	}
}

func TestTrayNavigationAndAppearance(t *testing.T) {
	target, theme := "", ""
	a := &Actions{ShowMain: func(value string) { target = value }, SetTrayAppearance: func(value string) { theme = value }}
	for _, test := range []struct {
		action, body string
		want         int
		wantTarget   string
	}{
		{"show-main", `{"page":"devices"}`, 200, "devices"},
		{"show-main", `{"page":"dashboard","section":"active-devices"}`, 200, "dashboard#active-devices"},
		{"show-main", `{"page":"connections","owner":"device:tv & family"}`, 200, "connections?owner=device%3Atv+%26+family"},
		{"show-main", `{"page":"https://example.com"}`, 400, ""},
		{"show-main", `{"page":"network","owner":"device:tv"}`, 400, ""},
		{"show-main", `{"page":"connections","owner":"bad\nowner"}`, 400, ""},
		{"show-main", `{"page":"network","section":"active-devices"}`, 400, ""},
		{"show-main", `{"page":"dashboard","section":"active-devices?owner=all"}`, 400, ""},
		{"tray-appearance", `{"theme":"light"}`, 200, ""},
		{"tray-appearance", `{"theme":"dark"}`, 200, ""},
		{"tray-appearance", `{"theme":"arbitrary"}`, 400, ""},
	} {
		target = ""
		w := httptest.NewRecorder()
		a.ServeHTTP(w, httptest.NewRequest("POST", "/desktop/v1/"+test.action, strings.NewReader(test.body)))
		if w.Code != test.want {
			t.Fatalf("%s: got %d: %s", test.body, w.Code, w.Body.String())
		}
		if target != test.wantTarget {
			t.Fatalf("%s: target=%q, want %q", test.body, target, test.wantTarget)
		}
	}
	if theme != "dark" {
		t.Fatalf("target=%q theme=%q", target, theme)
	}
}
