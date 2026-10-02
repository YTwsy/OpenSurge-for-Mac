package updates

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (r roundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return r(req) }
func TestHTTPBoundary(t *testing.T) {
	for _, test := range []struct {
		name           string
		status         int
		body, location string
		success        bool
	}{
		{"valid", 200, `{"tag_name":"v0.2.5"}`, "", true},
		{"http failure", 403, `{}`, "", false},
		{"oversized", 200, strings.Repeat(" ", 1<<20) + `{}`, "", false},
		{"bad json", 200, `{`, "", false},
		{"redirect", 302, `{}`, "https://example.com/untrusted", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			transport := roundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.String() != Endpoint || req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" {
					t.Fatal("unexpected endpoint or credentials")
				}
				return &http.Response{StatusCode: test.status, Header: http.Header{"Location": []string{test.location}}, Body: io.NopCloser(strings.NewReader(test.body)), Request: req}, nil
			})
			_, err := fetchLatest(context.Background(), transport)
			if (err == nil) != test.success || calls != 1 {
				t.Fatal(err, calls)
			}
		})
	}
}

func TestStableReleaseValidation(t *testing.T) {
	for _, test := range []struct {
		current, tag, url             string
		draft, pre, failed, available bool
	}{
		{"v0.2.4-next", "v0.2.4", "", false, false, false, true},
		{"0.2.4-rc.2", "v0.2.4", "", false, false, false, true},
		{"v0.2.4", "v0.2.4", "", false, false, false, false},
		{"v0.3.0", "v0.2.4", "", false, false, false, false},
		{"v0.2.4", "v0.2.5-rc.1", "", false, false, true, false},
		{"v0.2.4", "v0.2.5", "https://example.com/pkg", false, false, true, false},
		{"v0.2.4", "v0.2.5", "https://github.com/YTwsy/OpenSurge-for-Mac/releases/tag/v0.2.5?redirect=x", false, false, true, false},
		{"v0.2.4", "v0.2.5", "", true, false, true, false},
		{"v0.2.4", "v0.2.5", "", false, true, true, false},
		{"unknown", "v0.2.5", "", false, false, true, false},
	} {
		t.Run(test.current+test.tag+test.url, func(t *testing.T) {
			url := test.url
			if url == "" {
				url = releasePrefix + test.tag
			}
			c := New(test.current, func(context.Context) (Release, error) {
				return Release{Tag: test.tag, URL: url, Draft: test.draft, Prerelease: test.pre}, nil
			})
			result := c.Check(context.Background())
			if result.Failed != test.failed || (result.URL != "") != test.available || !result.Checked {
				t.Fatal(result)
			}
		})
	}
}
func TestFailureClearsDownloadAndConcurrentChecksDeduplicate(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	calls := 0
	c := New("v0.2.4", func(context.Context) (Release, error) {
		calls++
		if calls == 1 {
			return Release{Tag: "v0.2.5", URL: releasePrefix + "v0.2.5"}, nil
		}
		close(entered)
		<-release
		return Release{}, errors.New("offline")
	})
	first := c.Check(context.Background())
	if first.URL == "" {
		t.Fatal(first)
	}
	done := make(chan Snapshot)
	go func() { done <- c.Check(context.Background()) }()
	<-entered
	if !c.Check(context.Background()).Checking {
		t.Fatal("lost in-flight state")
	}
	close(release)
	last := <-done
	if !last.Failed || last.URL != "" || last.Version != "" || calls != 2 || last.Sequence <= first.Sequence {
		t.Fatal(last, calls)
	}
}
