package audiolib

import "context"

// Storage is the subset of the MinIO Store we need: upload bytes and delete.
type Storage interface {
	PutBytes(ctx context.Context, bucket, key string, data []byte, contentType string) error
	RemoveObject(ctx context.Context, bucket, key string) error
	Bucket() string
}

// URLFetcher downloads bytes from an arbitrary URL (for ImportFromURL).
type URLFetcher interface {
	Fetch(ctx context.Context, url string) ([]byte, string, error)
}

// PixabaySearcher knows how to talk to Pixabay (web scraping). Implementations
// may use the audio search HTML pages and extract CDN URLs.
type PixabaySearcher interface {
	Search(ctx context.Context, kind string, query string, limit int) ([]PixabayResult, error)
	Download(ctx context.Context, result PixabayResult) ([]byte, string, error)
}

type PixabayResult struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Tags        []string `json:"tags"`
	DurationMs  int      `json:"duration_ms"`
	PreviewURL  string   `json:"preview_url"`
	DownloadURL string   `json:"download_url"`
	PageURL     string   `json:"page_url"`
	Author      string   `json:"author"`
	Attribution string   `json:"attribution"`
	MimeType    string   `json:"mime_type"`
}
