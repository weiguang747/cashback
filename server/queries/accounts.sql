-- name: ListAccounts :many
-- One user's accounts, main first.
SELECT a.id, a.kind, a.currency, c.minor_unit, a.balance_minor
FROM accounts a
JOIN currencies c ON c.code = a.currency
WHERE a.user_id = $1
ORDER BY a.kind = 'main' DESC, a.currency;

-- name: UpsertAccount :one
INSERT INTO accounts (user_id, kind, currency)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, kind, currency) DO UPDATE SET kind = EXCLUDED.kind
RETURNING id;

-- name: SyncAccountBalance :exec
-- Sets the stored balance to the sum of the account's transactions. For
-- seeding only; payments must update the balance with each insert.
UPDATE accounts
SET balance_minor = (SELECT COALESCE(SUM(t.amount_minor), 0)::bigint FROM transactions t WHERE t.account_id = $1)
WHERE accounts.id = $1;

-- name: ReconcileAccounts :many
-- Accounts whose stored balance differs from the sum of their transactions.
-- Should always be empty.
SELECT a.id, a.balance_minor, COALESCE(SUM(t.amount_minor), 0)::bigint AS ledger_minor
FROM accounts a
LEFT JOIN transactions t ON t.account_id = a.id
GROUP BY a.id
HAVING a.balance_minor <> COALESCE(SUM(t.amount_minor), 0);
