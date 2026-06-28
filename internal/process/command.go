package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const DefaultCommandTimeout = 5 * time.Second

func Output(name string, args ...string) ([]byte, error) {
	return OutputTimeout(DefaultCommandTimeout, name, args...)
}

func OutputTimeout(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return out, fmt.Errorf("%s timed out after %s", formatCommand(name, args), timeout)
	}
	return out, err
}

func Run(name string, args ...string) error {
	return RunTimeout(DefaultCommandTimeout, name, args...)
}

func RunBuffered(output *bytes.Buffer, name string, args ...string) error {
	return RunBufferedTimeout(DefaultCommandTimeout, output, name, args...)
}

func RunTimeout(timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%s timed out after %s", formatCommand(name, args), timeout)
	}
	if err != nil && stderr.Len() > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return err
}

func RunBufferedTimeout(timeout time.Duration, output *bytes.Buffer, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%s timed out after %s", formatCommand(name, args), timeout)
	}
	return err
}

func formatCommand(name string, args []string) string {
	if len(args) == 0 {
		return name
	}
	return name + " " + strings.Join(args, " ")
}
