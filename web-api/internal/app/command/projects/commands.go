// Package projects holds write-side command handlers for the projects context.
package projects

import (
	"context"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/domain/projects"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

// --- Project CRUD ---

type CreateProject struct {
	Name        string
	Description string
	Defaults    map[string]any
}

func (CreateProject) IsCommand() {}

type CreateProjectResult struct{ ID string }

type CreateProjectHandler struct{ uow uow.Manager }

func NewCreateProjectHandler(m uow.Manager) *CreateProjectHandler {
	return &CreateProjectHandler{uow: m}
}

func (h *CreateProjectHandler) Handle(ctx context.Context, cmd CreateProject) (CreateProjectResult, error) {
	var out CreateProjectResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		p, err := projects.NewProject(cmd.Name, cmd.Description)
		if err != nil {
			return err
		}
		if cmd.Defaults != nil {
			p.SetDefaults(cmd.Defaults)
		}
		if err := u.Repositories().Projects().SaveProject(ctx, p); err != nil {
			return err
		}
		out.ID = p.ID().String()
		return nil
	})
	return out, err
}

type UpdateProject struct {
	ID          string
	Name        string
	Description string
	Defaults    map[string]any
	Archived    *bool
}

func (UpdateProject) IsCommand() {}

type UpdateProjectResult struct{}

type UpdateProjectHandler struct{ uow uow.Manager }

func NewUpdateProjectHandler(m uow.Manager) *UpdateProjectHandler {
	return &UpdateProjectHandler{uow: m}
}

func (h *UpdateProjectHandler) Handle(ctx context.Context, cmd UpdateProject) (UpdateProjectResult, error) {
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		p, err := u.Repositories().Projects().GetProject(ctx, projects.ProjectID(cmd.ID))
		if err != nil {
			return err
		}
		if cmd.Name != "" || cmd.Description != p.Description() {
			if err := p.Update(orDefault(cmd.Name, p.Name()), cmd.Description); err != nil {
				return err
			}
		}
		if cmd.Defaults != nil {
			p.SetDefaults(cmd.Defaults)
		}
		if cmd.Archived != nil {
			if *cmd.Archived {
				p.Archive()
			} else {
				p.Unarchive()
			}
		}
		return u.Repositories().Projects().SaveProject(ctx, p)
	})
	return UpdateProjectResult{}, err
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

type DeleteProject struct{ ID string }

func (DeleteProject) IsCommand() {}

type DeleteProjectResult struct{}

type DeleteProjectHandler struct{ uow uow.Manager }

func NewDeleteProjectHandler(m uow.Manager) *DeleteProjectHandler {
	return &DeleteProjectHandler{uow: m}
}

func (h *DeleteProjectHandler) Handle(ctx context.Context, cmd DeleteProject) (DeleteProjectResult, error) {
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		return u.Repositories().Projects().DeleteProject(ctx, projects.ProjectID(cmd.ID))
	})
	return DeleteProjectResult{}, err
}

// --- Character CRUD ---

type UpsertCharacter struct {
	ID          string // empty = create
	ProjectID   string
	Name        string
	Description string
	Traits      map[string]any
	RefAssetIDs []string
}

func (UpsertCharacter) IsCommand() {}

type UpsertCharacterResult struct{ ID string }

type UpsertCharacterHandler struct{ uow uow.Manager }

func NewUpsertCharacterHandler(m uow.Manager) *UpsertCharacterHandler {
	return &UpsertCharacterHandler{uow: m}
}

func (h *UpsertCharacterHandler) Handle(ctx context.Context, cmd UpsertCharacter) (UpsertCharacterResult, error) {
	var out UpsertCharacterResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Projects()
		var c *projects.Character
		if cmd.ID == "" {
			created, err := projects.NewCharacter(projects.ProjectID(cmd.ProjectID), cmd.Name, cmd.Description, cmd.Traits)
			if err != nil {
				return err
			}
			c = created
		} else {
			loaded, err := repo.GetCharacter(ctx, projects.CharacterID(cmd.ID))
			if err != nil {
				return err
			}
			loaded.Update(cmd.Name, cmd.Description, cmd.Traits)
			c = loaded
		}
		if cmd.RefAssetIDs != nil {
			c.SetRefAssetIDs(cmd.RefAssetIDs)
		}
		if err := repo.SaveCharacter(ctx, c); err != nil {
			return err
		}
		out.ID = c.ID().String()
		return nil
	})
	return out, err
}

type DeleteCharacter struct{ ID string }

func (DeleteCharacter) IsCommand() {}

type DeleteCharacterResult struct{}

type DeleteCharacterHandler struct{ uow uow.Manager }

func NewDeleteCharacterHandler(m uow.Manager) *DeleteCharacterHandler {
	return &DeleteCharacterHandler{uow: m}
}

func (h *DeleteCharacterHandler) Handle(ctx context.Context, cmd DeleteCharacter) (DeleteCharacterResult, error) {
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		return u.Repositories().Projects().DeleteCharacter(ctx, projects.CharacterID(cmd.ID))
	})
	return DeleteCharacterResult{}, err
}

// --- Environment CRUD ---

type UpsertEnvironment struct {
	ID          string
	ProjectID   string
	Name        string
	Description string
	Traits      map[string]any
	RefAssetIDs []string
}

func (UpsertEnvironment) IsCommand() {}

type UpsertEnvironmentResult struct{ ID string }

type UpsertEnvironmentHandler struct{ uow uow.Manager }

func NewUpsertEnvironmentHandler(m uow.Manager) *UpsertEnvironmentHandler {
	return &UpsertEnvironmentHandler{uow: m}
}

func (h *UpsertEnvironmentHandler) Handle(ctx context.Context, cmd UpsertEnvironment) (UpsertEnvironmentResult, error) {
	var out UpsertEnvironmentResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Projects()
		var e *projects.Environment
		if cmd.ID == "" {
			created, err := projects.NewEnvironment(projects.ProjectID(cmd.ProjectID), cmd.Name, cmd.Description, cmd.Traits)
			if err != nil {
				return err
			}
			e = created
		} else {
			loaded, err := repo.GetEnvironment(ctx, projects.EnvironmentID(cmd.ID))
			if err != nil {
				return err
			}
			loaded.Update(cmd.Name, cmd.Description, cmd.Traits)
			e = loaded
		}
		if cmd.RefAssetIDs != nil {
			e.SetRefAssetIDs(cmd.RefAssetIDs)
		}
		if err := repo.SaveEnvironment(ctx, e); err != nil {
			return err
		}
		out.ID = e.ID().String()
		return nil
	})
	return out, err
}

type DeleteEnvironment struct{ ID string }

func (DeleteEnvironment) IsCommand() {}

type DeleteEnvironmentResult struct{}

type DeleteEnvironmentHandler struct{ uow uow.Manager }

func NewDeleteEnvironmentHandler(m uow.Manager) *DeleteEnvironmentHandler {
	return &DeleteEnvironmentHandler{uow: m}
}

func (h *DeleteEnvironmentHandler) Handle(ctx context.Context, cmd DeleteEnvironment) (DeleteEnvironmentResult, error) {
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		return u.Repositories().Projects().DeleteEnvironment(ctx, projects.EnvironmentID(cmd.ID))
	})
	return DeleteEnvironmentResult{}, err
}

// --- Plot upsert ---

type UpsertPlot struct {
	ProjectID string
	Name      string
	Premise   string
	Beats     []projects.PlotBeat
}

func (UpsertPlot) IsCommand() {}

type UpsertPlotResult struct{ ID string }

type UpsertPlotHandler struct{ uow uow.Manager }

func NewUpsertPlotHandler(m uow.Manager) *UpsertPlotHandler { return &UpsertPlotHandler{uow: m} }

func (h *UpsertPlotHandler) Handle(ctx context.Context, cmd UpsertPlot) (UpsertPlotResult, error) {
	var out UpsertPlotResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Projects()
		existing, err := repo.GetPlotByProject(ctx, projects.ProjectID(cmd.ProjectID))
		if err != nil && err != projects.ErrPlotNotFound {
			return err
		}
		var p *projects.Plot
		if existing != nil {
			existing.Update(cmd.Name, cmd.Premise, cmd.Beats)
			p = existing
		} else {
			p = projects.NewPlot(projects.ProjectID(cmd.ProjectID), cmd.Name, cmd.Premise, cmd.Beats)
		}
		if err := repo.SavePlot(ctx, p); err != nil {
			return err
		}
		out.ID = p.ID().String()
		return nil
	})
	return out, err
}

// --- Bus registration ---

func CreateProjectOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[CreateProject, CreateProjectResult](r, NewCreateProjectHandler(m))
}
func UpdateProjectOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[UpdateProject, UpdateProjectResult](r, NewUpdateProjectHandler(m))
}
func DeleteProjectOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[DeleteProject, DeleteProjectResult](r, NewDeleteProjectHandler(m))
}
func UpsertCharacterOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[UpsertCharacter, UpsertCharacterResult](r, NewUpsertCharacterHandler(m))
}
func DeleteCharacterOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[DeleteCharacter, DeleteCharacterResult](r, NewDeleteCharacterHandler(m))
}
func UpsertEnvironmentOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[UpsertEnvironment, UpsertEnvironmentResult](r, NewUpsertEnvironmentHandler(m))
}
func DeleteEnvironmentOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[DeleteEnvironment, DeleteEnvironmentResult](r, NewDeleteEnvironmentHandler(m))
}
func UpsertPlotOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[UpsertPlot, UpsertPlotResult](r, NewUpsertPlotHandler(m))
}

// --- SocialAccount CRUD (global) ---

// UpsertSocialAccount creates or updates a global account row. Linking it
// to projects is a separate concern (LinkSocialAccount). When cmd.ProjectID
// is non-empty *and* this is a new account, the handler also links the
// freshly-minted account to that project so the legacy Firefox flow keeps
// "create + link in one step" UX.
type UpsertSocialAccount struct {
	ID                 string
	ProjectID          string // optional — auto-link target on create
	AsDefault          bool   // if ProjectID set + creating, mark this account default
	Platform           string
	Label              string
	FirefoxProfilePath string
	Extra              map[string]any
}

func (UpsertSocialAccount) IsCommand() {}

type UpsertSocialAccountResult struct{ ID string }

type UpsertSocialAccountHandler struct{ uow uow.Manager }

func NewUpsertSocialAccountHandler(m uow.Manager) *UpsertSocialAccountHandler {
	return &UpsertSocialAccountHandler{uow: m}
}

func (h *UpsertSocialAccountHandler) Handle(ctx context.Context, cmd UpsertSocialAccount) (UpsertSocialAccountResult, error) {
	var out UpsertSocialAccountResult
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Projects()
		var a *projects.SocialAccount
		isNew := cmd.ID == ""
		if isNew {
			created, err := projects.NewSocialAccount(cmd.Platform, cmd.Label, cmd.FirefoxProfilePath, cmd.Extra)
			if err != nil {
				return err
			}
			a = created
		} else {
			loaded, err := repo.GetSocialAccount(ctx, projects.SocialAccountID(cmd.ID))
			if err != nil {
				return err
			}
			loaded.Update(cmd.Platform, cmd.Label, cmd.FirefoxProfilePath, cmd.Extra)
			a = loaded
		}
		if err := repo.SaveSocialAccount(ctx, a); err != nil {
			return err
		}
		out.ID = a.ID().String()
		if isNew && cmd.ProjectID != "" {
			if err := repo.LinkSocialAccount(ctx, projects.ProjectID(cmd.ProjectID), a.ID(), cmd.AsDefault); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

type DeleteSocialAccount struct{ ID string }

func (DeleteSocialAccount) IsCommand() {}

type DeleteSocialAccountResult struct{}

type DeleteSocialAccountHandler struct{ uow uow.Manager }

func NewDeleteSocialAccountHandler(m uow.Manager) *DeleteSocialAccountHandler {
	return &DeleteSocialAccountHandler{uow: m}
}

func (h *DeleteSocialAccountHandler) Handle(ctx context.Context, cmd DeleteSocialAccount) (DeleteSocialAccountResult, error) {
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		return u.Repositories().Projects().DeleteSocialAccount(ctx, projects.SocialAccountID(cmd.ID))
	})
	return DeleteSocialAccountResult{}, err
}

// --- Project ↔ SocialAccount link CRUD ---

type LinkSocialAccount struct {
	ProjectID       string
	SocialAccountID string
	AsDefault       bool
}

func (LinkSocialAccount) IsCommand() {}

type LinkSocialAccountResult struct{}

type LinkSocialAccountHandler struct{ uow uow.Manager }

func NewLinkSocialAccountHandler(m uow.Manager) *LinkSocialAccountHandler {
	return &LinkSocialAccountHandler{uow: m}
}

func (h *LinkSocialAccountHandler) Handle(ctx context.Context, cmd LinkSocialAccount) (LinkSocialAccountResult, error) {
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		return u.Repositories().Projects().LinkSocialAccount(ctx,
			projects.ProjectID(cmd.ProjectID),
			projects.SocialAccountID(cmd.SocialAccountID),
			cmd.AsDefault)
	})
	return LinkSocialAccountResult{}, err
}

type UnlinkSocialAccount struct {
	ProjectID       string
	SocialAccountID string
}

func (UnlinkSocialAccount) IsCommand() {}

type UnlinkSocialAccountResult struct{}

type UnlinkSocialAccountHandler struct{ uow uow.Manager }

func NewUnlinkSocialAccountHandler(m uow.Manager) *UnlinkSocialAccountHandler {
	return &UnlinkSocialAccountHandler{uow: m}
}

func (h *UnlinkSocialAccountHandler) Handle(ctx context.Context, cmd UnlinkSocialAccount) (UnlinkSocialAccountResult, error) {
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		return u.Repositories().Projects().UnlinkSocialAccount(ctx,
			projects.ProjectID(cmd.ProjectID),
			projects.SocialAccountID(cmd.SocialAccountID))
	})
	return UnlinkSocialAccountResult{}, err
}

type SetDefaultSocialAccount struct {
	ProjectID       string
	SocialAccountID string
}

func (SetDefaultSocialAccount) IsCommand() {}

type SetDefaultSocialAccountResult struct{}

type SetDefaultSocialAccountHandler struct{ uow uow.Manager }

func NewSetDefaultSocialAccountHandler(m uow.Manager) *SetDefaultSocialAccountHandler {
	return &SetDefaultSocialAccountHandler{uow: m}
}

func (h *SetDefaultSocialAccountHandler) Handle(ctx context.Context, cmd SetDefaultSocialAccount) (SetDefaultSocialAccountResult, error) {
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		return u.Repositories().Projects().SetDefaultSocialAccount(ctx,
			projects.ProjectID(cmd.ProjectID),
			projects.SocialAccountID(cmd.SocialAccountID))
	})
	return SetDefaultSocialAccountResult{}, err
}

// SetSocialAccountLimits updates rate-limit fields on a social account. Use
// -1 for DailyUploadLimit / MinGapSeconds to leave them unchanged.
type SetSocialAccountLimits struct {
	ID               string
	DailyUploadLimit int
	LimitWindowHours int
	IsVerified       bool
	MinGapSeconds    int
}

func (SetSocialAccountLimits) IsCommand() {}

type SetSocialAccountLimitsResult struct{}

type SetSocialAccountLimitsHandler struct{ uow uow.Manager }

func NewSetSocialAccountLimitsHandler(m uow.Manager) *SetSocialAccountLimitsHandler {
	return &SetSocialAccountLimitsHandler{uow: m}
}

func (h *SetSocialAccountLimitsHandler) Handle(ctx context.Context, cmd SetSocialAccountLimits) (SetSocialAccountLimitsResult, error) {
	err := h.uow.WithinTx(ctx, func(ctx context.Context, u uow.UnitOfWork) error {
		repo := u.Repositories().Projects()
		acct, err := repo.GetSocialAccount(ctx, projects.SocialAccountID(cmd.ID))
		if err != nil {
			return err
		}
		acct.SetRateLimit(cmd.DailyUploadLimit, cmd.LimitWindowHours, cmd.MinGapSeconds, cmd.IsVerified)
		return repo.SaveSocialAccount(ctx, acct)
	})
	return SetSocialAccountLimitsResult{}, err
}

func SetSocialAccountLimitsOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[SetSocialAccountLimits, SetSocialAccountLimitsResult](r, NewSetSocialAccountLimitsHandler(m))
}

func UpsertSocialAccountOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[UpsertSocialAccount, UpsertSocialAccountResult](r, NewUpsertSocialAccountHandler(m))
}
func DeleteSocialAccountOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[DeleteSocialAccount, DeleteSocialAccountResult](r, NewDeleteSocialAccountHandler(m))
}
func LinkSocialAccountOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[LinkSocialAccount, LinkSocialAccountResult](r, NewLinkSocialAccountHandler(m))
}
func UnlinkSocialAccountOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[UnlinkSocialAccount, UnlinkSocialAccountResult](r, NewUnlinkSocialAccountHandler(m))
}
func SetDefaultSocialAccountOnBus(r *bus.Registry, m uow.Manager) {
	bus.RegisterCommand[SetDefaultSocialAccount, SetDefaultSocialAccountResult](r, NewSetDefaultSocialAccountHandler(m))
}
