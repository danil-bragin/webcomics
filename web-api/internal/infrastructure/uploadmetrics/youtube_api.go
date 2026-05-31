// Package uploadmetrics holds concrete Fetcher implementations.
//
// YouTubeFetcher uses the public Data API v3 with an API key. videos.list
// surfaces statistics (views/likes/comments) and contentDetails (duration)
// without needing OAuth. Private analytics (CTR, audience, watch-time)
// would require the YT Analytics API with an OAuth token — out of MVP scope.
package uploadmetrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/example/dddcqrs/internal/domain/uploadmetrics"
)

// YouTubeFetcher is HTTP-only; ignores profilePath.
type YouTubeFetcher struct {
	apiKey string
	client *http.Client
}

func NewYouTubeFetcher(apiKey string) *YouTubeFetcher {
	return &YouTubeFetcher{apiKey: apiKey, client: &http.Client{Timeout: 10 * time.Second}}
}

func (f *YouTubeFetcher) Platform() string { return "youtube_selenium" }

func (f *YouTubeFetcher) Fetch(ctx context.Context, externalRef, _profilePath string) (uploadmetrics.Snapshot, error) {
	if f.apiKey == "" {
		return uploadmetrics.Snapshot{}, errors.New("youtube fetcher: YOUTUBE_API_KEY not set")
	}
	videoID := extractYouTubeVideoID(externalRef)
	if videoID == "" {
		return uploadmetrics.Snapshot{}, fmt.Errorf("youtube fetcher: cannot extract video id from %q", externalRef)
	}
	u := fmt.Sprintf(
		"https://www.googleapis.com/youtube/v3/videos?part=statistics,contentDetails,snippet&id=%s&key=%s",
		url.QueryEscape(videoID), url.QueryEscape(f.apiKey),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return uploadmetrics.Snapshot{}, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return uploadmetrics.Snapshot{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return uploadmetrics.Snapshot{}, fmt.Errorf("youtube fetcher: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var body struct {
		Items []struct {
			ID         string `json:"id"`
			Statistics struct {
				ViewCount    string `json:"viewCount"`
				LikeCount    string `json:"likeCount"`
				CommentCount string `json:"commentCount"`
			} `json:"statistics"`
			ContentDetails struct {
				Duration string `json:"duration"` // ISO 8601 PT#M#S
			} `json:"contentDetails"`
			Snippet struct {
				PublishedAt string `json:"publishedAt"`
				Title       string `json:"title"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return uploadmetrics.Snapshot{}, err
	}
	if len(body.Items) == 0 {
		return uploadmetrics.Snapshot{}, fmt.Errorf("youtube fetcher: no video found for id %q (deleted? private?)", videoID)
	}
	it := body.Items[0]
	views := atoi64(it.Statistics.ViewCount)
	likes := atoi64(it.Statistics.LikeCount)
	comments := atoi64(it.Statistics.CommentCount)
	raw := map[string]any{
		"video_id":     it.ID,
		"duration_iso": it.ContentDetails.Duration,
		"duration_sec": parseISO8601Seconds(it.ContentDetails.Duration),
		"published_at": it.Snippet.PublishedAt,
		"title":        it.Snippet.Title,
	}
	return uploadmetrics.NewSnapshot(externalRef, views, likes, comments, 0, raw), nil
}

// extractYouTubeVideoID handles youtu.be/<id> and youtube.com/watch?v=<id>
// + youtube.com/shorts/<id>. Bare ids also pass through.
var ytIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

func extractYouTubeVideoID(s string) string {
	s = strings.TrimSpace(s)
	if ytIDRe.MatchString(s) {
		return s
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	switch {
	case strings.Contains(u.Host, "youtu.be"):
		return strings.TrimPrefix(u.Path, "/")
	case strings.Contains(u.Path, "/shorts/"):
		parts := strings.Split(u.Path, "/")
		for i, p := range parts {
			if p == "shorts" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	case strings.Contains(u.Path, "/watch"):
		return u.Query().Get("v")
	}
	return ""
}

func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// parseISO8601Seconds parses PT#H#M#S → seconds. Returns 0 on parse error.
var iso8601Re = regexp.MustCompile(`PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?`)

func parseISO8601Seconds(s string) int {
	m := iso8601Re.FindStringSubmatch(s)
	if len(m) == 0 {
		return 0
	}
	h, _ := strconv.Atoi(m[1])
	mn, _ := strconv.Atoi(m[2])
	sec, _ := strconv.Atoi(m[3])
	return h*3600 + mn*60 + sec
}
