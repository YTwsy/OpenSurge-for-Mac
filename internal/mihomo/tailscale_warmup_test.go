package mihomo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
)

func TestTailscaleWarmupReturnsAfterDispatchWithoutCancelingInitialization(t *testing.T) {
	cfg := config.Default()
	cfg.Tailscale.Enabled = true
	cfg.Tailscale.ExitNode = "100.90.3.4"
	cfg.Mihomo.Secret = "test-secret"
	requestSent := make(chan *http.Request, 1)
	release := make(chan struct{})
	var once sync.Once
	complete := func() { once.Do(func() { close(release) }) }
	defer complete()
	bodyClosed := make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestSent <- req
		httptrace.ContextClientTrace(req.Context()).WroteRequest(httptrace.WroteRequestInfo{})
		select {
		case <-release:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: &warmupTestBody{Reader: strings.NewReader(`{"delay":100}`), closed: bodyClosed}}, nil
	})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- initiateTailscaleWarmupWithClient(ctx, cfg, client) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("gateway warm-up waited for the exit health response")
	}
	req := <-requestSent
	if req.URL.Query().Get("timeout") != "15000" || req.URL.Query().Get("url") != DefaultTailscaleExitNodeTestURL || req.Header.Get("Authorization") != "Bearer test-secret" {
		t.Fatalf("warm-up lost the existing exit probe contract: %s", req.URL)
	}
	cancel() // lifecycle/request completion must not cancel lazy tsnet startup.
	if err := req.Context().Err(); err != nil {
		t.Fatalf("warm-up context ended with the lifecycle: %v", err)
	}
	complete()
	select {
	case <-bodyClosed:
	case <-time.After(time.Second):
		t.Fatal("background warm-up response body was not closed")
	}
}

type warmupTestBody struct {
	io.Reader
	closed chan struct{}
}

func (b *warmupTestBody) Close() error { close(b.closed); return nil }

func TestTailscaleWarmupCancelsUndispatchedRequest(t *testing.T) {
	cfg := config.Default()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	probeCanceled := make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		cancel()
		<-req.Context().Done()
		close(probeCanceled)
		return nil, req.Context().Err()
	})}
	if err := initiateTailscaleWarmupWithClient(ctx, cfg, client); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled dispatch = %v", err)
	}
	select {
	case <-probeCanceled:
	case <-time.After(time.Second):
		t.Fatal("undispatched request leaked")
	}
}

func TestTailscaleWarmupDispatchFailureRemainsBestEffort(t *testing.T) {
	want := errors.New("controller refused")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, want })}
	if err := initiateTailscaleWarmupWithClient(context.Background(), config.Default(), client); !errors.Is(err, want) {
		t.Fatalf("dispatch failure = %v", err)
	}
}
