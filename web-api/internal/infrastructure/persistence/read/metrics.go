package read

import (
	"context"

	"github.com/example/dddcqrs/internal/platform/postgres"

	umq "github.com/example/dddcqrs/internal/app/query/uploadmetrics"
)

type MetricsModel struct{ pool *postgres.ReadPool }

func NewMetricsModel(pool *postgres.ReadPool) *MetricsModel { return &MetricsModel{pool: pool} }

// ListSnapshots returns the newest `limit` snapshots for an upload, newest
// first. UI usually re-sorts ascending for charting.
func (m *MetricsModel) ListSnapshots(ctx context.Context, uploadRecordID string, limit int) ([]umq.SnapshotView, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := m.pool.Query(ctx,
		`SELECT id, fetched_at, views, likes, comments, shares
		   FROM upload_metrics_snapshots
		  WHERE upload_record_id = $1
		  ORDER BY fetched_at DESC
		  LIMIT $2`,
		uploadRecordID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []umq.SnapshotView{}
	for rows.Next() {
		var v umq.SnapshotView
		if err := rows.Scan(&v.ID, &v.FetchedAt, &v.Views, &v.Likes, &v.Comments, &v.Shares); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
