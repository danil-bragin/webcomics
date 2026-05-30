package audiolib

import (
	"context"

	"github.com/example/dddcqrs/internal/app/bus"
)

// ReadModel is the port. Implemented in infrastructure/persistence/read/audiolib.go.
type ReadModel interface {
	ListTracks(ctx context.Context, f ListTracksFilter) ([]TrackView, error)
	GetTrack(ctx context.Context, id string) (TrackView, error)
	PickTrack(ctx context.Context, f PickFilter) (*TrackView, error)
}

type ListTracksFilter struct {
	Kind      string `json:"kind,omitempty"`
	Scope     string `json:"scope,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Mood      string `json:"mood,omitempty"`
	Search    string `json:"search,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

type PickFilter struct {
	Kind      string
	Mood      string
	ProjectID string
}

type ListTracks struct{ Filter ListTracksFilter }

func (ListTracks) IsQuery() {}

type ListTracksHandler struct{ m ReadModel }

func (h ListTracksHandler) Handle(ctx context.Context, q ListTracks) ([]TrackView, error) {
	return h.m.ListTracks(ctx, q.Filter)
}

func ListTracksOnBus(r *bus.Registry, m ReadModel) {
	bus.RegisterQuery[ListTracks, []TrackView](r, ListTracksHandler{m: m})
}

type GetTrack struct{ ID string }

func (GetTrack) IsQuery() {}

type GetTrackHandler struct{ m ReadModel }

func (h GetTrackHandler) Handle(ctx context.Context, q GetTrack) (TrackView, error) {
	return h.m.GetTrack(ctx, q.ID)
}

func GetTrackOnBus(r *bus.Registry, m ReadModel) {
	bus.RegisterQuery[GetTrack, TrackView](r, GetTrackHandler{m: m})
}

type PickTrack struct{ Filter PickFilter }

func (PickTrack) IsQuery() {}

type PickTrackHandler struct{ m ReadModel }

func (h PickTrackHandler) Handle(ctx context.Context, q PickTrack) (*TrackView, error) {
	return h.m.PickTrack(ctx, q.Filter)
}

func PickTrackOnBus(r *bus.Registry, m ReadModel) {
	bus.RegisterQuery[PickTrack, *TrackView](r, PickTrackHandler{m: m})
}
