-- Read-side queries. Generate with sqlc against the READ schema if you adopt
-- codegen. These mirror the hand-written queries in ../model.go.

-- name: ReadUserByID :one
SELECT id, email, status, created_at FROM users WHERE id = $1;

-- name: ReadListUsers :many
SELECT id, email, status, created_at
FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2;
