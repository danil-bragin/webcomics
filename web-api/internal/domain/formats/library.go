// Package formats defines preset visual+composition recipes (manga, webtoon,
// noir, …) that wrap a bundle of prompt cues + recommended image model +
// aspect/fps/transition defaults. Selected at project or run level.
package formats

// Format is a recipe applied before project defaults + user overrides.
// All fields optional except ID, Name. Empty fields don't override anything.
type Format struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	Scope              string `json:"scope"` // "system" | "user"
	ScriptSystemSuffix string `json:"script_system_suffix,omitempty"`
	ImagePromptPrefix  string `json:"image_prompt_prefix,omitempty"`
	ImagePromptSuffix  string `json:"image_prompt_suffix,omitempty"`
	ImageModel         string `json:"image_model,omitempty"`
	StyleReference     string `json:"style_reference,omitempty"` // "none"|"anchor"|"previous"
	FPS                int    `json:"fps,omitempty"`
	Width              int    `json:"width,omitempty"`
	Height             int    `json:"height,omitempty"`
	Codec              string `json:"codec,omitempty"`
	PanelDurationMs    int    `json:"panel_duration_ms,omitempty"`
	Transition         string `json:"transition,omitempty"`
}

// System returns the built-in catalog. Library is intentionally hard-coded:
// changing a preset means a code change, which we want for reproducibility.
func System() []Format {
	return []Format{
		{
			ID: "slideshow", Name: "Slideshow", Scope: "system",
			Description: "Plain image-to-image transitions. No specific art-style cues. Closest to the original default.",
			FPS:         30, Width: 1080, Height: 1080, Codec: "h264",
			PanelDurationMs: 2500, Transition: "crossfade",
		},
		{
			ID: "manga", Name: "Manga", Scope: "system",
			Description:        "Black ink, halftone screentones, dramatic eyes, sound effects. Vertical portrait.",
			ScriptSystemSuffix: "Use snappy dialogue and Japanese-style sound effects (BANG! WHOOSH!). Describe each panel in a dramatic black-and-white manga aesthetic with motion lines.",
			ImagePromptPrefix:  "manga style, black ink linework, halftone screentone shading, dramatic motion lines, expressive eyes, ",
			ImagePromptSuffix:  ", traditional manga panel composition",
			ImageModel:         "fal-ai/flux-2/edit", StyleReference: "anchor",
			FPS: 24, Width: 1080, Height: 1920, Codec: "h264",
			PanelDurationMs: 3500, Transition: "wipe",
		},
		{
			ID: "webtoon", Name: "Webtoon / Manhwa", Scope: "system",
			Description:        "Full-color soft digital painting. Vertical scroll feel, expressive faces, frequent close-ups.",
			ScriptSystemSuffix: "Compose each panel as if seen scrolling vertically. Favor close-ups and emotional beats. Use modern conversational dialogue.",
			ImagePromptPrefix:  "webtoon manhwa style, soft digital painting, smooth shading, vibrant colors, expressive faces, ",
			ImagePromptSuffix:  ", clean vertical composition",
			ImageModel:         "fal-ai/flux-2/edit", StyleReference: "anchor",
			FPS: 30, Width: 1080, Height: 1920, Codec: "h264",
			PanelDurationMs: 4000, Transition: "fade",
		},
		{
			ID: "american_superhero", Name: "American Superhero", Scope: "system",
			Description:        "Bold ink, primary colors, dynamic action poses, halftone shading. Bronze-age comic feel.",
			ScriptSystemSuffix: "Write punchy action-comic dialogue. Use bold exclamations and dynamic poses. Captions in third-person omniscient narration.",
			ImagePromptPrefix:  "American comic book art style, bold ink outlines, primary colors red blue yellow, halftone shading, dynamic action pose, ",
			ImagePromptSuffix:  ", dramatic comic panel composition",
			ImageModel:         "fal-ai/flux-2/edit", StyleReference: "anchor",
			FPS: 30, Width: 1080, Height: 1080, Codec: "h264",
			PanelDurationMs: 3000, Transition: "slide",
		},
		{
			ID: "ligne_claire", Name: "Ligne Claire (Franco-Belgian)", Scope: "system",
			Description:        "Clean uniform line weights, flat colors, no shading, detailed backgrounds. Tintin / Asterix lineage.",
			ScriptSystemSuffix: "Write witty Franco-Belgian comic dialogue with detailed scenery descriptions in each panel.",
			ImagePromptPrefix:  "ligne claire style, clean uniform black ink lines, flat colors no shading, detailed background, Hergé Tintin style, ",
			ImagePromptSuffix:  ", classic European comic composition",
			ImageModel:         "fal-ai/flux-2/edit", StyleReference: "anchor",
			FPS: 30, Width: 1440, Height: 1080, Codec: "h264",
			PanelDurationMs: 3000, Transition: "crossfade",
		},
		{
			ID: "indie_alt", Name: "Indie / Alternative", Scope: "system",
			Description:        "Scratchy hand-drawn linework, muted earthtones, mundane subjects. Daniel Clowes / R. Crumb feel.",
			ScriptSystemSuffix: "Write deadpan, character-driven dialogue. Quiet observational moments. Avoid action — favor introspection.",
			ImagePromptPrefix:  "indie alternative comic art, scratchy hand-drawn linework, muted earthtone palette, awkward composition, ",
			ImagePromptSuffix:  ", quiet observational scene",
			ImageModel:         "fal-ai/flux/schnell", StyleReference: "none",
			FPS: 30, Width: 1080, Height: 1080, Codec: "h264",
			PanelDurationMs: 3500, Transition: "fade",
		},
		{
			ID: "graphic_novel", Name: "Graphic Novel (Painted)", Scope: "system",
			Description:        "Full painted illustration, cinematic lighting, complex composition. Premium look.",
			ScriptSystemSuffix: "Write each panel like a cinematic still. Detailed visual setup, atmospheric lighting cues. Sparse dialogue.",
			ImagePromptPrefix:  "graphic novel painted illustration, cinematic lighting, rich color palette, detailed atmospheric composition, ",
			ImagePromptSuffix:  ", premium graphic novel page composition",
			ImageModel:         "fal-ai/nano-banana-pro/edit", StyleReference: "anchor",
			FPS: 30, Width: 1920, Height: 1080, Codec: "h264",
			PanelDurationMs: 4500, Transition: "crossfade",
		},
		{
			ID: "newspaper_strip", Name: "Newspaper Strip", Scope: "system",
			Description:        "Cartoony, dialogue-heavy 3-4 panel horizontal strip. Calvin & Hobbes / Garfield vibe.",
			ScriptSystemSuffix: "Write a 3-4 panel setup-punchline strip. Snappy dialogue. End on a visual gag.",
			ImagePromptPrefix:  "newspaper comic strip cartoon style, simple cartoony lines, flat colors, expressive characters, ",
			ImagePromptSuffix:  ", clean strip panel composition",
			ImageModel:         "fal-ai/flux/schnell", StyleReference: "anchor",
			FPS: 30, Width: 1920, Height: 1080, Codec: "h264",
			PanelDurationMs: 3000, Transition: "slide",
		},
		{
			ID: "noir", Name: "Noir", Scope: "system",
			Description:        "High contrast black and white, harsh shadows. Lighter moderation profile.",
			ScriptSystemSuffix: "Write light noir narration in first person. Short clipped sentences. Atmospheric, not violent.",
			ImagePromptPrefix:  "noir comic illustration, high contrast black and white, soft chiaroscuro shadows, expressive faces, cozy mystery vibe, ",
			ImagePromptSuffix:  ", clean panel composition, family-friendly",
			ImageModel:         "fal-ai/flux/schnell", StyleReference: "anchor",
			FPS: 24, Width: 1080, Height: 1920, Codec: "h264",
			PanelDurationMs: 4000, Transition: "fade",
		},
		{
			ID: "watercolor", Name: "Watercolor Storybook", Scope: "system",
			Description:        "Soft watercolor washes, hand-lettered captions, gentle children's book feel.",
			ScriptSystemSuffix: "Write gentle storybook narration. Wonder-filled descriptions. Lyrical captions.",
			ImagePromptPrefix:  "soft watercolor illustration, gentle wash, hand-drawn lines, children's storybook style, ",
			ImagePromptSuffix:  ", whimsical composition",
			ImageModel:         "fal-ai/flux/schnell", StyleReference: "anchor",
			FPS: 30, Width: 1080, Height: 1080, Codec: "h264",
			PanelDurationMs: 4000, Transition: "fade",
		},
		{
			ID: "pixel_retro", Name: "Pixel / Retro Game", Scope: "system",
			Description:        "Chunky pixel art, limited palette, 16-bit JRPG feel.",
			ScriptSystemSuffix: "Write 16-bit JRPG cutscene dialogue. Short text-box style lines. Use stage directions like [BATTLE START].",
			ImagePromptPrefix:  "pixel art style, chunky pixels, limited 16-bit color palette, retro video game aesthetic, ",
			ImagePromptSuffix:  ", classic JRPG composition",
			ImageModel:         "fal-ai/flux/schnell", StyleReference: "anchor",
			FPS: 30, Width: 1080, Height: 1080, Codec: "h264",
			PanelDurationMs: 3000, Transition: "wipe",
		},
		{
			ID: "cinematic_3d", Name: "Cinematic 3D", Scope: "system",
			Description:        "Rendered CGI with soft global illumination. Pixar-adjacent premium look.",
			ScriptSystemSuffix: "Write cinematic scene descriptions with explicit camera direction. Wide → medium → close-up arcs.",
			ImagePromptPrefix:  "cinematic 3D rendered illustration, soft global illumination, Pixar-quality, detailed character expression, ",
			ImagePromptSuffix:  ", cinematic widescreen composition",
			ImageModel:         "fal-ai/nano-banana-pro/edit", StyleReference: "anchor",
			FPS: 30, Width: 1920, Height: 1080, Codec: "h264",
			PanelDurationMs: 4000, Transition: "crossfade",
		},
	}
}

// ByID looks up a system format. Returns nil if not found (caller should also
// check the user-format DB before giving up).
func ByID(id string) *Format {
	for _, f := range System() {
		if f.ID == id {
			fc := f
			return &fc
		}
	}
	return nil
}
