package email_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/email"
)

// A server that accepts the connection and never greets must not hang Send
// past the caller's deadline.
func TestSMTPPlainSendHonoursTheContext(t *testing.T) {
	lc := &net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		time.Sleep(5 * time.Second)
	}()
	host, port, _ := net.SplitHostPort(ln.Addr().String())

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = (&email.SMTPDriver{}).Send(ctx, &driver.OutboundMessage{
		To: "a@example.com", From: "b@example.com", Text: "hi",
		Data: map[string]string{"host": host, "port": port},
	})
	if err == nil {
		t.Fatal("a server that never answers produced no error")
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("Send took %v; the context allowed 300ms", took)
	}
}
