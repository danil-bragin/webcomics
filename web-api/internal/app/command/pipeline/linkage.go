package pipeline

import (
	"context"

	"github.com/example/dddcqrs/internal/domain/pipeline"
	"github.com/example/dddcqrs/internal/domain/projects"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/uow"
)

// resolveLinkedContext loads characters + environments + plot from the projects
// repo and resolves their ref_asset_ids into MinIO object_keys. Returns the
// fully-baked LinkedContext that the Run aggregate will pin to its events.
//
// projectID is required. characterIDs / environmentIDs filter the project's
// catalogue down to the ones the user picked. usePlot=true loads the project's
// plot when present.
func resolveLinkedContext(
	ctx context.Context,
	repos uow.Repositories,
	projectID string,
	characterIDs, environmentIDs []string,
	usePlot bool,
) (*pipeline.LinkedContext, string, error) {
	projRepo := repos.Projects()
	out := &pipeline.LinkedContext{ProjectID: projectID}

	wantChars := indexOf(characterIDs)
	wantEnvs := indexOf(environmentIDs)

	allChars, err := projRepo.ListCharacters(ctx, projects.ProjectID(projectID))
	if err != nil {
		return nil, "", err
	}
	allEnvs, err := projRepo.ListEnvironments(ctx, projects.ProjectID(projectID))
	if err != nil {
		return nil, "", err
	}

	// Collect all asset IDs needed so we can do one batched lookup.
	var allAssetIDs []string
	for _, c := range allChars {
		if len(characterIDs) > 0 && !wantChars[c.ID().String()] {
			continue
		}
		allAssetIDs = append(allAssetIDs, c.RefAssetIDs()...)
	}
	for _, e := range allEnvs {
		if len(environmentIDs) > 0 && !wantEnvs[e.ID().String()] {
			continue
		}
		allAssetIDs = append(allAssetIDs, e.RefAssetIDs()...)
	}
	keyByID, err := repos.PipelineRuns().GetAssetObjectKeys(ctx, allAssetIDs)
	if err != nil {
		return nil, "", err
	}

	for _, c := range allChars {
		if len(characterIDs) > 0 && !wantChars[c.ID().String()] {
			continue
		}
		out.Characters = append(out.Characters, pipeline.CharacterContext{
			ID: c.ID().String(), Name: c.Name(),
			Description:   c.Description(),
			Traits:        c.Traits(),
			RefObjectKeys: keysFor(c.RefAssetIDs(), keyByID),
		})
	}
	for _, e := range allEnvs {
		if len(environmentIDs) > 0 && !wantEnvs[e.ID().String()] {
			continue
		}
		out.Environments = append(out.Environments, pipeline.EnvironmentContext{
			ID: e.ID().String(), Name: e.Name(),
			Description:   e.Description(),
			Traits:        e.Traits(),
			RefObjectKeys: keysFor(e.RefAssetIDs(), keyByID),
		})
	}

	plotID := ""
	if usePlot {
		plot, err := projRepo.GetPlotByProject(ctx, projects.ProjectID(projectID))
		if err != nil && err != projects.ErrPlotNotFound {
			return nil, "", err
		}
		if plot != nil {
			plotID = plot.ID().String()
			beats := make([]pipeline.PlotBeatContext, 0, len(plot.Beats()))
			for _, b := range plot.Beats() {
				beats = append(beats, pipeline.PlotBeatContext{
					Name: b.Name, Description: b.Description, Order: b.Order,
				})
			}
			out.Plot = &pipeline.PlotContext{
				ID: plot.ID().String(), Name: plot.Name(),
				Premise: plot.Premise(), Beats: beats,
			}
		}
	}
	return out, plotID, nil
}

func indexOf(xs []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func keysFor(ids []string, byID map[string]string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if k, ok := byID[id]; ok && k != "" {
			out = append(out, k)
		}
	}
	return out
}
