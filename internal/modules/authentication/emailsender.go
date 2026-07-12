package authentication

import (
	"context"
	"log/slog"

	"github.com/williamokano/entitlements/internal/modules/authentication/ports"
)

// loggingEmailSender is the default ports.EmailSender: it logs the message
// (including the verification/reset link) instead of delivering it, so the flows
// are usable in development without a mail provider. Production deployments
// inject a real sender via WithEmailSender.
type loggingEmailSender struct{ logger *slog.Logger }

// NewLoggingEmailSender builds a dev email sender that logs each message.
func NewLoggingEmailSender(logger *slog.Logger) ports.EmailSender {
	return &loggingEmailSender{logger: logger}
}

func (s *loggingEmailSender) Send(ctx context.Context, msg ports.Email) error {
	s.logger.InfoContext(ctx, "dev email (not delivered)",
		"to", msg.To, "subject", msg.Subject, "body", msg.TextBody)
	return nil
}
