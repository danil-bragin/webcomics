package pipeline

// panelIndicesSet reads cfg.Params["panel_indices"] = [0,2] and returns a set.
// Used by partial image regen — only the listed panels are re-emitted; the
// rest are seeded from the prior active attempt.
func panelIndicesSet(params map[string]any) map[int]bool {
	out := map[int]bool{}
	raw, ok := params["panel_indices"]
	if !ok {
		return out
	}
	arr, ok := raw.([]any)
	if !ok {
		return out
	}
	for _, v := range arr {
		switch n := v.(type) {
		case float64:
			out[int(n)] = true
		case int:
			out[n] = true
		}
	}
	return out
}

// panelOverrides reads cfg.Params["panel_overrides"] = {"0":{...},"1":{...}}.
// Each value may carry model / prompt_append / refs (string[]).
func panelOverrides(params map[string]any) map[int]map[string]any {
	out := map[int]map[string]any{}
	raw, ok := params["panel_overrides"]
	if !ok {
		return out
	}
	switch m := raw.(type) {
	case map[string]any:
		for k, v := range m {
			if cfg, ok := v.(map[string]any); ok {
				out[atoiSafePipeline(k)] = cfg
			}
		}
	case []any:
		for _, e := range m {
			if cfg, ok := e.(map[string]any); ok {
				if idx, ok := cfg["index"].(float64); ok {
					out[int(idx)] = cfg
				}
			}
		}
	}
	return out
}

// applyPanelOverride returns the model and prompt for one panel after applying
// any per-panel override (model swap, prompt_append).
func applyPanelOverride(defaultModel, prompt string, ov map[string]any) (string, string) {
	model := defaultModel
	out := prompt
	if ov == nil {
		return model, out
	}
	if m, ok := ov["model"].(string); ok && m != "" {
		model = m
	}
	if pa, ok := ov["prompt_append"].(string); ok && pa != "" {
		out = out + " " + pa
	}
	return model, out
}

func atoiSafePipeline(s string) int {
	n := 0
	neg := false
	for i, c := range s {
		if i == 0 && c == '-' {
			neg = true
			continue
		}
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		return -n
	}
	return n
}
