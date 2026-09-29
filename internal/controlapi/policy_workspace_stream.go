package controlapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"open-mihomo-gateway/internal/mihomo"
)

type policyWorkspaceResultKey struct{}

// Results are observational and keep the existing workspace lease, lifecycle
// lock, engine selection and concurrency. Serialise callbacks from the workers.
func withPolicyWorkspaceResults(ctx context.Context, report func(mihomo.ProxyDelayResult)) context.Context {
	var mu sync.Mutex
	return context.WithValue(ctx, policyWorkspaceResultKey{}, func(result mihomo.ProxyDelayResult) {
		mu.Lock()
		defer mu.Unlock()
		report(result)
	})
}

func policyWorkspaceResultReporter(ctx context.Context) func(mihomo.ProxyDelayResult) {
	report, _ := ctx.Value(policyWorkspaceResultKey{}).(func(mihomo.ProxyDelayResult))
	return report
}

func measurePolicyWorkspaceNodes(ctx context.Context, names []string, measure func(context.Context, string) mihomo.ProxyDelayResult) []mihomo.ProxyDelayResult {
	results := make([]mihomo.ProxyDelayResult, len(names))
	jobs := make(chan int)
	report := policyWorkspaceResultReporter(ctx)
	var workers sync.WaitGroup
	for range min(proxyHealthConcurrency, len(names)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				result := measure(ctx, names[index])
				results[index] = result
				if report != nil && ctx.Err() == nil {
					report(result)
				}
			}
		}()
	}
enqueue:
	for index := range names {
		select {
		case jobs <- index:
		case <-ctx.Done():
			break enqueue
		}
	}
	close(jobs)
	workers.Wait()
	return results
}

func withHelperPolicyResults(ctx context.Context, conn net.Conn, cancel context.CancelFunc) context.Context {
	go func() {
		_, _ = io.Copy(io.Discard, conn)
		cancel()
	}()
	deadline := time.Now().Add(helperConnectionTimeout)
	return withPolicyWorkspaceResults(ctx, func(result mihomo.ProxyDelayResult) {
		if ctx.Err() != nil {
			return
		}
		_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
		if err := json.NewEncoder(conn).Encode(HelperResponse{PolicyResult: &result}); err != nil {
			cancel()
		}
		_ = conn.SetWriteDeadline(deadline)
	})
}

type policyWorkspaceEvent struct {
	Type      string                   `json:"type"`
	Result    *mihomo.ProxyDelayResult `json:"result,omitempty"`
	Workspace *PolicyWorkspaceResponse `json:"workspace,omitempty"`
	Error     string                   `json:"error,omitempty"`
}

func (s *Server) streamPolicyWorkspace(w http.ResponseWriter, r *http.Request, input PolicyWorkspaceInput) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	controller := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	if err := controller.Flush(); err != nil {
		return
	}
	write := func(event policyWorkspaceEvent) {
		if ctx.Err() != nil {
			return
		}
		data, err := json.Marshal(event)
		if err != nil {
			cancel()
			return
		}
		_ = controller.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, err = fmt.Fprintf(w, "data: %s\n\n", data)
		if err != nil || controller.Flush() != nil {
			cancel()
		}
		_ = controller.SetWriteDeadline(time.Time{})
	}
	ctx = withPolicyWorkspaceResults(ctx, func(result mihomo.ProxyDelayResult) {
		write(policyWorkspaceEvent{Type: "result", Result: &result})
	})
	response, err := s.policyWorkspaceRunner.PolicyWorkspace(ctx, s.configPath, input)
	if err != nil {
		_ = s.policyWorkspaceLease.reset()
		write(policyWorkspaceEvent{Type: "error", Error: err.Error()})
		return
	}
	write(policyWorkspaceEvent{Type: "complete", Workspace: &response})
}
