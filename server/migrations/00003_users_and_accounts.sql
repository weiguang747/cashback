-- +goose Up
CREATE TABLE users (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email      TEXT        NOT NULL UNIQUE CHECK (email = lower(email)),
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One balance per user, kind and currency. balance_minor must always equal
-- SUM(transactions.amount_minor) for the account, so every write updates
-- both in one database transaction.
CREATE TABLE accounts (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID        NOT NULL REFERENCES users (id),
    kind          TEXT        NOT NULL CHECK (kind IN ('main', 'cashback')),
    currency      CHAR(3)     NOT NULL REFERENCES currencies (code),
    balance_minor BIGINT      NOT NULL DEFAULT 0 CHECK (balance_minor >= 0),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, kind, currency),
    -- Target for the composite foreign key on transactions.
    UNIQUE (id, currency)
);

-- Fails if transactions already has rows: owners are never invented.
ALTER TABLE transactions ADD COLUMN account_id UUID NOT NULL;

-- A transaction's currency must match its account's currency.
ALTER TABLE transactions ADD CONSTRAINT transactions_account_fkey
    FOREIGN KEY (account_id, currency) REFERENCES accounts (id, currency);

DROP INDEX transactions_created_at_idx;
CREATE INDEX transactions_account_created_at_idx ON transactions (account_id, created_at DESC, id DESC);

-- +goose Down
DROP INDEX transactions_account_created_at_idx;
CREATE INDEX transactions_created_at_idx ON transactions (created_at DESC, id DESC);
ALTER TABLE transactions DROP CONSTRAINT transactions_account_fkey;
ALTER TABLE transactions DROP COLUMN account_id;
DROP TABLE accounts;
DROP TABLE users;
