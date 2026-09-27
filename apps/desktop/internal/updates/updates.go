// Package updates discovers stable releases; it never downloads or installs one.
package updates

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

const Endpoint = "https://api.github.com/repos/YTwsy/OpenSurge-for-Mac/releases/latest"
const releasePrefix = "https://github.com/YTwsy/OpenSurge-for-Mac/releases/tag/"

type Release struct {
	Tag        string `json:"tag_name"`
	URL        string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}
type Fetch func(context.Context) (Release, error)
type Snapshot struct {
	Current  string `json:"current"`
	Version  string `json:"version,omitempty"`
	URL      string `json:"url,omitempty"`
	Checking bool   `json:"checking"`
	Checked  bool   `json:"checked"`
	Failed   bool   `json:"failed"`
	Sequence uint64 `json:"sequence"`
}
type Checker struct {
	mu       sync.Mutex
	fetch    Fetch
	snapshot Snapshot
}

func New(current string, fetch Fetch) *Checker {
	return &Checker{fetch: fetch, snapshot: Snapshot{Current: current}}
}
func (c *Checker) Snapshot() Snapshot { c.mu.Lock(); defer c.mu.Unlock(); return c.snapshot }
func version(value string) string {
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	return value
}
func (c *Checker) Check(ctx context.Context) Snapshot {
	c.mu.Lock()
	if c.snapshot.Checking {
		result := c.snapshot
		c.mu.Unlock()
		return result
	}
	c.snapshot.Checking = true
	c.snapshot.Failed = false
	c.snapshot.Sequence++
	current := version(c.snapshot.Current)
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	release, err := c.fetch(ctx)
	available := version(release.Tag)
	if !semver.IsValid(current) || !semver.IsValid(available) || semver.Prerelease(available) != "" || release.Draft || release.Prerelease || release.URL != releasePrefix+release.Tag {
		err = errors.New("invalid stable release")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snapshot.Checking = false
	c.snapshot.Checked = true
	c.snapshot.Failed = err != nil
	c.snapshot.Sequence++
	c.snapshot.Version = ""
	c.snapshot.URL = ""
	if err == nil && semver.Compare(available, current) > 0 {
		c.snapshot.Version = release.Tag
		c.snapshot.URL = release.URL
	}
	return c.snapshot
}
func (c *Checker) Run(ctx context.Context) {
	for ctx.Err() == nil {
		result := c.Check(ctx)
		delay := 24 * time.Hour
		if result.Failed {
			delay = 15 * time.Minute
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func FetchLatest(ctx context.Context) (Release, error) {
	return fetchLatest(ctx, http.DefaultTransport)
}
func fetchLatest(ctx context.Context, transport http.RoundTripper) (Release, error) {
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("release redirects are not allowed") }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Endpoint, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "OpenSurge-Desktop")
	response, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, errors.New("release request failed")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return Release{}, errors.New("release response exceeds limit")
	}
	var release Release
	err = json.Unmarshal(data, &release)
	return release, err
}
