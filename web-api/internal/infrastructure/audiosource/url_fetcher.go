// Package audiosource implements the URL fetcher + Pixabay scraper that the
// audiolib commands depend on.
package audiosource

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const userAgent = "webcomics-audiolib/1.0 (+https://localhost)"

// HTTPFetcher is the simplest URLFetcher: GET the URL, return the body and
// Content-Type header. Cap responses at 32MB so a runaway link can't pin RAM.
type HTTPFetcher struct {
	client *http.Client
}

func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{client: &http.Client{Timeout: 60 * time.Second}}
}

const maxBody = 32 << 20 // 32 MiB

func (f *HTTPFetcher) Fetch(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("url fetch: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > maxBody {
		return nil, "", fmt.Errorf("url fetch: body exceeds %d bytes", maxBody)
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "audio/mpeg"
	}
	return body, ct, nil
}
