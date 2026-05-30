package pipeline

import "time"

type AssetKind string

const (
	AssetScriptJSON AssetKind = "script_json"
	AssetPanelImage AssetKind = "panel_image"
	AssetAudio      AssetKind = "audio"
	AssetMusic      AssetKind = "music"
	AssetVideo      AssetKind = "video"
)

// Asset is metadata only — the bytes live in the object store (MinIO).
type Asset struct {
	ID        AssetID
	RunID     RunID
	StepID    StepID
	AttemptID AttemptID
	Kind      AssetKind
	Bucket    string
	ObjectKey string
	Mime      string
	Bytes     int64
	CreatedAt time.Time
}

func NewAsset(runID RunID, stepID StepID, attemptID AttemptID, kind AssetKind, bucket, key, mime string, bytes int64) Asset {
	return Asset{
		ID:        NewAssetID(),
		RunID:     runID,
		StepID:    stepID,
		AttemptID: attemptID,
		Kind:      kind,
		Bucket:    bucket,
		ObjectKey: key,
		Mime:      mime,
		Bytes:     bytes,
		CreatedAt: time.Now().UTC(),
	}
}

// CostEntry is one provider invocation. Sums into Run.TotalCostUSD.
type CostEntry struct {
	ID           string
	RunID        RunID
	StepID       StepID
	AttemptID    AttemptID
	Provider     string
	Model        string
	Units        float64
	UnitLabel    string
	UnitCostUSD  float64
	TotalCostUSD float64
	OccurredAt   time.Time
}
