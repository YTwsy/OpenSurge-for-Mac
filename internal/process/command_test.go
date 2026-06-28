package process

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRunTimeout(t *testing.T) {
	err := RunTimeout(10*time.Millisecond, "sh", "-c", "sleep 1")
	if err == nil {
		t.Fatalf("RunTimeout() succeeded")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("RunTimeout() error = %q", err)
	}
}

func TestRunBufferedCapturesOutput(t *testing.T) {
	var output bytes.Buffer
	err := RunBufferedTimeout(time.Second, &output, "sh", "-c", "printf problem >&2; exit 2")
	if err == nil {
		t.Fatalf("RunBufferedTimeout() succeeded")
	}
	if got := output.String(); got != "problem" {
		t.Fatalf("buffered output = %q, want problem", got)
	}
}
