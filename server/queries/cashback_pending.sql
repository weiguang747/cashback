-- name: InsertPendingAward :exec
-- An award reserved by the Redis gate, in the payment's transaction. The
-- worker credits it later (ClaimPendingAwards).
INSERT INTO cashback_pending (payment_transaction_id, campaign_id, user_id, cashback_account_id, currency,
                              amount_minor, campaign_day, credit_description, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ClaimPendingAwards :many
-- The oldest awards waiting to be credited, locked for this worker. SKIP
-- LOCKED lets several workers (one per server) share the queue without
-- waiting on each other or crediting an award twice.
SELECT p.payment_transaction_id, p.campaign_id, p.user_id, p.cashback_account_id, p.currency, p.amount_minor,
       p.campaign_day, p.credit_description, c.account_id AS campaign_account_id
FROM cashback_pending p
JOIN campaigns c ON c.id = p.campaign_id
WHERE p.failed_at IS NULL
ORDER BY p.created_at
LIMIT sqlc.arg('limit')::int
FOR UPDATE OF p SKIP LOCKED;

-- name: DeletePendingAward :exec
DELETE FROM cashback_pending WHERE payment_transaction_id = $1;

-- name: MarkPendingFailed :exec
-- The award can never be written. The row stays (still counted against the
-- budget, still shown as pending to the user) until someone resolves it.
UPDATE cashback_pending SET failed_at = now(), attempts = attempts + 1
WHERE payment_transaction_id = $1;

-- name: OldestPendingAgeSecs :one
-- How long the oldest award still waiting to be credited has waited, in
-- seconds (0 when none is). For the backlog alert.
SELECT COALESCE(EXTRACT(EPOCH FROM now() - MIN(created_at)), 0)::int FROM cashback_pending WHERE failed_at IS NULL;

-- name: CountFailedPendingAwards :one
SELECT COUNT(*) FROM cashback_pending WHERE failed_at IS NOT NULL;

-- name: PendingForCampaign :one
-- Reserved but not yet credited: already spent from the campaign's budget.
SELECT COALESCE(SUM(amount_minor), 0)::bigint FROM cashback_pending WHERE campaign_id = $1;

-- name: GetPendingByPayment :one
SELECT campaign_id, amount_minor FROM cashback_pending WHERE payment_transaction_id = $1;

-- name: CommittedReservations :many
-- Which of these payments' reservations reached the database (pending or
-- credited). The reconciler runs this in the same REPEATABLE READ snapshot
-- as CampaignBudgetLeft, so the two agree.
-- The column is always qualified (r.payment_id): a bare name inside the
-- subqueries would resolve to the subquery's own table first
-- (cashback_awards.id), silently matching nothing.
SELECT r.payment_id::uuid AS payment_transaction_id
FROM unnest(sqlc.arg('payment_ids')::uuid[]) AS r (payment_id)
WHERE EXISTS (SELECT 1 FROM cashback_pending p WHERE p.payment_transaction_id = r.payment_id)
   OR EXISTS (SELECT 1 FROM cashback_awards ca WHERE ca.payment_transaction_id = r.payment_id);

-- name: SetBudgetGate :exec
-- Run with the campaign locked (LockCampaign). Clears the degraded flag:
-- whoever switches the gate has just seeded or closed it.
UPDATE campaigns SET budget_gate = sqlc.arg('budget_gate'), gate_degraded_at = NULL WHERE id = sqlc.arg('id');

-- name: DegradeGate :one
-- Payments stop using the Redis gate and, after a grace period, award
-- under the campaign lock. Only the first caller changes anything, so a
-- burst of failures writes the campaign row once. Returns whether this call
-- set it.
UPDATE campaigns SET gate_degraded_at = now()
WHERE id = $1 AND budget_gate = 'redis' AND gate_degraded_at IS NULL
RETURNING id;

-- name: ClearGateDegraded :exec
-- Run with the campaign locked, right after reseeding the Redis counter.
UPDATE campaigns SET gate_degraded_at = NULL WHERE id = $1;

-- name: ListGatedCampaigns :many
-- Live campaigns that use the Redis gate (redis or shadow), for the
-- reconciler: the counter should equal the account balance minus pending
-- awards minus reservations still in flight.
SELECT c.id, c.budget_gate, (c.gate_degraded_at IS NOT NULL)::bool AS gate_degraded, c.award_increment_minor
FROM campaigns c
WHERE c.budget_gate <> 'db'
  AND c.starts_at <= now() AND (c.ends_at IS NULL OR c.ends_at > now());

-- name: CampaignBudgetLeft :one
-- The campaign account balance minus awards reserved but not yet credited:
-- what the Redis counter should hold when nothing is in flight. Crediting
-- an award doesn't change it (the balance and pending drop together);
-- committing a reserved payment lowers it.
SELECT (a.balance_minor - COALESCE((SELECT SUM(p.amount_minor) FROM cashback_pending p WHERE p.campaign_id = c.id), 0))::bigint
FROM campaigns c JOIN accounts a ON a.id = c.account_id
WHERE c.id = $1;
