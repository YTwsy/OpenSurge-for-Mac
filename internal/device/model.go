package device

import "time"

type Client struct {
	IP        string
	MAC       string
	Hostname  string
	ExpiresAt time.Time
	Online    bool
}
