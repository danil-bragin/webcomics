package formats

import (
	"strings"
	"testing"
)

func TestSystem_AllFormatsHaveRequiredFields(t *testing.T) {
	for _, f := range System() {
		if f.ID == "" {
			t.Errorf("format %q has empty ID", f.Name)
		}
		if f.Name == "" {
			t.Errorf("format %q has empty Name", f.ID)
		}
		if f.Scope != "system" {
			t.Errorf("format %s: expected scope=system, got %q", f.ID, f.Scope)
		}
		if f.Description == "" {
			t.Errorf("format %s: empty Description", f.ID)
		}
	}
}

func TestSystem_IDsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range System() {
		if seen[f.ID] {
			t.Errorf("duplicate format ID: %s", f.ID)
		}
		seen[f.ID] = true
	}
}

func TestByID_RoundTrip(t *testing.T) {
	for _, f := range System() {
		got := ByID(f.ID)
		if got == nil {
			t.Errorf("ByID(%q) returned nil", f.ID)
			continue
		}
		if got.ID != f.ID {
			t.Errorf("ByID(%q): got ID %q", f.ID, got.ID)
		}
		if got.Name != f.Name {
			t.Errorf("ByID(%q): got Name %q want %q", f.ID, got.Name, f.Name)
		}
	}
}

func TestByID_UnknownReturnsNil(t *testing.T) {
	if got := ByID("does-not-exist"); got != nil {
		t.Errorf("ByID for unknown id should be nil, got %+v", got)
	}
	if got := ByID(""); got != nil {
		t.Errorf("ByID(\"\") should be nil")
	}
}

func TestSystem_HasExpectedCoreFormats(t *testing.T) {
	expected := []string{
		"slideshow", "manga", "webtoon", "american_superhero",
		"ligne_claire", "indie_alt", "graphic_novel",
		"newspaper_strip", "noir", "watercolor", "pixel_retro", "cinematic_3d",
	}
	for _, id := range expected {
		if ByID(id) == nil {
			t.Errorf("missing core format %q", id)
		}
	}
}

func TestSystem_FormatsWithStylesHavePromptCues(t *testing.T) {
	// Everything except plain slideshow should carry at least one cue.
	for _, f := range System() {
		if f.ID == "slideshow" {
			continue
		}
		if f.ImagePromptPrefix == "" && f.ImagePromptSuffix == "" {
			t.Errorf("format %s: expected an image prompt cue", f.ID)
		}
	}
}

func TestSystem_StyleRefValid(t *testing.T) {
	for _, f := range System() {
		switch f.StyleReference {
		case "", "none", "anchor", "previous":
		default:
			t.Errorf("format %s: invalid StyleReference %q", f.ID, f.StyleReference)
		}
	}
}

func TestSystem_ResolutionAspectsSane(t *testing.T) {
	for _, f := range System() {
		if f.Width == 0 || f.Height == 0 {
			t.Errorf("format %s: zero W or H", f.ID)
			continue
		}
		if f.Width > 4096 || f.Height > 4096 {
			t.Errorf("format %s: dimensions too large %dx%d", f.ID, f.Width, f.Height)
		}
		// Width/Height ratio sanity (between 1:3 and 3:1).
		ratio := float64(f.Width) / float64(f.Height)
		if ratio < 0.3 || ratio > 3.5 {
			t.Errorf("format %s: weird aspect %dx%d", f.ID, f.Width, f.Height)
		}
	}
}

func TestSystem_TransitionAndFPSSane(t *testing.T) {
	validTransitions := map[string]bool{
		"":          true, // not set → renderer default
		"crossfade": true,
		"fade":      true,
		"slide":     true,
		"push":      true,
		"zoom":      true,
		"wipe":      true,
		"none":      true,
	}
	for _, f := range System() {
		if !validTransitions[f.Transition] {
			t.Errorf("format %s: unknown transition %q", f.ID, f.Transition)
		}
		if f.FPS < 0 || f.FPS > 120 {
			t.Errorf("format %s: bad FPS %d", f.ID, f.FPS)
		}
		if f.PanelDurationMs < 0 || f.PanelDurationMs > 30000 {
			t.Errorf("format %s: bad panel duration %d ms", f.ID, f.PanelDurationMs)
		}
	}
}

func TestSystem_PromptCuesDontStartOrEndWithBareCommas(t *testing.T) {
	// Prefix should end with ", " so concatenation with the panel prompt is
	// natural. Suffix should start with ", ".
	for _, f := range System() {
		if f.ImagePromptPrefix != "" && !strings.HasSuffix(f.ImagePromptPrefix, ", ") {
			t.Errorf("format %s: ImagePromptPrefix should end with ', ' — got %q", f.ID, lastChunk(f.ImagePromptPrefix, 20))
		}
		if f.ImagePromptSuffix != "" && !strings.HasPrefix(f.ImagePromptSuffix, ", ") {
			t.Errorf("format %s: ImagePromptSuffix should start with ', ' — got %q", f.ID, firstChunk(f.ImagePromptSuffix, 20))
		}
	}
}

func lastChunk(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func firstChunk(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
