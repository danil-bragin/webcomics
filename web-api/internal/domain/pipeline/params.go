package pipeline

import (
	"encoding/json"
	"maps"
)

// assembleDims extracts width/height/fps from assemble step params with sane
// 1080×1080 / 30fps defaults.
func assembleDims(params map[string]any) (int, int, int) {
	width, height, fps := 1080, 1080, 30
	if v, ok := params["width"].(float64); ok && v > 0 {
		width = int(v)
	}
	if v, ok := params["height"].(float64); ok && v > 0 {
		height = int(v)
	}
	if v, ok := params["fps"].(float64); ok && v > 0 {
		fps = int(v)
	}
	return width, height, fps
}

// assembleDefaults reads panel_duration_ms + transition from assemble step
// params, defaulting to 2.5s crossfade.
func assembleDefaults(params map[string]any) (int, string) {
	dur := 2500
	transition := "crossfade"
	if v, ok := params["panel_duration_ms"].(float64); ok && v > 0 {
		dur = int(v)
	}
	if v, ok := params["transition"].(string); ok && v != "" {
		transition = v
	}
	return dur, transition
}

// assemblePanelDurationMs walks the run's config snapshot and returns the
// panel_duration_ms from the assemble step. Used by the audio step to align
// voiceover timing with each panel's on-screen duration.
func assemblePanelDurationMs(steps []StepConfig) int {
	for _, s := range steps {
		if s.Type != StepAssemble {
			continue
		}
		dur, _ := assembleDefaults(s.Params)
		return dur
	}
	return 2500
}

// anchorModel returns the model to use for panel 0 in ref modes. Falls back
// to flux/schnell when the image step config doesn't override it.
func anchorModel(cfg StepConfig) string {
	if cfg.Params != nil {
		if v, ok := cfg.Params["anchor_model"].(string); ok && v != "" {
			return v
		}
	}
	return "fal-ai/flux/schnell"
}

// styleRefMode normalises image step's style_reference param.
func styleRefMode(params map[string]any) string {
	if params == nil {
		return "none"
	}
	if v, ok := params["style_reference"].(string); ok {
		switch v {
		case "anchor", "previous":
			return v
		}
	}
	return "none"
}

func cloneParams(p map[string]any) map[string]any {
	if p == nil {
		return nil
	}
	out := make(map[string]any, len(p))
	maps.Copy(out, p)
	return out
}

func marshalParams(p map[string]any) json.RawMessage {
	if len(p) == 0 {
		return nil
	}
	b, _ := json.Marshal(p)
	return b
}

// stringsFromParam reads a []string out of an interface{}-stored array.
// Handles both []any{string,...} and []string forms — useful for params
// fetched from the JSON config snapshot.
func stringsFromParam(params map[string]any, key string) []string {
	if params == nil {
		return nil
	}
	raw, ok := params[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
