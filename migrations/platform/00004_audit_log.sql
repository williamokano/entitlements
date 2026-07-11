-- +goose Up
-- Append-only audit trail shared by every module (who / what / when / why).
CREATE TABLE platform.audit_log (
    id         uuid PRIMARY KEY,
    actor      text NOT NULL,          -- user id, api-key id, or "system"
    tenant_id  uuid NOT NULL,          -- uuid.Nil for platform-wide actions
    action     text NOT NULL,          -- e.g. "tenant.suspended"
    resource   text NOT NULL,          -- e.g. "tenant/<uuid>"
    before     jsonb,                  -- state snapshot before the change (nullable)
    after      jsonb,                  -- state snapshot after the change (nullable)
    reason     text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL
);

CREATE INDEX audit_log_tenant_created_idx
    ON platform.audit_log (tenant_id, created_at DESC);

-- Enforce append-only at the database level. A trigger rejects UPDATE/DELETE
-- for every role including the table owner (a REVOKE would not bind the owner).
-- +goose StatementBegin
CREATE FUNCTION platform.audit_log_reject_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'platform.audit_log is append-only';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER audit_log_no_update
    BEFORE UPDATE ON platform.audit_log
    FOR EACH ROW EXECUTE FUNCTION platform.audit_log_reject_mutation();

CREATE TRIGGER audit_log_no_delete
    BEFORE DELETE ON platform.audit_log
    FOR EACH ROW EXECUTE FUNCTION platform.audit_log_reject_mutation();

-- +goose Down
DROP TABLE platform.audit_log;
DROP FUNCTION IF EXISTS platform.audit_log_reject_mutation();
