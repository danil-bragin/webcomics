// Package audiolib holds write-side commands for the audio library: upload a
// local file, import a URL (auto-fetch), import a Pixabay search result, retag
// a track, delete a track.
package audiolib

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/domain/audiolib"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

// --- Upload (bytes-in-memory) ---

type UploadTrack struct {
	Kind        string
	Title       string
	Tags        []string
	Mood        string
	DurationMs  int
	Data        []byte
	ContentType string
	Scope       string // global|project
	ProjectID   string
	CreatedBy   string
}

func (UploadTrack) IsCommand() {}

type UploadTrackResult struct {
	ID        string
	ObjectKey string
}

type UploadTrackHandler struct {
	uow     uow.Manager
	storage Storage
}

func NewUploadTrackHandler(m uow.Manager, s Storage) *UploadTrackHandler {
	return &UploadTrackHandler{uow: m, storage: s}
}

func (h *UploadTrackHandler) Handle(ctx context.Context, cmd UploadTrack) (UploadTrackResult, error) {
	var out UploadTrackResult
	if len(cmd.Data) == 0 {
		return out, errors.New("audiolib: empty upload body")
	}
	kind := audiolib.Kind(strings.ToLower(cmd.Kind))
	if !kind.Valid() {
		return out, audiolib.ErrTrackKindInvalid
	}
	scope := audiolib.Scope(strings.ToLower(cmd.Scope))
	if scope == "" {
		scope = audiolib.ScopeGlobal
	}
	objectKey := buildObjectKey(string(kind), cmd.Title)
	if err := h.storage.PutBytes(ctx, h.storage.Bucket(), objectKey, cmd.Data, cmd.ContentType); err != nil {
		return out, fmt.Errorf("upload object: %w", err)
	}
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		t, err := audiolib.NewTrack(audiolib.NewTrackParams{
			Kind:        kind,
			Title:       cmd.Title,
			Tags:        cmd.Tags,
			Mood:        cmd.Mood,
			DurationMs:  cmd.DurationMs,
			ObjectKey:   objectKey,
			Bucket:      h.storage.Bucket(),
			Source:      audiolib.SourceManual,
			Scope:       scope,
			ProjectID:   cmd.ProjectID,
			Bytes:       int64(len(cmd.Data)),
			CreatedBy:   cmd.CreatedBy,
		})
		if err != nil {
			return err
		}
		if err := u.Repositories().AudioLib().Save(ctx, t); err != nil {
			return err
		}
		out.ID = t.ID().String()
		out.ObjectKey = t.ObjectKey()
		return nil
	})
	if err != nil {
		// best-effort cleanup so we don't leave orphan object on partial failure
		_ = h.storage.RemoveObject(ctx, h.storage.Bucket(), objectKey)
		return out, err
	}
	return out, nil
}

// --- ImportFromURL ---

type ImportFromURL struct {
	Kind        string
	URL         string
	Title       string
	Tags        []string
	Mood        string
	Scope       string
	ProjectID   string
	CreatedBy   string
	Attribution string
}

func (ImportFromURL) IsCommand() {}

type ImportFromURLResult struct {
	ID        string
	ObjectKey string
}

type ImportFromURLHandler struct {
	uow     uow.Manager
	storage Storage
	fetcher URLFetcher
}

func NewImportFromURLHandler(m uow.Manager, s Storage, f URLFetcher) *ImportFromURLHandler {
	return &ImportFromURLHandler{uow: m, storage: s, fetcher: f}
}

func (h *ImportFromURLHandler) Handle(ctx context.Context, cmd ImportFromURL) (ImportFromURLResult, error) {
	var out ImportFromURLResult
	if strings.TrimSpace(cmd.URL) == "" {
		return out, errors.New("audiolib: empty url")
	}
	data, contentType, err := h.fetcher.Fetch(ctx, cmd.URL)
	if err != nil {
		return out, fmt.Errorf("fetch url: %w", err)
	}
	title := cmd.Title
	if title == "" {
		title = guessTitleFromURL(cmd.URL)
	}
	upload := UploadTrack{
		Kind:        cmd.Kind,
		Title:       title,
		Tags:        cmd.Tags,
		Mood:        cmd.Mood,
		Data:        data,
		ContentType: contentType,
		Scope:       cmd.Scope,
		ProjectID:   cmd.ProjectID,
		CreatedBy:   cmd.CreatedBy,
	}
	uh := NewUploadTrackHandler(h.uow, h.storage)
	res, err := uh.Handle(ctx, upload)
	if err != nil {
		return out, err
	}
	// patch source + attribution after the base upload
	if err := h.patchSource(ctx, res.ID, audiolib.SourceURL, cmd.URL, cmd.Attribution); err != nil {
		return out, err
	}
	out.ID = res.ID
	out.ObjectKey = res.ObjectKey
	return out, nil
}

// --- ImportFromPixabay ---

type ImportFromPixabay struct {
	Kind      string
	Result    PixabayResult
	Mood      string
	Scope     string
	ProjectID string
	CreatedBy string
}

func (ImportFromPixabay) IsCommand() {}

type ImportFromPixabayResult struct {
	ID        string
	ObjectKey string
}

type ImportFromPixabayHandler struct {
	uow      uow.Manager
	storage  Storage
	searcher PixabaySearcher
}

func NewImportFromPixabayHandler(m uow.Manager, s Storage, p PixabaySearcher) *ImportFromPixabayHandler {
	return &ImportFromPixabayHandler{uow: m, storage: s, searcher: p}
}

func (h *ImportFromPixabayHandler) Handle(ctx context.Context, cmd ImportFromPixabay) (ImportFromPixabayResult, error) {
	var out ImportFromPixabayResult
	if cmd.Result.DownloadURL == "" {
		return out, errors.New("audiolib: pixabay result missing download_url")
	}
	data, contentType, err := h.searcher.Download(ctx, cmd.Result)
	if err != nil {
		return out, fmt.Errorf("pixabay download: %w", err)
	}
	upload := UploadTrack{
		Kind:        cmd.Kind,
		Title:       cmd.Result.Title,
		Tags:        cmd.Result.Tags,
		Mood:        cmd.Mood,
		DurationMs:  cmd.Result.DurationMs,
		Data:        data,
		ContentType: contentType,
		Scope:       cmd.Scope,
		ProjectID:   cmd.ProjectID,
		CreatedBy:   cmd.CreatedBy,
	}
	uh := NewUploadTrackHandler(h.uow, h.storage)
	res, err := uh.Handle(ctx, upload)
	if err != nil {
		return out, err
	}
	attribution := cmd.Result.Attribution
	if attribution == "" && cmd.Result.Author != "" {
		attribution = "Pixabay — " + cmd.Result.Author
	}
	if err := h.patchSourceWith(ctx, res.ID, audiolib.SourcePixabay, cmd.Result.PageURL, attribution); err != nil {
		return out, err
	}
	out.ID = res.ID
	out.ObjectKey = res.ObjectKey
	return out, nil
}

// --- DeleteTrack ---

type DeleteTrack struct{ ID string }

func (DeleteTrack) IsCommand() {}

type DeleteTrackResult struct{}

type DeleteTrackHandler struct {
	uow     uow.Manager
	storage Storage
}

func NewDeleteTrackHandler(m uow.Manager, s Storage) *DeleteTrackHandler {
	return &DeleteTrackHandler{uow: m, storage: s}
}

func (h *DeleteTrackHandler) Handle(ctx context.Context, cmd DeleteTrack) (DeleteTrackResult, error) {
	var key, bucket string
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		t, err := u.Repositories().AudioLib().Get(ctx, audiolib.TrackID(cmd.ID))
		if err != nil {
			return err
		}
		key = t.ObjectKey()
		bucket = t.Bucket()
		return u.Repositories().AudioLib().Delete(ctx, t.ID())
	})
	if err != nil {
		return DeleteTrackResult{}, err
	}
	_ = h.storage.RemoveObject(ctx, bucket, key)
	return DeleteTrackResult{}, nil
}

// --- RetagTrack ---

type RetagTrack struct {
	ID   string
	Tags []string
	Mood string
}

func (RetagTrack) IsCommand() {}

type RetagTrackResult struct{}

type RetagTrackHandler struct{ uow uow.Manager }

func NewRetagTrackHandler(m uow.Manager) *RetagTrackHandler {
	return &RetagTrackHandler{uow: m}
}

func (h *RetagTrackHandler) Handle(ctx context.Context, cmd RetagTrack) (RetagTrackResult, error) {
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		t, err := u.Repositories().AudioLib().Get(ctx, audiolib.TrackID(cmd.ID))
		if err != nil {
			return err
		}
		t.Retag(cmd.Tags, cmd.Mood)
		return u.Repositories().AudioLib().Save(ctx, t)
	})
	return RetagTrackResult{}, err
}

// --- Bus registration ---

func UploadTrackOnBus(r *bus.Registry, m uow.Manager, s Storage) {
	bus.RegisterCommand[UploadTrack, UploadTrackResult](r, NewUploadTrackHandler(m, s))
}

func ImportFromURLOnBus(r *bus.Registry, m uow.Manager, s Storage, f URLFetcher) {
	bus.RegisterCommand[ImportFromURL, ImportFromURLResult](r, NewImportFromURLHandler(m, s, f))
}

func ImportFromPixabayOnBus(r *bus.Registry, m uow.Manager, s Storage, p PixabaySearcher) {
	bus.RegisterCommand[ImportFromPixabay, ImportFromPixabayResult](r, NewImportFromPixabayHandler(m, s, p))
}

func DeleteTrackOnBus(r *bus.Registry, m uow.Manager, s Storage) {
	bus.RegisterCommand[DeleteTrack, DeleteTrackResult](r, NewDeleteTrackHandler(m, s))
}

func RetagTrackOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[RetagTrack, RetagTrackResult](r, NewRetagTrackHandler(m))
}

// --- helpers ---

// patchSource re-loads the track that was just created by UploadTrack and
// stamps the URL/Pixabay source onto it. We do this as a separate UoW because
// UploadTrack runs its own tx; the cost is one extra row update which is
// negligible against the network IO of the fetch.
func (h *ImportFromURLHandler) patchSource(ctx context.Context, id string, src audiolib.Source, ref, attribution string) error {
	return h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		t, err := u.Repositories().AudioLib().Get(ctx, audiolib.TrackID(id))
		if err != nil {
			return err
		}
		nt := audiolib.Reconstitute(
			t.ID(), t.Kind(), t.Title(), t.Tags(), t.Mood(),
			t.DurationMs(), t.ObjectKey(), t.Bucket(), src, ref, attribution,
			t.Scope(), t.ProjectID(), t.Bytes(), t.CreatedAt(), t.CreatedBy(),
		)
		return u.Repositories().AudioLib().Save(ctx, nt)
	})
}

func (h *ImportFromPixabayHandler) patchSourceWith(ctx context.Context, id string, src audiolib.Source, ref, attribution string) error {
	return h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		t, err := u.Repositories().AudioLib().Get(ctx, audiolib.TrackID(id))
		if err != nil {
			return err
		}
		nt := audiolib.Reconstitute(
			t.ID(), t.Kind(), t.Title(), t.Tags(), t.Mood(),
			t.DurationMs(), t.ObjectKey(), t.Bucket(), src, ref, attribution,
			t.Scope(), t.ProjectID(), t.Bytes(), t.CreatedAt(), t.CreatedBy(),
		)
		return u.Repositories().AudioLib().Save(ctx, nt)
	})
}

func buildObjectKey(kind, title string) string {
	safe := strings.ToLower(strings.TrimSpace(title))
	out := make([]rune, 0, len(safe))
	for _, r := range safe {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
		case r == ' ' || r == '_' || r == '-':
			out = append(out, '-')
		}
	}
	slug := string(out)
	if slug == "" {
		slug = "track"
	}
	return fmt.Sprintf("library/audio/%s/%s-%s.mp3", kind, slug, audiolib.NewTrackID())
}

func guessTitleFromURL(u string) string {
	idx := strings.LastIndex(u, "/")
	tail := u
	if idx >= 0 {
		tail = u[idx+1:]
	}
	if i := strings.Index(tail, "?"); i >= 0 {
		tail = tail[:i]
	}
	tail = strings.TrimSuffix(tail, ".mp3")
	tail = strings.TrimSuffix(tail, ".wav")
	tail = strings.TrimSuffix(tail, ".ogg")
	tail = strings.ReplaceAll(tail, "-", " ")
	tail = strings.ReplaceAll(tail, "_", " ")
	tail = strings.TrimSpace(tail)
	if tail == "" {
		return "imported"
	}
	return tail
}
