package mihomo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"open-mihomo-gateway/internal/config"
)

type Version struct {
	Version string `json:"version"`
	Meta    bool   `json:"meta"`
}

func FetchVersion(ctx context.Context, cfg config.Config) (Version, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	return fetchVersionWithClient(ctx, cfg, client)
}

func fetchVersionWithClient(ctx context.Context, cfg config.Config, client *http.Client) (Version, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL(cfg, "/version"), nil)
	if err != nil {
		return Version{}, err
	}
	if cfg.Mihomo.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Mihomo.Secret)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Version{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Version{}, fmt.Errorf("mihomo API returned %s", resp.Status)
	}

	var version Version
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		if errors.Is(err, io.EOF) {
			return Version{}, fmt.Errorf("empty mihomo API response")
		}
		return Version{}, err
	}
	return version, nil
}

func apiURL(cfg config.Config, path string) string {
	base := cfg.Mihomo.APIAddr
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}
	return strings.TrimRight(base, "/") + path
}
