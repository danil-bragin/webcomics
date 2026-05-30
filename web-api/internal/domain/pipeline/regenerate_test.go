package pipeline

import (
	"encoding/json"
	"testing"
)

// ----- helpers -----

func newRunAt(t *testing.T, panels int, autoAssemble bool) *Run {
	t.Helper()
	tpl := newTestTemplate(t, panels)
	run, err := NewRunWithOptions("test", tpl, RunOptions{AutoAssemble: autoAssemble})
	if err != nil {
		t.Fatalf("NewRunWithOptions: %v", err)
	}
	if err := run.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return run
}

func completeScript(t *testing.T, run *Run, panels []PanelDef) {
	t.Helper()
	if err := run.RecordScriptCompleted(0, "runs/x/0/script.json", panels, CostInfo{TotalCostUSD: 0.001}, 100); err != nil {
		t.Fatalf("RecordScriptCompleted: %v", err)
	}
}

func completeImagePanels(t *testing.T, run *Run, stepIdx, count int) {
	t.Helper()
	for i := range count {
		key := jsonFmt("runs/x/%d/panel-%d.png", stepIdx, i)
		if err := run.RecordImageCompleted(stepIdx, i, key, CostInfo{TotalCostUSD: 0.003}, 50); err != nil {
			t.Fatalf("RecordImageCompleted(%d, %d): %v", stepIdx, i, err)
		}
	}
}

func jsonFmt(format string, args ...any) string {
	b, _ := json.Marshal(format)
	_ = b
	// Just sprintf — kept tiny helper so import path matches existing style.
	return formatString(format, args...)
}

func formatString(format string, args ...any) string {
	out := []byte{}
	ai := 0
	for i := 0; i < len(format); i++ {
		if format[i] == '%' && i+1 < len(format) && format[i+1] == 'd' {
			out = append(out, []byte(itoa(args[ai].(int)))...)
			ai++
			i++
			continue
		}
		out = append(out, format[i])
	}
	return string(out)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func panelsOf(n int) []PanelDef {
	out := make([]PanelDef, n)
	for i := range n {
		out[i] = PanelDef{Index: i, Prompt: "p" + itoa(i)}
	}
	return out
}

// ----- AutoAssemble gate -----

func TestRun_AutoAssembleFalsePausesBeforeAssemble(t *testing.T) {
	run := newRunAt(t, 2, false)
	completeScript(t, run, panelsOf(2))
	completeImagePanels(t, run, 1, 2)
	if run.Status() != RunStatusAwaitingAction {
		t.Errorf("expected awaiting_action, got %s", run.Status())
	}
}

func TestRun_AutoAssembleTrueProceedsToAssemble(t *testing.T) {
	run := newRunAt(t, 2, true)
	completeScript(t, run, panelsOf(2))
	completeImagePanels(t, run, 1, 2)
	if run.Status() != RunStatusRunning {
		t.Errorf("expected running (assemble queued), got %s", run.Status())
	}
	// Verify assemble step has fresh attempt waiting.
	asm := run.Steps()[2]
	if asm.ActiveAttempt() == nil {
		t.Error("assemble step has no attempt after auto-cascade")
	}
}

func TestRun_RequestAssembleUnblocksAwaitingAction(t *testing.T) {
	run := newRunAt(t, 2, false)
	completeScript(t, run, panelsOf(2))
	completeImagePanels(t, run, 1, 2)
	if err := run.RequestAssemble(map[string]any{"transition": "wipe"}); err != nil {
		t.Fatalf("RequestAssemble: %v", err)
	}
	if run.Status() != RunStatusRunning {
		t.Errorf("expected running after RequestAssemble, got %s", run.Status())
	}
	asm := run.Steps()[2]
	att := asm.ActiveAttempt()
	if att == nil {
		t.Fatal("assemble missing attempt")
	}
	var inp map[string]any
	if err := json.Unmarshal(att.input, &inp); err != nil {
		t.Fatalf("attempt input not JSON: %v", err)
	}
	// transition override should reach the assemble step's params block.
	params, _ := inp["params"].(map[string]any)
	if params["transition"] != "wipe" {
		t.Errorf("transition override missing: %v", params)
	}
}

// ----- RegenerateStep manual cascade gate -----

func TestRun_RegenerateScript_BumpsVersionAndMarksDownstreamStale(t *testing.T) {
	run := newRunAt(t, 2, true)
	completeScript(t, run, panelsOf(2))
	completeImagePanels(t, run, 1, 2)
	// Run has cascaded into assemble; that's fine. Now regen script.
	if err := run.RegenerateStep(0, map[string]any{"system_prompt": "different"}); err != nil {
		t.Fatalf("RegenerateStep: %v", err)
	}
	script := run.Steps()[0]
	if script.CurrentVersion() != 2 {
		t.Errorf("script version: got %d, want 2", script.CurrentVersion())
	}
	if len(script.Attempts()) != 2 {
		t.Errorf("script attempts: got %d, want 2", len(script.Attempts()))
	}
	for i := 1; i < len(run.Steps()); i++ {
		if !run.Steps()[i].IsStale() {
			t.Errorf("step %d should be stale after script regen", i)
		}
	}
}

func TestRun_RegeneratedStepCompletion_ParksRunInAwaitingAction(t *testing.T) {
	run := newRunAt(t, 2, true)
	completeScript(t, run, panelsOf(2))
	completeImagePanels(t, run, 1, 2)
	// Drain assemble too so cascade settles.
	_ = run.RecordAssembleCompleted(2, "runs/x/2/video.mp4", CostInfo{TotalCostUSD: 0}, 100)
	if run.Status() != RunStatusCompleted {
		t.Fatalf("setup: expected completed, got %s", run.Status())
	}
	// Regenerate the script step.
	if err := run.RegenerateStep(0, nil); err != nil {
		t.Fatalf("RegenerateStep: %v", err)
	}
	// Complete the new script attempt.
	if err := run.RecordScriptCompleted(0, "runs/x/0/script2.json", panelsOf(2), CostInfo{TotalCostUSD: 0.001}, 100); err != nil {
		t.Fatalf("RecordScriptCompleted v2: %v", err)
	}
	// Per manual-cascade policy: downstream is stale + run should pause.
	if run.Status() != RunStatusAwaitingAction {
		t.Errorf("expected awaiting_action after script regen completion, got %s", run.Status())
	}
	if !run.Steps()[1].IsStale() {
		t.Errorf("image step should remain stale until user re-regenerates")
	}
}

func TestRun_RegenerateStep_RejectsCancelled(t *testing.T) {
	run := newRunAt(t, 1, true)
	_ = run.Cancel()
	if err := run.RegenerateStep(0, nil); err == nil {
		t.Errorf("expected error regenerating cancelled run")
	}
}

func TestRun_RegenerateStep_IndexOutOfRange(t *testing.T) {
	run := newRunAt(t, 1, true)
	if err := run.RegenerateStep(99, nil); err == nil {
		t.Errorf("expected ErrStepIndexMismatch")
	}
}

// ----- Partial image regen (panel_indices) -----

func TestRun_PartialImageRegen_SeedsPriorOutputsAndEmitsSubset(t *testing.T) {
	run := newRunAt(t, 3, true)
	completeScript(t, run, panelsOf(3))
	completeImagePanels(t, run, 1, 3)
	// Capture prior keys.
	priorAttempt := run.Steps()[1].ActiveAttempt()
	var priorOut []map[string]any
	_ = json.Unmarshal(priorAttempt.outputs, &priorOut)

	// Regen only panel 1 (middle).
	if err := run.RegenerateStep(1, map[string]any{"panel_indices": []any{float64(1)}}); err != nil {
		t.Fatalf("RegenerateStep partial: %v", err)
	}
	img := run.Steps()[1]
	att := img.ActiveAttempt()
	if att == nil {
		t.Fatal("new attempt missing")
	}
	if att.panelsExpected != 3 {
		t.Errorf("panelsExpected: got %d want 3", att.panelsExpected)
	}
	if att.panelsCompleted != 2 {
		t.Errorf("seeded panels: got %d want 2 (the unchanged ones)", att.panelsCompleted)
	}
	// Verify panel 1 slot is empty + panels 0/2 carry prior keys.
	var arr []map[string]any
	_ = json.Unmarshal(att.outputs, &arr)
	if k, _ := arr[0]["object_key"].(string); k != priorOut[0]["object_key"].(string) {
		t.Errorf("panel 0 not seeded from prior")
	}
	if k, _ := arr[1]["object_key"].(string); k != "" {
		t.Errorf("panel 1 should be empty for regen, got %q", k)
	}
	if k, _ := arr[2]["object_key"].(string); k != priorOut[2]["object_key"].(string) {
		t.Errorf("panel 2 not seeded from prior")
	}
}
