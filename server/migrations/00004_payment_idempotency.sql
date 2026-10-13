-- +goose Up
-- The client's Idempotency-Key for transactions created through the API.
-- Redis is the first line against duplicate payments; this constraint is
-- the backstop if Redis loses a key. Seeded rows keep NULL (NULLs never
-- conflict).
ALTER TABLE transactions ADD COLUMN idempotency_key TEXT;
ALTER TABLE transactions ADD CONSTRAINT transactions_account_idempotency_key UNIQUE (account_id, idempotency_key);

-- +goose Down
ALTER TABLE transactions DROP CONSTRAINT transactions_account_idempotency_key;
ALTER TABLE transactions DROP COLUMN idempotency_key;
