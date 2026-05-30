package projects

import (
	"testing"
	"time"
)

// ----- Project -----

func TestNewProject_RejectsEmptyName(t *testing.T) {
	if _, err := NewProject("", "desc"); err != ErrProjectNameEmpty {
		t.Errorf("expected ErrProjectNameEmpty, got %v", err)
	}
	if _, err := NewProject("   ", "desc"); err != ErrProjectNameEmpty {
		t.Errorf("expected ErrProjectNameEmpty for whitespace, got %v", err)
	}
}

func TestNewProject_TrimsName(t *testing.T) {
	p, err := NewProject("  Saga  ", "")
	if err != nil {
		t.Fatalf("NewProject: %v", err)
	}
	if p.Name() != "Saga" {
		t.Errorf("name not trimmed: %q", p.Name())
	}
}

func TestProject_UpdateAndArchive(t *testing.T) {
	p, _ := NewProject("Saga", "desc")
	prevUpd := p.UpdatedAt()
	// Sleep a touch so monotonic clock advances on fast machines.
	time.Sleep(time.Millisecond)
	if err := p.Update("Saga 2", "updated"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if p.Name() != "Saga 2" || p.Description() != "updated" {
		t.Errorf("update did not apply: %+v", p)
	}
	if !p.UpdatedAt().After(prevUpd) {
		t.Errorf("UpdatedAt not bumped (prev=%v now=%v)", prevUpd, p.UpdatedAt())
	}
	p.Archive()
	if !p.Archived() {
		t.Error("expected archived")
	}
	p.Unarchive()
	if p.Archived() {
		t.Error("expected unarchived")
	}
}

func TestProject_SetDefaults_NormalisesNil(t *testing.T) {
	p, _ := NewProject("Saga", "")
	p.SetDefaults(nil)
	if p.Defaults() == nil {
		t.Errorf("Defaults should be empty map, got nil")
	}
	p.SetDefaults(map[string]any{"foo": "bar"})
	if p.Defaults()["foo"] != "bar" {
		t.Errorf("defaults not set: %+v", p.Defaults())
	}
}

// ----- Character -----

func TestNewCharacter_RequiresName(t *testing.T) {
	pid := NewProjectID()
	if _, err := NewCharacter(pid, "", "desc", nil); err == nil {
		t.Errorf("expected error for empty character name")
	}
}

func TestCharacter_AddRefAssetDedup(t *testing.T) {
	pid := NewProjectID()
	c, _ := NewCharacter(pid, "Aria", "", nil)
	c.AddRefAsset("a1")
	c.AddRefAsset("a2")
	c.AddRefAsset("a1") // dup
	if len(c.RefAssetIDs()) != 2 {
		t.Errorf("dup not skipped: %+v", c.RefAssetIDs())
	}
}

func TestCharacter_RemoveRefAsset(t *testing.T) {
	pid := NewProjectID()
	c, _ := NewCharacter(pid, "Aria", "", nil)
	c.AddRefAsset("a1")
	c.AddRefAsset("a2")
	c.RemoveRefAsset("a1")
	got := c.RefAssetIDs()
	if len(got) != 1 || got[0] != "a2" {
		t.Errorf("remove failed: %+v", got)
	}
	// Removing missing is no-op.
	c.RemoveRefAsset("nope")
	if len(c.RefAssetIDs()) != 1 {
		t.Errorf("noop remove changed list")
	}
}

func TestCharacter_SetRefAssetIDsCopies(t *testing.T) {
	pid := NewProjectID()
	c, _ := NewCharacter(pid, "Aria", "", nil)
	ids := []string{"a", "b"}
	c.SetRefAssetIDs(ids)
	ids[0] = "MUTATED"
	got := c.RefAssetIDs()
	if got[0] != "a" {
		t.Errorf("SetRefAssetIDs aliased input: got %v", got)
	}
}

func TestCharacter_UpdateNilTraitsKeepsPrevious(t *testing.T) {
	pid := NewProjectID()
	c, _ := NewCharacter(pid, "Aria", "", map[string]any{"color": "red"})
	c.Update("Aria", "new desc", nil)
	if c.Traits()["color"] != "red" {
		t.Errorf("nil traits should preserve previous, got %v", c.Traits())
	}
}

// ----- Environment -----

func TestNewEnvironment_RequiresName(t *testing.T) {
	pid := NewProjectID()
	if _, err := NewEnvironment(pid, "", "desc", nil); err == nil {
		t.Errorf("expected error for empty environment name")
	}
}

func TestEnvironment_RefAssetOps(t *testing.T) {
	pid := NewProjectID()
	e, _ := NewEnvironment(pid, "Forest", "", nil)
	e.AddRefAsset("ref1")
	e.AddRefAsset("ref1") // dup
	if len(e.RefAssetIDs()) != 1 {
		t.Errorf("dup not skipped: %+v", e.RefAssetIDs())
	}
	e.RemoveRefAsset("ref1")
	if len(e.RefAssetIDs()) != 0 {
		t.Errorf("remove failed: %+v", e.RefAssetIDs())
	}
}

// ----- Plot -----

func TestNewPlot_DefaultsNameToMain(t *testing.T) {
	pid := NewProjectID()
	p := NewPlot(pid, "", "premise", nil)
	if p.Name() != "Main" {
		t.Errorf("expected default 'Main', got %q", p.Name())
	}
}

func TestPlot_UpdatePreservesEmptyName(t *testing.T) {
	pid := NewProjectID()
	p := NewPlot(pid, "Arc 1", "premise", []PlotBeat{{Name: "b1", Order: 0}})
	prevName := p.Name()
	p.Update("", "new premise", nil)
	if p.Name() != prevName {
		t.Errorf("empty name should preserve previous: got %q", p.Name())
	}
	if p.Premise() != "new premise" {
		t.Errorf("premise: %q", p.Premise())
	}
	if len(p.Beats()) != 0 {
		t.Errorf("Update(nil beats) should clear beats list, got %v", p.Beats())
	}
}

func TestReconstituteProject_DefaultsNilFillsEmpty(t *testing.T) {
	now := time.Now()
	p := ReconstituteProject(NewProjectID(), "S", "d", nil, false, now, now)
	if p.Defaults() == nil {
		t.Errorf("Defaults should be non-nil")
	}
}
