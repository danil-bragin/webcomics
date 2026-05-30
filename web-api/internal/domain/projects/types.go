// Package projects holds the Project aggregate plus its child entities
// (Character, Environment, Plot). Pure domain — no SQL, no HTTP. The repo
// port is defined here; the pgx implementation lives in
// infrastructure/persistence/write/projects_repository.go.
package projects

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrProjectNameEmpty    = errors.New("projects: name empty")
	ErrProjectNotFound     = errors.New("projects: not found")
	ErrCharacterNotFound   = errors.New("projects: character not found")
	ErrEnvironmentNotFound = errors.New("projects: environment not found")
	ErrPlotNotFound        = errors.New("projects: plot not found")
)

type ProjectID string

func NewProjectID() ProjectID       { return ProjectID(uuid.NewString()) }
func (id ProjectID) String() string { return string(id) }

type CharacterID string

func NewCharacterID() CharacterID     { return CharacterID(uuid.NewString()) }
func (id CharacterID) String() string { return string(id) }

type EnvironmentID string

func NewEnvironmentID() EnvironmentID   { return EnvironmentID(uuid.NewString()) }
func (id EnvironmentID) String() string { return string(id) }

type PlotID string

func NewPlotID() PlotID          { return PlotID(uuid.NewString()) }
func (id PlotID) String() string { return string(id) }

// Project is a loose container for runs, characters, environments and a plot.
// `defaults` is a free-form map of override defaults — populated by the user
// in the project settings UI, hydrated into the Studio form when this project
// is picked. Schema is documented in migration 00008.
type Project struct {
	id          ProjectID
	name        string
	description string
	defaults    map[string]any
	archived    bool
	createdAt   time.Time
	updatedAt   time.Time
}

func NewProject(name, description string) (*Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrProjectNameEmpty
	}
	now := time.Now().UTC()
	return &Project{
		id: NewProjectID(), name: name, description: description,
		defaults:  map[string]any{},
		createdAt: now, updatedAt: now,
	}, nil
}

func (p *Project) ID() ProjectID            { return p.id }
func (p *Project) Name() string             { return p.name }
func (p *Project) Description() string      { return p.description }
func (p *Project) Defaults() map[string]any { return p.defaults }
func (p *Project) Archived() bool           { return p.archived }
func (p *Project) CreatedAt() time.Time     { return p.createdAt }
func (p *Project) UpdatedAt() time.Time     { return p.updatedAt }

func (p *Project) Update(name, description string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrProjectNameEmpty
	}
	p.name = name
	p.description = description
	p.updatedAt = time.Now().UTC()
	return nil
}

func (p *Project) SetDefaults(d map[string]any) {
	if d == nil {
		d = map[string]any{}
	}
	p.defaults = d
	p.updatedAt = time.Now().UTC()
}

func (p *Project) Archive()   { p.archived = true; p.updatedAt = time.Now().UTC() }
func (p *Project) Unarchive() { p.archived = false; p.updatedAt = time.Now().UTC() }

func ReconstituteProject(id ProjectID, name, description string, defaults map[string]any, archived bool, created, updated time.Time) *Project {
	if defaults == nil {
		defaults = map[string]any{}
	}
	return &Project{id: id, name: name, description: description, defaults: defaults, archived: archived, createdAt: created, updatedAt: updated}
}

// Character is a reusable character bundle: text description, structured traits
// (appearance, clothing, voice, …) and a list of reference image asset IDs that
// get fed into image generation as style anchors.
type Character struct {
	id          CharacterID
	projectID   ProjectID
	name        string
	description string
	traits      map[string]any
	refAssetIDs []string
	createdAt   time.Time
	updatedAt   time.Time
}

func NewCharacter(projectID ProjectID, name, description string, traits map[string]any) (*Character, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("projects: character name empty")
	}
	now := time.Now().UTC()
	if traits == nil {
		traits = map[string]any{}
	}
	return &Character{
		id: NewCharacterID(), projectID: projectID,
		name: name, description: description, traits: traits,
		createdAt: now, updatedAt: now,
	}, nil
}

func (c *Character) ID() CharacterID        { return c.id }
func (c *Character) ProjectID() ProjectID   { return c.projectID }
func (c *Character) Name() string           { return c.name }
func (c *Character) Description() string    { return c.description }
func (c *Character) Traits() map[string]any { return c.traits }
func (c *Character) RefAssetIDs() []string  { return c.refAssetIDs }
func (c *Character) CreatedAt() time.Time   { return c.createdAt }
func (c *Character) UpdatedAt() time.Time   { return c.updatedAt }

func (c *Character) Update(name, description string, traits map[string]any) {
	if name != "" {
		c.name = name
	}
	c.description = description
	if traits != nil {
		c.traits = traits
	}
	c.updatedAt = time.Now().UTC()
}

func (c *Character) SetRefAssetIDs(ids []string) {
	c.refAssetIDs = append([]string{}, ids...)
	c.updatedAt = time.Now().UTC()
}

func (c *Character) AddRefAsset(id string) {
	for _, existing := range c.refAssetIDs {
		if existing == id {
			return
		}
	}
	c.refAssetIDs = append(c.refAssetIDs, id)
	c.updatedAt = time.Now().UTC()
}

func (c *Character) RemoveRefAsset(id string) {
	out := c.refAssetIDs[:0]
	for _, existing := range c.refAssetIDs {
		if existing != id {
			out = append(out, existing)
		}
	}
	c.refAssetIDs = out
	c.updatedAt = time.Now().UTC()
}

func ReconstituteCharacter(id CharacterID, pid ProjectID, name, description string, traits map[string]any, refs []string, created, updated time.Time) *Character {
	if traits == nil {
		traits = map[string]any{}
	}
	return &Character{
		id: id, projectID: pid,
		name: name, description: description, traits: traits, refAssetIDs: refs,
		createdAt: created, updatedAt: updated,
	}
}

// Environment mirrors Character but for settings (time of day, weather, mood,
// architectural style, …).
type Environment struct {
	id          EnvironmentID
	projectID   ProjectID
	name        string
	description string
	traits      map[string]any
	refAssetIDs []string
	createdAt   time.Time
	updatedAt   time.Time
}

func NewEnvironment(projectID ProjectID, name, description string, traits map[string]any) (*Environment, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("projects: environment name empty")
	}
	if traits == nil {
		traits = map[string]any{}
	}
	now := time.Now().UTC()
	return &Environment{
		id: NewEnvironmentID(), projectID: projectID,
		name: name, description: description, traits: traits,
		createdAt: now, updatedAt: now,
	}, nil
}

func (e *Environment) ID() EnvironmentID      { return e.id }
func (e *Environment) ProjectID() ProjectID   { return e.projectID }
func (e *Environment) Name() string           { return e.name }
func (e *Environment) Description() string    { return e.description }
func (e *Environment) Traits() map[string]any { return e.traits }
func (e *Environment) RefAssetIDs() []string  { return e.refAssetIDs }
func (e *Environment) CreatedAt() time.Time   { return e.createdAt }
func (e *Environment) UpdatedAt() time.Time   { return e.updatedAt }

func (e *Environment) Update(name, description string, traits map[string]any) {
	if name != "" {
		e.name = name
	}
	e.description = description
	if traits != nil {
		e.traits = traits
	}
	e.updatedAt = time.Now().UTC()
}

func (e *Environment) SetRefAssetIDs(ids []string) {
	e.refAssetIDs = append([]string{}, ids...)
	e.updatedAt = time.Now().UTC()
}

func (e *Environment) AddRefAsset(id string) {
	for _, existing := range e.refAssetIDs {
		if existing == id {
			return
		}
	}
	e.refAssetIDs = append(e.refAssetIDs, id)
	e.updatedAt = time.Now().UTC()
}

func (e *Environment) RemoveRefAsset(id string) {
	out := e.refAssetIDs[:0]
	for _, existing := range e.refAssetIDs {
		if existing != id {
			out = append(out, existing)
		}
	}
	e.refAssetIDs = out
	e.updatedAt = time.Now().UTC()
}

func ReconstituteEnvironment(id EnvironmentID, pid ProjectID, name, description string, traits map[string]any, refs []string, created, updated time.Time) *Environment {
	if traits == nil {
		traits = map[string]any{}
	}
	return &Environment{
		id: id, projectID: pid,
		name: name, description: description, traits: traits, refAssetIDs: refs,
		createdAt: created, updatedAt: updated,
	}
}

// Plot is the project's overall story setup: premise (free text) and a list of
// key beats. One plot per project (the unique key enforces that).
type Plot struct {
	id        PlotID
	projectID ProjectID
	name      string
	premise   string
	beats     []PlotBeat
	createdAt time.Time
	updatedAt time.Time
}

type PlotBeat struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Order       int    `json:"order"`
}

func NewPlot(projectID ProjectID, name, premise string, beats []PlotBeat) *Plot {
	now := time.Now().UTC()
	if name == "" {
		name = "Main"
	}
	return &Plot{
		id: NewPlotID(), projectID: projectID,
		name: name, premise: premise, beats: beats,
		createdAt: now, updatedAt: now,
	}
}

func (p *Plot) ID() PlotID           { return p.id }
func (p *Plot) ProjectID() ProjectID { return p.projectID }
func (p *Plot) Name() string         { return p.name }
func (p *Plot) Premise() string      { return p.premise }
func (p *Plot) Beats() []PlotBeat    { return p.beats }
func (p *Plot) CreatedAt() time.Time { return p.createdAt }
func (p *Plot) UpdatedAt() time.Time { return p.updatedAt }

func (p *Plot) Update(name, premise string, beats []PlotBeat) {
	if name != "" {
		p.name = name
	}
	p.premise = premise
	p.beats = beats
	p.updatedAt = time.Now().UTC()
}

func ReconstitutePlot(id PlotID, pid ProjectID, name, premise string, beats []PlotBeat, created, updated time.Time) *Plot {
	return &Plot{
		id: id, projectID: pid,
		name: name, premise: premise, beats: beats,
		createdAt: created, updatedAt: updated,
	}
}
