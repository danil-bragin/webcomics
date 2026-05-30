// Package write contains write-side repositories. They operate on a pgx.Tx
// (provided by the UoW), deal in DOMAIN aggregates, and map to/from rows by
// hand (no ORM). They write to the master via the transaction.
package write

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/example/dddcqrs/internal/domain/user"
)

// UserRepository implements user.WriteRepository over a transaction.
type UserRepository struct {
	tx pgx.Tx
}

func NewUserRepository(tx pgx.Tx) *UserRepository {
	return &UserRepository{tx: tx}
}

// Save upserts the aggregate. Uses ON CONFLICT so it handles both insert and
// update of an existing aggregate loaded via GetByID.
func (r *UserRepository) Save(ctx context.Context, u *user.User) error {
	const q = `
		INSERT INTO users (id, email, password_hash, status, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE
		SET email = EXCLUDED.email,
		    password_hash = EXCLUDED.password_hash,
		    status = EXCLUDED.status`
	_, err := r.tx.Exec(ctx, q,
		u.ID().String(),
		u.Email().String(),
		u.PasswordHash(),
		string(u.Status()),
		u.CreatedAt(),
	)
	return err
}

func (r *UserRepository) GetByID(ctx context.Context, id user.ID) (*user.User, error) {
	const q = `SELECT id, email, password_hash, status, created_at FROM users WHERE id = $1`
	return r.scanOne(ctx, q, id.String())
}

func (r *UserRepository) GetByEmail(ctx context.Context, email user.Email) (*user.User, error) {
	const q = `SELECT id, email, password_hash, status, created_at FROM users WHERE email = $1`
	return r.scanOne(ctx, q, email.String())
}

func (r *UserRepository) ExistsByEmail(ctx context.Context, email user.Email) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`
	var exists bool
	if err := r.tx.QueryRow(ctx, q, email.String()).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *UserRepository) scanOne(ctx context.Context, q string, arg any) (*user.User, error) {
	var (
		id, email, hash, status string
		createdAt               time.Time
	)
	row := r.tx.QueryRow(ctx, q, arg)
	if err := row.Scan(&id, &email, &hash, &status, &createdAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, user.ErrNotFound
		}
		return nil, err
	}
	em, err := user.NewEmail(email)
	if err != nil {
		return nil, err
	}
	return user.Reconstitute(
		user.ID(id), em, hash, user.Status(status), createdAt,
	), nil
}
