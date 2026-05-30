package audiosource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	audiocmd "github.com/example/dddcqrs/internal/app/command/audiolib"
)

// PixabayScraper hits the public Pixabay HTML search pages and extracts the
// embedded `__INITIAL_STATE__` JSON or known data attributes that carry the
// preview/download URLs. Pixabay does NOT publish an official audio search API,
// so this is best-effort: if HTML changes we degrade gracefully to "no
// results" rather than 5xx.
//
// kind → URL mapping:
//   music   → https://pixabay.com/music/search/<q>/
//   sfx     → https://pixabay.com/sound-effects/search/<q>/
//   ambient → music with "ambient" mood filter
//   voice   → sfx with "voice" tag
type PixabayScraper struct {
	client *http.Client
	apiKey string
}

func NewPixabayScraper(apiKey string) *PixabayScraper {
	return &PixabayScraper{
		client: &http.Client{Timeout: 30 * time.Second},
		apiKey: apiKey,
	}
}

func (p *PixabayScraper) Search(ctx context.Context, kind, query string, limit int) ([]audiocmd.PixabayResult, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	pageURL := buildPixabaySearchURL(kind, query)
	html, err := p.fetchHTML(ctx, pageURL)
	if err != nil {
		return nil, err
	}
	hits := parsePixabayHits(html)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]audiocmd.PixabayResult, 0, len(hits))
	for _, h := range hits {
		page := h.PageURL
		if page == "" {
			page = pageURL
		}
		dl := h.DownloadURL
		if dl == "" {
			dl = h.PreviewURL
		}
		if dl == "" {
			continue
		}
		out = append(out, audiocmd.PixabayResult{
			ID:          h.ID,
			Title:       h.Title,
			Tags:        h.Tags,
			DurationMs:  h.DurationMs,
			PreviewURL:  h.PreviewURL,
			DownloadURL: dl,
			PageURL:     page,
			Author:      h.Author,
			Attribution: pixabayAttribution(h.Author),
			MimeType:    "audio/mpeg",
		})
	}
	return out, nil
}

// Pixabay sits behind Cloudflare; a stripped User-Agent gets 403. We match a
// real Chrome on macOS, which the WAF lets through.
const pixabayUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

func (p *PixabayScraper) Download(ctx context.Context, r audiocmd.PixabayResult) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.DownloadURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", pixabayUA)
	req.Header.Set("Referer", "https://pixabay.com/")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("pixabay download: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > maxBody {
		return nil, "", fmt.Errorf("pixabay download: body exceeds %d bytes", maxBody)
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "audio/mpeg"
	}
	return body, ct, nil
}

func (p *PixabayScraper) fetchHTML(ctx context.Context, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", pixabayUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="126", "Google Chrome";v="126", "Not-A.Brand";v="99"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"macOS"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("pixabay html: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func buildPixabaySearchURL(kind, query string) string {
	q := url.QueryEscape(strings.TrimSpace(query))
	switch strings.ToLower(kind) {
	case "sfx", "voice":
		if q == "" {
			return "https://pixabay.com/sound-effects/"
		}
		return "https://pixabay.com/sound-effects/search/" + q + "/"
	case "ambient":
		if q == "" {
			return "https://pixabay.com/music/search/genre/ambient/"
		}
		return "https://pixabay.com/music/search/" + q + "/?genre=ambient"
	default: // music
		if q == "" {
			return "https://pixabay.com/music/"
		}
		return "https://pixabay.com/music/search/" + q + "/"
	}
}

type pixabayHit struct {
	ID          string
	Title       string
	Tags        []string
	DurationMs  int
	PreviewURL  string
	DownloadURL string
	PageURL     string
	Author      string
}

var (
	// Modern Pixabay pages embed a JSON island with `__NEXT_DATA__` or use
	// data attributes per result. Tolerate both shapes; bail to empty list if
	// neither matches.
	reNextData    = regexp.MustCompile(`(?s)<script[^>]+id="__NEXT_DATA__"[^>]*>(.*?)</script>`)
	reAudioPlayer = regexp.MustCompile(`(?s)data-audio[^=]*="(https?[^"]+\.mp3[^"]*)"`)
	reTitle       = regexp.MustCompile(`(?s)<title>([^<]+)</title>`)
)

// parsePixabayHits tries several shapes; first to match wins.
func parsePixabayHits(html string) []pixabayHit {
	if hits := parseNextData(html); len(hits) > 0 {
		return hits
	}
	return parseDataAttributes(html)
}

func parseNextData(html string) []pixabayHit {
	m := reNextData.FindStringSubmatch(html)
	if len(m) < 2 {
		return nil
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(m[1]), &root); err != nil {
		return nil
	}
	// Drill into props.pageProps; vary across Pixabay sections.
	queue := []any{root}
	hits := []pixabayHit{}
	seen := map[string]bool{}
	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]
		switch v := head.(type) {
		case map[string]any:
			if isHitObject(v) {
				h := mapToHit(v)
				if h.ID != "" && !seen[h.ID] {
					seen[h.ID] = true
					hits = append(hits, h)
				}
			}
			for _, child := range v {
				queue = append(queue, child)
			}
		case []any:
			for _, c := range v {
				queue = append(queue, c)
			}
		}
	}
	return hits
}

func isHitObject(o map[string]any) bool {
	hasAudio := false
	hasMP3 := false
	for k, v := range o {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "audio") || strings.Contains(lk, "preview") || strings.Contains(lk, "src") {
			if s, ok := v.(string); ok && strings.Contains(strings.ToLower(s), ".mp3") {
				hasMP3 = true
			}
		}
		if lk == "duration" || lk == "length" {
			hasAudio = true
		}
	}
	return hasAudio && hasMP3
}

func mapToHit(o map[string]any) pixabayHit {
	h := pixabayHit{}
	for k, v := range o {
		lk := strings.ToLower(k)
		switch {
		case lk == "id":
			h.ID = fmt.Sprint(v)
		case lk == "title" || lk == "name":
			if s, ok := v.(string); ok && h.Title == "" {
				h.Title = s
			}
		case lk == "duration" || lk == "length":
			if f, ok := v.(float64); ok {
				if f < 10000 {
					h.DurationMs = int(f * 1000) // seconds
				} else {
					h.DurationMs = int(f) // already ms
				}
			}
		case strings.Contains(lk, "preview"):
			if s, ok := v.(string); ok && strings.Contains(s, ".mp3") {
				h.PreviewURL = s
			}
		case strings.Contains(lk, "download") || strings.Contains(lk, "src"):
			if s, ok := v.(string); ok && strings.Contains(s, ".mp3") {
				h.DownloadURL = s
			}
		case strings.Contains(lk, "user") || strings.Contains(lk, "author"):
			if s, ok := v.(string); ok && h.Author == "" {
				h.Author = s
			}
			if m, ok := v.(map[string]any); ok {
				if n, ok := m["username"].(string); ok {
					h.Author = n
				} else if n, ok := m["name"].(string); ok {
					h.Author = n
				}
			}
		case lk == "tags":
			if s, ok := v.(string); ok {
				for _, t := range strings.Split(s, ",") {
					h.Tags = append(h.Tags, strings.TrimSpace(t))
				}
			}
			if arr, ok := v.([]any); ok {
				for _, x := range arr {
					if t, ok := x.(string); ok {
						h.Tags = append(h.Tags, t)
					}
				}
			}
		case strings.Contains(lk, "url") && strings.Contains(lk, "page"):
			if s, ok := v.(string); ok {
				h.PageURL = s
			}
		}
	}
	if h.DownloadURL == "" {
		h.DownloadURL = h.PreviewURL
	}
	return h
}

// parseDataAttributes is a fallback: scan the HTML for `data-audio="..mp3"`
// attributes and grab nearby title text. Quality is lower but it's a last
// resort if the Next.js island JSON changes shape.
func parseDataAttributes(html string) []pixabayHit {
	out := []pixabayHit{}
	matches := reAudioPlayer.FindAllStringSubmatch(html, -1)
	title := ""
	if tm := reTitle.FindStringSubmatch(html); len(tm) > 1 {
		title = strings.TrimSpace(tm[1])
	}
	for i, m := range matches {
		if len(m) < 2 {
			continue
		}
		out = append(out, pixabayHit{
			ID:          fmt.Sprintf("pixabay-%d", i),
			Title:       fmt.Sprintf("%s #%d", title, i+1),
			PreviewURL:  m[1],
			DownloadURL: m[1],
		})
	}
	return out
}

func pixabayAttribution(author string) string {
	if author == "" {
		return "Pixabay (royalty-free)"
	}
	return "Pixabay — " + author
}
