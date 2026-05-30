package audiolib

import "context"

// WriteRepo persists Track inside a UoW tx.
type WriteRepo interface {
	Save(ctx context.Context, t *Track) error
	Get(ctx context.Context, id TrackID) (*Track, error)
	Delete(ctx context.Context, id TrackID) error
}

// ListFilter is used by the read model to query the catalogue.
type ListFilter struct {
	Kind      string // music|sfx|ambient|voice|"" (all)
	Scope     string // global|project|""
	ProjectID string // when scope=project; or include both global+project when set with Scope=""
	Mood      string
	Search    string // substring match on title/tags
	Limit     int
	Offset    int
}
