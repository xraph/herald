package email_test

import (
	"bufio"
	"context"
	"net"
	"net/textproto"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/driver/email"
	"github.com/xraph/herald/internal/telemetry/telemetrytest"
)

// rejectingSMTP accepts the conversation up to RCPT and refuses the
// recipient with a reply that quotes the address, the way real servers do.
func rejectingSMTP(t *testing.T) (host, port string) {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		tc := textproto.NewConn(conn)
		_ = tc.PrintfLine("220 mock ESMTP")
		r := bufio.NewReader(conn)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"), strings.HasPrefix(cmd, "MAIL"):
				_ = tc.PrintfLine("250 OK")
			case strings.HasPrefix(cmd, "RCPT"):
				_ = tc.PrintfLine("550 5.1.1 <alice@example.com>: user unknown")
			default:
				_ = tc.PrintfLine("221 Bye")
				return
			}
		}
	}()
	h, p, _ := net.SplitHostPort(ln.Addr().String())
	return h, p
}

func TestSMTPSendRecordsAClientSpan(t *testing.T) {
	rec := telemetrytest.Spans(t)
	host, port, dataCh := startMockSMTP(t)

	ctx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	_, err := (&email.SMTPDriver{}).Send(ctx, &driver.OutboundMessage{
		To: "alice@example.com", From: "from@example.com", Subject: "Your code", Text: "123456",
		Data: map[string]string{"host": host, "port": port, "password": "pw-SECRET"},
	})
	parent.End()
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	<-dataCh

	span := telemetrytest.One(t, rec, "send smtp")
	if span.SpanKind() != trace.SpanKindClient {
		t.Errorf("kind = %v", span.SpanKind())
	}
	if span.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Error("the smtp span isn't a child of the caller's span")
	}
	a := telemetrytest.Attrs(span)
	if len(a) != 2 || a["server.address"] != host || a["server.port"] != port {
		t.Errorf("attrs = %v, want only server.address and server.port", a)
	}
	if span.Status().Code == codes.Error {
		t.Errorf("status = %+v", span.Status())
	}
	for _, v := range []string{"alice@example.com", "from@example.com", "Your code", "123456", "pw-SECRET"} {
		if leaks := telemetrytest.Leaks(rec, v); len(leaks) > 0 {
			t.Errorf("%q reached telemetry: %v", v, leaks)
		}
	}
}

func TestSMTPRefusalReportsTheReplyCodeOnly(t *testing.T) {
	rec := telemetrytest.Spans(t)
	host, port := rejectingSMTP(t)

	_, err := (&email.SMTPDriver{}).Send(context.Background(), &driver.OutboundMessage{
		To: "alice@example.com", From: "from@example.com", Text: "hi",
		Data: map[string]string{"host": host, "port": port},
	})
	if err == nil {
		t.Fatal("a refused recipient produced no error")
	}

	span := telemetrytest.One(t, rec, "send smtp")
	if span.Status().Code != codes.Error || span.Status().Description != "550" {
		t.Errorf("status = %+v, want error 550", span.Status())
	}
	if types := telemetrytest.ExceptionTypes(span); len(types) != 1 || types[0] != "*textproto.Error" {
		t.Errorf("exception types = %q", types)
	}
	if leaks := telemetrytest.Leaks(rec, "alice@example.com"); len(leaks) > 0 {
		t.Errorf("the server's reply reached telemetry: %v", leaks)
	}
}

func TestResendSendRecordsAClientSpan(t *testing.T) {
	rec := telemetrytest.Spans(t)
	srv := drivertest.NewServer(t, 200, `{"id":"re_1"}`)

	_, err := (&email.ResendDriver{}).Send(context.Background(), &driver.OutboundMessage{
		To: "alice@example.com", From: "from@example.com", Text: "hi",
		Data: map[string]string{"api_key": "re-SECRET", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	span := telemetrytest.One(t, rec, "POST /emails")
	if a := telemetrytest.Attrs(span); a["herald.driver"] != "resend" || a["http.response.status_code"] != "200" {
		t.Errorf("attrs = %v", a)
	}
	for _, v := range []string{"alice@example.com", "re-SECRET"} {
		if leaks := telemetrytest.Leaks(rec, v); len(leaks) > 0 {
			t.Errorf("%q reached telemetry: %v", v, leaks)
		}
	}
}
