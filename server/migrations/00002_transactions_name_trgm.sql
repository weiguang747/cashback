-- +goose NO TRANSACTION
-- +goose Up
-- Searching transactions by name (?q= on /v1/transactions) is an ILIKE
-- '%text%', which no btree can serve: for a rare term the list query walks
-- the user's whole history. A trigram index finds the matches directly.
-- SearchTransactions is always planned with the actual search text (see
-- pgStore.SearchTransactions), so short or common terms still walk the
-- (account_id, created_at, id) index instead.
--
-- CONCURRENTLY, so building it on a large table doesn't block payments;
-- that needs NO TRANSACTION.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX CONCURRENTLY IF NOT EXISTS transactions_name_trgm_idx ON transactions USING gin (name gin_trgm_ops);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS transactions_name_trgm_idx;
DROP EXTENSION IF EXISTS pg_trgm;
