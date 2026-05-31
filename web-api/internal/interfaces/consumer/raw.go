// Raw Redis Streams consumer that bypasses Watermill for pipeline
// completion streams. Watermill's redis-stream subscriber spawns multiple
// internal goroutines per Subscribe call that ACK messages from a separate
// consumer name than the one running the handler — when a real worker
// publishes a single completion event, it can be claimed by an idle
// internal consumer and never reach the user handler.
//
// This file owns a single goroutine per (stream, consumer-name) pair and
// dispatches directly into the bus. Idempotency is handled by the aggregate.
package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/example/dddcqrs/internal/app/bus"
	pipecmd "github.com/example/dddcqrs/internal/app/command/pipeline"
	umcmd "github.com/example/dddcqrs/internal/app/command/uploadmetrics"
	"github.com/example/dddcqrs/internal/domain/pipeline"
)

const rawConsumerName = "raw-completion-consumer"

// RawCompletionConsumer reads pipeline.*.completed/failed streams directly
// via go-redis and dispatches commands on the bus.
type RawCompletionConsumer struct {
	rdb *redis.Client
	reg *bus.Registry
	log *slog.Logger
}

func NewRawCompletionConsumer(rdb *redis.Client, reg *bus.Registry, log *slog.Logger) *RawCompletionConsumer {
	return &RawCompletionConsumer{rdb: rdb, reg: reg, log: log}
}

// Run launches one goroutine per completion stream. Returns when ctx is done.
func (c *RawCompletionConsumer) Run(ctx context.Context) {
	streams := []struct {
		stream  string
		handler func(context.Context, []byte) error
	}{
		{"pipeline.script.completed", c.handleScript},
		{"pipeline.script.failed", c.handleStepFailed},
		{"pipeline.image.completed", c.handleImage},
		{"pipeline.image.failed", c.handleStepFailed},
		{"pipeline.audio.completed", c.handleAudio},
		{"pipeline.audio.failed", c.handleStepFailed},
		{"pipeline.music.completed", c.handleMusic},
		{"pipeline.music.failed", c.handleStepFailed},
		{"pipeline.assemble.completed", c.handleAssemble},
		{"pipeline.assemble.failed", c.handleStepFailed},
		{"pipeline.upload.completed", c.handleUpload},
		{"pipeline.upload.failed", c.handleUploadFailed},
		{"pipeline.upload.failed", c.handleStepFailed},
		{"pipeline.caption.completed", c.handleCaption},
		{"pipeline.caption.failed", c.handleStepFailed},
		{"pipeline.metrics.completed", c.handleMetricsCompleted},
		{"pipeline.metrics.failed", c.handleMetricsFailed},
	}
	for _, s := range streams {
		go c.consume(ctx, s.stream, s.handler)
	}
}

func (c *RawCompletionConsumer) consume(ctx context.Context, stream string, h func(context.Context, []byte) error) {
	// Ensure the group exists.
	if err := c.rdb.XGroupCreateMkStream(ctx, stream, ConsumerGroup, "0").Err(); err != nil {
		if !contains(err.Error(), "BUSYGROUP") {
			c.log.Error("xgroup create", "stream", stream, "err", err)
			return
		}
	}
	c.log.Info("raw consumer listening", "stream", stream)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		resp, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    ConsumerGroup,
			Consumer: rawConsumerName + "-" + stream,
			Streams:  []string{stream, ">"},
			Count:    8,
			Block:    5 * time.Second,
		}).Result()
		if err == redis.Nil || err == context.Canceled {
			continue
		}
		if err != nil && err.Error() == "redis: nil" {
			continue
		}
		if err != nil {
			c.log.Warn("xreadgroup", "stream", stream, "err", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		for _, s := range resp {
			for _, msg := range s.Messages {
				payload, _ := msg.Values["payload"].(string)
				if payload == "" {
					if b, ok := msg.Values["payload"].([]byte); ok {
						payload = string(b)
					}
				}
				if payload != "" {
					if err := h(ctx, []byte(payload)); err != nil {
						c.log.Error("handler", "stream", stream, "msg_id", msg.ID, "err", err)
					}
				}
				c.rdb.XAck(ctx, stream, ConsumerGroup, msg.ID)
			}
		}
	}
}

func (c *RawCompletionConsumer) handleScript(ctx context.Context, body []byte) error {
	var p pipeline.ScriptCompletedPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	_, err := bus.Dispatch[pipecmd.RecordStepResult](ctx, c.reg, pipecmd.RecordScriptCompleted{
		RunID: p.RunID, StepIndex: p.StepIndex,
		ScriptKey: p.ScriptKey, Bucket: p.Bucket, Bytes: p.Bytes, Panels: p.Panels,
		Cost: p.Cost, DurationMs: p.DurationMs,
	})
	return err
}

func (c *RawCompletionConsumer) handleImage(ctx context.Context, body []byte) error {
	var p pipeline.ImageCompletedPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	_, err := bus.Dispatch[pipecmd.RecordStepResult](ctx, c.reg, pipecmd.RecordImageCompleted{
		RunID: p.RunID, StepIndex: p.StepIndex,
		PanelIndex: p.PanelIndex, ObjectKey: p.ObjectKey, Bucket: p.Bucket,
		Bytes: p.Bytes,
		Cost:  p.Cost, DurationMs: p.DurationMs,
	})
	return err
}

func (c *RawCompletionConsumer) handleCaption(ctx context.Context, body []byte) error {
	var p pipeline.CaptionCompletedPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	if _, err := bus.Dispatch[pipecmd.RecordStepResult](ctx, c.reg, pipecmd.RecordCaptionCompleted{
		RunID: p.RunID, StepIndex: p.StepIndex,
		Captions: p.Captions, Cost: p.Cost, DurationMs: p.DurationMs,
	}); err != nil {
		return err
	}
	// Backfill UploadRecord rows on the run with the LLM-generated metadata
	// so the review UI immediately sees the title/description/tags etc. The
	// previous flow left them blank because the row is created at run start.
	if p.Metadata != nil && len(p.Metadata.Platforms) > 0 {
		_, _ = bus.Dispatch[pipecmd.BackfillUploadMetadataResult](ctx, c.reg, pipecmd.BackfillUploadMetadata{
			RunID: p.RunID, Metadata: p.Metadata,
		})
	}
	return nil
}

func (c *RawCompletionConsumer) handleAudio(ctx context.Context, body []byte) error {
	var p pipeline.AudioCompletedPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	_, err := bus.Dispatch[pipecmd.RecordStepResult](ctx, c.reg, pipecmd.RecordAudioCompleted{
		RunID: p.RunID, StepIndex: p.StepIndex,
		ObjectKey: p.ObjectKey, Bucket: p.Bucket, Bytes: p.Bytes, Cost: p.Cost, DurationMs: p.DurationMs,
	})
	return err
}

func (c *RawCompletionConsumer) handleMusic(ctx context.Context, body []byte) error {
	var p pipeline.MusicCompletedPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	_, err := bus.Dispatch[pipecmd.RecordStepResult](ctx, c.reg, pipecmd.RecordMusicCompleted{
		RunID: p.RunID, StepIndex: p.StepIndex,
		ObjectKey: p.ObjectKey, Bucket: p.Bucket, Cost: p.Cost, DurationMs: p.DurationMs,
	})
	return err
}

func (c *RawCompletionConsumer) handleAssemble(ctx context.Context, body []byte) error {
	var p pipeline.AssembleCompletedPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	_, err := bus.Dispatch[pipecmd.RecordStepResult](ctx, c.reg, pipecmd.RecordAssembleCompleted{
		RunID: p.RunID, StepIndex: p.StepIndex,
		ObjectKey: p.ObjectKey, Bucket: p.Bucket, Bytes: p.Bytes, Cost: p.Cost, DurationMs: p.DurationMs,
	})
	return err
}

func (c *RawCompletionConsumer) handleUpload(ctx context.Context, body []byte) error {
	var p pipeline.UploadCompletedPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	if _, err := bus.Dispatch[pipecmd.RecordStepResult](ctx, c.reg, pipecmd.RecordUploadCompleted{
		RunID: p.RunID, StepIndex: p.StepIndex,
		ExternalRef: p.ExternalRef, Cost: p.Cost, DurationMs: p.DurationMs,
	}); err != nil {
		return err
	}
	// Best-effort: also mark the matching UploadRecord row uploaded so the UI
	// sees the youtu.be URL + final visibility without polling YT itself.
	_, _ = bus.Dispatch[pipecmd.MarkUploadRecordCompletedResult](ctx, c.reg, pipecmd.MarkUploadRecordCompleted{
		RunID: p.RunID, ExternalRef: p.ExternalRef, ExternalID: p.ExternalID,
		FinalVisibility: p.FinalVisibility,
		ScreenshotTrail: p.ScreenshotTrail,
	})
	return nil
}

func (c *RawCompletionConsumer) handleUploadFailed(ctx context.Context, body []byte) error {
	var p pipeline.UploadFailedPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	// Advance the run step first.
	if _, err := bus.Dispatch[pipecmd.RecordStepResult](ctx, c.reg, pipecmd.RecordStepFailed{
		RunID: p.RunID, StepIndex: p.StepIndex, Error: p.Error,
	}); err != nil {
		return err
	}
	// Then patch the UploadRecord with the screenshot + error.
	_, _ = bus.Dispatch[pipecmd.MarkUploadRecordFailedResult](ctx, c.reg, pipecmd.MarkUploadRecordFailed{
		RunID: p.RunID, Error: p.Error, ErrorScreenshotAssetID: p.ErrorScreenshotAssetID,
		ScreenshotTrail: p.ScreenshotTrail,
	})
	return nil
}

// handleMetricsCompleted is the inverse of the Python metrics worker. The
// worker publishes views/likes/comments/shares for a given upload_record_id;
// we persist a snapshot row + denormalised last-known counters.
func (c *RawCompletionConsumer) handleMetricsCompleted(ctx context.Context, body []byte) error {
	var p struct {
		UploadRecordID string         `json:"upload_record_id"`
		Views          int64          `json:"views"`
		Likes          int64          `json:"likes"`
		Comments       int64          `json:"comments"`
		Shares         int64          `json:"shares"`
		Raw            map[string]any `json:"raw"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	if p.UploadRecordID == "" {
		return nil
	}
	_, err := bus.Dispatch[umcmd.RecordMetricsSnapshotResult](ctx, c.reg, umcmd.RecordMetricsSnapshot{
		UploadRecordID: p.UploadRecordID,
		Views:          p.Views,
		Likes:          p.Likes,
		Comments:       p.Comments,
		Shares:         p.Shares,
		Raw:            p.Raw,
	})
	return err
}

func (c *RawCompletionConsumer) handleMetricsFailed(ctx context.Context, body []byte) error {
	var p struct {
		UploadRecordID string `json:"upload_record_id"`
		Error          string `json:"error"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	if p.UploadRecordID == "" {
		return nil
	}
	_, err := bus.Dispatch[umcmd.RecordMetricsFailureResult](ctx, c.reg, umcmd.RecordMetricsFailure{
		UploadRecordID: p.UploadRecordID, Error: p.Error,
	})
	return err
}

func (c *RawCompletionConsumer) handleStepFailed(ctx context.Context, body []byte) error {
	var p pipeline.StepFailedPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	_, err := bus.Dispatch[pipecmd.RecordStepResult](ctx, c.reg, pipecmd.RecordStepFailed{
		RunID: p.RunID, StepIndex: p.StepIndex, Error: p.Error,
	})
	return err
}

func contains(s, sub string) bool {
	if len(s) < len(sub) {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
