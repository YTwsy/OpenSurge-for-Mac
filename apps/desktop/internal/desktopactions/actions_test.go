package desktopactions

import (
	"net/http/httptest"
	"strings"
	"testing"
)

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
	}{
		{"show-main", `{"page":"devices"}`, 200},
		{"show-main", `{"page":"connections","owner":"device:tv & family"}`, 200},
		{"show-main", `{"page":"https://example.com"}`, 400},
		{"show-main", `{"page":"network","owner":"device:tv"}`, 400},
		{"show-main", `{"page":"connections","owner":"bad\nowner"}`, 400},
		{"tray-appearance", `{"theme":"light"}`, 200},
		{"tray-appearance", `{"theme":"dark"}`, 200},
		{"tray-appearance", `{"theme":"arbitrary"}`, 400},
	} {
		w := httptest.NewRecorder()
		a.ServeHTTP(w, httptest.NewRequest("POST", "/desktop/v1/"+test.action, strings.NewReader(test.body)))
		if w.Code != test.want {
			t.Fatalf("%s: got %d: %s", test.body, w.Code, w.Body.String())
		}
	}
	if target != "connections?owner=device%3Atv+%26+family" || theme != "dark" {
		t.Fatalf("target=%q theme=%q", target, theme)
	}
}
