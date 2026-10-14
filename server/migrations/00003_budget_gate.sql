-- +goose Up
-- How a campaign's budget is enforced (see internal/budgetgate):
--   db      awards under the campaign row lock, inside the payment (as before)
--   shadow  as db, and mirrors each award into Redis to compare decisions
--   redis   reserves the award in Redis; a worker credits it from cashback_pending
-- gate_degraded_at is set when the Redis gate keeps failing: payments then
-- fall back to the lock (after a grace period) until the reconciler reseeds
-- Redis and clears it.
ALTER TABLE campaigns
    ADD COLUMN budget_gate TEXT NOT NULL DEFAULT 'db'
        CONSTRAINT campaigns_budget_gate_check CHECK (budget_gate IN ('db', 'shadow', 'redis')),
    ADD COLUMN gate_degraded_at TIMESTAMPTZ;

-- Awards reserved by the Redis gate and not yet credited: the outbox that
-- the award worker drains. A row is inserted in the payment's transaction
-- and deleted in the same transaction that writes its cashback_awards row,
-- so an award is always in exactly one of the two tables.
--
-- The budget left is the campaign account balance minus these rows.
CREATE TABLE cashback_pending (
    -- One award per payment, like cashback_awards.
    payment_transaction_id UUID        PRIMARY KEY REFERENCES transactions (id),
    -- No foreign key to campaigns on purpose: checking it takes a KEY SHARE
    -- lock on the campaign row, which every awarding payment would then
    -- contend on, the very lock the gate exists to avoid. Campaigns are
    -- never deleted.
    campaign_id            UUID        NOT NULL,
    user_id                UUID        NOT NULL REFERENCES users (id),
    cashback_account_id    UUID        NOT NULL,
    currency               CHAR(3)     NOT NULL,
    amount_minor           BIGINT      NOT NULL CHECK (amount_minor > 0),
    -- The payment's calendar day in the campaign's time zone (for the daily cap).
    campaign_day           DATE        NOT NULL,
    credit_description     TEXT        NOT NULL,
    created_at             TIMESTAMPTZ NOT NULL,
    -- The worker counts failed tries; failed_at is set when the award can
    -- never be written (the campaign account can't cover it). Such a row
    -- stays, still counted against the budget, until resolved by hand.
    attempts               INT         NOT NULL DEFAULT 0,
    failed_at              TIMESTAMPTZ,
    FOREIGN KEY (cashback_account_id, currency) REFERENCES accounts (id, currency)
);
-- Daily cap and budget sums (per campaign, user and day; per campaign).
CREATE INDEX cashback_pending_daily_idx ON cashback_pending (campaign_id, user_id, campaign_day);
-- The pending amount shown with a user's cash back balance.
CREATE INDEX cashback_pending_account_idx ON cashback_pending (cashback_account_id);
-- The worker claims the oldest rows that haven't failed.
CREATE INDEX cashback_pending_queue_idx ON cashback_pending (created_at) WHERE failed_at IS NULL;

-- +goose Down
DROP TABLE cashback_pending;
ALTER TABLE campaigns DROP COLUMN gate_degraded_at, DROP COLUMN budget_gate;
