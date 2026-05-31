package projects

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrSocialAccountNotFound   = errors.New("projects: social account not found")
	ErrSocialAccountNoPlatform = errors.New("projects: social account platform empty")
)

type SocialAccountID string

func NewSocialAccountID() SocialAccountID { return SocialAccountID(uuid.NewString()) }
func (id SocialAccountID) String() string { return string(id) }

// SocialAccountStatus codifies the scheduler's view of an account.
type SocialAccountStatus string

const (
	SocialAccountStatusActive       SocialAccountStatus = "active"
	SocialAccountStatusNeedsRelogin SocialAccountStatus = "needs_relogin"
	SocialAccountStatusBanned       SocialAccountStatus = "banned"
	SocialAccountStatusDisabled     SocialAccountStatus = "disabled"
)

// SocialAccount represents a pre-authenticated identity on one platform.
// For selenium providers it stores the Firefox profile path; future API
// providers can stash OAuth tokens in `extra`.
//
// Global by design: no project ownership. Projects reference accounts via
// the project_social_account_links table; one account can be linked to N
// projects. Cooldown / failure streak live here once, shared by all links.
type SocialAccount struct {
	id                 SocialAccountID
	platform           string // youtube_selenium | twitter_selenium | post_bridge | ...
	label              string
	firefoxProfilePath string
	extra              map[string]any

	// Scheduling.
	status        SocialAccountStatus
	lastUsedAt    *time.Time
	cooldownUntil *time.Time
	failureStreak int

	// Per-account upload defaults applied when run/template/project don't override.
	defaultVisibility    string // public | unlisted | private
	defaultMadeForKids   bool
	defaultCategoryID    string
	defaultCategoryLabel string

	// Rate-limit config (Phase: upload scheduler).
	dailyUploadLimit int
	limitWindowHours int
	isVerified       bool
	minGapSeconds    int

	createdAt time.Time
	updatedAt time.Time
}

func NewSocialAccount(platform, label, firefoxProfilePath string, extra map[string]any) (*SocialAccount, error) {
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return nil, ErrSocialAccountNoPlatform
	}
	if extra == nil {
		extra = map[string]any{}
	}
	now := time.Now().UTC()
	return &SocialAccount{
		id:                   NewSocialAccountID(),
		platform:             platform,
		label:                label,
		firefoxProfilePath:   firefoxProfilePath,
		extra:                extra,
		status:               SocialAccountStatusActive,
		defaultVisibility:    "unlisted",
		defaultCategoryID:    "22",
		defaultCategoryLabel: "People & Blogs",
		dailyUploadLimit:     15, // YT unverified
		limitWindowHours:     24,
		isVerified:           false,
		minGapSeconds:        60,
		createdAt:            now,
		updatedAt:            now,
	}, nil
}

func (s *SocialAccount) ID() SocialAccountID          { return s.id }
func (s *SocialAccount) Platform() string             { return s.platform }
func (s *SocialAccount) Label() string                { return s.label }
func (s *SocialAccount) FirefoxProfilePath() string   { return s.firefoxProfilePath }
func (s *SocialAccount) Extra() map[string]any        { return s.extra }
func (s *SocialAccount) Status() SocialAccountStatus  { return s.status }
func (s *SocialAccount) LastUsedAt() *time.Time       { return s.lastUsedAt }
func (s *SocialAccount) CooldownUntil() *time.Time    { return s.cooldownUntil }
func (s *SocialAccount) FailureStreak() int           { return s.failureStreak }
func (s *SocialAccount) DefaultVisibility() string    { return s.defaultVisibility }
func (s *SocialAccount) DefaultMadeForKids() bool     { return s.defaultMadeForKids }
func (s *SocialAccount) DefaultCategoryID() string    { return s.defaultCategoryID }
func (s *SocialAccount) DefaultCategoryLabel() string { return s.defaultCategoryLabel }
func (s *SocialAccount) DailyUploadLimit() int        { return s.dailyUploadLimit }
func (s *SocialAccount) LimitWindowHours() int        { return s.limitWindowHours }
func (s *SocialAccount) IsVerified() bool             { return s.isVerified }
func (s *SocialAccount) MinGapSeconds() int           { return s.minGapSeconds }
func (s *SocialAccount) CreatedAt() time.Time         { return s.createdAt }
func (s *SocialAccount) UpdatedAt() time.Time         { return s.updatedAt }

// SetRateLimit applies new limit fields. Negative input keeps existing value.
func (s *SocialAccount) SetRateLimit(dailyLimit, windowHours, minGapSec int, verified bool) {
	if dailyLimit >= 0 {
		s.dailyUploadLimit = dailyLimit
	}
	if windowHours > 0 {
		s.limitWindowHours = windowHours
	}
	if minGapSec >= 0 {
		s.minGapSeconds = minGapSec
	}
	s.isVerified = verified
	s.updatedAt = time.Now().UTC()
}

// SetStatus updates the lifecycle state of the account. Setters bumped here
// because the scheduler + worker need them across packages.
func (s *SocialAccount) SetStatus(st SocialAccountStatus) {
	s.status = st
	s.updatedAt = time.Now().UTC()
}

// MarkUsed records a successful upload — resets the failure streak and updates
// the last_used_at clock so the scheduler can rotate.
func (s *SocialAccount) MarkUsed(now time.Time, cooldown time.Duration) {
	s.lastUsedAt = &now
	if cooldown > 0 {
		until := now.Add(cooldown)
		s.cooldownUntil = &until
	}
	s.failureStreak = 0
	s.updatedAt = now
}

// MarkFailed bumps the failure streak and (optionally) sets a cooldown.
func (s *SocialAccount) MarkFailed(now time.Time, cooldown time.Duration) {
	s.failureStreak++
	if cooldown > 0 {
		until := now.Add(cooldown)
		s.cooldownUntil = &until
	}
	s.updatedAt = now
}

// SetDefaults patches per-account upload defaults.
func (s *SocialAccount) SetDefaults(visibility string, kids bool, categoryID, categoryLabel string) {
	if visibility != "" {
		s.defaultVisibility = visibility
	}
	s.defaultMadeForKids = kids
	if categoryID != "" {
		s.defaultCategoryID = categoryID
	}
	if categoryLabel != "" {
		s.defaultCategoryLabel = categoryLabel
	}
	s.updatedAt = time.Now().UTC()
}

func (s *SocialAccount) Update(platform, label, profilePath string, extra map[string]any) {
	platform = strings.TrimSpace(platform)
	if platform != "" {
		s.platform = platform
	}
	s.label = label
	s.firefoxProfilePath = profilePath
	if extra != nil {
		s.extra = extra
	}
	s.updatedAt = time.Now().UTC()
}

// ReconstituteSocialAccountFull rebuilds the aggregate including scheduling
// + per-account defaults + rate-limit config. Used by the write repository
// to load full state. Limit fields default to YT-unverified values when zero.
func ReconstituteSocialAccountFull(
	id SocialAccountID,
	platform, label, profilePath string, extra map[string]any,
	status SocialAccountStatus, lastUsed, cooldown *time.Time, failureStreak int,
	defVis string, defKids bool, defCatID, defCatLabel string,
	dailyLimit, windowHours, minGapSec int, verified bool,
	created, updated time.Time,
) *SocialAccount {
	if extra == nil {
		extra = map[string]any{}
	}
	if status == "" {
		status = SocialAccountStatusActive
	}
	if defVis == "" {
		defVis = "unlisted"
	}
	if defCatID == "" {
		defCatID = "22"
	}
	if defCatLabel == "" {
		defCatLabel = "People & Blogs"
	}
	if dailyLimit < 0 {
		dailyLimit = 15
	}
	if windowHours <= 0 {
		windowHours = 24
	}
	if minGapSec < 0 {
		minGapSec = 60
	}
	return &SocialAccount{
		id: id, platform: platform, label: label,
		firefoxProfilePath: profilePath, extra: extra,
		status: status, lastUsedAt: lastUsed, cooldownUntil: cooldown, failureStreak: failureStreak,
		defaultVisibility: defVis, defaultMadeForKids: defKids,
		defaultCategoryID: defCatID, defaultCategoryLabel: defCatLabel,
		dailyUploadLimit: dailyLimit, limitWindowHours: windowHours,
		isVerified: verified, minGapSeconds: minGapSec,
		createdAt: created, updatedAt: updated,
	}
}

// ProjectSocialAccountLink is the join entity: which projects use which
// global account, with an optional "is_default for this platform" flag.
type ProjectSocialAccountLink struct {
	ProjectID       ProjectID
	SocialAccountID SocialAccountID
	IsDefault       bool
	CreatedAt       time.Time
}
