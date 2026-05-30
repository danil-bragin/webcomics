package pipeline

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrUploadRecordNotFound = errors.New("pipeline: upload record not found")
)

// UploadRecordID is the public identifier of one upload attempt.
type UploadRecordID string

func NewUploadRecordID() UploadRecordID { return UploadRecordID(uuid.NewString()) }
func (id UploadRecordID) String() string { return string(id) }

// UploadRecordStatus codifies the lifecycle of an upload.
type UploadRecordStatus string

const (
	UploadStatusPending        UploadRecordStatus = "pending"
	UploadStatusMetadataReady  UploadRecordStatus = "metadata_ready"
	UploadStatusPendingReview  UploadRecordStatus = "pending_review"
	UploadStatusApproved       UploadRecordStatus = "approved"
	UploadStatusRejected       UploadRecordStatus = "rejected"
	UploadStatusUploading      UploadRecordStatus = "uploading"
	UploadStatusUploaded       UploadRecordStatus = "uploaded"
	UploadStatusPublished      UploadRecordStatus = "published"
	UploadStatusFailed         UploadRecordStatus = "failed"
)

// UploadMetadata is the resolved snapshot of what we actually pushed to YT.
// Kept as a flat struct so we can persist + diff + reproduce without re-deriving
// from project/template/run.
type UploadMetadata struct {
	Title            string
	Description      string
	Tags             []string
	Hashtags         []string
	Visibility       string // public | unlisted | private
	MadeForKids      bool
	AgeRestriction   string // none | 18plus
	CategoryID       string
	CategoryLabel    string
	CommentsEnabled  bool
	PlaylistNames    []string
	ScheduledAt      *time.Time
	ThumbnailAssetID string
}

// ScreenshotEntry is one frame in the per-stage selenium trail. Worker pushes
// every step into MinIO so the UI can render a scrubbable thumbnail strip.
type ScreenshotEntry struct {
	Stage     string `json:"stage"`
	ObjectKey string `json:"object_key"`
}

// UploadRecord aggregates one upload attempt. Mutates through behavior methods
// so the lifecycle is explicit and reviewable in UI:
//   pending → metadata_ready → pending_review → approved → uploading → uploaded → published
//                                       ↓                      ↓
//                                    rejected               failed
type UploadRecord struct {
	id                     UploadRecordID
	runID                  string
	projectID              string
	socialAccountID        string
	stepIndex              int
	status                 UploadRecordStatus
	provider               string
	platformTarget         string // youtube_shorts | youtube_long | instagram_reels | tiktok | twitter
	metadata               UploadMetadata
	metadataOverridden     bool
	audienceConfidence     float64
	audienceReasoning      string
	hook                   string
	externalRef            string // youtu.be/<id>
	externalID             string
	attempts               int
	errorMessage           string
	errorScreenshotAssetID string
	screenshotTrail        []ScreenshotEntry
	startedAt              *time.Time
	finishedAt             *time.Time
	createdAt              time.Time
	updatedAt              time.Time
}

// NewUploadRecord creates a pending record. The handler that processes the
// upload step calls this at command time before dispatching to the worker.
func NewUploadRecord(runID, projectID, socialAccountID, provider string, stepIndex int, meta UploadMetadata) *UploadRecord {
	now := time.Now().UTC()
	if meta.Tags == nil {
		meta.Tags = []string{}
	}
	if meta.Hashtags == nil {
		meta.Hashtags = []string{}
	}
	if meta.PlaylistNames == nil {
		meta.PlaylistNames = []string{}
	}
	return &UploadRecord{
		id:              NewUploadRecordID(),
		runID:           runID,
		projectID:       projectID,
		socialAccountID: socialAccountID,
		provider:        provider,
		stepIndex:       stepIndex,
		status:          UploadStatusPending,
		metadata:        meta,
		createdAt:       now,
		updatedAt:       now,
	}
}

// MarkUploaded records success — selenium accepted the video and returned a URL.
// status promotes to `uploaded` unless the resolved visibility is already public,
// in which case we jump straight to `published`.
func (r *UploadRecord) MarkUploaded(externalRef, externalID, finalVisibility string) {
	now := time.Now().UTC()
	r.externalRef = externalRef
	r.externalID = externalID
	if finalVisibility != "" {
		r.metadata.Visibility = finalVisibility
	}
	if finalVisibility == "public" {
		r.status = UploadStatusPublished
	} else {
		r.status = UploadStatusUploaded
	}
	if r.startedAt == nil {
		r.startedAt = &r.createdAt
	}
	r.finishedAt = &now
	r.attempts++
	r.errorMessage = ""
	r.errorScreenshotAssetID = ""
	r.updatedAt = now
}

// MarkFailed records a worker failure.
func (r *UploadRecord) MarkFailed(errorMessage, screenshotAssetID string) {
	now := time.Now().UTC()
	r.status = UploadStatusFailed
	r.errorMessage = errorMessage
	r.errorScreenshotAssetID = screenshotAssetID
	r.attempts++
	r.finishedAt = &now
	r.updatedAt = now
}

// PromoteToPublished marks an already-uploaded video as public after a manual
// "publish public" flow finishes.
func (r *UploadRecord) PromoteToPublished() {
	if r.status != UploadStatusUploaded {
		return
	}
	r.status = UploadStatusPublished
	r.metadata.Visibility = "public"
	r.updatedAt = time.Now().UTC()
}

// ApplyLLMMetadata overlays caption-LLM output onto fields the user hasn't
// already overridden. Run-level overrides set at create time are preserved.
// Empty LLM fields are ignored — they never blank out an existing value.
// Also stores audience reasoning + hook for UI surfacing.
func (r *UploadRecord) ApplyLLMMetadata(title, description, hook string, tags, hashtags []string, kids *bool, audienceConf float64, audienceReasoning string, requireReview bool) {
	if !r.metadataOverridden {
		if r.metadata.Title == "" && title != "" {
			r.metadata.Title = title
		}
		if r.metadata.Description == "" && description != "" {
			r.metadata.Description = description
		}
		if len(r.metadata.Tags) == 0 && len(tags) > 0 {
			r.metadata.Tags = append([]string{}, tags...)
		}
		if len(r.metadata.Hashtags) == 0 && len(hashtags) > 0 {
			r.metadata.Hashtags = append([]string{}, hashtags...)
		}
	}
	if kids != nil && *kids {
		r.metadata.MadeForKids = true // safety: LLM raises bar only
	}
	r.audienceConfidence = audienceConf
	if audienceReasoning != "" {
		r.audienceReasoning = audienceReasoning
	}
	if hook != "" {
		r.hook = hook
	}
	// Transition: pending → (review gate?) metadata_ready vs pending_review.
	if r.status == UploadStatusPending {
		if requireReview {
			r.status = UploadStatusPendingReview
		} else {
			r.status = UploadStatusMetadataReady
		}
	}
	r.updatedAt = time.Now().UTC()
}

// OverrideMetadata is the operator-side edit. Wipes the LLM-baked title/desc/
// tags/etc with user-supplied values and locks the row so further LLM retries
// don't overwrite the manual edit.
func (r *UploadRecord) OverrideMetadata(m UploadMetadata) {
	r.metadata = m
	r.metadataOverridden = true
	r.updatedAt = time.Now().UTC()
}

// Approve transitions a pending_review (or metadata_ready) record to approved.
// The upload worker watches for approved + emits the actual upload request.
// Forces #Shorts into the description when the target platform is vertical so
// YouTube classifies it as a Short.
func (r *UploadRecord) Approve() {
	if r.status != UploadStatusPendingReview && r.status != UploadStatusMetadataReady {
		return
	}
	r.ensureShortsHashtag()
	r.status = UploadStatusApproved
	r.updatedAt = time.Now().UTC()
}

// ensureShortsHashtag guarantees #Shorts is somewhere in the description / hashtags
// for vertical-targeted uploads. YT uses this signal (plus 9:16 + duration ≤180s)
// to classify the upload as a Short.
func (r *UploadRecord) ensureShortsHashtag() {
	if r.provider != "youtube_selenium" && r.provider != "youtube_shorts_selenium" {
		return
	}
	hasShorts := false
	for _, h := range r.metadata.Hashtags {
		if strings.EqualFold(strings.TrimPrefix(h, "#"), "shorts") {
			hasShorts = true
			break
		}
	}
	if !hasShorts {
		r.metadata.Hashtags = append([]string{"Shorts"}, r.metadata.Hashtags...)
	}
	if !strings.Contains(strings.ToLower(r.metadata.Description), "#shorts") {
		if r.metadata.Description == "" {
			r.metadata.Description = "#Shorts"
		} else {
			r.metadata.Description += "\n\n#Shorts"
		}
	}
}

// AppendMusicAttribution adds the music library attribution line to the
// description if not already present. Called when the upload provider needs
// to credit the music (CC BY licenses).
func (r *UploadRecord) AppendMusicAttribution(attribution string) {
	if attribution == "" {
		return
	}
	if strings.Contains(r.metadata.Description, attribution) {
		return
	}
	if r.metadata.Description == "" {
		r.metadata.Description = "🎵 " + attribution
	} else {
		r.metadata.Description += "\n\n🎵 " + attribution
	}
	r.updatedAt = time.Now().UTC()
}

// Reject ends the lifecycle without uploading.
func (r *UploadRecord) Reject() {
	if r.status == UploadStatusUploaded || r.status == UploadStatusPublished {
		return
	}
	r.status = UploadStatusRejected
	r.updatedAt = time.Now().UTC()
}

// MarkUploading is set when the worker picks up an approved record.
func (r *UploadRecord) MarkUploading() {
	r.status = UploadStatusUploading
	now := time.Now().UTC()
	if r.startedAt == nil {
		r.startedAt = &now
	}
	r.attempts++
	r.updatedAt = now
}

// SetPlatformTarget tags the row with a target platform so the multi-platform
// upload pipeline knows which encoded asset to grab.
func (r *UploadRecord) SetPlatformTarget(p string) {
	r.platformTarget = p
	r.updatedAt = time.Now().UTC()
}

// Getters for the new fields.
func (r *UploadRecord) MetadataOverridden() bool { return r.metadataOverridden }
func (r *UploadRecord) AudienceConfidence() float64 { return r.audienceConfidence }
func (r *UploadRecord) AudienceReasoning() string { return r.audienceReasoning }
func (r *UploadRecord) Hook() string { return r.hook }
func (r *UploadRecord) PlatformTarget() string { return r.platformTarget }

// Getters.
func (r *UploadRecord) ID() UploadRecordID                  { return r.id }
func (r *UploadRecord) RunID() string                       { return r.runID }
func (r *UploadRecord) ProjectID() string                   { return r.projectID }
func (r *UploadRecord) SocialAccountID() string             { return r.socialAccountID }
func (r *UploadRecord) StepIndex() int                      { return r.stepIndex }
func (r *UploadRecord) Status() UploadRecordStatus          { return r.status }
func (r *UploadRecord) Provider() string                    { return r.provider }
func (r *UploadRecord) Metadata() UploadMetadata            { return r.metadata }
func (r *UploadRecord) ExternalRef() string                 { return r.externalRef }
func (r *UploadRecord) ExternalID() string                  { return r.externalID }
func (r *UploadRecord) Attempts() int                       { return r.attempts }
func (r *UploadRecord) ErrorMessage() string                { return r.errorMessage }
func (r *UploadRecord) ErrorScreenshotAssetID() string      { return r.errorScreenshotAssetID }
func (r *UploadRecord) ScreenshotTrail() []ScreenshotEntry  { return r.screenshotTrail }

// SetScreenshotTrail replaces the per-stage screenshot trail. Called by the
// MarkUploaded / MarkFailed commands when the worker payload includes the
// trail of MinIO object keys it captured during the selenium walk.
func (r *UploadRecord) SetScreenshotTrail(trail []ScreenshotEntry) {
	r.screenshotTrail = trail
	r.updatedAt = time.Now().UTC()
}
func (r *UploadRecord) StartedAt() *time.Time               { return r.startedAt }
func (r *UploadRecord) FinishedAt() *time.Time              { return r.finishedAt }
func (r *UploadRecord) CreatedAt() time.Time                { return r.createdAt }
func (r *UploadRecord) UpdatedAt() time.Time                { return r.updatedAt }

// ReconstituteUploadRecord rebuilds the aggregate from storage without emitting
// events. Used by the write repo's loader.
func ReconstituteUploadRecord(
	id UploadRecordID, runID, projectID, socialAccountID, provider string,
	stepIndex int, status UploadRecordStatus, meta UploadMetadata,
	externalRef, externalID string, attempts int,
	errMsg, errShotAssetID string,
	startedAt, finishedAt *time.Time,
	createdAt, updatedAt time.Time,
) *UploadRecord {
	return &UploadRecord{
		id: id, runID: runID, projectID: projectID, socialAccountID: socialAccountID,
		provider: provider, stepIndex: stepIndex, status: status, metadata: meta,
		externalRef: externalRef, externalID: externalID, attempts: attempts,
		errorMessage: errMsg, errorScreenshotAssetID: errShotAssetID,
		startedAt: startedAt, finishedAt: finishedAt,
		createdAt: createdAt, updatedAt: updatedAt,
	}
}

// ReconstituteUploadRecordFull is the v2 loader including the lifecycle fields
// added in migration 00011.
func ReconstituteUploadRecordFull(
	id UploadRecordID, runID, projectID, socialAccountID, provider, platformTarget string,
	stepIndex int, status UploadRecordStatus, meta UploadMetadata,
	metadataOverridden bool, audienceConf float64, audienceReasoning, hook string,
	externalRef, externalID string, attempts int,
	errMsg, errShotAssetID string,
	startedAt, finishedAt *time.Time,
	createdAt, updatedAt time.Time,
) *UploadRecord {
	return &UploadRecord{
		id: id, runID: runID, projectID: projectID, socialAccountID: socialAccountID,
		provider: provider, platformTarget: platformTarget,
		stepIndex: stepIndex, status: status, metadata: meta,
		metadataOverridden: metadataOverridden,
		audienceConfidence: audienceConf, audienceReasoning: audienceReasoning, hook: hook,
		externalRef: externalRef, externalID: externalID, attempts: attempts,
		errorMessage: errMsg, errorScreenshotAssetID: errShotAssetID,
		startedAt: startedAt, finishedAt: finishedAt,
		createdAt: createdAt, updatedAt: updatedAt,
	}
}
