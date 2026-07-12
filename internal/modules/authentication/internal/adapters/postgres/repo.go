// Package postgres is the authentication module's driven adapter: implements
// the domain user and refresh-token repositories over the platform pgx pool,
// routing queries through platform postgres.Q so they join the ambient
// UnitOfWork transaction.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

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
		u        domain.User
		status   string
		verified *time.Time
	)
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT id, email, status, email_verified_at, created_at, updated_at FROM authn.users `+where, arg).
		Scan(&u.ID, &u.Email, &status, &verified, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("authn: query user: %w", err)
	}
	u.Status = domain.Status(status)
	u.EmailVerifiedAt = verified
	return &u, nil
}

// MarkEmailVerified sets the user's email_verified_at.
func (r *Repo) MarkEmailVerified(ctx context.Context, userID uuid.UUID, verifiedAt time.Time) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE authn.users SET email_verified_at = COALESCE(email_verified_at, $2), updated_at = $2 WHERE id = $1`,
		userID, verifiedAt)
	if err != nil {
		return fmt.Errorf("authn: mark email verified: %w", err)
	}
	return nil
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

// UpdateCredentialSecret replaces a user's credential secret of the given type.
func (r *Repo) UpdateCredentialSecret(ctx context.Context, userID uuid.UUID, typ domain.FactorType, secret string, updatedAt time.Time) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE authn.credentials SET secret = $3, updated_at = $4 WHERE user_id = $1 AND type = $2`,
		userID, string(typ), secret, updatedAt)
	if err != nil {
		return fmt.Errorf("authn: update credential secret: %w", err)
	}
	return nil
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

// RevokeFamiliesExcept revokes every still-active family of the user except keep.
func (r *Repo) RevokeFamiliesExcept(ctx context.Context, userID, keep uuid.UUID) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE authn.refresh_tokens SET status = 'revoked'
		 WHERE user_id = $1 AND family_id <> $2 AND status <> 'revoked'`, userID, keep)
	if err != nil {
		return fmt.Errorf("authn: revoke other families: %w", err)
	}
	return nil
}

// ListSessions returns the user's active sessions (one per live family): the
// still-active head token of each family.
func (r *Repo) ListSessions(ctx context.Context, userID uuid.UUID) ([]domain.Session, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT family_id, issued_at, expires_at
		 FROM authn.refresh_tokens
		 WHERE user_id = $1 AND status = 'active'
		 ORDER BY issued_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("authn: list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []domain.Session
	for rows.Next() {
		var s domain.Session
		if err := rows.Scan(&s.ID, &s.IssuedAt, &s.ExpiresAt); err != nil {
			return nil, fmt.Errorf("authn: scan session: %w", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("authn: iterate sessions: %w", err)
	}
	return sessions, nil
}

// AuthTokens is a Postgres-backed domain.AuthTokenRepository for single-use
// verification/reset tokens. It is a separate type from Repo so its Insert and
// FindByHash do not collide with the refresh-token methods.
type AuthTokens struct {
	pool *pgxpool.Pool
}

// NewAuthTokens builds an AuthTokens repository.
func NewAuthTokens(pool *pgxpool.Pool) *AuthTokens { return &AuthTokens{pool: pool} }

// Insert stores a single-use token.
func (r *AuthTokens) Insert(ctx context.Context, t *domain.AuthToken) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO authn.auth_tokens (id, user_id, purpose, token_hash, expires_at, consumed_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		t.ID, t.UserID, string(t.Purpose), t.Hash, t.ExpiresAt, t.ConsumedAt, t.CreatedAt)
	if err != nil {
		return fmt.Errorf("authn: insert auth token: %w", err)
	}
	return nil
}

// FindByHash returns a single-use token by purpose and hash.
func (r *AuthTokens) FindByHash(ctx context.Context, purpose domain.TokenPurpose, hash string) (*domain.AuthToken, error) {
	var (
		t        domain.AuthToken
		purp     string
		consumed *time.Time
	)
	err := platformpg.Q(ctx, r.pool).QueryRow(ctx,
		`SELECT id, user_id, purpose, token_hash, expires_at, consumed_at, created_at
		 FROM authn.auth_tokens WHERE purpose = $1 AND token_hash = $2`, string(purpose), hash).
		Scan(&t.ID, &t.UserID, &purp, &t.Hash, &t.ExpiresAt, &consumed, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("auth token not found")
	}
	if err != nil {
		return nil, fmt.Errorf("authn: query auth token: %w", err)
	}
	t.Purpose = domain.TokenPurpose(purp)
	t.ConsumedAt = consumed
	return &t, nil
}

// Consume marks a single-use token consumed, but only if it is not already
// consumed — the affected-row count guarantees single use even under a race.
func (r *AuthTokens) Consume(ctx context.Context, id uuid.UUID, consumedAt time.Time) error {
	tag, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE authn.auth_tokens SET consumed_at = $2 WHERE id = $1 AND consumed_at IS NULL`,
		id, consumedAt)
	if err != nil {
		return fmt.Errorf("authn: consume auth token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.Unauthorized("token is invalid or expired")
	}
	return nil
}

// APIKeys is a Postgres-backed domain.APIKeyRepository.
type APIKeys struct {
	pool *pgxpool.Pool
}

// NewAPIKeys builds an APIKeys repository.
func NewAPIKeys(pool *pgxpool.Pool) *APIKeys { return &APIKeys{pool: pool} }

// Insert stores a new API key.
func (r *APIKeys) Insert(ctx context.Context, k *domain.APIKey) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`INSERT INTO authn.api_keys (id, tenant_id, name, prefix, secret_hash, scopes, last_used_at, revoked_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		k.ID, k.TenantID, k.Name, k.Prefix, k.SecretHash, k.Scopes, k.LastUsedAt, k.RevokedAt, k.CreatedAt)
	if err != nil {
		return fmt.Errorf("authn: insert api key: %w", err)
	}
	return nil
}

// GetByPrefix returns an API key by its public prefix (revoked or not — the
// caller decides how to treat a revoked key).
func (r *APIKeys) GetByPrefix(ctx context.Context, prefix string) (*domain.APIKey, error) {
	k, err := r.scanOne(ctx, `WHERE prefix = $1`, prefix)
	if err != nil {
		return nil, err
	}
	return k, nil
}

// ListByTenant returns a tenant's non-revoked API keys, newest first.
func (r *APIKeys) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*domain.APIKey, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT id, tenant_id, name, prefix, secret_hash, scopes, last_used_at, revoked_at, created_at
		 FROM authn.api_keys WHERE tenant_id = $1 AND revoked_at IS NULL ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("authn: list api keys: %w", err)
	}
	defer rows.Close()

	var keys []*domain.APIKey
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("authn: iterate api keys: %w", err)
	}
	return keys, nil
}

// Revoke marks a tenant's API key revoked. It is scoped by tenant so one tenant
// cannot revoke another's key.
func (r *APIKeys) Revoke(ctx context.Context, id, tenantID uuid.UUID, at time.Time) error {
	tag, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE authn.api_keys SET revoked_at = $3 WHERE id = $1 AND tenant_id = $2 AND revoked_at IS NULL`,
		id, tenantID, at)
	if err != nil {
		return fmt.Errorf("authn: revoke api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("api key not found")
	}
	return nil
}

// TouchLastUsed records the most recent use of a key (best-effort telemetry).
func (r *APIKeys) TouchLastUsed(ctx context.Context, id uuid.UUID, at time.Time) error {
	_, err := platformpg.Q(ctx, r.pool).Exec(ctx,
		`UPDATE authn.api_keys SET last_used_at = $2 WHERE id = $1`, id, at)
	if err != nil {
		return fmt.Errorf("authn: touch api key: %w", err)
	}
	return nil
}

func (r *APIKeys) scanOne(ctx context.Context, where string, args ...any) (*domain.APIKey, error) {
	rows, err := platformpg.Q(ctx, r.pool).Query(ctx,
		`SELECT id, tenant_id, name, prefix, secret_hash, scopes, last_used_at, revoked_at, created_at
		 FROM authn.api_keys `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("authn: query api key: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("authn: query api key: %w", err)
		}
		return nil, apperr.NotFound("api key not found")
	}
	return scanKey(rows)
}

func scanKey(rows pgx.Rows) (*domain.APIKey, error) {
	var k domain.APIKey
	if err := rows.Scan(&k.ID, &k.TenantID, &k.Name, &k.Prefix, &k.SecretHash, &k.Scopes,
		&k.LastUsedAt, &k.RevokedAt, &k.CreatedAt); err != nil {
		return nil, fmt.Errorf("authn: scan api key: %w", err)
	}
	return &k, nil
}
