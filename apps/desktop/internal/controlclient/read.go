package controlclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// Read lets native desktop capabilities consume a bounded existing API response
// through the same authentication/reconnect path as the renderer.
func (c *Client) Read(ctx context.Context, path string, limit int) ([]byte, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	w := &boundedResponse{header: make(http.Header), limit: limit}
	c.ServeHTTP(w, r)
	if w.status != http.StatusOK || w.exceeded {
		return nil, ErrUnavailable
	}
	return w.body.Bytes(), nil
}

func (c *Client) ReadJSON(ctx context.Context, path string, value any) error {
	data, err := c.Read(ctx, path, 1<<20)
	if err != nil {
		return err
	}
	if json.Unmarshal(data, value) != nil {
		return ErrResponse
	}
	return nil
}

type boundedResponse struct {
	header        http.Header
	status, limit int
	body          bytes.Buffer
	exceeded      bool
}

func (w *boundedResponse) Header() http.Header { return w.header }
func (w *boundedResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *boundedResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.body.Len()+len(data) > w.limit {
		w.exceeded = true
		return 0, io.ErrShortBuffer
	}
	return w.body.Write(data)
}
