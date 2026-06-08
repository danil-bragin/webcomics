// Package uploadmetrics holds per-upload analytics primitives. Pure domain:
// no SQL / no HTTP. Fetchers (YT API, IG scraper, ...) live in infrastructure.
package uploadmetrics

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type SnapshotID string

func NewSnapshotID() SnapshotID      { return SnapshotID(uuid.NewString()) }
func (id SnapshotID) String() string { return string(id) }

// Snapshot captures one point-in-time view of an upload's public metrics.
// raw_json carries platform-specific fields (e.g. YT's contentDetails) for
// future-proofing without schema churn.
type Snapshot struct {
	ID             SnapshotID
	UploadRecordID string
	FetchedAt      time.Time
	Views          int64
	Likes          int64
	Comments       int64
	Shares         int64
	Raw            map[string]any
}

func NewSnapshot(uploadRecordID string, views, likes, comments, shares int64, raw map[string]any) Snapshot {
	if raw == nil {
		raw = map[string]any{}
	}
	return Snapshot{
		ID:             NewSnapshotID(),
		UploadRecordID: uploadRecordID,
		FetchedAt:      time.Now().UTC(),
		Views:          views,
		Likes:          likes,
		Comments:       comments,
		Shares:         shares,
		Raw:            raw,
	}
}

// Fetcher is the port a metrics provider implements. Platform() returns
// the social_accounts.platform value the fetcher handles (e.g.
// "youtube_selenium"). Fetch is called with the externalRef captured at
// upload time (YT URL, IG reel URL, etc.) and an optional Firefox profile
// path for selenium-based providers; HTTP-only providers ignore it.
type Fetcher interface {
	Platform() string
	Fetch(ctx context.Context, externalRef, profilePath string) (Snapshot, error)
}
