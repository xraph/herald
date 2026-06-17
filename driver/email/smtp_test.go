package email_test

import (
	"bufio"
	"context"
	"net"
	"net/textproto"
	"strings"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/email"
	"github.com/xraph/herald/message"
)

// startMockSMTP starts a minimal SMTP server on 127.0.0.1 that accepts one
// message and returns the raw DATA payload via the channel.
func startMockSMTP(t *testing.T) (host, port string, dataCh <-chan string) {
	t.Helper()
	lc := &net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ch := make(chan string, 1)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		tc := textproto.NewConn(conn)
		_ = tc.PrintfLine("220 mock ESMTP")
		var data strings.Builder
		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				_ = tc.PrintfLine("250 mock")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				_ = tc.PrintfLine("250 OK")
			case strings.HasPrefix(cmd, "DATA"):
				_ = tc.PrintfLine("354 End data with <CR><LF>.<CR><LF>")
				for {
					dl, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					if strings.TrimRight(dl, "\r\n") == "." {
						break
					}
					data.WriteString(dl)
				}
				_ = tc.PrintfLine("250 OK queued")
			case strings.HasPrefix(cmd, "QUIT"):
				_ = tc.PrintfLine("221 Bye")
				ch <- data.String()
				return
			}
		}
	}()

	h, p, _ := net.SplitHostPort(ln.Addr().String())
	return h, p, ch
}

func TestSMTPSend(t *testing.T) {
	host, port, dataCh := startMockSMTP(t)
	d := &email.SMTPDriver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", From: "from@x.com", Subject: "Hello", Text: "body text",
		Data: map[string]string{"host": host, "port": port}, // no username -> no auth
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent {
		t.Errorf("status = %q", res.Status)
	}
	payload := <-dataCh
	if !strings.Contains(payload, "Subject: Hello") {
		t.Errorf("missing subject in payload:\n%s", payload)
	}
	if !strings.Contains(payload, "To: to@x.com") {
		t.Errorf("missing To in payload:\n%s", payload)
	}
	if !strings.Contains(payload, "body text") {
		t.Errorf("missing body in payload:\n%s", payload)
	}
}

func TestSMTPValidate(t *testing.T) {
	d := &email.SMTPDriver{}
	if err := d.Validate(map[string]string{"host": "h"}, nil); err == nil {
		t.Error("expected error for missing port")
	}
	if err := d.Validate(map[string]string{"host": "h", "port": "25"}, nil); err != nil {
		t.Errorf("unexpected: %v", err)
	}
}
