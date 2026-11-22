package email

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/message"
)

// FileDriver writes each email to a directory as an .eml file instead of
// sending it, for local development. Any mail client opens the files.
// They hold whole messages, sign-in links included, so the directory and
// files are readable by their owner only.
type FileDriver struct{}

var (
	_ driver.Driver    = (*FileDriver)(nil)
	_ driver.Describer = (*FileDriver)(nil)
)

func (d *FileDriver) Name() string    { return "file" }
func (d *FileDriver) Channel() string { return "email" }

func (d *FileDriver) Validate(credentials, settings map[string]string) error {
	if credentials["dir"] == "" && settings["dir"] == "" {
		return fmt.Errorf("file: missing required setting %q", "dir")
	}
	return nil
}

// Fields describes what the file driver reads.
func (d *FileDriver) Fields() []driver.Field {
	return append([]driver.Field{
		{Key: "dir", Label: "Directory", Required: true, Help: "Where each email is written as an .eml file.", Placement: driver.PlacementSetting},
	}, driver.SenderFields()...)
}

func (d *FileDriver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir := msg.Data["dir"]
	if dir == "" {
		return nil, fmt.Errorf("file: missing required setting %q", "dir")
	}
	id, err := randomID()
	if err != nil {
		return nil, fmt.Errorf("file: %w", err)
	}
	now := time.Now().UTC()
	raw, err := composeEML(msg, now, id)
	if err != nil {
		return nil, fmt.Errorf("file: %w", err)
	}
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("file: %w", err)
	}
	// Sorts by time in a file listing; the random part keeps two messages
	// in the same instant apart.
	name := now.Format("20060102T150405.000000000Z") + "-" + id + ".eml"
	// Write then rename, so whatever watches the directory never sees half
	// a message.
	tmp, err := os.CreateTemp(dir, ".herald-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("file: %w", err)
	}
	defer os.Remove(tmp.Name()) // a no-op once renamed
	if _, err = tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return nil, fmt.Errorf("file: %w", err)
	}
	if err = os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		return nil, fmt.Errorf("file: %w", err)
	}
	return &driver.DeliveryResult{ProviderMessageID: name, Status: message.StatusSent}, nil
}

// composeEML builds an RFC 5322 message: multipart/alternative when there
// are both text and HTML bodies, a single part otherwise.
func composeEML(msg *driver.OutboundMessage, now time.Time, id string) ([]byte, error) {
	var b strings.Builder
	from := (&mail.Address{Name: oneLine(msg.FromName), Address: oneLine(msg.From)}).String()
	header := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	header("Date", now.Format(time.RFC1123Z))
	header("Message-ID", "<"+id+"@herald.local>")
	header("From", from)
	header("To", (&mail.Address{Address: oneLine(msg.To)}).String())
	header("Subject", mime.QEncoding.Encode("utf-8", oneLine(msg.Subject)))
	header("MIME-Version", "1.0")

	if msg.Text == "" || msg.HTML == "" {
		body, contentType := msg.Text, "text/plain"
		if msg.HTML != "" {
			body, contentType = msg.HTML, "text/html"
		}
		header("Content-Type", contentType+"; charset=UTF-8")
		if err := writeFileBody(&b, body); err != nil {
			return nil, err
		}
		return []byte(b.String()), nil
	}

	boundary := "herald-" + id
	header("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n")
	for _, part := range []struct{ contentType, body string }{{"text/plain", msg.Text}, {"text/html", msg.HTML}} {
		b.WriteString("--" + boundary + "\r\n")
		header("Content-Type", part.contentType+"; charset=UTF-8")
		if err := writeFileBody(&b, part.body); err != nil {
			return nil, err
		}
		b.WriteString("\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String()), nil
}

// File previews often recognize a URL in the raw text. Keep each MIME part
// readable when it fits the 998-byte limit for an email line.
func writeFileBody(b *strings.Builder, body string) error {
	// Match the MIME writer's CRLF line endings for all three encodings.
	body = strings.ReplaceAll(body, "\r\n", "\n")
	encoding := fileBodyEncoding(body)
	b.WriteString("Content-Transfer-Encoding: " + encoding + "\r\n\r\n")
	if encoding != "quoted-printable" {
		b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
		return nil
	}

	w := quotedprintable.NewWriter(b)
	if _, err := w.Write([]byte(body)); err != nil {
		return err
	}
	return w.Close()
}

func fileBodyEncoding(body string) string {
	if !utf8.ValidString(body) {
		return "quoted-printable"
	}
	encoding := "7bit"
	for line := range strings.SplitSeq(body, "\n") {
		if len(line) > 998 {
			return "quoted-printable"
		}
		for _, r := range line {
			switch {
			case (r < ' ' && r != '\t') || r == 0x7f:
				return "quoted-printable"
			case r >= utf8.RuneSelf:
				encoding = "8bit"
			}
		}
	}
	return encoding
}

// oneLine drops line breaks, so no value can start a header of its own.
func oneLine(s string) string {
	return strings.NewReplacer("\r", "", "\n", " ").Replace(s)
}

func randomID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
