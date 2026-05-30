// Consumer handlers for the pipeline bounded context.
//
// Two roles:
//  1. *.completed / *.failed messages from workers → bus.Dispatch to advance
//     the run aggregate (the permanent path).
//  2. *.requested messages → an in-process *echo* handler that publishes the
//     matching completed event immediately. This is a scaffolding stand-in
//     for the real Python/Node workers (Phases 2–4) and is gated behind
//     ECHO_PIPELINE=1 so it disappears as real workers come online.
package consumer

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ThreeDotsLabs/watermill/message"

	"github.com/example/dddcqrs/internal/app/bus"
	pipecmd "github.com/example/dddcqrs/internal/app/command/pipeline"
	"github.com/example/dddcqrs/internal/domain/pipeline"
)

// echoEnabled returns the set of step types for which the echo handler should
// run. ECHO_PIPELINE=all  → script,image,assemble.
// ECHO_PIPELINE=image,assemble → only those two.
// (Used during phase transitions: real Python script worker comes online →
//
//	set ECHO_PIPELINE=image,assemble; real image worker → ECHO_PIPELINE=assemble.)
func echoEnabled() map[string]bool {
	raw := os.Getenv("ECHO_PIPELINE")
	if raw == "" {
		return nil
	}
	out := map[string]bool{}
	if raw == "all" {
		out["script"] = true
		out["image"] = true
		out["assemble"] = true
		return out
	}
	for _, tok := range splitCSV(raw) {
		out[tok] = true
	}
	return out
}

func splitCSV(s string) []string {
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// RegisterPipeline wires both echo handlers (if enabled) and real completion
// handlers onto the router.
func (c *Consumer) RegisterPipeline(router *message.Router, sub message.Subscriber, pub message.Publisher) {
	echo := echoEnabled()

	if echo["script"] {
		router.AddNoPublisherHandler("echo_script", "pipeline.script.requested", sub, c.echoScript(pub))
	}
	if echo["image"] {
		router.AddNoPublisherHandler("echo_image", "pipeline.image.requested", sub, c.echoImage(pub))
	}
	if echo["assemble"] {
		router.AddNoPublisherHandler("echo_assemble", "pipeline.assemble.requested", sub, c.echoAssemble(pub))
	}
	if echo["audio"] {
		router.AddNoPublisherHandler("echo_audio", "pipeline.audio.requested", sub, c.echoAudio(pub))
	}
	if echo["music"] {
		router.AddNoPublisherHandler("echo_music", "pipeline.music.requested", sub, c.echoMusic(pub))
	}
	if echo["upload"] {
		router.AddNoPublisherHandler("echo_upload", "pipeline.upload.requested", sub, c.echoUpload(pub))
	}

	// Completion + failure handlers moved to RawCompletionConsumer (raw
	// go-redis loop) — Watermill's redis-stream subscriber spawns multiple
	// internal consumers that ACK messages without invoking the handler.
}

// --- echo handlers (temporary) ---

func (c *Consumer) echoScript(pub message.Publisher) func(*message.Message) error {
	return func(msg *message.Message) error {
		var in pipeline.ScriptRequested
		if err := json.Unmarshal(msg.Payload, &in); err != nil {
			return fmt.Errorf("echo script unmarshal: %w", err)
		}
		// Generate 3 fake panels.
		panels := []pipeline.PanelDef{
			{Index: 0, Prompt: in.Prompt + " — panel 1"},
			{Index: 1, Prompt: in.Prompt + " — panel 2"},
			{Index: 2, Prompt: in.Prompt + " — panel 3"},
		}
		hint := 0
		if in.Params != nil {
			if v, ok := in.Params["panel_count"].(float64); ok {
				hint = int(v)
			}
			if v, ok := in.Params["panel_count"].(int); ok {
				hint = v
			}
		}
		if hint > 0 {
			panels = panels[:0]
			for i := 0; i < hint; i++ {
				panels = append(panels, pipeline.PanelDef{Index: i, Prompt: fmt.Sprintf("%s — panel %d", in.Prompt, i+1)})
			}
		}
		payload, _ := json.Marshal(pipeline.ScriptCompletedPayload{
			RunID:     in.RunID,
			StepIndex: in.StepIndex,
			ScriptKey: fmt.Sprintf("runs/%s/%d/script.json", in.RunID, in.StepIndex),
			Panels:    panels,
			Cost: pipeline.CostInfo{
				Provider: "echo", Model: "echo-llm",
				Units: 100, UnitLabel: "tokens", UnitCostUSD: 0.00001, TotalCostUSD: 0.001,
			},
			DurationMs: 50,
		})
		return pub.Publish("pipeline.script.completed", message.NewMessage(msg.UUID+"-c", payload))
	}
}

func (c *Consumer) echoImage(pub message.Publisher) func(*message.Message) error {
	return func(msg *message.Message) error {
		var in pipeline.ImageRequested
		if err := json.Unmarshal(msg.Payload, &in); err != nil {
			return fmt.Errorf("echo image unmarshal: %w", err)
		}
		payload, _ := json.Marshal(pipeline.ImageCompletedPayload{
			RunID:      in.RunID,
			StepIndex:  in.StepIndex,
			PanelIndex: in.PanelIndex,
			ObjectKey:  in.OutputKey,
			Cost: pipeline.CostInfo{
				Provider: "echo", Model: "echo-flux",
				Units: 1, UnitLabel: "images", UnitCostUSD: 0.003, TotalCostUSD: 0.003,
			},
			DurationMs: 100,
		})
		return pub.Publish("pipeline.image.completed", message.NewMessage(msg.UUID+"-c", payload))
	}
}

func (c *Consumer) echoAudio(pub message.Publisher) func(*message.Message) error {
	return func(msg *message.Message) error {
		var in pipeline.AudioRequested
		if err := json.Unmarshal(msg.Payload, &in); err != nil {
			return fmt.Errorf("echo audio unmarshal: %w", err)
		}
		payload, _ := json.Marshal(pipeline.AudioCompletedPayload{
			RunID:     in.RunID,
			StepIndex: in.StepIndex,
			ObjectKey: in.OutputKey,
			Cost: pipeline.CostInfo{
				Provider: "echo", Model: "echo-tts",
				Units: 30, UnitLabel: "seconds", UnitCostUSD: 0.0001, TotalCostUSD: 0.003,
			},
			DurationMs: 150,
		})
		return pub.Publish("pipeline.audio.completed", message.NewMessage(msg.UUID+"-c", payload))
	}
}

func (c *Consumer) echoMusic(pub message.Publisher) func(*message.Message) error {
	return func(msg *message.Message) error {
		var in pipeline.MusicRequested
		if err := json.Unmarshal(msg.Payload, &in); err != nil {
			return fmt.Errorf("echo music unmarshal: %w", err)
		}
		payload, _ := json.Marshal(pipeline.MusicCompletedPayload{
			RunID:     in.RunID,
			StepIndex: in.StepIndex,
			ObjectKey: in.OutputKey,
			Cost: pipeline.CostInfo{
				Provider: "echo", Model: "echo-music",
				Units: 30, UnitLabel: "seconds", UnitCostUSD: 0, TotalCostUSD: 0,
			},
			DurationMs: 200,
		})
		return pub.Publish("pipeline.music.completed", message.NewMessage(msg.UUID+"-c", payload))
	}
}

func (c *Consumer) echoUpload(pub message.Publisher) func(*message.Message) error {
	return func(msg *message.Message) error {
		var in pipeline.UploadRequested
		if err := json.Unmarshal(msg.Payload, &in); err != nil {
			return fmt.Errorf("echo upload unmarshal: %w", err)
		}
		payload, _ := json.Marshal(pipeline.UploadCompletedPayload{
			RunID:       in.RunID,
			StepIndex:   in.StepIndex,
			ExternalRef: "echo://" + in.Provider + "/" + in.RunID,
			Cost: pipeline.CostInfo{
				Provider: "echo-" + in.Provider, Model: "echo",
				Units: 1, UnitLabel: "uploads", UnitCostUSD: 0, TotalCostUSD: 0,
			},
			DurationMs: 100,
		})
		return pub.Publish("pipeline.upload.completed", message.NewMessage(msg.UUID+"-c", payload))
	}
}

func (c *Consumer) echoAssemble(pub message.Publisher) func(*message.Message) error {
	return func(msg *message.Message) error {
		var in pipeline.AssembleRequested
		if err := json.Unmarshal(msg.Payload, &in); err != nil {
			return fmt.Errorf("echo assemble unmarshal: %w", err)
		}
		payload, _ := json.Marshal(pipeline.AssembleCompletedPayload{
			RunID:     in.RunID,
			StepIndex: in.StepIndex,
			ObjectKey: in.OutputKey,
			Cost: pipeline.CostInfo{
				Provider: "local", Model: "echo-ffmpeg",
				Units: 5, UnitLabel: "seconds", UnitCostUSD: 0, TotalCostUSD: 0,
			},
			DurationMs: 200,
		})
		return pub.Publish("pipeline.assemble.completed", message.NewMessage(msg.UUID+"-c", payload))
	}
}

// --- real completion handlers (permanent) ---

func (c *Consumer) onScriptCompleted(msg *message.Message) error {
	var p pipeline.ScriptCompletedPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return err
	}
	_, err := bus.Dispatch[pipecmd.RecordStepResult](msg.Context(), c.reg, pipecmd.RecordScriptCompleted{
		RunID: p.RunID, StepIndex: p.StepIndex,
		ScriptKey: p.ScriptKey, Bucket: p.Bucket, Bytes: p.Bytes, Panels: p.Panels,
		Cost: p.Cost, DurationMs: p.DurationMs,
	})
	return err
}

func (c *Consumer) onImageCompleted(msg *message.Message) error {
	var p pipeline.ImageCompletedPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return err
	}
	_, err := bus.Dispatch[pipecmd.RecordStepResult](msg.Context(), c.reg, pipecmd.RecordImageCompleted{
		RunID: p.RunID, StepIndex: p.StepIndex,
		PanelIndex: p.PanelIndex, ObjectKey: p.ObjectKey, Bucket: p.Bucket,
		Bytes: p.Bytes,
		Cost:  p.Cost, DurationMs: p.DurationMs,
	})
	return err
}

func (c *Consumer) onAudioCompleted(msg *message.Message) error {
	var p pipeline.AudioCompletedPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return err
	}
	_, err := bus.Dispatch[pipecmd.RecordStepResult](msg.Context(), c.reg, pipecmd.RecordAudioCompleted{
		RunID: p.RunID, StepIndex: p.StepIndex,
		ObjectKey: p.ObjectKey, Bucket: p.Bucket, Bytes: p.Bytes,
		Cost: p.Cost, DurationMs: p.DurationMs,
	})
	return err
}

func (c *Consumer) onMusicCompleted(msg *message.Message) error {
	var p pipeline.MusicCompletedPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return err
	}
	_, err := bus.Dispatch[pipecmd.RecordStepResult](msg.Context(), c.reg, pipecmd.RecordMusicCompleted{
		RunID: p.RunID, StepIndex: p.StepIndex,
		ObjectKey: p.ObjectKey, Bucket: p.Bucket,
		Cost: p.Cost, DurationMs: p.DurationMs,
	})
	return err
}

func (c *Consumer) onUploadCompleted(msg *message.Message) error {
	var p pipeline.UploadCompletedPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return err
	}
	_, err := bus.Dispatch[pipecmd.RecordStepResult](msg.Context(), c.reg, pipecmd.RecordUploadCompleted{
		RunID: p.RunID, StepIndex: p.StepIndex,
		ExternalRef: p.ExternalRef,
		Cost:        p.Cost, DurationMs: p.DurationMs,
	})
	return err
}

func (c *Consumer) onAssembleCompleted(msg *message.Message) error {
	var p pipeline.AssembleCompletedPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return err
	}
	_, err := bus.Dispatch[pipecmd.RecordStepResult](msg.Context(), c.reg, pipecmd.RecordAssembleCompleted{
		RunID: p.RunID, StepIndex: p.StepIndex,
		ObjectKey: p.ObjectKey, Bucket: p.Bucket, Bytes: p.Bytes,
		Cost: p.Cost, DurationMs: p.DurationMs,
	})
	return err
}

func (c *Consumer) onStepFailed(msg *message.Message) error {
	var p pipeline.StepFailedPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return err
	}
	_, err := bus.Dispatch[pipecmd.RecordStepResult](msg.Context(), c.reg, pipecmd.RecordStepFailed{
		RunID: p.RunID, StepIndex: p.StepIndex, Error: p.Error,
	})
	return err
}
