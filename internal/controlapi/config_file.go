package controlapi

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/runtime"
)

const (
	maxConfigFileSize       = 256 << 10
	redactedConfigFileValue = "<redacted>"
)

// ConfigFile is the authenticated, redacted view of the one configuration file
// owned by OpenSurge. The path is informational; callers never provide a file
// path to the API.
type ConfigFile struct {
	SchemaVersion   int      `json:"schema_version"`
	Path            string   `json:"path"`
	Revision        string   `json:"revision"`
	Content         string   `json:"content"`
	ProtectedFields []string `json:"protected_fields"`
}

type ConfigFileSaveRequest struct {
	Content string `json:"content"`
}

func (s *Server) handleConfigFile(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		response, err := s.configFileResponse()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "config_file_read_failed", err.Error())
			return
		}
		w.Header().Set("ETag", `"`+response.Revision+`"`)
		writeJSON(w, http.StatusOK, response)
		return
	}

	recovery, _ := s.store.Recovery()
	if recovery.Required && recovery.Stage != RecoveryPrepared {
		writeError(w, http.StatusConflict, "recovery_required", "finish network recovery before editing the configuration file")
		return
	}
	currentRevision := fileDigest(s.configPath)
	match := strings.Trim(r.Header.Get("If-Match"), `"`)
	if match == "" || match != currentRevision {
		writeError(w, http.StatusConflict, "revision_conflict", "If-Match must contain the current configuration file revision")
		return
	}
	var request ConfigFileSaveRequest
	if err := decodeJSON(r, &request, maxConfigFileSize+8<<10); err != nil {
		writeError(w, http.StatusBadRequest, "config_file_invalid", err.Error())
		return
	}
	if len(request.Content) == 0 || len(request.Content) > maxConfigFileSize {
		writeError(w, http.StatusRequestEntityTooLarge, "config_file_too_large", "configuration file must be between 1 byte and 256 KiB")
		return
	}
	newRevision, err := s.configRunner.ApplyConfigFile(r.Context(), s.configPath, match, []byte(request.Content))
	if err != nil {
		status, code := http.StatusUnprocessableEntity, "config_file_validation_failed"
		if strings.Contains(err.Error(), "revision conflict") {
			status, code = http.StatusConflict, "revision_conflict"
		} else if strings.Contains(err.Error(), "must be stopped") {
			status, code = http.StatusConflict, "gateway_running"
		} else if strings.Contains(err.Error(), "cannot be edited") || strings.Contains(err.Error(), "is protected") {
			code = "config_file_protected"
		}
		writeError(w, status, code, err.Error())
		return
	}

	cfg, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_file_reload_failed", err.Error())
		return
	}
	if recovery.Stage == RecoveryPrepared {
		if err := s.store.DiscardPreparedRecovery(cfg.Gateway.Mode); err != nil {
			writeError(w, http.StatusInternalServerError, "recovery_discard_failed", "configuration was saved but the prepared recovery card could not be discarded: "+err.Error())
			return
		}
	} else if err := s.store.SaveRecovery(RecoveryState{SchemaVersion: SchemaVersion, Stage: RecoveryIdle, Topology: cfg.Gateway.Mode}); err != nil {
		writeError(w, http.StatusInternalServerError, "recovery_write_failed", err.Error())
		return
	}
	response, err := s.configFileResponse()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_file_read_failed", err.Error())
		return
	}
	if response.Revision != newRevision {
		writeError(w, http.StatusInternalServerError, "config_file_revision_failed", "saved configuration revision could not be confirmed")
		return
	}
	w.Header().Set("ETag", `"`+response.Revision+`"`)
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) configFileResponse() (ConfigFile, error) {
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return ConfigFile{}, err
	}
	return ConfigFile{
		SchemaVersion:   SchemaVersion,
		Path:            filepath.ToSlash(s.configPath),
		Revision:        fileDigest(s.configPath),
		Content:         redactConfigFile(data),
		ProtectedFields: []string{"mihomo.secret", "upstream_proxy.password"},
	}, nil
}

// redactConfigFile only redacts the two credentials that are persisted in the
// authoritative OpenSurge file. The editor always receives canonical scalar
// config, but doing this line-by-line also keeps the read path safe if an
// operator is recovering a syntactically invalid file.
func redactConfigFile(data []byte) string {
	var output strings.Builder
	section := ""
	for _, line := range strings.SplitAfter(string(data), "\n") {
		lineBody, newline := strings.TrimSuffix(line, "\n"), ""
		if strings.HasSuffix(lineBody, "\r") {
			lineBody = strings.TrimSuffix(lineBody, "\r")
			newline = "\n"
		} else if strings.HasSuffix(line, "\n") {
			newline = "\n"
		}

		raw := strings.TrimSpace(stripConfigFileComment(lineBody))
		if strings.HasSuffix(raw, ":") {
			section = strings.TrimSuffix(raw, ":")
			output.WriteString(lineBody)
			output.WriteString(newline)
			continue
		}
		key, value, ok := strings.Cut(raw, ":")
		if !ok || !protectedConfigFileKey(section, strings.TrimSpace(key)) || configFileScalar(value) == "" {
			output.WriteString(lineBody)
			output.WriteString(newline)
			continue
		}

		indent := lineBody[:len(lineBody)-len(strings.TrimLeft(lineBody, " \t"))]
		output.WriteString(indent)
		output.WriteString(strings.TrimSpace(key))
		output.WriteString(": \"")
		output.WriteString(redactedConfigFileValue)
		output.WriteString("\"")
		output.WriteString(newline)
	}
	return output.String()
}

func stripConfigFileComment(line string) string {
	inSingle, inDouble := false, false
	for index, char := range line {
		switch char {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:index]
			}
		}
	}
	return line
}

func configFileScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		return value[1 : len(value)-1]
	}
	return value
}

func protectedConfigFileKey(section, key string) bool {
	return (section == "mihomo" && key == "secret") || (section == "upstream_proxy" && key == "password")
}

func (DirectRunner) ApplyConfigFile(ctx context.Context, configPath, revision string, payload []byte) (string, error) {
	if os.Geteuid() != 0 {
		return "", fmt.Errorf("privileged helper is required")
	}
	var result string
	err := withConfigurationLifecycleLock(configPath, func() error {
		var err error
		result, err = applyConfigFile(ctx, configPath, revision, payload)
		return err
	})
	return result, err
}

func applyConfigFile(ctx context.Context, configPath, revision string, payload []byte) (string, error) {
	gateway.ReportProgress(ctx, "preparing_config")
	if len(payload) == 0 || len(payload) > maxConfigFileSize {
		return "", fmt.Errorf("config file must be between 1 byte and 256 KiB")
	}
	if revision == "" || revision != fileDigest(configPath) {
		return "", fmt.Errorf("config revision conflict")
	}

	current, err := config.Load(configPath)
	if err != nil {
		return "", err
	}
	paths := runtime.NewPaths(current)
	if _, exists, err := runtime.LoadState(paths.StateFile); err != nil {
		return "", err
	} else if exists {
		return "", fmt.Errorf("gateway must be stopped before editing the configuration file")
	}

	temporary, err := os.CreateTemp(filepath.Dir(configPath), ".opensurge-config-edit-*.yaml")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}

	// LoadRuntime parses and validates the candidate without reading a
	// user-selected device-policy file. The file path is checked below before a
	// full validation reads the existing, OpenSurge-owned policy document.
	gateway.ReportProgress(ctx, "validating_config")
	candidate, err := config.LoadRuntime(temporaryPath)
	if err != nil {
		return "", err
	}
	if err := validateConfigFileCandidate(current, candidate); err != nil {
		return "", err
	}

	// These values are intentionally not editable in the Web editor. Restoring
	// them from the authoritative config also means deleting a redacted line
	// cannot accidentally clear a credential or redirect the helper to a new
	// root-owned input file.
	candidate.DHCP.Binary = current.DHCP.Binary
	candidate.DevicePolicy.File = current.DevicePolicy.File
	candidate.DevicePolicy.Bundle = nil
	candidate.Mihomo.Binary = current.Mihomo.Binary
	candidate.Mihomo.Config = current.Mihomo.Config
	candidate.Mihomo.Profile = current.Mihomo.Profile
	candidate.Mihomo.Secret = current.Mihomo.Secret
	candidate.UpstreamProxy.Password = current.UpstreamProxy.Password
	candidate.Tailscale.AuthKeyFile = current.Tailscale.AuthKeyFile
	candidate.Tailscale.StateDir = current.Tailscale.StateDir
	candidate.Runtime.Dir = current.Runtime.Dir
	if err := config.Normalize(&candidate); err != nil {
		return "", err
	}
	if err := config.Validate(candidate); err != nil {
		return "", err
	}

	gateway.ReportProgress(ctx, "saving_config")
	if err := writeAtomic(configPath, []byte(config.Render(candidate)), 0o640); err != nil {
		return "", err
	}
	return fileDigest(configPath), nil
}

func validateConfigFileCandidate(current, candidate config.Config) error {
	immutable := []struct {
		name  string
		equal func() bool
	}{
		{name: "dhcp.binary", equal: func() bool { return current.DHCP.Binary == candidate.DHCP.Binary }},
		{name: "device_policy.file", equal: func() bool { return current.DevicePolicy.File == candidate.DevicePolicy.File }},
		{name: "mihomo.binary", equal: func() bool { return current.Mihomo.Binary == candidate.Mihomo.Binary }},
		{name: "mihomo.config", equal: func() bool { return current.Mihomo.Config == candidate.Mihomo.Config }},
		{name: "mihomo.profile", equal: func() bool { return current.Mihomo.Profile == candidate.Mihomo.Profile }},
		{name: "tailscale.auth_key_file", equal: func() bool { return current.Tailscale.AuthKeyFile == candidate.Tailscale.AuthKeyFile }},
		{name: "tailscale.state_dir", equal: func() bool { return current.Tailscale.StateDir == candidate.Tailscale.StateDir }},
		{name: "runtime.dir", equal: func() bool { return current.Runtime.Dir == candidate.Runtime.Dir }},
	}
	for _, field := range immutable {
		if !field.equal() {
			return fmt.Errorf("%s cannot be edited in the Web configuration editor", field.name)
		}
	}
	if candidate.Mihomo.Secret != current.Mihomo.Secret && candidate.Mihomo.Secret != redactedConfigFileValue {
		return fmt.Errorf("mihomo.secret is protected and cannot be edited in the Web configuration editor")
	}
	if candidate.UpstreamProxy.Password != current.UpstreamProxy.Password && candidate.UpstreamProxy.Password != redactedConfigFileValue {
		return fmt.Errorf("upstream_proxy.password is protected and cannot be edited in the Web configuration editor")
	}
	return nil
}
