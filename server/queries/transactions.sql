-- name: ListTransactions :many
-- One page of a user's main-account activity, newest first, with the cash
-- back each payment earned (NULL if none). The page starts after the cursor
-- (created_at, id) of the previous page's last row; NULL starts at the
-- newest. The cursor is always a row comparison, so even a generic plan
-- seeks transactions_account_created_at_idx instead of scanning from the top.
-- kind and name_pattern (ILIKE) are optional filters.
SELECT t.id, t.name, t.description, t.kind, t.amount_minor, t.currency, c.minor_unit, t.created_at,
       ca.amount_minor AS cashback_minor
FROM transactions t
JOIN accounts a ON a.id = t.account_id
JOIN currencies c ON c.code = t.currency
LEFT JOIN cashback_awards ca ON ca.payment_transaction_id = t.id
WHERE a.user_id = sqlc.arg('user_id')::uuid AND a.kind = 'main'
  AND (t.created_at, t.id) < (COALESCE(sqlc.narg('before_created_at')::timestamptz, 'infinity'),
                              COALESCE(sqlc.narg('before_id')::uuid, 'ffffffff-ffff-ffff-ffff-ffffffffffff'))
  AND (sqlc.narg('kind')::text IS NULL OR t.kind = sqlc.narg('kind')::text)
  AND (sqlc.narg('name_pattern')::text IS NULL OR t.name ILIKE sqlc.narg('name_pattern')::text)
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.arg('limit')::int;

-- name: InsertTransaction :one
INSERT INTO transactions (account_id, name, description, kind, amount_minor, currency, created_at, idempotency_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: InsertPayment :one
-- Records a payment and returns it as GetTransactionByIdempotencyKey
-- would, so Pay needn't read it back.
INSERT INTO transactions (account_id, name, description, kind, amount_minor, currency, created_at, idempotency_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, name, description, kind, amount_minor, currency,
          (SELECT c.minor_unit FROM currencies c WHERE c.code = transactions.currency) AS minor_unit, created_at;

-- name: GetTransactionByIdempotencyKey :one
SELECT t.id, t.name, t.description, t.kind, t.amount_minor, t.currency, c.minor_unit, t.created_at
FROM transactions t
JOIN currencies c ON c.code = t.currency
WHERE t.account_id = $1 AND t.idempotency_key = $2;
