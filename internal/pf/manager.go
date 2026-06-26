package pf

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

type Manager struct {
	cfg   config.Config
	paths runtime.Paths
}

func New(cfg config.Config, paths runtime.Paths) Manager {
	return Manager{cfg: cfg, paths: paths}
}

func (m Manager) Check() error {
	if _, err := exec.LookPath("pfctl"); err != nil {
		return fmt.Errorf("pfctl not found in PATH")
	}
	return nil
}

func (m Manager) WriteAnchor() error {
	rendered, err := RenderAnchor(m.cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(m.paths.PFAnchor, []byte(rendered), 0o644)
}

func (m Manager) Enabled() (bool, error) {
	out, err := exec.Command("pfctl", "-s", "info").Output()
	if err != nil {
		return false, err
	}
	return parseEnabled(string(out)), nil
}

func (m Manager) Load(enablePF bool) error {
	if err := runPF("pfctl", "-a", m.cfg.PF.AnchorName, "-f", m.paths.PFAnchor); err != nil {
		return err
	}
	if enablePF {
		if err := runPF("pfctl", "-e"); err != nil {
			_ = m.Unload(false)
			return err
		}
	}
	return nil
}

func (m Manager) Unload(disablePF bool) error {
	err := runPF("pfctl", "-a", m.cfg.PF.AnchorName, "-F", "all")
	if disablePF {
		if disableErr := runPF("pfctl", "-d"); err == nil {
			err = disableErr
		}
	}
	return err
}

func (m Manager) Loaded() (bool, error) {
	anchorName := strings.TrimSpace(m.cfg.PF.AnchorName)
	parent, child, nested := splitAnchor(anchorName)
	args := []string{"-s", "Anchors"}
	if nested {
		args = []string{"-a", parent, "-s", "Anchors"}
	}
	out, err := exec.Command("pfctl", args...).Output()
	if err != nil {
		return false, err
	}
	return anchorOutputContains(string(out), child), nil
}

func splitAnchor(anchorName string) (parent string, child string, nested bool) {
	anchorName = strings.Trim(anchorName, "/")
	if anchorName == "" {
		return "", "", false
	}
	idx := strings.LastIndex(anchorName, "/")
	if idx < 0 {
		return "", anchorName, false
	}
	return anchorName[:idx], anchorName[idx+1:], true
}

func anchorOutputContains(output string, anchorName string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == anchorName {
			return true
		}
	}
	return false
}

func parseEnabled(info string) bool {
	for _, line := range strings.Split(info, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "Status:" {
			return fields[1] == "Enabled"
		}
	}
	return false
}

func runPF(name string, args ...string) error {
	cmd := exec.Command(name, args...)
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
