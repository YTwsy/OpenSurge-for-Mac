package mihomo

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
)

func TestFetchVersion(t *testing.T) {
	cfg := config.Default()
	cfg.Mihomo.APIAddr = "127.0.0.1:9090"
	cfg.Mihomo.Secret = "test-secret"

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "http://127.0.0.1:9090/version" {
			t.Fatalf("URL = %q", req.URL.String())
		}
		if got := req.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"version":"v1.2.3","meta":true}`)),
			Header:     make(http.Header),
		}, nil
	})}

	version, err := fetchVersionWithClient(context.Background(), cfg, client)
	if err != nil {
		t.Fatalf("fetchVersionWithClient() error = %v", err)
	}
	if version.Version != "v1.2.3" {
		t.Fatalf("Version = %q", version.Version)
	}
	if !version.Meta {
		t.Fatalf("Meta = false")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
