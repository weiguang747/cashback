-- name: GetUser :one
SELECT id, email, name FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT id, email, name FROM users WHERE email = $1;

-- name: UpsertUser :one
INSERT INTO users (email, name)
VALUES ($1, $2)
ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name
RETURNING id;
