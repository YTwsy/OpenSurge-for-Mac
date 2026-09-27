package native

import "testing"

func TestExternalURLBoundary(t *testing.T) {
	for _, url := range []string{"https://github.com/YTwsy/OpenSurge-for-Mac", "http://192.0.2.1/"} {
		if !ExternalURLAllowed(url) {
			t.Fatalf("valid link rejected: %s", url)
		}
	}
	for _, url := range []string{"file:///etc/passwd", "javascript:alert(1)", "wails://localhost/api/v1/overview", "https://user:secret@example.com", "mailto:me@example.com", "https:", ""} {
		if ExternalURLAllowed(url) {
			t.Fatalf("unsafe link accepted: %s", url)
		}
	}
}
