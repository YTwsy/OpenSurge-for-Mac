package macosnetwork

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/runtime"
)

func TestParseSystemProxySetting(t *testing.T) {
	got, err := parseSystemProxySetting("Enabled: Yes\nServer: proxy.example\nPort: 8080\nAuthenticated Proxy Enabled: 0\n")
	if err != nil {
		t.Fatal(err)
	}
	want := runtime.SystemProxySetting{Enabled: true, Server: "proxy.example", Port: 8080}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseSystemProxySetting() = %#v, want %#v", got, want)
	}
	if _, err := parseSystemProxySetting("Server: proxy.example\n"); err == nil {
		t.Fatal("missing enabled state should fail")
	}
}

func TestSystemProxyPrepareCapturesDisabledService(t *testing.T) {
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(_ context.Context, binary string, args ...string) (string, error) {
		if binary != "/usr/sbin/networksetup" {
			t.Fatalf("binary = %q", binary)
		}
		switch args[0] {
		case "-listnetworkserviceorder":
			return "(1) Wi-Fi\n(Hardware Port: Wi-Fi, Device: en0)\n", nil
		case "-getwebproxy":
			return "Enabled: No\nServer: old.proxy\nPort: 8080\nAuthenticated Proxy Enabled: 0\n", nil
		case "-getsecurewebproxy":
			return "Enabled: No\nServer:\nPort: 0\nAuthenticated Proxy Enabled: 0\n", nil
		case "-getautoproxyurl":
			return "URL: (null)\nEnabled: No\n", nil
		case "-getproxyautodiscovery":
			return "Auto Proxy Discovery: Off\n", nil
		default:
			t.Fatalf("unexpected command %#v", args)
			return "", nil
		}
	}

	snapshot, err := (SystemProxy{}).Prepare(t.Context(), "en0", 7890)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.NetworkService != "Wi-Fi" || snapshot.Interface != "en0" || snapshot.HTTP.Server != "old.proxy" || snapshot.HTTP.Port != 8080 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestSystemProxyPrepareRejectsConflictingProxyState(t *testing.T) {
	tests := []struct {
		name      string
		web       string
		secure    string
		autoURL   string
		discovery string
		want      string
	}{
		{
			name: "active HTTP proxy",
			web:  "Enabled: Yes\nServer: proxy.example\nPort: 8080\nAuthenticated Proxy Enabled: 0\n",
			want: "already has an active",
		},
		{
			name: "authenticated proxy",
			web:  "Enabled: No\nServer: proxy.example\nPort: 8080\nAuthenticated Proxy Enabled: 1\n",
			want: "authenticated",
		},
		{
			name:    "automatic proxy configuration",
			autoURL: "URL: https://proxy.example/config.pac\nEnabled: Yes\n",
			want:    "auto-configuration",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := runCommand
			t.Cleanup(func() { runCommand = original })
			web := tt.web
			if web == "" {
				web = disabledProxyOutput()
			}
			secure := tt.secure
			if secure == "" {
				secure = disabledProxyOutput()
			}
			autoURL := tt.autoURL
			if autoURL == "" {
				autoURL = "URL: (null)\nEnabled: No\n"
			}
			discovery := tt.discovery
			if discovery == "" {
				discovery = "Auto Proxy Discovery: Off\n"
			}
			runCommand = func(_ context.Context, _ string, args ...string) (string, error) {
				switch args[0] {
				case "-listnetworkserviceorder":
					return "(1) Wi-Fi\n(Hardware Port: Wi-Fi, Device: en0)\n", nil
				case "-getwebproxy":
					return web, nil
				case "-getsecurewebproxy":
					return secure, nil
				case "-getautoproxyurl":
					return autoURL, nil
				case "-getproxyautodiscovery":
					return discovery, nil
				default:
					return "", fmt.Errorf("unexpected command %q", args[0])
				}
			}
			_, err := (SystemProxy{}).Prepare(t.Context(), "en0", 7890)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Prepare() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestSystemProxyEnableAndRestore(t *testing.T) {
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	currentHTTP := runtime.SystemProxySetting{}
	currentHTTPS := runtime.SystemProxySetting{}
	writes := []string{}
	runCommand = func(_ context.Context, _ string, args ...string) (string, error) {
		writes = append(writes, strings.Join(args, " "))
		switch args[0] {
		case "-setwebproxy":
			currentHTTP = runtime.SystemProxySetting{Enabled: true, Server: args[2], Port: mustPort(t, args[3])}
		case "-setsecurewebproxy":
			currentHTTPS = runtime.SystemProxySetting{Enabled: true, Server: args[2], Port: mustPort(t, args[3])}
		case "-setwebproxystate":
			currentHTTP.Enabled = args[2] == "on"
		case "-setsecurewebproxystate":
			currentHTTPS.Enabled = args[2] == "on"
		case "-getwebproxy":
			return proxyOutput(currentHTTP), nil
		case "-getsecurewebproxy":
			return proxyOutput(currentHTTPS), nil
		default:
			return "", fmt.Errorf("unexpected command %q", args[0])
		}
		return "", nil
	}
	snapshot := runtime.SystemProxySnapshot{
		NetworkService: "Wi-Fi",
		Interface:      "en0",
		HTTP:           runtime.SystemProxySetting{Server: "old.proxy", Port: 8080},
		HTTPS:          runtime.SystemProxySetting{},
	}
	manager := SystemProxy{}
	if err := manager.Enable(t.Context(), snapshot, 7890); err != nil {
		t.Fatal(err)
	}
	if !currentHTTP.Enabled || !currentHTTPS.Enabled || currentHTTP.Server != localSystemProxyHost || currentHTTPS.Port != 7890 {
		t.Fatalf("enabled HTTP=%#v HTTPS=%#v", currentHTTP, currentHTTPS)
	}
	if err := manager.Restore(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	if currentHTTP.Enabled || currentHTTPS.Enabled || currentHTTP.Server != "old.proxy" || currentHTTP.Port != 8080 {
		t.Fatalf("restored HTTP=%#v HTTPS=%#v", currentHTTP, currentHTTPS)
	}
	for _, write := range writes {
		if strings.Contains(write, "socks") || strings.Contains(write, "autoproxy") || strings.Contains(write, "bypass") {
			t.Fatalf("unexpected proxy-scope write %q", write)
		}
	}
}

func TestSystemProxyEnableRestoresHTTPWhenHTTPSWriteFails(t *testing.T) {
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	httpEnabled := false
	runCommand = func(_ context.Context, _ string, args ...string) (string, error) {
		switch args[0] {
		case "-setwebproxy":
			httpEnabled = true
			return "", nil
		case "-setsecurewebproxy":
			return "", errors.New("secure proxy write failed")
		case "-setwebproxystate":
			httpEnabled = args[2] == "on"
			return "", nil
		case "-setsecurewebproxystate":
			return "", nil
		default:
			return "", fmt.Errorf("unexpected command %q", args[0])
		}
	}
	err := (SystemProxy{}).Enable(t.Context(), runtime.SystemProxySnapshot{NetworkService: "Wi-Fi"}, 7890)
	if err == nil || !strings.Contains(err.Error(), "secure proxy write failed") {
		t.Fatalf("Enable() error = %v", err)
	}
	if httpEnabled {
		t.Fatal("HTTP proxy remained enabled after HTTPS write failure")
	}
}

func disabledProxyOutput() string {
	return "Enabled: No\nServer:\nPort: 0\nAuthenticated Proxy Enabled: 0\n"
}

func proxyOutput(setting runtime.SystemProxySetting) string {
	enabled := "No"
	if setting.Enabled {
		enabled = "Yes"
	}
	authenticated := 0
	if setting.Authenticated {
		authenticated = 1
	}
	return fmt.Sprintf("Enabled: %s\nServer: %s\nPort: %d\nAuthenticated Proxy Enabled: %d\n", enabled, setting.Server, setting.Port, authenticated)
}

func mustPort(t *testing.T, value string) int {
	t.Helper()
	var port int
	if _, err := fmt.Sscanf(value, "%d", &port); err != nil {
		t.Fatal(err)
	}
	return port
}
