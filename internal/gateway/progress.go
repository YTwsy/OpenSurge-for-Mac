package gateway

import "context"

// Progress reports observed lifecycle boundaries, never estimated percentages.
// Notices describe work that was initiated, not a guarantee of egress health.
type Progress struct {
	Phase  string `json:"phase,omitempty"`
	Notice string `json:"notice,omitempty"`
}

type progressContextKey struct{}

func WithProgress(ctx context.Context, report func(Progress)) context.Context {
	return context.WithValue(ctx, progressContextKey{}, report)
}

func ProgressReporter(ctx context.Context) func(Progress) {
	report, _ := ctx.Value(progressContextKey{}).(func(Progress))
	return report
}

func ReportProgress(ctx context.Context, phase string) {
	if report := ProgressReporter(ctx); report != nil {
		report(Progress{Phase: phase})
	}
}

func reportNotice(ctx context.Context, notice string) {
	if report := ProgressReporter(ctx); report != nil {
		report(Progress{Notice: notice})
	}
}
