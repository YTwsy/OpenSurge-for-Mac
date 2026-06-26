package runtime

import (
	"os"
	"path/filepath"

	"open-mihomo-gateway/internal/config"
)

type Paths struct {
	Dir            string
	LogDir         string
	StateFile      string
	DNSMasqConf    string
	DNSMasqPIDFile string
	MihomoConfig   string
	MihomoLog      string
	PFAnchor       string
	LeaseFile      string
}

func NewPaths(cfg config.Config) Paths {
	dir := cfg.Runtime.Dir
	return Paths{
		Dir:            dir,
		LogDir:         filepath.Join(dir, "logs"),
		StateFile:      filepath.Join(dir, "state.json"),
		DNSMasqConf:    filepath.Join(dir, "dnsmasq.conf"),
		DNSMasqPIDFile: filepath.Join(dir, "dnsmasq.pid"),
		MihomoConfig:   cfg.Mihomo.Config,
		MihomoLog:      filepath.Join(dir, "logs", "mihomo.log"),
		PFAnchor:       filepath.Join(dir, "pf.anchor"),
		LeaseFile:      filepath.Join(dir, "dnsmasq.leases"),
	}
}

func Ensure(paths Paths) error {
	if err := os.MkdirAll(paths.Dir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(paths.LogDir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Dir(paths.MihomoConfig), 0o755)
}
