package device

import "time"

type Client struct {
	IP        string    `json:"ip"`
	MAC       string    `json:"mac"`
	Hostname  string    `json:"hostname,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	Online    bool      `json:"online"`
}
