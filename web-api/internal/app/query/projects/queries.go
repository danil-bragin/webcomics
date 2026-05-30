package projects

import (
	"context"

	"github.com/example/dddcqrs/internal/app/bus"
)

// ReadModel is the port. Implemented in infrastructure/persistence/read/projects.go.
type ReadModel interface {
	GetProject(ctx context.Context, id string) (ProjectView, error)
	ListProjects(ctx context.Context) ([]ProjectView, error)
	GetProjectDetail(ctx context.Context, id string) (ProjectDetailView, error)
	ListCharacters(ctx context.Context, projectID string) ([]CharacterView, error)
	GetCharacter(ctx context.Context, id string) (CharacterView, error)
	ListEnvironments(ctx context.Context, projectID string) ([]EnvironmentView, error)
	GetEnvironment(ctx context.Context, id string) (EnvironmentView, error)
	GetPlotByProject(ctx context.Context, projectID string) (*PlotView, error)
	ListSocialAccounts(ctx context.Context, projectID string) ([]SocialAccountView, error)
}

type GetProject struct{ ID string }

func (GetProject) IsQuery() {}

type GetProjectHandler struct{ m ReadModel }

func (h GetProjectHandler) Handle(ctx context.Context, q GetProject) (ProjectView, error) {
	return h.m.GetProject(ctx, q.ID)
}

func GetProjectOnBus(r *bus.Registry, m ReadModel) {
	bus.RegisterQuery[GetProject, ProjectView](r, GetProjectHandler{m: m})
}

type ListProjects struct{}

func (ListProjects) IsQuery() {}

type ListProjectsHandler struct{ m ReadModel }

func (h ListProjectsHandler) Handle(ctx context.Context, q ListProjects) ([]ProjectView, error) {
	return h.m.ListProjects(ctx)
}

func ListProjectsOnBus(r *bus.Registry, m ReadModel) {
	bus.RegisterQuery[ListProjects, []ProjectView](r, ListProjectsHandler{m: m})
}

type GetProjectDetail struct{ ID string }

func (GetProjectDetail) IsQuery() {}

type GetProjectDetailHandler struct{ m ReadModel }

func (h GetProjectDetailHandler) Handle(ctx context.Context, q GetProjectDetail) (ProjectDetailView, error) {
	return h.m.GetProjectDetail(ctx, q.ID)
}

func GetProjectDetailOnBus(r *bus.Registry, m ReadModel) {
	bus.RegisterQuery[GetProjectDetail, ProjectDetailView](r, GetProjectDetailHandler{m: m})
}
