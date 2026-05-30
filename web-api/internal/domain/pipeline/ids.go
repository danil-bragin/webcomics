// Package pipeline is the Pipeline aggregate (write model). Pure domain:
// invariants, behavior, domain events. No SQL, no SDK, no transport.
package pipeline

import "github.com/google/uuid"

type RunID string

func NewRunID() RunID           { return RunID(uuid.NewString()) }
func (id RunID) String() string { return string(id) }

type StepID string

func NewStepID() StepID          { return StepID(uuid.NewString()) }
func (id StepID) String() string { return string(id) }

type TemplateID string

func NewTemplateID() TemplateID      { return TemplateID(uuid.NewString()) }
func (id TemplateID) String() string { return string(id) }

type AssetID string

func NewAssetID() AssetID         { return AssetID(uuid.NewString()) }
func (id AssetID) String() string { return string(id) }

type AttemptID string

func NewAttemptID() AttemptID       { return AttemptID(uuid.NewString()) }
func (id AttemptID) String() string { return string(id) }
