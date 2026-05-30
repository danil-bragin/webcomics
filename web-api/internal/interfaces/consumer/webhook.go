package consumer

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"

	"github.com/example/dddcqrs/internal/app/bus"
	pipeq "github.com/example/dddcqrs/internal/app/query/pipeline"
	"github.com/example/dddcqrs/internal/domain/pipeline"
)

// WebhookSender POSTs a run summary to an external URL when a run reaches a
// terminal state. Subscribes to pipeline.run.completed / .failed / .cancelled.
type WebhookSender struct {
	url    string
	secret string
	client *http.Client
	log    *slog.Logger
}

func NewWebhookSender(url, secret string, log *slog.Logger) *WebhookSender {
	return &WebhookSender{
		url:    url,
		secret: secret,
		client: &http.Client{Timeout: 10 * time.Second},
		log:    log,
	}
}

func (w *WebhookSender) Register(router *message.Router, sub message.Subscriber, reg *bus.Registry) {
	if w.url == "" {
		return // disabled
	}
	router.AddNoPublisherHandler("webhook_run_completed", "pipeline.run.completed", sub, w.onRunCompleted(reg))
	router.AddNoPublisherHandler("webhook_run_failed", "pipeline.run.failed", sub, w.onRunFailed(reg))
	router.AddNoPublisherHandler("webhook_run_cancelled", "pipeline.run.cancelled", sub, w.onRunCancelled(reg))
}

func (w *WebhookSender) onRunCompleted(reg *bus.Registry) func(*message.Message) error {
	return w.handlerFor("run.completed", reg, func(payload []byte) (string, error) {
		var ev pipeline.RunCompleted
		if err := json.Unmarshal(payload, &ev); err != nil {
			return "", err
		}
		return ev.RunID, nil
	})
}

func (w *WebhookSender) onRunFailed(reg *bus.Registry) func(*message.Message) error {
	return w.handlerFor("run.failed", reg, func(payload []byte) (string, error) {
		var ev pipeline.RunFailed
		if err := json.Unmarshal(payload, &ev); err != nil {
			return "", err
		}
		return ev.RunID, nil
	})
}

func (w *WebhookSender) onRunCancelled(reg *bus.Registry) func(*message.Message) error {
	return w.handlerFor("run.cancelled", reg, func(payload []byte) (string, error) {
		var ev pipeline.RunCancelled
		if err := json.Unmarshal(payload, &ev); err != nil {
			return "", err
		}
		return ev.RunID, nil
	})
}

func (w *WebhookSender) handlerFor(eventName string, reg *bus.Registry, runIDFrom func([]byte) (string, error)) func(*message.Message) error {
	return func(msg *message.Message) error {
		runID, err := runIDFrom(msg.Payload)
		if err != nil {
			return err
		}
		view, err := bus.Ask[pipeq.RunView](msg.Context(), reg, pipeq.GetRun{RunID: runID})
		if err != nil {
			return err
		}
		body, _ := json.Marshal(map[string]any{
			"event":  eventName,
			"run_id": runID,
			"run":    view,
			"ts":     time.Now().UTC().Format(time.RFC3339Nano),
		})
		req, err := http.NewRequestWithContext(msg.Context(), http.MethodPost, w.url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if w.secret != "" {
			mac := hmac.New(sha256.New, []byte(w.secret))
			mac.Write(body)
			req.Header.Set("X-Webhook-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		}
		resp, err := w.client.Do(req)
		if err != nil {
			w.log.Error("webhook post", "event", eventName, "err", err)
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("webhook non-2xx: %d", resp.StatusCode)
		}
		w.log.Info("webhook ok", "event", eventName, "run_id", runID)
		return nil
	}
}
