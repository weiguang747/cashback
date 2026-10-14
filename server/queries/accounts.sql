-- name: ListAccounts :many
-- One user's accounts, main first, with the cash back reserved for each
-- but not yet credited (always 0 for a main account).
SELECT a.id, a.kind, a.currency, c.minor_unit, a.balance_minor,
       (SELECT COALESCE(SUM(p.amount_minor), 0) FROM cashback_pending p WHERE p.cashback_account_id = a.id)::bigint
           AS pending_minor
FROM accounts a
JOIN currencies c ON c.code = a.currency
WHERE a.user_id = sqlc.arg('user_id')::uuid
ORDER BY a.kind = 'main' DESC, a.currency;

-- name: UpsertAccount :one
INSERT INTO accounts (user_id, kind, currency)
VALUES (sqlc.arg('user_id')::uuid, sqlc.arg('kind'), sqlc.arg('currency'))
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

-- name: GetMainAccount :one
SELECT a.id, a.currency, c.minor_unit, a.balance_minor
FROM accounts a
JOIN currencies c ON c.code = a.currency
WHERE a.user_id = sqlc.arg('user_id')::uuid AND a.kind = 'main' AND a.currency = sqlc.arg('currency');

-- name: DebitAccount :one
-- The overdraft guard. Matches no row when the balance is too low, and the
-- row lock it takes makes concurrent debits of one account run in turn.
UPDATE accounts
SET balance_minor = balance_minor - sqlc.arg('amount_minor')
WHERE id = sqlc.arg('id') AND balance_minor >= sqlc.arg('amount_minor')
RETURNING balance_minor;

-- name: GetAccountBalance :one
SELECT balance_minor FROM accounts WHERE id = $1;

-- name: LockUserAccounts :many
-- A user's main and cash back accounts in one currency, locked in the
-- money lock order: main first, then cash back. Rows are locked after the
-- sort, so the ORDER BY is the lock order.
--
-- pending_minor is only shown to the user. It isn't locked: if this waited
-- for the worker to credit an award, the award can be in both the balance
-- and pending for this one response.
SELECT a.id, a.kind, a.balance_minor, c.minor_unit,
       (SELECT COALESCE(SUM(p.amount_minor), 0) FROM cashback_pending p WHERE p.cashback_account_id = a.id)::bigint
           AS pending_minor
FROM accounts a
JOIN currencies c ON c.code = a.currency
WHERE a.user_id = sqlc.arg('user_id')::uuid AND a.currency = sqlc.arg('currency') AND a.kind IN ('main', 'cashback')
ORDER BY a.kind = 'main' DESC
FOR UPDATE OF a;

-- name: PendingForAccount :one
-- Cash back reserved for a cash back account but not yet credited.
SELECT COALESCE(SUM(amount_minor), 0)::bigint FROM cashback_pending WHERE cashback_account_id = $1;
