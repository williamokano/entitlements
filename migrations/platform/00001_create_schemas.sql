-- +goose Up
-- One Postgres schema per module (see docs/PLAN.md "Enforced rules").
-- Note: the authentication/authorization modules use the short schema names
-- authn/authz because AUTHORIZATION is a reserved word in SQL.
CREATE SCHEMA IF NOT EXISTS platform;
CREATE SCHEMA IF NOT EXISTS tenant;
CREATE SCHEMA IF NOT EXISTS authn;
CREATE SCHEMA IF NOT EXISTS authz;
CREATE SCHEMA IF NOT EXISTS catalog;
CREATE SCHEMA IF NOT EXISTS subscription;
CREATE SCHEMA IF NOT EXISTS entitlements;
CREATE SCHEMA IF NOT EXISTS billing;

-- +goose Down
DROP SCHEMA IF EXISTS billing CASCADE;
DROP SCHEMA IF EXISTS entitlements CASCADE;
DROP SCHEMA IF EXISTS subscription CASCADE;
DROP SCHEMA IF EXISTS catalog CASCADE;
DROP SCHEMA IF EXISTS authz CASCADE;
DROP SCHEMA IF EXISTS authn CASCADE;
DROP SCHEMA IF EXISTS tenant CASCADE;
DROP SCHEMA IF EXISTS platform CASCADE;
