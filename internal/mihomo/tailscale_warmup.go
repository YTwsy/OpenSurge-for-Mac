package mihomo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"time"

	"open-mihomo-gateway/internal/config"
)

const tailscaleWarmupDispatchTimeout = 2 * time.Second

// InitiateTailscaleWarmup waits only for the existing delay request to be sent,
// not for the remote egress to answer. The pinned mihomo delay handler owns its
// own context, so its full 4s/15s probe and lazy tsnet initialization continue
// even when a short-lived CLI caller exits after dispatch. A Helper keeps
// draining the response in the background. This adds no retries or probes.
func InitiateTailscaleWarmup(ctx context.Context, cfg config.Config) error {
	client := &http.Client{Timeout: tailscaleWarmupTimeout(cfg) + time.Second}
	return initiateTailscaleWarmupWithClient(ctx, cfg, client)
}

func initiateTailscaleWarmupWithClient(ctx context.Context, cfg config.Config, client *http.Client) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	probeCtx, cancel := context.WithTimeout(context.Background(), tailscaleWarmupTimeout(cfg)+2*time.Second)
	sent := make(chan error, 1)
	finished := make(chan error, 1)
	trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
		select {
		case sent <- info.Err:
		default:
		}
	}}
	probeCtx = httptrace.WithClientTrace(probeCtx, trace)
	query := url.Values{
		"url":     {tailscaleWarmupURL(cfg)},
		"timeout": {fmt.Sprintf("%d", tailscaleWarmupTimeout(cfg).Milliseconds())},
	}
	path := "/proxies/" + url.PathEscape(config.TailscaleProxyName) + "/delay?" + query.Encode()
	req, err := newAPIRequest(probeCtx, cfg, http.MethodGet, path, nil)
	if err != nil {
		cancel()
		return err
	}
	go func() {
		defer cancel()
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			_ = resp.Body.Close()
		}
		finished <- err
	}()
	timer := time.NewTimer(tailscaleWarmupDispatchTimeout)
	defer timer.Stop()
	select {
	case err := <-sent:
		if err != nil {
			cancel()
		}
		return err
	case err := <-finished:
		return err
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	case <-timer.C:
		cancel()
		return fmt.Errorf("Tailscale warm-up request could not be dispatched in %s", tailscaleWarmupDispatchTimeout)
	}
}
