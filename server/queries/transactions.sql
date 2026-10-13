-- name: ListTransactions :many
-- A user's main-account activity, newest first, with the cash back each
-- payment earned (NULL if none). A NULL limit returns every row.
SELECT t.id, t.name, t.description, t.kind, t.amount_minor, t.currency, c.minor_unit, t.created_at,
       ca.amount_minor AS cashback_minor
FROM transactions t
JOIN accounts a ON a.id = t.account_id
JOIN currencies c ON c.code = t.currency
LEFT JOIN cashback_awards ca ON ca.payment_transaction_id = t.id
WHERE a.user_id = sqlc.arg('user_id')::uuid AND a.kind = 'main'
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.narg('limit');

-- name: InsertTransaction :one
INSERT INTO transactions (account_id, name, description, kind, amount_minor, currency, created_at, idempotency_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: GetTransactionByIdempotencyKey :one
SELECT t.id, t.name, t.description, t.kind, t.amount_minor, t.currency, c.minor_unit, t.created_at
FROM transactions t
JOIN currencies c ON c.code = t.currency
WHERE t.account_id = $1 AND t.idempotency_key = $2;
