// Package postgres is the authentication module's driven adapter: implements
// the domain user and refresh-token repositories over the platform pgx pool,
// routing queries through platform postgres.Q so they join the ambient
// UnitOfWork transaction.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/williamokano/entitlements/internal/modules/authentication/internal/domain"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	platformpg "github.com/williamokano/entitlements/internal/platform/postgres"
)

// uniqueViolation is the Postgres SQLSTATE for a unique-constraint violation.
const uniqueViolation = "23505"

// Repo is a Postgres-backed implementation of the authentication repositories.
type Repo struct {
	pool *pgxpool.Pool
}

// New builds a Repo.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// CreateUser inserts a user, mapping an email uniqueness violation to a conflict.
func (r *Repo) CreateUser(ctx context.Context, u *domain.User) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO authn.users (id, email, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		u.ID, u.Email, string(u.Status), u.CreatedAt, u.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("email already registered")
		}
		return fmt.Errorf("authn: insert user: %w", err)
	}
	return nil
}

// GetUserByEmail returns a non-deleted user by normalized email.
func (r *Repo) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.getUser(ctx, `WHERE email = $1 AND status <> 'deleted'`, email)
}

// GetUserByID returns a non-deleted user by id.
func (r *Repo) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return r.getUser(ctx, `WHERE id = $1 AND status <> 'deleted'`, id)
}

func (r *Repo) getUser(ctx context.Context, where string, arg any) (*domain.User, error) {
	var (
		u      domain.User
		status string
	)
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT id, email, status, created_at, updated_at FROM authn.users `+where, arg).
		Scan(&u.ID, &u.Email, &status, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("authn: query user: %w", err)
	}
	u.Status = domain.Status(status)
	return &u, nil
}

// CreateCredential inserts a credential factor.
func (r *Repo) CreateCredential(ctx context.Context, c *domain.Credential) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO authn.credentials (id, user_id, type, secret, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		c.ID, c.UserID, string(c.Type), c.Secret, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("authn: insert credential: %w", err)
	}
	return nil
}

// GetCredential returns a user's credential of the given factor type.
func (r *Repo) GetCredential(ctx context.Context, userID uuid.UUID, typ domain.FactorType) (*domain.Credential, error) {
	var c domain.Credential
	var t string
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT id, user_id, type, secret, created_at, updated_at
		 FROM authn.credentials WHERE user_id = $1 AND type = $2`, userID, string(typ)).
		Scan(&c.ID, &c.UserID, &t, &c.Secret, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("credential not found")
	}
	if err != nil {
		return nil, fmt.Errorf("authn: query credential: %w", err)
	}
	c.Type = domain.FactorType(t)
	return &c, nil
}

// Insert stores a refresh token.
func (r *Repo) Insert(ctx context.Context, t *domain.RefreshToken) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO authn.refresh_tokens (id, user_id, family_id, parent_id, token_hash, status, issued_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		t.ID, t.UserID, t.FamilyID, t.ParentID, t.Hash, string(t.Status), t.IssuedAt, t.ExpiresAt)
	if err != nil {
		return fmt.Errorf("authn: insert refresh token: %w", err)
	}
	return nil
}

// FindByHash returns a refresh token by its stored hash.
func (r *Repo) FindByHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	var (
		t      domain.RefreshToken
		status string
	)
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT id, user_id, family_id, parent_id, token_hash, status, issued_at, expires_at
		 FROM authn.refresh_tokens WHERE token_hash = $1`, hash).
		Scan(&t.ID, &t.UserID, &t.FamilyID, &t.ParentID, &t.Hash, &status, &t.IssuedAt, &t.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("refresh token not found")
	}
	if err != nil {
		return nil, fmt.Errorf("authn: query refresh token: %w", err)
	}
	t.Status = domain.RefreshStatus(status)
	return &t, nil
}

// MarkRotated marks a refresh token as rotated (its successor was issued).
func (r *Repo) MarkRotated(ctx context.Context, id uuid.UUID) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE authn.refresh_tokens SET status = 'rotated' WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("authn: mark refresh rotated: %w", err)
	}
	return nil
}

// RevokeFamily revokes every still-active token in a family.
func (r *Repo) RevokeFamily(ctx context.Context, familyID uuid.UUID) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE authn.refresh_tokens SET status = 'revoked'
		 WHERE family_id = $1 AND status <> 'revoked'`, familyID)
	if err != nil {
		return fmt.Errorf("authn: revoke refresh family: %w", err)
	}
	return nil
}

// RevokeUser revokes every still-active token for a user (all sessions).
func (r *Repo) RevokeUser(ctx context.Context, userID uuid.UUID) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE authn.refresh_tokens SET status = 'revoked'
		 WHERE user_id = $1 AND status <> 'revoked'`, userID)
	if err != nil {
		return fmt.Errorf("authn: revoke user refresh tokens: %w", err)
	}
	return nil
}
