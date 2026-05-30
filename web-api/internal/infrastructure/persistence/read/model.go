// Package read implements the query.ReadModel port. It uses ONLY the ReadPool
// (replica in prod) and returns flat DTOs. No domain objects, no transactions.
package read

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/dddcqrs/internal/app/query"
)

// Model implements query.ReadModel over the read pool.
type Model struct {
	pool *pgxpool.Pool
}

// NewModel takes the READ pool. Never pass the write pool here.
func NewModel(readPool *pgxpool.Pool) *Model {
	return &Model{pool: readPool}
}

var ErrNotFound = errors.New("read: not found")

func (m *Model) UserByID(ctx context.Context, id string) (query.UserView, error) {
	const q = `SELECT id, email, status, created_at FROM users WHERE id = $1`
	var v query.UserView
	err := m.pool.QueryRow(ctx, q, id).Scan(&v.ID, &v.Email, &v.Status, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return query.UserView{}, ErrNotFound
	}
	return v, err
}

func (m *Model) UserByEmailWithSecret(ctx context.Context, email string) (query.UserAuthView, error) {
	const q = `SELECT id, email, password_hash, status FROM users WHERE email = $1`
	var v query.UserAuthView
	err := m.pool.QueryRow(ctx, q, email).Scan(&v.ID, &v.Email, &v.PasswordHash, &v.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return query.UserAuthView{}, ErrNotFound
	}
	return v, err
}

func (m *Model) ListUsers(ctx context.Context, limit, offset int32) ([]query.UserView, error) {
	const q = `SELECT id, email, status, created_at FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	rows, err := m.pool.Query(ctx, q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []query.UserView{}
	for rows.Next() {
		var v query.UserView
		if err := rows.Scan(&v.ID, &v.Email, &v.Status, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
