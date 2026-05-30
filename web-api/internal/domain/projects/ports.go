package projects

import "context"

// WriteRepo persists Project + Character + Environment + Plot + global
// SocialAccount inside a UoW tx. All methods operate on the bound
// transaction.
type WriteRepo interface {
	// Project.
	SaveProject(ctx context.Context, p *Project) error
	GetProject(ctx context.Context, id ProjectID) (*Project, error)
	DeleteProject(ctx context.Context, id ProjectID) error

	// Characters.
	SaveCharacter(ctx context.Context, c *Character) error
	GetCharacter(ctx context.Context, id CharacterID) (*Character, error)
	ListCharacters(ctx context.Context, projectID ProjectID) ([]*Character, error)
	DeleteCharacter(ctx context.Context, id CharacterID) error

	// Environments.
	SaveEnvironment(ctx context.Context, e *Environment) error
	GetEnvironment(ctx context.Context, id EnvironmentID) (*Environment, error)
	ListEnvironments(ctx context.Context, projectID ProjectID) ([]*Environment, error)
	DeleteEnvironment(ctx context.Context, id EnvironmentID) error

	// Plot — one per project.
	SavePlot(ctx context.Context, p *Plot) error
	GetPlotByProject(ctx context.Context, projectID ProjectID) (*Plot, error)
	DeletePlot(ctx context.Context, projectID ProjectID) error

	// Social accounts (global).
	SaveSocialAccount(ctx context.Context, a *SocialAccount) error
	GetSocialAccount(ctx context.Context, id SocialAccountID) (*SocialAccount, error)
	ListAllSocialAccounts(ctx context.Context) ([]*SocialAccount, error)
	DeleteSocialAccount(ctx context.Context, id SocialAccountID) error

	// Project ↔ SocialAccount links.
	LinkSocialAccount(ctx context.Context, projectID ProjectID, accountID SocialAccountID, asDefault bool) error
	UnlinkSocialAccount(ctx context.Context, projectID ProjectID, accountID SocialAccountID) error
	SetDefaultSocialAccount(ctx context.Context, projectID ProjectID, accountID SocialAccountID) error
	ListProjectLinks(ctx context.Context, projectID ProjectID) ([]ProjectSocialAccountLink, error)
	ListLinkedSocialAccounts(ctx context.Context, projectID ProjectID) ([]LinkedSocialAccount, error)
}

// LinkedSocialAccount bundles a SocialAccount with its is_default flag in a
// project context. Returned by ListLinkedSocialAccounts so callers don't need
// two queries.
type LinkedSocialAccount struct {
	Account   *SocialAccount
	IsDefault bool
}
