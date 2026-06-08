package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/example/dddcqrs/internal/app/bus"
	pipeq "github.com/example/dddcqrs/internal/app/query/pipeline"
)

// MountUploadMetadata registers POST /api/runs/{id}/generate-metadata, which
// uses the LLM to produce viral, SEO-tuned YouTube metadata (title, description
// with hashtags, tags) from the run's premise + panel captions. The schedule
// modal calls it to prefill editable fields before an upload.
func (s *Server) MountUploadMetadata(r chi.Router) {
	r.Post("/api/runs/{id}/generate-metadata", func(w http.ResponseWriter, req *http.Request) {
		if s.llm == nil || !s.llm.Enabled() {
			writeErr(w, http.StatusServiceUnavailable, "metadata generation disabled: set OPENROUTER_API_KEY")
			return
		}
		runID := chi.URLParam(req, "id")
		var body struct {
			Platform string `json:"platform"`
			Language string `json:"language"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		if body.Platform == "" {
			body.Platform = "youtube"
		}
		if body.Language == "" {
			body.Language = "en"
		}

		run, err := bus.Ask[pipeq.RunView](req.Context(), s.reg, pipeq.GetRun{RunID: runID})
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		premise, beats := extractRunContent(run)

		meta, err := generateUploadMetadata(req.Context(), s.llm, body.Platform, body.Language, run.Prompt, premise, beats)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, meta)
	})
}

// panelCaption is the per-panel shape emitted by the script step.
type panelCaption struct {
	Index   int    `json:"index"`
	Prompt  string `json:"prompt"`
	Caption string `json:"caption"`
}

// extractRunContent pulls the run premise (prompt) and the ordered panel
// captions from the script step output, to ground the LLM in the actual video.
func extractRunContent(run pipeq.RunView) (premise string, beats []string) {
	premise = strings.TrimSpace(run.Prompt)
	for _, st := range run.Steps {
		if st.Type != "script" || len(st.Outputs) == 0 {
			continue
		}
		var panels []panelCaption
		if err := json.Unmarshal(st.Outputs, &panels); err != nil {
			continue
		}
		for _, p := range panels {
			c := strings.TrimSpace(p.Caption)
			if c == "" {
				c = strings.TrimSpace(p.Prompt)
			}
			if c != "" {
				beats = append(beats, c)
			}
		}
		break
	}
	return premise, beats
}

// UploadMetadata is the generated, editable payload returned to the UI.
type UploadMetadata struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Hashtags    []string `json:"hashtags"`
}

func generateUploadMetadata(ctx context.Context, llm captionLLM, platform, language, prompt, premise string, beats []string) (*UploadMetadata, error) {
	langName := map[string]string{"en": "English", "ru": "Russian", "fr": "French"}[language]
	if langName == "" {
		langName = "English"
	}
	system := fmt.Sprintf(`You are a viral short-form video growth strategist for the %s platform.
Write metadata that maximizes click-through, watch time, and discovery for a YouTube Short.
Respond ONLY with a JSON object of this exact shape:
{"title": string, "description": string, "tags": [string], "hashtags": [string]}

Rules:
- Language: %s. Write the title and description in this language.
- title: <= 90 chars, scroll-stopping hook, natural curiosity, 1-2 fitting emoji, NO clickbait lies.
- description: 2-4 short lines — an opening hook line, one line of context, a soft call-to-action (like/subscribe), then a blank line, then the hashtags joined by spaces.
- tags: 12-18 lowercase SEO keywords/phrases relevant to the content (no # symbol).
- hashtags: 5-8 relevant tags WITHOUT the # symbol (e.g. "shorts", "funnydogs"); always include "shorts". For YouTube keep them tight and on-topic.
- Be specific to the actual video content below, not generic.`, platform, langName)

	var b strings.Builder
	fmt.Fprintf(&b, "Video premise: %s\n", premise)
	if prompt != "" && prompt != premise {
		fmt.Fprintf(&b, "Original prompt: %s\n", prompt)
	}
	if len(beats) > 0 {
		b.WriteString("Scene-by-scene captions:\n")
		for i, beat := range beats {
			fmt.Fprintf(&b, "%d. %s\n", i+1, beat)
		}
	}
	b.WriteString("\nGenerate the JSON metadata now.")

	raw, err := llm.CompleteJSON(ctx, system, b.String(), 0.8)
	if err != nil {
		return nil, err
	}
	var out UploadMetadata
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		// Some models wrap JSON in prose or code fences — salvage the object.
		if i, j := strings.Index(raw, "{"), strings.LastIndex(raw, "}"); i >= 0 && j > i {
			if err2 := json.Unmarshal([]byte(raw[i:j+1]), &out); err2 != nil {
				return nil, fmt.Errorf("bad LLM JSON: %w", err)
			}
		} else {
			return nil, fmt.Errorf("bad LLM JSON: %w", err)
		}
	}
	// Normalise hashtags: strip leading '#', drop blanks.
	clean := out.Hashtags[:0]
	for _, h := range out.Hashtags {
		h = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(h), "#"))
		if h != "" {
			clean = append(clean, h)
		}
	}
	out.Hashtags = clean
	return &out, nil
}
