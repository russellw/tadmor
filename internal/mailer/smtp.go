package mailer

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

// smtpMailer delivers messages through an SMTP server using the standard
// library's client, which upgrades to STARTTLS when the server advertises it.
type smtpMailer struct {
	cfg Config
	log *slog.Logger
}

func (m *smtpMailer) Send(_ context.Context, msg Message) error {
	if len(msg.To) == 0 {
		return errors.New("mailer: message has no recipients")
	}
	raw, err := build(m.cfg.From, msg)
	if err != nil {
		return fmt.Errorf("mailer: build message: %w", err)
	}
	host, _, err := net.SplitHostPort(m.cfg.Addr)
	if err != nil {
		return fmt.Errorf("mailer: parse SMTP address %q: %w", m.cfg.Addr, err)
	}
	var auth smtp.Auth
	if m.cfg.Username != "" {
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, host)
	}
	if err := smtp.SendMail(m.cfg.Addr, auth, m.cfg.From, msg.To, raw); err != nil {
		return fmt.Errorf("mailer: send: %w", err)
	}
	if m.log != nil {
		m.log.Info("email sent", "subject", msg.Subject, "recipients", len(msg.To))
	}
	return nil
}

// build assembles an RFC 5322 message. With no attachments it is a plain-text
// message; otherwise a multipart/mixed body with the text as the first part
// and each attachment base64-encoded after it.
func build(from string, msg Message) ([]byte, error) {
	var buf bytes.Buffer
	writeHeader(&buf, "From", from)
	writeHeader(&buf, "To", strings.Join(msg.To, ", "))
	writeHeader(&buf, "Date", time.Now().Format(time.RFC1123Z))
	writeHeader(&buf, "Subject", mime.QEncoding.Encode("utf-8", msg.Subject))
	writeHeader(&buf, "MIME-Version", "1.0")

	if len(msg.Attachments) == 0 {
		writeHeader(&buf, "Content-Type", "text/plain; charset=utf-8")
		buf.WriteString("\r\n")
		buf.WriteString(msg.Body)
		return buf.Bytes(), nil
	}

	mw := multipart.NewWriter(&buf)
	writeHeader(&buf, "Content-Type", "multipart/mixed; boundary="+mw.Boundary())
	buf.WriteString("\r\n")

	text, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"text/plain; charset=utf-8"},
	})
	if err != nil {
		return nil, err
	}
	if _, err := text.Write([]byte(msg.Body)); err != nil {
		return nil, err
	}

	for _, a := range msg.Attachments {
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		part, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {ct},
			"Content-Transfer-Encoding": {"base64"},
			"Content-Disposition":       {`attachment; filename="` + a.Filename + `"`},
		})
		if err != nil {
			return nil, err
		}
		if err := writeBase64(part, a.Data); err != nil {
			return nil, err
		}
	}

	if err := mw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeHeader(buf *bytes.Buffer, key, value string) {
	buf.WriteString(key)
	buf.WriteString(": ")
	buf.WriteString(value)
	buf.WriteString("\r\n")
}

// writeBase64 encodes data in base64 wrapped to 76-character lines, as MIME
// requires for the base64 transfer encoding.
func writeBase64(w io.Writer, data []byte) error {
	const lineLen = 76
	encoded := base64.StdEncoding.EncodeToString(data)
	for len(encoded) > 0 {
		n := lineLen
		if n > len(encoded) {
			n = len(encoded)
		}
		if _, err := io.WriteString(w, encoded[:n]+"\r\n"); err != nil {
			return err
		}
		encoded = encoded[n:]
	}
	return nil
}
