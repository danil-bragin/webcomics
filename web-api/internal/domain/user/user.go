// Package user is the User aggregate (write model). Pure domain: invariants,
// behavior, and domain events. No SQL, no transport, no framework.
package user

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/example/dddcqrs/internal/domain/shared"
)

var (
	ErrEmailEmpty       = errors.New("user: email is empty")
	ErrEmailInvalid     = errors.New("user: email is invalid")
	ErrPasswordTooShort = errors.New("user: password hash empty")
	ErrAlreadyActive    = errors.New("user: already active")
	ErrNotFound         = errors.New("user: not found")
)

// ID is a typed identifier (value object) for the User aggregate.
type ID string

func NewID() ID              { return ID(uuid.NewString()) }
func (id ID) String() string { return string(id) }

// Email is a value object enforcing its own validity.
type Email string

func NewEmail(raw string) (Email, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "", ErrEmailEmpty
	}
	if !strings.Contains(raw, "@") || strings.HasPrefix(raw, "@") || strings.HasSuffix(raw, "@") {
		return "", ErrEmailInvalid
	}
	return Email(raw), nil
}

func (e Email) String() string { return string(e) }

// Status is the lifecycle state of a user.
type Status string

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)

// User is the aggregate root.
type User struct {
	shared.AggregateRoot

	id           ID
	email        Email
	passwordHash string
	status       Status
	createdAt    time.Time
}

// Register is the factory that creates a new user and records the
// UserRegistered domain event. passwordHash is already hashed by the app layer.
func Register(email Email, passwordHash string) (*User, error) {
	if passwordHash == "" {
		return nil, ErrPasswordTooShort
	}
	u := &User{
		id:           NewID(),
		email:        email,
		passwordHash: passwordHash,
		status:       StatusPending,
		createdAt:    time.Now().UTC(),
	}
	u.Record(UserRegistered{
		BaseEvent: shared.BaseEvent{ID: u.id.String(), Occurred: u.createdAt},
		Email:     u.email.String(),
	})
	return u, nil
}

// Activate transitions a pending user to active, enforcing the invariant.
func (u *User) Activate() error {
	if u.status == StatusActive {
		return ErrAlreadyActive
	}
	u.status = StatusActive
	u.Record(UserActivated{
		BaseEvent: shared.BaseEvent{ID: u.id.String(), Occurred: time.Now().UTC()},
	})
	return nil
}

// Getters (no setters — state changes go through behavior).
func (u *User) ID() ID               { return u.id }
func (u *User) Email() Email         { return u.email }
func (u *User) PasswordHash() string { return u.passwordHash }
func (u *User) Status() Status       { return u.status }
func (u *User) CreatedAt() time.Time { return u.createdAt }

// Reconstitute rebuilds a User from persistence WITHOUT emitting events.
// Used by the write repository when loading an existing aggregate.
func Reconstitute(id ID, email Email, passwordHash string, status Status, createdAt time.Time) *User {
	return &User{
		id:           id,
		email:        email,
		passwordHash: passwordHash,
		status:       status,
		createdAt:    createdAt,
	}
}
