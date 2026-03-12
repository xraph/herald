// Package email provides email notification drivers.
package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"

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
		if err := smtp.SendMail(addr, auth, from, []string{msg.To}, []byte(body.String())); err != nil {
			return nil, fmt.Errorf("smtp: send mail: %w", err)
		}
	}

	return &driver.DeliveryResult{Status: message.StatusSent}, nil
}

func sendWithTLS(ctx context.Context, addr, host, from string, to []string, body string, auth smtp.Auth) error {
	tlsConfig := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}

	dialer := &tls.Dialer{Config: tlsConfig}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp: tls dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp: new client: %w", err)
	}
	defer client.Close()

	if auth != nil {
		if authErr := client.Auth(auth); authErr != nil {
			return fmt.Errorf("smtp: auth: %w", authErr)
		}
	}

	if mailErr := client.Mail(from); mailErr != nil {
		return fmt.Errorf("smtp: mail from: %w", mailErr)
	}
	for _, recipient := range to {
		if rcptErr := client.Rcpt(recipient); rcptErr != nil {
			return fmt.Errorf("smtp: rcpt to %s: %w", recipient, rcptErr)
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
