package sysctl

import (
	"fmt"
	"os/exec"
	"strings"

	"open-mihomo-gateway/internal/process"
)

const keyIPForwarding = "net.inet.ip.forwarding"

type Manager struct{}

func New() Manager {
	return Manager{}
}

func (m Manager) Check() error {
	if _, err := exec.LookPath("sysctl"); err != nil {
		return fmt.Errorf("sysctl not found in PATH")
	}
	return nil
}

func (m Manager) Current() (string, error) {
	out, err := process.Output("sysctl", "-n", keyIPForwarding)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (m Manager) Enable() error {
	return setIPForwarding("1")
}

func (m Manager) Restore(previous string) error {
	previous = strings.TrimSpace(previous)
	if previous == "" {
		return nil
	}
	return setIPForwarding(previous)
}

func setIPForwarding(value string) error {
	return process.Run("sysctl", "-w", keyIPForwarding+"="+value)
}

func FormatForwarding(value string) string {
	switch strings.TrimSpace(value) {
	case "1":
		return "enabled"
	case "0":
		return "disabled"
	default:
		return "unknown"
	}
}
