-- +goose Up
-- The email a member was invited by. A membership is only ever created by
-- accepting an invitation, which already carries the (normalized) email — so the
-- tenant context keeps its own view of member identity and never has to ask the
-- authentication module who a user_id belongs to.
--
-- The default keeps rows that predate this column valid; they simply have no
-- email to show and callers fall back to the user id.
ALTER TABLE tenant.memberships ADD COLUMN email text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE tenant.memberships DROP COLUMN email;
