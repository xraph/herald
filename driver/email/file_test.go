package email_test

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/email"
	"github.com/xraph/herald/message"
)

func TestFileDriverWritesAMailReadersCanOpen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mail")
	d := &email.FileDriver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "ada@example.com", From: "noreply@example.com", FromName: "Loomplane",
		Subject: "Reset your password ✓", Text: "Open the link.", HTML: "<p>Open the link.</p>",
		Data: map[string]string{"dir": dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != message.StatusSent {
		t.Fatalf("status %q", res.Status)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("want one file in %s: %v, %v", dir, entries, err)
	}
	name := entries[0].Name()
	if !strings.HasSuffix(name, ".eml") || res.ProviderMessageID != name {
		t.Fatalf("file %q, provider id %q", name, res.ProviderMessageID)
	}
	info, _ := entries[0].Info()
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v: a mail file can hold a reset link", info.Mode().Perm())
	}

	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	m, err := mail.ReadMessage(f)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	if err != nil || subject != "Reset your password ✓" {
		t.Fatalf("subject %q, %v", subject, err)
	}
	from, err := m.Header.AddressList("From")
	if err != nil || from[0].Name != "Loomplane" || from[0].Address != "noreply@example.com" {
		t.Fatalf("from %v, %v", from, err)
	}
	to, err := m.Header.AddressList("To")
	if err != nil || to[0].Address != "ada@example.com" {
		t.Fatalf("to %v, %v", to, err)
	}
	if m.Header.Get("Date") == "" || m.Header.Get("Message-ID") == "" {
		t.Fatal("missing Date or Message-ID")
	}
	mediaType, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/alternative" {
		t.Fatalf("content type %q, %v", mediaType, err)
	}
	parts := map[string]string{}
	r := multipart.NewReader(m.Body, params["boundary"])
	for {
		p, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(p)
		ct, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		parts[ct] = string(body)
	}
	if parts["text/plain"] != "Open the link." || parts["text/html"] != "<p>Open the link.</p>" {
		t.Fatalf("parts %q", parts)
	}
}

func TestFileDriverKeepsHeadersOnOneLine(t *testing.T) {
	dir := t.TempDir()
	_, err := (&email.FileDriver{}).Send(context.Background(), &driver.OutboundMessage{
		To: "ada@example.com", From: "noreply@example.com",
		Subject: "Hi\r\nBcc: eve@example.com", Text: "hello",
		Data: map[string]string{"dir": dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	raw, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	m, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if m.Header.Get("Bcc") != "" {
		t.Fatal("a subject added a Bcc header")
	}
}

func TestFileDriverNeedsADirectory(t *testing.T) {
	d := &email.FileDriver{}
	if err := d.Validate(nil, nil); err == nil {
		t.Fatal("validate passed without dir")
	}
	if err := d.Validate(nil, map[string]string{"dir": "/tmp/mail"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Send(context.Background(), &driver.OutboundMessage{To: "a@example.com", Text: "x"}); err == nil {
		t.Fatal("send passed without dir")
	}
}
