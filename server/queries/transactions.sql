-- name: ListTransactions :many
-- One page of a user's main-account activity, newest first, with the cash
-- back each payment earned (NULL if none). The page starts after the cursor
-- (created_at, id) of the previous page's last row; NULL starts at the
-- newest. kind is an optional filter.
--
-- The rows are read per account inside the LATERAL, so Postgres walks
-- transactions_account_created_at_idx in order and stops after the page.
-- Joining accounts to transactions directly doesn't: a join only keeps its
-- outer side's order, so every page read and sorted the whole history.
SELECT t.id, t.name, t.description, t.kind, t.amount_minor, t.currency, c.minor_unit, t.created_at,
       ca.amount_minor AS cashback_minor
FROM accounts a
CROSS JOIN LATERAL (
    SELECT t.id, t.name, t.description, t.kind, t.amount_minor, t.currency, t.created_at
    FROM transactions t
    WHERE t.account_id = a.id
      AND (t.created_at, t.id) < (COALESCE(sqlc.narg('before_created_at')::timestamptz, 'infinity'),
                                  COALESCE(sqlc.narg('before_id')::uuid, 'ffffffff-ffff-ffff-ffff-ffffffffffff'))
      AND (sqlc.narg('kind')::text IS NULL OR t.kind = sqlc.narg('kind')::text)
    ORDER BY t.created_at DESC, t.id DESC
    LIMIT sqlc.arg('limit')::int
) t
JOIN currencies c ON c.code = t.currency
LEFT JOIN cashback_awards ca ON ca.payment_transaction_id = t.id
WHERE a.user_id = sqlc.arg('user_id')::uuid AND a.kind = 'main'
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.arg('limit')::int;

-- name: SearchTransactions :many
-- ListTransactions for rows whose name matches name_pattern (ILIKE, already
-- escaped). A separate query so the pattern is required: the best plan
-- depends on it (transactions_name_trgm_idx for a rare term, walking
-- transactions_account_created_at_idx for a short or common one), so it
-- must be planned with the actual pattern every time, never as a cached
-- generic plan (see pgStore.SearchTransactions).
SELECT t.id, t.name, t.description, t.kind, t.amount_minor, t.currency, c.minor_unit, t.created_at,
       ca.amount_minor AS cashback_minor
FROM accounts a
CROSS JOIN LATERAL (
    SELECT t.id, t.name, t.description, t.kind, t.amount_minor, t.currency, t.created_at
    FROM transactions t
    WHERE t.account_id = a.id
      AND (t.created_at, t.id) < (COALESCE(sqlc.narg('before_created_at')::timestamptz, 'infinity'),
                                  COALESCE(sqlc.narg('before_id')::uuid, 'ffffffff-ffff-ffff-ffff-ffffffffffff'))
      AND (sqlc.narg('kind')::text IS NULL OR t.kind = sqlc.narg('kind')::text)
      AND t.name ILIKE sqlc.arg('name_pattern')::text
    ORDER BY t.created_at DESC, t.id DESC
    LIMIT sqlc.arg('limit')::int
) t
JOIN currencies c ON c.code = t.currency
LEFT JOIN cashback_awards ca ON ca.payment_transaction_id = t.id
WHERE a.user_id = sqlc.arg('user_id')::uuid AND a.kind = 'main'
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
