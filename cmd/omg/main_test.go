package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderMihomoCommandPrintsOverlayConfig(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	profile := `allow-lan: false
dns:
  enable: false
proxies:
  - name: Imported
    type: http
    server: 203.0.113.10
    port: 8080
proxy-groups:
  - name: Proxy
    type: select
    proxies:
      - Imported
rules:
  - DOMAIN-SUFFIX,example.com,Proxy
  - MATCH,DIRECT
`
	if err := os.WriteFile(profilePath, []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
mihomo:
  profile_mode: "imported"
  profile: "` + profilePath + `"
  mixed_port: 17890
  api_addr: "127.0.0.1:19090"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"render-mihomo", "--config", configPath})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}

	for _, want := range []string{
		"mixed-port: 17890",
		"allow-lan: true",
		"external-controller: 127.0.0.1:19090",
		"proxies:",
		"- DOMAIN-SUFFIX,example.com,Proxy",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("render-mihomo output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "allow-lan: false") || strings.Contains(output, "enable: false") {
		t.Fatalf("render-mihomo output kept gateway-owned profile fields:\n%s", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = old
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
