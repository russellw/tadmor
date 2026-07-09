// Package mailer sends transactional email, such as a rendered document to a
// counterparty. Sending is off unless SMTP is configured: New returns a no-op
// Mailer when given an empty address, so a deployment that sets no SMTP
// environment (the demo) never sends anything and every attempt reports
// ErrNotConfigured. Supply an address and the same New returns a real sender.
// The only dependency is the standard library (net/smtp).
package mailer

import (
	"context"
	"errors"
	"log/slog"
)

// ErrNotConfigured is returned by the no-op Mailer when no SMTP transport has
// been configured. Callers should treat it as "email is not enabled here"
// rather than as a delivery failure.
var ErrNotConfigured = errors.New("mailer: email sending is not configured")

// Attachment is a file attached to a Message, e.g. a rendered PDF.
type Attachment struct {
	Filename    string
	ContentType string // MIME type, e.g. "application/pdf"
	Data        []byte
}

// Message is a single email to send. Body is plain UTF-8 text.
type Message struct {
	To          []string
	Subject     string
	Body        string
	Attachments []Attachment
}

// Mailer sends email. Send may ignore the context deadline depending on the
// transport (the stdlib SMTP client does), so callers should not rely on it to
// bound delivery time.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// Config selects and configures the transport. A zero Addr yields the no-op
// Mailer; any other value yields the SMTP sender.
type Config struct {
	Addr     string // SMTP server "host:port"; empty selects the no-op mailer
	Username string // SMTP auth username; empty sends without authentication
	Password string // SMTP auth password
	From     string // envelope + header From address
}

// New returns an SMTP-backed Mailer when cfg.Addr is set, otherwise a no-op
// Mailer that reports ErrNotConfigured on every Send. log may be nil.
func New(cfg Config, log *slog.Logger) Mailer {
	if cfg.Addr == "" {
		return noopMailer{log: log}
	}
	return &smtpMailer{cfg: cfg, log: log}
}

// noopMailer is the default: it sends nothing and reports that email is not
// configured, logging the intent so a misconfigured production is visible.
type noopMailer struct{ log *slog.Logger }

func (m noopMailer) Send(_ context.Context, msg Message) error {
	if m.log != nil {
		m.log.Info("email not sent: mailer not configured",
			"subject", msg.Subject, "recipients", len(msg.To), "attachments", len(msg.Attachments))
	}
	return ErrNotConfigured
}
