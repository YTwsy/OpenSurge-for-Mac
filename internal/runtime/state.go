package runtime

import (
	"encoding/json"
	"errors"
	"os"
	"time"
)

type State struct {
	PIDDNSMasq          int       `json:"pid_dnsmasq,omitempty"`
	PIDMihomo           int       `json:"pid_mihomo,omitempty"`
	IPForwardingBefore  string    `json:"ip_forwarding_before,omitempty"`
	PFAnchorLoaded      bool      `json:"pf_anchor_loaded"`
	StartedAt           time.Time `json:"started_at"`
	RuntimePreparedOnly bool      `json:"runtime_prepared_only,omitempty"`
}

func LoadState(path string) (State, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, false, err
	}
	return state, true, nil
}

func SaveState(path string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func RemoveState(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
