package email_test

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
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

func TestFileDriverLinkInRawAndDecodedParts(t *testing.T) {
	link := "http://localhost:3000/app/invite/" + strings.Repeat("ab", 32)
	for _, tc := range []struct{ name, text, encoding string }{
		{"ascii", "Accept the invitation: " + link, "7bit"},
		{"utf8", "Invitación: " + link, "8bit"},
		{"limit", strings.Repeat("a", 998) + "\r\n" + link, "7bit"},
		{"long line", strings.Repeat("é", 500) + "\r\n" + link, "quoted-printable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			html := `<a href="` + link + `">Accept invitation</a>`
			result, err := (&email.FileDriver{}).Send(t.Context(), &driver.OutboundMessage{
				To: "invitee@example.test", From: "sender@example.test", Subject: "Invite",
				Text: tc.text, HTML: html, Data: map[string]string{"dir": dir},
			})
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, result.ProviderMessageID))
			if err != nil {
				t.Fatal(err)
			}
			m, err := mail.ReadMessage(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			_, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
			if err != nil {
				t.Fatal(err)
			}
			parts := multipart.NewReader(m.Body, params["boundary"])
			for _, want := range []struct{ encoding, body string }{{tc.encoding, tc.text}, {"7bit", html}} {
				part, err := parts.NextRawPart()
				if err != nil {
					t.Fatal(err)
				}
				if got := part.Header.Get("Content-Transfer-Encoding"); got != want.encoding {
					t.Fatalf("encoding = %s, want %s", got, want.encoding)
				}
				var reader io.Reader = part
				if want.encoding == "quoted-printable" {
					reader = quotedprintable.NewReader(part)
				}
				body, err := io.ReadAll(reader)
				if err != nil || string(body) != want.body {
					t.Fatalf("invitation body differs with %s encoding: %v", want.encoding, err)
				}
			}
		})
	}
}
