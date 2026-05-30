// Package postgres provides two pgxpool pools: WritePool (master) and ReadPool
// (replica). In dev both DSNs point at the same instance; in prod ReadPool
// targets a read replica. The CQRS split guarantees queries only use ReadPool.
package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/samber/do/v2"

	"github.com/example/dddcqrs/internal/infrastructure/config"
)

// WritePool wraps the master pool (commands / transactions).
type WritePool struct{ *pgxpool.Pool }

// ReadPool wraps the replica pool (queries only, read-only).
type ReadPool struct{ *pgxpool.Pool }

func NewWritePool(i do.Injector) (*WritePool, error) {
	cfg := do.MustInvoke[*config.Config](i)
	p, err := newPool(cfg.WriteDatabaseURL)
	if err != nil {
		return nil, err
	}
	return &WritePool{Pool: p}, nil
}

func NewReadPool(i do.Injector) (*ReadPool, error) {
	cfg := do.MustInvoke[*config.Config](i)
	dsn := cfg.ReadDatabaseURL
	if dsn == "" {
		dsn = cfg.WriteDatabaseURL // dev fallback: replica == master
	}
	p, err := newPool(dsn)
	if err != nil {
		return nil, err
	}
	return &ReadPool{Pool: p}, nil
}

func newPool(dsn string) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func (p *WritePool) Shutdown() error { p.Pool.Close(); return nil }
func (p *ReadPool) Shutdown() error  { p.Pool.Close(); return nil }

func (p *WritePool) HealthCheck() error { return ping(p.Pool) }
func (p *ReadPool) HealthCheck() error  { return ping(p.Pool) }

func ping(p *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return p.Ping(ctx)
}
