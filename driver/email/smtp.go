// Package email provides email notification drivers.
package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/message"
)

// SMTPDriver delivers email via standard SMTP.
type SMTPDriver struct{}

var _ driver.Driver = (*SMTPDriver)(nil)

func (d *SMTPDriver) Name() string    { return "smtp" }
func (d *SMTPDriver) Channel() string { return "email" }

func (d *SMTPDriver) Validate(credentials, _ map[string]string) error {
	required := []string{"host", "port"}
	for _, key := range required {
		if credentials[key] == "" {
			return fmt.Errorf("smtp: missing required credential %q", key)
		}
	}
	return nil
}

func (d *SMTPDriver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	// Credentials are passed via the OutboundMessage.Data field from the provider
	host := msg.Data["host"]
	port := msg.Data["port"]
	username := msg.Data["username"]
	password := msg.Data["password"]
	useTLS := msg.Data["use_tls"] == "true"

	from := msg.From
	addr := net.JoinHostPort(host, port)

	// Build RFC 2822 message
	var body strings.Builder
	body.WriteString("From: ")
	if msg.FromName != "" {
		body.WriteString(msg.FromName + " <" + from + ">")
	} else {
		body.WriteString(from)
	}
	body.WriteString("\r\n")
	body.WriteString("To: " + msg.To + "\r\n")
	body.WriteString("Subject: " + msg.Subject + "\r\n")
	body.WriteString("MIME-Version: 1.0\r\n")

	content := msg.Text
	contentType := "text/plain"
	if msg.HTML != "" {
		content = msg.HTML
		contentType = "text/html"
	}
	body.WriteString("Content-Type: " + contentType + "; charset=UTF-8\r\n")
	body.WriteString("\r\n")
	body.WriteString(content)

	var auth smtp.Auth
	if username != "" {
		auth = smtp.PlainAuth("", username, password, host)
	}

	if useTLS {
		if err := sendWithTLS(ctx, addr, host, from, []string{msg.To}, body.String(), auth); err != nil {
			return nil, err
		}
	} else {
		if err := sendPlain(ctx, addr, host, from, []string{msg.To}, body.String(), auth); err != nil {
			return nil, err
		}
	}

	return &driver.DeliveryResult{Status: message.StatusSent}, nil
}

// sendTimeout bounds a whole SMTP conversation when the caller's context has
// no earlier deadline.
const sendTimeout = 30 * time.Second

// sendPlain does what smtp.SendMail does (STARTTLS when the server offers it,
// then auth and delivery), but dials with ctx and bounds the conversation
// with a deadline, so a server that stops answering can't hang a send.
func sendPlain(ctx context.Context, addr, host, from string, to []string, body string, auth smtp.Auth) error {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp: dial: %w", err)
	}
	defer conn.Close()
	if err = conn.SetDeadline(deadline(ctx)); err != nil {
		return fmt.Errorf("smtp: set deadline: %w", err)
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp: new client: %w", err)
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("smtp: starttls: %w", err)
		}
	}
	return deliver(client, from, to, body, auth)
}

func sendWithTLS(ctx context.Context, addr, host, from string, to []string, body string, auth smtp.Auth) error {
	dialer := &tls.Dialer{Config: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp: tls dial: %w", err)
	}
	defer conn.Close()
	if err = conn.SetDeadline(deadline(ctx)); err != nil {
		return fmt.Errorf("smtp: set deadline: %w", err)
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp: new client: %w", err)
	}
	defer client.Close()
	return deliver(client, from, to, body, auth)
}

// deliver runs auth, MAIL, RCPT, DATA and QUIT on an open client.
func deliver(client *smtp.Client, from string, to []string, body string, auth smtp.Auth) error {
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp: auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp: mail from: %w", err)
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("smtp: rcpt to %s: %w", recipient, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp: data: %w", err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		return fmt.Errorf("smtp: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp: close data: %w", err)
	}
	return client.Quit()
}

// deadline is the earlier of ctx's deadline and sendTimeout from now.
func deadline(ctx context.Context) time.Time {
	d := time.Now().Add(sendTimeout)
	if c, ok := ctx.Deadline(); ok && c.Before(d) {
		return c
	}
	return d
}
