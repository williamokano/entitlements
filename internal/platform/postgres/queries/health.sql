-- name: Ping :one
-- Trivial connectivity probe; also keeps the sqlc toolchain exercised until
-- the first real platform queries arrive (outbox, T-005).
SELECT 1 AS ok;
