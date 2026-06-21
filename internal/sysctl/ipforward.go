package sysctl

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
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
	out, err := exec.Command("sysctl", "-n", keyIPForwarding).Output()
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
	cmd := exec.Command("sysctl", "-w", keyIPForwarding+"="+value)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
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
