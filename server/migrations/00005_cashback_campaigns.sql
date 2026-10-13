-- +goose Up
-- Cash back campaigns, recorded double-entry. Each campaign's budget lives
-- in a system-owned "campaign" account, funded at launch from a per-currency
-- "funding" account. Every award moves money from the campaign account to
-- the user's cash back account, so every movement nets to zero.

CREATE EXTENSION IF NOT EXISTS btree_gist;

-- System accounts (campaign budgets, funding) have no user.
ALTER TABLE accounts ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE accounts DROP CONSTRAINT accounts_kind_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_kind_check
    CHECK (kind IN ('main', 'cashback', 'campaign', 'funding'));
ALTER TABLE accounts ADD CONSTRAINT accounts_owner_check
    CHECK ((kind IN ('main', 'cashback')) = (user_id IS NOT NULL));
-- Only the funding account may go negative: it is money the business put in.
ALTER TABLE accounts DROP CONSTRAINT accounts_balance_minor_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_balance_check CHECK (kind = 'funding' OR balance_minor >= 0);
CREATE UNIQUE INDEX accounts_one_funding_per_currency ON accounts (currency) WHERE kind = 'funding';

CREATE TABLE campaigns (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name                  TEXT        NOT NULL,
    -- Holds the remaining budget.
    account_id            UUID        NOT NULL UNIQUE,
    currency              CHAR(3)     NOT NULL,
    rate_bps              INT         NOT NULL CHECK (rate_bps BETWEEN 1 AND 10000),
    min_spend_minor       BIGINT      NOT NULL CHECK (min_spend_minor >= 0),
    daily_cap_minor       BIGINT      NOT NULL CHECK (daily_cap_minor > 0),
    -- Awards are rounded down to a multiple of this (100 = whole rupiah).
    award_increment_minor BIGINT      NOT NULL DEFAULT 1 CHECK (award_increment_minor > 0),
    -- The budget as launched, for reporting. What's left is the account balance.
    budget_minor          BIGINT      NOT NULL CHECK (budget_minor > 0),
    -- Defines "per day" for the daily cap.
    timezone              TEXT        NOT NULL DEFAULT 'Asia/Jakarta',
    starts_at             TIMESTAMPTZ NOT NULL,
    -- NULL while open; set when ended by hand or when the budget runs out.
    ends_at               TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (account_id, currency) REFERENCES accounts (id, currency),
    CHECK (ends_at IS NULL OR ends_at > starts_at),
    -- At most one campaign per currency at any moment; later ones can be scheduled.
    CONSTRAINT campaigns_no_overlap
        EXCLUDE USING gist (currency WITH =, tstzrange(starts_at, COALESCE(ends_at, 'infinity')) WITH &&)
);

CREATE TABLE cashback_awards (
    id                     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id            UUID        NOT NULL REFERENCES campaigns (id),
    user_id                UUID        NOT NULL REFERENCES users (id),
    -- One award per payment.
    payment_transaction_id UUID        NOT NULL UNIQUE REFERENCES transactions (id),
    -- The two halves of the award: out of the campaign account, into the user's cash back account.
    debit_transaction_id   UUID        NOT NULL UNIQUE REFERENCES transactions (id),
    credit_transaction_id  UUID        NOT NULL UNIQUE REFERENCES transactions (id),
    amount_minor           BIGINT      NOT NULL CHECK (amount_minor > 0),
    -- The payment's calendar day in the campaign's time zone.
    campaign_day           DATE        NOT NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX cashback_awards_daily_idx ON cashback_awards (campaign_id, user_id, campaign_day);

-- +goose Down
DROP TABLE cashback_awards;
DROP TABLE campaigns;
DROP INDEX accounts_one_funding_per_currency;
ALTER TABLE accounts DROP CONSTRAINT accounts_balance_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_balance_minor_check CHECK (balance_minor >= 0);
ALTER TABLE accounts DROP CONSTRAINT accounts_owner_check;
ALTER TABLE accounts DROP CONSTRAINT accounts_kind_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_kind_check CHECK (kind IN ('main', 'cashback'));
ALTER TABLE accounts ALTER COLUMN user_id SET NOT NULL;
