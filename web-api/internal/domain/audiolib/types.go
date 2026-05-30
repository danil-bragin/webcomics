// Package audiolib holds the AudioTrack aggregate — entries in the audio
// library that the music/audio workers pick from at run time (background
// music, SFX stings, ambient loops, voice samples).
//
// Pure domain. No SQL, no transport. Schema in migration 00014.
package audiolib

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrTrackTitleEmpty = errors.New("audiolib: title empty")
	ErrTrackKindInvalid = errors.New("audiolib: kind must be one of music|sfx|ambient|voice")
	ErrTrackScopeInvalid = errors.New("audiolib: scope must be global or project")
	ErrTrackSourceInvalid = errors.New("audiolib: source must be manual|url|pixabay")
	ErrTrackProjectIDMissing = errors.New("audiolib: project_id required when scope=project")
	ErrTrackObjectKeyEmpty = errors.New("audiolib: object_key empty")
	ErrTrackNotFound = errors.New("audiolib: track not found")
)

type Kind string

const (
	KindMusic   Kind = "music"
	KindSFX     Kind = "sfx"
	KindAmbient Kind = "ambient"
	KindVoice   Kind = "voice"
)

func (k Kind) Valid() bool {
	switch k {
	case KindMusic, KindSFX, KindAmbient, KindVoice:
		return true
	}
	return false
}

type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeProject Scope = "project"
)

func (s Scope) Valid() bool { return s == ScopeGlobal || s == ScopeProject }

type Source string

const (
	SourceManual  Source = "manual"
	SourceURL     Source = "url"
	SourcePixabay Source = "pixabay"
)

func (s Source) Valid() bool {
	switch s {
	case SourceManual, SourceURL, SourcePixabay:
		return true
	}
	return false
}

type TrackID string

func NewTrackID() TrackID       { return TrackID(uuid.NewString()) }
func (id TrackID) String() string { return string(id) }

type Track struct {
	id          TrackID
	kind        Kind
	title       string
	tags        []string
	mood        string
	durationMs  int
	objectKey   string
	bucket      string
	source      Source
	sourceRef   string
	attribution string
	scope       Scope
	projectID   string
	bytes       int64
	createdAt   time.Time
	createdBy   string
}

type NewTrackParams struct {
	Kind        Kind
	Title       string
	Tags        []string
	Mood        string
	DurationMs  int
	ObjectKey   string
	Bucket      string
	Source      Source
	SourceRef   string
	Attribution string
	Scope       Scope
	ProjectID   string
	Bytes       int64
	CreatedBy   string
}

func NewTrack(p NewTrackParams) (*Track, error) {
	title := strings.TrimSpace(p.Title)
	if title == "" {
		return nil, ErrTrackTitleEmpty
	}
	if !p.Kind.Valid() {
		return nil, ErrTrackKindInvalid
	}
	if !p.Scope.Valid() {
		return nil, ErrTrackScopeInvalid
	}
	if !p.Source.Valid() {
		return nil, ErrTrackSourceInvalid
	}
	if p.Scope == ScopeProject && strings.TrimSpace(p.ProjectID) == "" {
		return nil, ErrTrackProjectIDMissing
	}
	if strings.TrimSpace(p.ObjectKey) == "" {
		return nil, ErrTrackObjectKeyEmpty
	}
	bucket := p.Bucket
	if bucket == "" {
		bucket = "webcomics"
	}
	tags := append([]string{}, p.Tags...)
	return &Track{
		id:          NewTrackID(),
		kind:        p.Kind,
		title:       title,
		tags:        tags,
		mood:        strings.TrimSpace(p.Mood),
		durationMs:  p.DurationMs,
		objectKey:   p.ObjectKey,
		bucket:      bucket,
		source:      p.Source,
		sourceRef:   p.SourceRef,
		attribution: p.Attribution,
		scope:       p.Scope,
		projectID:   p.ProjectID,
		bytes:       p.Bytes,
		createdAt:   time.Now().UTC(),
		createdBy:   p.CreatedBy,
	}, nil
}

func (t *Track) ID() TrackID         { return t.id }
func (t *Track) Kind() Kind          { return t.kind }
func (t *Track) Title() string       { return t.title }
func (t *Track) Tags() []string      { return append([]string{}, t.tags...) }
func (t *Track) Mood() string        { return t.mood }
func (t *Track) DurationMs() int     { return t.durationMs }
func (t *Track) ObjectKey() string   { return t.objectKey }
func (t *Track) Bucket() string      { return t.bucket }
func (t *Track) Source() Source      { return t.source }
func (t *Track) SourceRef() string   { return t.sourceRef }
func (t *Track) Attribution() string { return t.attribution }
func (t *Track) Scope() Scope        { return t.scope }
func (t *Track) ProjectID() string   { return t.projectID }
func (t *Track) Bytes() int64        { return t.bytes }
func (t *Track) CreatedAt() time.Time { return t.createdAt }
func (t *Track) CreatedBy() string   { return t.createdBy }

func (t *Track) Retag(tags []string, mood string) {
	t.tags = append([]string{}, tags...)
	if mood != "" {
		t.mood = strings.TrimSpace(mood)
	}
}

func Reconstitute(
	id TrackID, kind Kind, title string, tags []string, mood string,
	durationMs int, objectKey, bucket string, source Source, sourceRef, attribution string,
	scope Scope, projectID string, bytes int64, createdAt time.Time, createdBy string,
) *Track {
	if tags == nil {
		tags = []string{}
	}
	if bucket == "" {
		bucket = "webcomics"
	}
	return &Track{
		id: id, kind: kind, title: title, tags: tags, mood: mood,
		durationMs: durationMs, objectKey: objectKey, bucket: bucket,
		source: source, sourceRef: sourceRef, attribution: attribution,
		scope: scope, projectID: projectID, bytes: bytes,
		createdAt: createdAt, createdBy: createdBy,
	}
}
