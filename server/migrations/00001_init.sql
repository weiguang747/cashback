-- +goose Up
-- The whole schema as of the first release. Earlier development migrations
-- were squashed into this one before anything reached production.
--
-- Money is an exact integer count of the currency's minor unit (sen, cents).
-- Never use REAL/DOUBLE PRECISION (inexact) or MONEY (depends on lc_monetary).

CREATE EXTENSION IF NOT EXISTS btree_gist;

-- Currencies the app can hold money in. minor_unit is the ISO 4217 number
-- of decimal places, so amounts can be stored as integers of the minor unit.
CREATE TABLE currencies (
    code       CHAR(3)  PRIMARY KEY CHECK (code ~ '^[A-Z]{3}$'),
    minor_unit SMALLINT NOT NULL CHECK (minor_unit BETWEEN 0 AND 4)
);

INSERT INTO currencies (code, minor_unit) VALUES
    ('IDR', 2),
    ('USD', 2),
    ('SGD', 2),
    ('MYR', 2),
    ('EUR', 2),
    ('JPY', 0);

CREATE TABLE users (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email      TEXT        NOT NULL UNIQUE CHECK (email = lower(email)),
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- User accounts (main, cashback: one per user, kind and currency) and system
-- accounts with no user (a campaign's budget; one funding account per
-- currency, which pays for campaigns). balance_minor must always equal
-- SUM(transactions.amount_minor) for the account, so every write updates
-- both in one database transaction.
CREATE TABLE accounts (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID        REFERENCES users (id),
    kind          TEXT        NOT NULL,
    currency      CHAR(3)     NOT NULL REFERENCES currencies (code),
    balance_minor BIGINT      NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT accounts_kind_check CHECK (kind IN ('main', 'cashback', 'campaign', 'funding')),
    CONSTRAINT accounts_owner_check CHECK ((kind IN ('main', 'cashback')) = (user_id IS NOT NULL)),
    -- Only the funding account may go negative: it is money the business put in.
    CONSTRAINT accounts_balance_check CHECK (kind = 'funding' OR balance_minor >= 0),
    UNIQUE (user_id, kind, currency),
    -- Target for the composite foreign keys on transactions and campaigns.
    UNIQUE (id, currency)
);
CREATE UNIQUE INDEX accounts_one_funding_per_currency ON accounts (currency) WHERE kind = 'funding';

CREATE TABLE transactions (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      UUID        NOT NULL,
    name            TEXT        NOT NULL,
    description     TEXT        NOT NULL,
    kind            TEXT        NOT NULL CHECK (kind IN ('in', 'out', 'cashback')),
    -- Signed: negative for money out.
    amount_minor    BIGINT      NOT NULL CHECK (amount_minor <> 0),
    currency        CHAR(3)     NOT NULL REFERENCES currencies (code),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- The client's Idempotency-Key for transactions created through the API.
    -- Redis is the first line against duplicate payments; the unique
    -- constraint below is the backstop if Redis loses a key. Other rows keep
    -- NULL (NULLs never conflict).
    idempotency_key TEXT,
    CONSTRAINT transactions_sign_matches_kind CHECK ((kind = 'out') = (amount_minor < 0)),
    -- A transaction's currency must match its account's currency.
    CONSTRAINT transactions_account_fkey FOREIGN KEY (account_id, currency) REFERENCES accounts (id, currency),
    CONSTRAINT transactions_account_idempotency_key UNIQUE (account_id, idempotency_key)
);
-- Lists and their cursor paging: one account's rows, newest first.
CREATE INDEX transactions_account_created_at_idx ON transactions (account_id, created_at DESC, id DESC);

-- Cash back campaigns, recorded double-entry. Each campaign's budget lives
-- in a system-owned "campaign" account, funded at launch from the
-- currency's "funding" account. Every award moves money from the campaign
-- account to the user's cash back account, so every movement nets to zero.
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
DROP TABLE transactions;
DROP TABLE accounts;
DROP TABLE users;
DROP TABLE currencies;
