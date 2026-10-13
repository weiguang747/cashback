-- name: ListTransactions :many
-- Newest first. A NULL limit returns every row.
SELECT t.id, t.name, t.description, t.kind, t.amount_minor, t.currency, c.minor_unit, t.created_at
FROM transactions t
JOIN currencies c ON c.code = t.currency
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.narg('limit');

-- name: InsertTransaction :one
INSERT INTO transactions (name, description, kind, amount_minor, currency, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;
