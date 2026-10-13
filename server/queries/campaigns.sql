-- name: LockActiveCampaign :one
-- The campaign live for a currency at a moment, locked so awards from one
-- budget (and ending the campaign) happen one at a time. Also returns the
-- moment's calendar day in the campaign's time zone.
SELECT c.id, c.account_id, c.rate_bps, c.min_spend_minor, c.daily_cap_minor, c.award_increment_minor,
       c.starts_at, (sqlc.arg('at')::timestamptz AT TIME ZONE c.timezone)::date AS campaign_day
FROM campaigns c
WHERE c.currency = sqlc.arg('currency')
  AND c.starts_at <= sqlc.arg('at')
  AND (c.ends_at IS NULL OR c.ends_at > sqlc.arg('at'))
FOR UPDATE;

-- name: UserAwardedOnDay :one
SELECT COALESCE(SUM(amount_minor), 0)::bigint
FROM cashback_awards
WHERE campaign_id = $1 AND user_id = $2 AND campaign_day = $3;

-- name: AddToBalance :one
-- Unconditional change; the accounts_balance_check constraint still keeps
-- every account except funding at or above zero.
UPDATE accounts SET balance_minor = balance_minor + sqlc.arg('delta_minor')
WHERE id = sqlc.arg('id')
RETURNING balance_minor;

-- name: CloseCampaign :exec
UPDATE campaigns
SET ends_at = GREATEST(sqlc.arg('at')::timestamptz, starts_at + interval '1 microsecond')
WHERE id = sqlc.arg('id') AND (ends_at IS NULL OR ends_at > sqlc.arg('at'));

-- name: InsertCashbackAward :exec
INSERT INTO cashback_awards (campaign_id, user_id, payment_transaction_id, debit_transaction_id, credit_transaction_id, amount_minor, campaign_day)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetAwardByPayment :one
SELECT campaign_id, amount_minor FROM cashback_awards WHERE payment_transaction_id = $1;

-- name: GetUserAccount :one
SELECT a.id, a.balance_minor, c.minor_unit
FROM accounts a
JOIN currencies c ON c.code = a.currency
WHERE a.user_id = sqlc.arg('user_id')::uuid AND a.kind = sqlc.arg('kind') AND a.currency = sqlc.arg('currency');

-- name: GetCampaignStatus :one
-- The campaign a user should see: the live one, else the latest ended one.
SELECT c.id, c.name, c.currency, cur.minor_unit, c.rate_bps, c.min_spend_minor, c.daily_cap_minor,
       a.balance_minor AS budget_remaining_minor,
       (c.ends_at IS NULL OR c.ends_at > sqlc.arg('at')::timestamptz)::bool AS active,
       COALESCE((
           SELECT SUM(ca.amount_minor) FROM cashback_awards ca
           WHERE ca.campaign_id = c.id AND ca.user_id = sqlc.arg('user_id')::uuid
             AND ca.campaign_day = (sqlc.arg('at')::timestamptz AT TIME ZONE c.timezone)::date
       ), 0)::bigint AS earned_today_minor,
       ((date_trunc('day', sqlc.arg('at')::timestamptz AT TIME ZONE c.timezone) + interval '1 day')
           AT TIME ZONE c.timezone)::timestamptz AS reset_at
FROM campaigns c
JOIN accounts a ON a.id = c.account_id
JOIN currencies cur ON cur.code = c.currency
WHERE c.currency = sqlc.arg('currency') AND c.starts_at <= sqlc.arg('at')::timestamptz
ORDER BY (c.ends_at IS NULL OR c.ends_at > sqlc.arg('at')::timestamptz) DESC, c.starts_at DESC
LIMIT 1;

-- name: ListCashbackTransactions :many
-- One page of a user's cash back account activity, newest first. Paged by
-- cursor like ListTransactions.
SELECT t.id, t.name, t.description, t.kind, t.amount_minor, t.currency, c.minor_unit, t.created_at
FROM transactions t
JOIN accounts a ON a.id = t.account_id
JOIN currencies c ON c.code = t.currency
WHERE a.user_id = sqlc.arg('user_id')::uuid AND a.kind = 'cashback' AND a.currency = sqlc.arg('currency')
  AND (t.created_at, t.id) < (COALESCE(sqlc.narg('before_created_at')::timestamptz, 'infinity'),
                              COALESCE(sqlc.narg('before_id')::uuid, 'ffffffff-ffff-ffff-ffff-ffffffffffff'))
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.arg('limit')::int;

-- name: ReconcileAwards :many
-- Awards whose two halves don't match their amount. Should always be empty.
SELECT ca.id
FROM cashback_awards ca
JOIN transactions d ON d.id = ca.debit_transaction_id
JOIN transactions cr ON cr.id = ca.credit_transaction_id
WHERE d.amount_minor <> -ca.amount_minor OR cr.amount_minor <> ca.amount_minor;

-- name: CampaignFlowImbalance :one
-- Funding + campaign account activity plus everything awarded. Double-entry
-- means this is always zero.
SELECT (
    COALESCE((SELECT SUM(t.amount_minor) FROM transactions t JOIN accounts a ON a.id = t.account_id
              WHERE a.kind IN ('funding', 'campaign')), 0)
  + COALESCE((SELECT SUM(amount_minor) FROM cashback_awards), 0)
)::bigint;

-- name: UpsertFundingAccount :one
INSERT INTO accounts (kind, currency) VALUES ('funding', $1)
ON CONFLICT (currency) WHERE kind = 'funding' DO UPDATE SET kind = EXCLUDED.kind
RETURNING id;

-- name: CreateCampaignAccount :one
INSERT INTO accounts (kind, currency) VALUES ('campaign', $1) RETURNING id;

-- name: CreateCampaign :one
INSERT INTO campaigns (name, account_id, currency, rate_bps, min_spend_minor, daily_cap_minor,
                       award_increment_minor, budget_minor, timezone, starts_at, ends_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id;

-- name: LockCampaign :one
SELECT c.id, c.name, c.account_id, c.currency, c.starts_at, c.ends_at, a.balance_minor
FROM campaigns c JOIN accounts a ON a.id = c.account_id
WHERE c.id = $1
FOR UPDATE OF c;

-- name: ListCampaigns :many
SELECT c.id, c.name, c.currency, cur.minor_unit, c.rate_bps, c.min_spend_minor, c.daily_cap_minor,
       c.budget_minor, a.balance_minor AS remaining_minor, c.timezone, c.starts_at, c.ends_at
FROM campaigns c
JOIN accounts a ON a.id = c.account_id
JOIN currencies cur ON cur.code = c.currency
ORDER BY c.starts_at DESC;

-- name: GetFundingAccount :one
SELECT id FROM accounts WHERE kind = 'funding' AND currency = $1;

-- name: ValidateTimezone :exec
-- Fails for a time zone name Postgres doesn't know.
SELECT now() AT TIME ZONE sqlc.arg('timezone')::text;
