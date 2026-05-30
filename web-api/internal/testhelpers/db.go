// Package testhelpers exposes shared integration-test plumbing: pgx pool
// against PG_TEST_DSN, auto-migrate, truncate-all, and a fixture seeder.
//
// All helpers t.Skip when PG_TEST_DSN isn't set so unit-only `go test ./...`
// remains green without external dependencies.
package testhelpers

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/example/dddcqrs/migrations"
)

var (
	migrateOnce sync.Once
	migrateErr  error
)

// MustPool returns a pgxpool against PG_TEST_DSN or t.Skip-s. Runs the goose
// migrations once per process. Closes pool on t.Cleanup.
func MustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skip integration test")
	}
	ctx := context.Background()
	migrateOnce.Do(func() {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			migrateErr = err
			return
		}
		defer db.Close()
		goose.SetBaseFS(migrations.FS)
		if err := goose.SetDialect("postgres"); err != nil {
			migrateErr = err
			return
		}
		if err := goose.UpContext(ctx, db, "."); err != nil {
			migrateErr = err
			return
		}
	})
	if migrateErr != nil {
		t.Fatalf("migrate: %v", migrateErr)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Wipe truncates every pipeline+project+outbox table. Use at start of each
// test so cases don't interfere. CASCADE removes any leftover row.
func Wipe(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	const sql = `TRUNCATE
		pipeline_cost_entries,
		pipeline_assets,
		pipeline_step_attempts,
		pipeline_steps,
		pipeline_runs,
		plots,
		characters,
		environments,
		projects,
		outbox,
		processed_messages
		RESTART IDENTITY CASCADE`
	if _, err := pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("wipe: %v", err)
	}
}

// SeedDefaultTemplate inserts a minimal 3-step template (script→image→assemble)
// and returns its id. Useful when a test needs an existing template_id without
// going through the command layer.
func SeedDefaultTemplate(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	const sql = `INSERT INTO pipeline_templates (id, name, steps, max_cost_usd)
		VALUES ($1, $2, $3::jsonb, $4)
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`
	id := "00000000-0000-0000-0000-000000000099"
	steps := `[
		{"type":"script","model":"openai/gpt-4o-mini","params":{"panel_count":3}},
		{"type":"image","model":"fal-ai/flux/schnell"},
		{"type":"assemble","params":{"width":1080,"height":1080,"fps":30}}
	]`
	if _, err := pool.Exec(context.Background(), sql, id, "default-test", steps, 0); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	return id
}
