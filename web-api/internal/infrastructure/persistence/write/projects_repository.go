package write

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/example/dddcqrs/internal/domain/projects"
)

type ProjectsRepository struct{ tx pgx.Tx }

func NewProjectsRepository(tx pgx.Tx) *ProjectsRepository {
	return &ProjectsRepository{tx: tx}
}

// --- Project ---

const upsProject = `
INSERT INTO projects (id, name, description, defaults, archived, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  description = EXCLUDED.description,
  defaults = EXCLUDED.defaults,
  archived = EXCLUDED.archived,
  updated_at = EXCLUDED.updated_at`

func (r *ProjectsRepository) SaveProject(ctx context.Context, p *projects.Project) error {
	defJSON, _ := json.Marshal(p.Defaults())
	_, err := r.tx.Exec(ctx, upsProject,
		p.ID().String(), p.Name(), p.Description(), defJSON, p.Archived(),
		p.CreatedAt(), p.UpdatedAt())
	return err
}

const selProject = `
SELECT id, name, description, COALESCE(defaults,'{}'::jsonb), archived, created_at, updated_at
FROM projects WHERE id = $1 FOR UPDATE`

func (r *ProjectsRepository) GetProject(ctx context.Context, id projects.ProjectID) (*projects.Project, error) {
	var (
		pid, name, description string
		defRaw                 []byte
		archived               bool
		created, updated       time.Time
	)
	row := r.tx.QueryRow(ctx, selProject, id.String())
	if err := row.Scan(&pid, &name, &description, &defRaw, &archived, &created, &updated); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, projects.ErrProjectNotFound
		}
		return nil, err
	}
	defaults := map[string]any{}
	_ = json.Unmarshal(defRaw, &defaults)
	return projects.ReconstituteProject(projects.ProjectID(pid), name, description, defaults, archived, created, updated), nil
}

func (r *ProjectsRepository) DeleteProject(ctx context.Context, id projects.ProjectID) error {
	_, err := r.tx.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id.String())
	return err
}

// --- Characters ---

const upsCharacter = `
INSERT INTO characters (id, project_id, name, description, traits, ref_asset_ids, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  description = EXCLUDED.description,
  traits = EXCLUDED.traits,
  ref_asset_ids = EXCLUDED.ref_asset_ids,
  updated_at = EXCLUDED.updated_at`

func (r *ProjectsRepository) SaveCharacter(ctx context.Context, c *projects.Character) error {
	traitsJSON, _ := json.Marshal(c.Traits())
	refs := c.RefAssetIDs()
	if refs == nil {
		refs = []string{}
	}
	_, err := r.tx.Exec(ctx, upsCharacter,
		c.ID().String(), c.ProjectID().String(),
		c.Name(), c.Description(), traitsJSON, refs,
		c.CreatedAt(), c.UpdatedAt())
	return err
}

const selCharacter = `
SELECT id, project_id, name, description, COALESCE(traits,'{}'::jsonb), ref_asset_ids, created_at, updated_at
FROM characters WHERE id = $1 FOR UPDATE`

func (r *ProjectsRepository) GetCharacter(ctx context.Context, id projects.CharacterID) (*projects.Character, error) {
	var (
		cid, pid, name, description string
		traitsRaw                   []byte
		refs                        []string
		created, updated            time.Time
	)
	row := r.tx.QueryRow(ctx, selCharacter, id.String())
	if err := row.Scan(&cid, &pid, &name, &description, &traitsRaw, &refs, &created, &updated); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, projects.ErrCharacterNotFound
		}
		return nil, err
	}
	traits := map[string]any{}
	_ = json.Unmarshal(traitsRaw, &traits)
	return projects.ReconstituteCharacter(
		projects.CharacterID(cid), projects.ProjectID(pid),
		name, description, traits, refs, created, updated,
	), nil
}

func (r *ProjectsRepository) ListCharacters(ctx context.Context, projectID projects.ProjectID) ([]*projects.Character, error) {
	const q = `SELECT id, project_id, name, description, COALESCE(traits,'{}'::jsonb), ref_asset_ids, created_at, updated_at
		FROM characters WHERE project_id = $1 ORDER BY created_at ASC`
	rows, err := r.tx.Query(ctx, q, projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*projects.Character
	for rows.Next() {
		var (
			cid, pid, name, description string
			traitsRaw                   []byte
			refs                        []string
			created, updated            time.Time
		)
		if err := rows.Scan(&cid, &pid, &name, &description, &traitsRaw, &refs, &created, &updated); err != nil {
			return nil, err
		}
		traits := map[string]any{}
		_ = json.Unmarshal(traitsRaw, &traits)
		out = append(out, projects.ReconstituteCharacter(
			projects.CharacterID(cid), projects.ProjectID(pid),
			name, description, traits, refs, created, updated,
		))
	}
	return out, rows.Err()
}

func (r *ProjectsRepository) DeleteCharacter(ctx context.Context, id projects.CharacterID) error {
	_, err := r.tx.Exec(ctx, `DELETE FROM characters WHERE id = $1`, id.String())
	return err
}

// --- Environments ---

const upsEnvironment = `
INSERT INTO environments (id, project_id, name, description, traits, ref_asset_ids, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  description = EXCLUDED.description,
  traits = EXCLUDED.traits,
  ref_asset_ids = EXCLUDED.ref_asset_ids,
  updated_at = EXCLUDED.updated_at`

func (r *ProjectsRepository) SaveEnvironment(ctx context.Context, e *projects.Environment) error {
	traitsJSON, _ := json.Marshal(e.Traits())
	refs := e.RefAssetIDs()
	if refs == nil {
		refs = []string{}
	}
	_, err := r.tx.Exec(ctx, upsEnvironment,
		e.ID().String(), e.ProjectID().String(),
		e.Name(), e.Description(), traitsJSON, refs,
		e.CreatedAt(), e.UpdatedAt())
	return err
}

const selEnvironment = `
SELECT id, project_id, name, description, COALESCE(traits,'{}'::jsonb), ref_asset_ids, created_at, updated_at
FROM environments WHERE id = $1 FOR UPDATE`

func (r *ProjectsRepository) GetEnvironment(ctx context.Context, id projects.EnvironmentID) (*projects.Environment, error) {
	var (
		eid, pid, name, description string
		traitsRaw                   []byte
		refs                        []string
		created, updated            time.Time
	)
	row := r.tx.QueryRow(ctx, selEnvironment, id.String())
	if err := row.Scan(&eid, &pid, &name, &description, &traitsRaw, &refs, &created, &updated); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, projects.ErrEnvironmentNotFound
		}
		return nil, err
	}
	traits := map[string]any{}
	_ = json.Unmarshal(traitsRaw, &traits)
	return projects.ReconstituteEnvironment(
		projects.EnvironmentID(eid), projects.ProjectID(pid),
		name, description, traits, refs, created, updated,
	), nil
}

func (r *ProjectsRepository) ListEnvironments(ctx context.Context, projectID projects.ProjectID) ([]*projects.Environment, error) {
	const q = `SELECT id, project_id, name, description, COALESCE(traits,'{}'::jsonb), ref_asset_ids, created_at, updated_at
		FROM environments WHERE project_id = $1 ORDER BY created_at ASC`
	rows, err := r.tx.Query(ctx, q, projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*projects.Environment
	for rows.Next() {
		var (
			eid, pid, name, description string
			traitsRaw                   []byte
			refs                        []string
			created, updated            time.Time
		)
		if err := rows.Scan(&eid, &pid, &name, &description, &traitsRaw, &refs, &created, &updated); err != nil {
			return nil, err
		}
		traits := map[string]any{}
		_ = json.Unmarshal(traitsRaw, &traits)
		out = append(out, projects.ReconstituteEnvironment(
			projects.EnvironmentID(eid), projects.ProjectID(pid),
			name, description, traits, refs, created, updated,
		))
	}
	return out, rows.Err()
}

func (r *ProjectsRepository) DeleteEnvironment(ctx context.Context, id projects.EnvironmentID) error {
	_, err := r.tx.Exec(ctx, `DELETE FROM environments WHERE id = $1`, id.String())
	return err
}

// --- Plot ---

const upsPlot = `
INSERT INTO plots (id, project_id, name, premise, beats, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (project_id) DO UPDATE SET
  name = EXCLUDED.name,
  premise = EXCLUDED.premise,
  beats = EXCLUDED.beats,
  updated_at = EXCLUDED.updated_at`

func (r *ProjectsRepository) SavePlot(ctx context.Context, p *projects.Plot) error {
	beatsJSON, _ := json.Marshal(p.Beats())
	_, err := r.tx.Exec(ctx, upsPlot,
		p.ID().String(), p.ProjectID().String(),
		p.Name(), p.Premise(), beatsJSON,
		p.CreatedAt(), p.UpdatedAt())
	return err
}

func (r *ProjectsRepository) GetPlotByProject(ctx context.Context, projectID projects.ProjectID) (*projects.Plot, error) {
	const q = `SELECT id, project_id, name, premise, COALESCE(beats,'[]'::jsonb), created_at, updated_at
		FROM plots WHERE project_id = $1 FOR UPDATE`
	var (
		pid, prj, name, premise string
		beatsRaw                []byte
		created, updated        time.Time
	)
	row := r.tx.QueryRow(ctx, q, projectID.String())
	if err := row.Scan(&pid, &prj, &name, &premise, &beatsRaw, &created, &updated); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, projects.ErrPlotNotFound
		}
		return nil, err
	}
	var beats []projects.PlotBeat
	_ = json.Unmarshal(beatsRaw, &beats)
	return projects.ReconstitutePlot(
		projects.PlotID(pid), projects.ProjectID(prj),
		name, premise, beats, created, updated,
	), nil
}

func (r *ProjectsRepository) DeletePlot(ctx context.Context, projectID projects.ProjectID) error {
	_, err := r.tx.Exec(ctx, `DELETE FROM plots WHERE project_id = $1`, projectID.String())
	return err
}

// --- Social accounts ---

const upsSocialAccount = `
INSERT INTO social_accounts (
  id, project_id, platform, label, firefox_profile_path, extra,
  status, last_used_at, cooldown_until, failure_streak,
  default_visibility, default_made_for_kids, default_category_id, default_category_label,
  created_at, updated_at
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
ON CONFLICT (id) DO UPDATE SET
  platform = EXCLUDED.platform,
  label = EXCLUDED.label,
  firefox_profile_path = EXCLUDED.firefox_profile_path,
  extra = EXCLUDED.extra,
  status = EXCLUDED.status,
  last_used_at = EXCLUDED.last_used_at,
  cooldown_until = EXCLUDED.cooldown_until,
  failure_streak = EXCLUDED.failure_streak,
  default_visibility = EXCLUDED.default_visibility,
  default_made_for_kids = EXCLUDED.default_made_for_kids,
  default_category_id = EXCLUDED.default_category_id,
  default_category_label = EXCLUDED.default_category_label,
  updated_at = EXCLUDED.updated_at`

func (r *ProjectsRepository) SaveSocialAccount(ctx context.Context, a *projects.SocialAccount) error {
	extraJSON, _ := json.Marshal(a.Extra())
	_, err := r.tx.Exec(ctx, upsSocialAccount,
		a.ID().String(), a.ProjectID().String(),
		a.Platform(), a.Label(), a.FirefoxProfilePath(), extraJSON,
		string(a.Status()), a.LastUsedAt(), a.CooldownUntil(), a.FailureStreak(),
		a.DefaultVisibility(), a.DefaultMadeForKids(), a.DefaultCategoryID(), a.DefaultCategoryLabel(),
		a.CreatedAt(), a.UpdatedAt())
	return err
}

const selSocialAccount = `
SELECT id, project_id, platform, label, firefox_profile_path, COALESCE(extra,'{}'::jsonb),
       COALESCE(status,'active'), last_used_at, cooldown_until, COALESCE(failure_streak,0),
       COALESCE(default_visibility,'unlisted'), COALESCE(default_made_for_kids,false),
       COALESCE(default_category_id,'22'), COALESCE(default_category_label,'People & Blogs'),
       created_at, updated_at
FROM social_accounts WHERE id = $1 FOR UPDATE`

func (r *ProjectsRepository) GetSocialAccount(ctx context.Context, id projects.SocialAccountID) (*projects.SocialAccount, error) {
	var (
		sid, pid, platform, label, profilePath string
		extraRaw                               []byte
		statusStr, defVis, defCatID, defCatLab string
		defKids                                bool
		failureStreak                          int
		lastUsed, cooldown                     *time.Time
		created, updated                       time.Time
	)
	row := r.tx.QueryRow(ctx, selSocialAccount, id.String())
	if err := row.Scan(&sid, &pid, &platform, &label, &profilePath, &extraRaw,
		&statusStr, &lastUsed, &cooldown, &failureStreak,
		&defVis, &defKids, &defCatID, &defCatLab,
		&created, &updated); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, projects.ErrSocialAccountNotFound
		}
		return nil, err
	}
	extra := map[string]any{}
	_ = json.Unmarshal(extraRaw, &extra)
	return projects.ReconstituteSocialAccountFull(
		projects.SocialAccountID(sid), projects.ProjectID(pid),
		platform, label, profilePath, extra,
		projects.SocialAccountStatus(statusStr), lastUsed, cooldown, failureStreak,
		defVis, defKids, defCatID, defCatLab,
		created, updated,
	), nil
}

const listSocialAccountsSQL = `SELECT id, project_id, platform, label, firefox_profile_path, COALESCE(extra,'{}'::jsonb),
       COALESCE(status,'active'), last_used_at, cooldown_until, COALESCE(failure_streak,0),
       COALESCE(default_visibility,'unlisted'), COALESCE(default_made_for_kids,false),
       COALESCE(default_category_id,'22'), COALESCE(default_category_label,'People & Blogs'),
       created_at, updated_at
   FROM social_accounts WHERE project_id = $1 ORDER BY created_at ASC`

func (r *ProjectsRepository) ListSocialAccounts(ctx context.Context, projectID projects.ProjectID) ([]*projects.SocialAccount, error) {
	rows, err := r.tx.Query(ctx, listSocialAccountsSQL, projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*projects.SocialAccount{}
	for rows.Next() {
		var (
			sid, pid, platform, label, profilePath string
			extraRaw                               []byte
			statusStr, defVis, defCatID, defCatLab string
			defKids                                bool
			failureStreak                          int
			lastUsed, cooldown                     *time.Time
			created, updated                       time.Time
		)
		if err := rows.Scan(&sid, &pid, &platform, &label, &profilePath, &extraRaw,
			&statusStr, &lastUsed, &cooldown, &failureStreak,
			&defVis, &defKids, &defCatID, &defCatLab,
			&created, &updated); err != nil {
			return nil, err
		}
		extra := map[string]any{}
		_ = json.Unmarshal(extraRaw, &extra)
		out = append(out, projects.ReconstituteSocialAccountFull(
			projects.SocialAccountID(sid), projects.ProjectID(pid),
			platform, label, profilePath, extra,
			projects.SocialAccountStatus(statusStr), lastUsed, cooldown, failureStreak,
			defVis, defKids, defCatID, defCatLab,
			created, updated,
		))
	}
	return out, rows.Err()
}

func (r *ProjectsRepository) DeleteSocialAccount(ctx context.Context, id projects.SocialAccountID) error {
	_, err := r.tx.Exec(ctx, `DELETE FROM social_accounts WHERE id = $1`, id.String())
	return err
}
