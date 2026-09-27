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
