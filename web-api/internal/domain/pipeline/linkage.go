package pipeline

// CharacterContext is the resolved data for one character attached to a run.
// Resolved before NewRunWithOptions / regenerate so the aggregate can emit
// fully-formed events without touching any repo.
type CharacterContext struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Traits        map[string]any `json:"traits,omitempty"`
	RefObjectKeys []string       `json:"ref_object_keys,omitempty"`
}

// EnvironmentContext mirrors CharacterContext for environments (settings).
type EnvironmentContext struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Traits        map[string]any `json:"traits,omitempty"`
	RefObjectKeys []string       `json:"ref_object_keys,omitempty"`
}

// PlotBeatContext is one beat in the story arc.
type PlotBeatContext struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Order       int    `json:"order"`
}

// PlotContext bundles the project's story-arc data for one run.
type PlotContext struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Premise string            `json:"premise"`
	Beats   []PlotBeatContext `json:"beats,omitempty"`
}

// LinkedContext bundles the project-scoped data attached to a run. All fields
// optional — runs without a project leave them empty.
type LinkedContext struct {
	ProjectID    string
	Characters   []CharacterContext
	Environments []EnvironmentContext
	Plot         *PlotContext
}

// linkedRefKeys flattens the character + environment ref object keys in a
// stable order: characters first, environments second.
func (r *Run) linkedRefKeys() []string {
	if r.linked == nil {
		return nil
	}
	var out []string
	for _, c := range r.linked.Characters {
		out = append(out, c.RefObjectKeys...)
	}
	for _, e := range r.linked.Environments {
		out = append(out, e.RefObjectKeys...)
	}
	return out
}

// composePromptWithLinks prepends compact character/environment cues to a
// per-panel image prompt. We don't dump full descriptions here — that runs in
// the LLM-side script worker. This adds short visual anchors that survive even
// when the model only weakly attends to ref images.
func composePromptWithLinks(prompt string, ctx *LinkedContext) string {
	if ctx == nil || (len(ctx.Characters) == 0 && len(ctx.Environments) == 0) {
		return prompt
	}
	parts := []string{}
	for _, c := range ctx.Characters {
		if c.Description != "" {
			parts = append(parts, c.Name+": "+c.Description)
		} else if c.Name != "" {
			parts = append(parts, c.Name)
		}
	}
	for _, e := range ctx.Environments {
		if e.Description != "" {
			parts = append(parts, "Setting "+e.Name+": "+e.Description)
		} else if e.Name != "" {
			parts = append(parts, "Setting "+e.Name)
		}
	}
	if len(parts) == 0 {
		return prompt
	}
	return prompt + ". " + joinSentences(parts)
}

func joinSentences(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ". "
		}
		out += p
	}
	return out
}

// applyFormatPromptCues wraps a panel prompt with format-driven prefix/suffix.
// Pulled from cfg.Params["image_prompt_prefix"|"image_prompt_suffix"] which
// applyOverrides populates from the selected format.
func applyFormatPromptCues(prompt string, params map[string]any) string {
	if params == nil {
		return prompt
	}
	if v, ok := params["image_prompt_prefix"].(string); ok && v != "" {
		prompt = v + prompt
	}
	if v, ok := params["image_prompt_suffix"].(string); ok && v != "" {
		prompt = prompt + v
	}
	return prompt
}
