package audiolib

import "time"

type TrackView struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Title       string    `json:"title"`
	Tags        []string  `json:"tags"`
	Mood        string    `json:"mood"`
	DurationMs  int       `json:"duration_ms"`
	ObjectKey   string    `json:"object_key"`
	Bucket      string    `json:"bucket"`
	Source      string    `json:"source"`
	SourceRef   string    `json:"source_ref"`
	Attribution string    `json:"attribution"`
	Scope       string    `json:"scope"`
	ProjectID   string    `json:"project_id,omitempty"`
	Bytes       int64     `json:"bytes"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   string    `json:"created_by,omitempty"`
}
