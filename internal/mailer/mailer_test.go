package mailer

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

// TestNoopReportsNotConfigured is the crux of the inert design: with an empty
// address, New returns a mailer that sends nothing and reports ErrNotConfigured.
func TestNoopReportsNotConfigured(t *testing.T) {
	m := New(Config{}, nil)
	err := m.Send(context.Background(), Message{To: []string{"x@example.com"}, Subject: "Hi"})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Send err = %v, want ErrNotConfigured", err)
	}
}

// TestNewSelectsSMTPWhenAddrSet proves a configured address flips New to the
// real sender (which would attempt delivery), not the no-op.
func TestNewSelectsSMTPWhenAddrSet(t *testing.T) {
	if _, ok := New(Config{Addr: "smtp.example.com:587"}, nil).(*smtpMailer); !ok {
		t.Fatal("New with Addr set did not return an smtpMailer")
	}
	if _, ok := New(Config{}, nil).(noopMailer); !ok {
		t.Fatal("New with empty Addr did not return a noopMailer")
	}
}

// TestBuildMultipartWithAttachment checks the assembled message is a valid MIME
// document: parseable headers, a text part, and the attachment recovered
// byte-for-byte from its base64 encoding.
func TestBuildMultipartWithAttachment(t *testing.T) {
	pdf := []byte("%PDF-1.4\n<binary\x00\x01\x02payload>\n%%EOF")
	raw, err := build("billing@tadmor.example", Message{
		To:      []string{"a@example.com", "b@example.com"},
		Subject: "Invoice INV/2026 01",
		Body:    "Please find attached invoice INV/2026 01.",
		Attachments: []Attachment{{
			Filename:    "invoice-INV-2026-01.pdf",
			ContentType: "application/pdf",
			Data:        pdf,
		}},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if got := msg.Header.Get("To"); got != "a@example.com, b@example.com" {
		t.Errorf("To = %q", got)
	}
	// The subject's slash survives round-tripping through the header encoder.
	subj, err := (&mime.WordDecoder{}).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil {
		t.Fatalf("decode subject: %v", err)
	}
	if subj != "Invoice INV/2026 01" {
		t.Errorf("Subject = %q", subj)
	}

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/mixed" {
		t.Fatalf("Content-Type = %q (err %v)", msg.Header.Get("Content-Type"), err)
	}

	mr := multipart.NewReader(msg.Body, params["boundary"])
	// First part: the text body.
	text, err := mr.NextPart()
	if err != nil {
		t.Fatalf("first part: %v", err)
	}
	if ct := text.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("first part Content-Type = %q", ct)
	}
	body, _ := io.ReadAll(text)
	if !strings.Contains(string(body), "invoice INV/2026 01") {
		t.Errorf("text body = %q", body)
	}

	// Second part: the base64 PDF attachment, decoded by the reader.
	att, err := mr.NextPart()
	if err != nil {
		t.Fatalf("attachment part: %v", err)
	}
	if enc := att.Header.Get("Content-Transfer-Encoding"); enc != "base64" {
		t.Errorf("attachment encoding = %q", enc)
	}
	got, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, att))
	if err != nil {
		t.Fatalf("read attachment: %v", err)
	}
	if !bytes.Equal(got, pdf) {
		t.Errorf("attachment payload mismatch:\n got %q\nwant %q", got, pdf)
	}
}
