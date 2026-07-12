package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/authentication/internal/domain"
	"github.com/williamokano/entitlements/internal/modules/authentication/internal/security"
	"github.com/williamokano/entitlements/internal/modules/authentication/ports"
	"github.com/williamokano/entitlements/internal/platform/apperr"
	"github.com/williamokano/entitlements/internal/platform/audit"
	"github.com/williamokano/entitlements/internal/platform/events"
)

// invalidToken is the constant error returned for any unusable single-use token
// (unknown, already consumed, or expired) so callers cannot probe token state.
var invalidToken = apperr.Unauthorized("token is invalid or expired")

// RequestEmailVerification issues a verification token for the email and sends
// its link. It is indistinguishable for unknown or already-verified emails: the
// caller always sees success, so registration cannot be used to enumerate users.
func (s *Service) RequestEmailVerification(ctx context.Context, email string) error {
	return s.issueAndSend(ctx, email, domain.PurposeEmailVerification, s.verificationTTL,
		"Verify your email", "verify-email", "Confirm your email with this link: %s")
}

// RequestPasswordReset issues a reset token for the email and sends its link. It
// is indistinguishable for unknown emails.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	return s.issueAndSend(ctx, email, domain.PurposePasswordReset, s.resetTTL,
		"Reset your password", "reset-password", "Reset your password with this link: %s")
}

// issueAndSend looks up the user, issues a single-use token, and emails the
// rendered link. A missing user is silently ignored (same success result).
func (s *Service) issueAndSend(ctx context.Context, email string, purpose domain.TokenPurpose, ttl time.Duration, subject, path, bodyTemplate string) error {
	normalized, err := domain.NormalizeEmail(email)
	if err != nil {
		return nil // do not reveal that the address was malformed
	}

	var (
		toSend string
		rawTok string
	)
	err = s.uow.Do(ctx, func(ctx context.Context) error {
		user, err := s.users.GetUserByEmail(ctx, normalized)
		if err != nil {
			if apperr.KindOf(err) == apperr.KindNotFound {
				return nil // unknown email: no-op, indistinguishable
			}
			return err
		}
		now := s.clk.Now().UTC()
		raw, err := security.GenerateOpaqueToken()
		if err != nil {
			return err
		}
		tok := domain.NewAuthToken(s.ids.New(), user.ID, purpose, raw, now, now.Add(ttl))
		if err := s.authTokens.Insert(ctx, tok); err != nil {
			return err
		}
		toSend = user.Email
		rawTok = raw
		return nil
	})
	if err != nil {
		return err
	}
	if rawTok == "" {
		return nil // no user; nothing to send
	}
	link := s.buildLink(path, rawTok)
	return s.sender.Send(ctx, ports.Email{
		To:       toSend,
		Subject:  subject,
		TextBody: fmt.Sprintf(bodyTemplate, link),
	})
}

// VerifyEmail redeems a verification token and marks the user's email verified.
// The token is single-use (consumed in the same transaction).
func (s *Service) VerifyEmail(ctx context.Context, rawToken string) error {
	return s.uow.Do(ctx, func(ctx context.Context) error {
		tok, err := s.redeem(ctx, domain.PurposeEmailVerification, rawToken)
		if err != nil {
			return err
		}
		now := s.clk.Now().UTC()
		if err := s.users.MarkEmailVerified(ctx, tok.UserID, now); err != nil {
			return err
		}
		_, err = s.outbox.Publish(ctx, events.EventInput{
			Module:  "authentication",
			Type:    ports.EventEmailVerified,
			Payload: ports.EmailVerified{UserID: tok.UserID},
		})
		return err
	})
}

// ResetPassword redeems a reset token, sets a new password, and revokes every
// live session for the user (a recovery implies the account may be compromised).
func (s *Service) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	hash, err := security.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.uow.Do(ctx, func(ctx context.Context) error {
		tok, err := s.redeem(ctx, domain.PurposePasswordReset, rawToken)
		if err != nil {
			return err
		}
		now := s.clk.Now().UTC()
		if err := s.users.UpdateCredentialSecret(ctx, tok.UserID, domain.FactorPassword, hash, now); err != nil {
			return err
		}
		if err := s.refreshRaw.RevokeUser(ctx, tok.UserID); err != nil {
			return err
		}
		if err := s.recordAudit(ctx, tok.UserID, "authn.password.reset", "password recovery"); err != nil {
			return err
		}
		_, err = s.outbox.Publish(ctx, events.EventInput{
			Module:  "authentication",
			Type:    ports.EventPasswordReset,
			Payload: ports.PasswordReset{UserID: tok.UserID},
		})
		return err
	})
}

// ChangePassword verifies the user's current password and sets a new one. It
// keeps the caller's current session (identified by keepSession) alive and
// revokes all others, and records an audit entry.
func (s *Service) ChangePassword(ctx context.Context, userID, keepSession uuid.UUID, currentPassword, newPassword string) error {
	newHash, err := security.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.uow.Do(ctx, func(ctx context.Context) error {
		cred, err := s.users.GetCredential(ctx, userID, domain.FactorPassword)
		if err != nil {
			return invalidCredentials
		}
		ok, err := security.VerifyPassword(currentPassword, cred.Secret)
		if err != nil || !ok {
			return invalidCredentials
		}
		now := s.clk.Now().UTC()
		if err := s.users.UpdateCredentialSecret(ctx, userID, domain.FactorPassword, newHash, now); err != nil {
			return err
		}
		if err := s.refreshRaw.RevokeFamiliesExcept(ctx, userID, keepSession); err != nil {
			return err
		}
		if err := s.recordAudit(ctx, userID, "authn.password.changed", "password change"); err != nil {
			return err
		}
		_, err = s.outbox.Publish(ctx, events.EventInput{
			Module:  "authentication",
			Type:    ports.EventPasswordChanged,
			Payload: ports.PasswordChanged{UserID: userID},
		})
		return err
	})
}

// ListSessions returns the user's active sessions.
func (s *Service) ListSessions(ctx context.Context, userID uuid.UUID) ([]domain.Session, error) {
	return s.refreshRaw.ListSessions(ctx, userID)
}

// RevokeOtherSessions revokes every session of the user except keepSession
// (the caller's current session).
func (s *Service) RevokeOtherSessions(ctx context.Context, userID, keepSession uuid.UUID) error {
	return s.uow.Do(ctx, func(ctx context.Context) error {
		return s.refreshRaw.RevokeFamiliesExcept(ctx, userID, keepSession)
	})
}

// redeem loads a single-use token of the given purpose, verifies it is
// redeemable, and consumes it — all within the ambient transaction. Any failure
// yields the same opaque error.
func (s *Service) redeem(ctx context.Context, purpose domain.TokenPurpose, rawToken string) (*domain.AuthToken, error) {
	tok, err := s.authTokens.FindByHash(ctx, purpose, domain.HashToken(rawToken))
	if err != nil {
		if apperr.KindOf(err) == apperr.KindNotFound {
			return nil, invalidToken
		}
		return nil, err
	}
	now := s.clk.Now().UTC()
	if !tok.Redeemable(now) {
		return nil, invalidToken
	}
	if err := s.authTokens.Consume(ctx, tok.ID, now); err != nil {
		return nil, err
	}
	return tok, nil
}

func (s *Service) recordAudit(ctx context.Context, userID uuid.UUID, action, reason string) error {
	if s.audit == nil {
		return nil
	}
	return s.audit.Record(ctx, audit.Entry{
		Actor:    userID.String(),
		Action:   action,
		Resource: "authn/user/" + userID.String(),
		Reason:   reason,
	})
}

func (s *Service) buildLink(path, rawToken string) string {
	base := strings.TrimSuffix(s.baseURL, "/")
	return fmt.Sprintf("%s/%s?token=%s", base, path, url.QueryEscape(rawToken))
}
