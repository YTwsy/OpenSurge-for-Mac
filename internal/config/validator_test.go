package config

import (
	"strings"
	"testing"
)

func TestValidateRejectsMihomoRedirPort(t *testing.T) {
	cfg := Default()
	cfg.Mihomo.RedirPort = 7892

	err := Validate(cfg)
	if err == nil {
		t.Fatalf("Validate() succeeded with unsupported mihomo.redir_port")
	}
	if !strings.Contains(err.Error(), `use transparent.mode: "tun"`) {
		t.Fatalf("Validate() error = %q", err)
	}
}

func TestValidateRejectsPFRedirectTCPTo(t *testing.T) {
	cfg := Default()
	cfg.PF.RedirectTCPTo = 7892

	err := Validate(cfg)
	if err == nil {
		t.Fatalf("Validate() succeeded with unsupported pf.redirect_tcp_to")
	}
	if !strings.Contains(err.Error(), `use transparent.mode: "tun"`) {
		t.Fatalf("Validate() error = %q", err)
	}
}

func TestValidateAcceptsTUNTransparentMode(t *testing.T) {
	cfg := Default()
	cfg.Transparent.Mode = TransparentModeTUN

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
