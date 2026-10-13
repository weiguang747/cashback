-- +goose Up
-- Money is an exact integer count of the currency's minor unit (sen, cents).
-- Never use REAL/DOUBLE PRECISION (inexact) or MONEY (depends on lc_monetary).
CREATE TABLE transactions (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT        NOT NULL,
    description  TEXT        NOT NULL,
    kind         TEXT        NOT NULL CHECK (kind IN ('in', 'out', 'cashback')),
    -- Signed: negative for money out.
    amount_minor BIGINT      NOT NULL CHECK (amount_minor <> 0),
    currency     CHAR(3)     NOT NULL REFERENCES currencies (code),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT transactions_sign_matches_kind CHECK ((kind = 'out') = (amount_minor < 0))
);

CREATE INDEX transactions_created_at_idx ON transactions (created_at DESC, id DESC);

-- +goose Down
DROP TABLE transactions;
