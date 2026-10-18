package telemetry_test

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/xraph/herald/internal/telemetry"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestNetClass(t *testing.T) {
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}
	cases := []struct {
		err  error
		want string
	}{
		{context.Canceled, "canceled"},
		{fmt.Errorf("wrapped: %w", context.DeadlineExceeded), "timeout"},
		{&net.DNSError{Err: "no such host", Name: "api.example.com"}, "dns"},
		{&net.DNSError{Err: "timeout", IsTimeout: true}, "timeout"},
		{&net.OpError{Op: "read", Err: timeoutError{}}, "timeout"},
		{fmt.Errorf("x: %w", x509.UnknownAuthorityError{}), "tls"},
		{refused, "connection"},
		{io.ErrUnexpectedEOF, "connection"},
		{errors.New("API error 500: alice@example.com"), "other"},
	}
	for _, tc := range cases {
		if got := telemetry.NetClass(tc.err); got != tc.want {
			t.Errorf("NetClass(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

func TestCallClassTrustsTheContext(t *testing.T) {
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	canceled, cancel2 := context.WithCancel(context.Background())
	cancel2()
	opaque := errors.New("net/http: request canceled")

	if got := telemetry.CallClass(expired, opaque); got != "timeout" {
		t.Errorf("past deadline = %q, want timeout", got)
	}
	if got := telemetry.CallClass(canceled, opaque); got != "canceled" {
		t.Errorf("canceled ctx = %q, want canceled", got)
	}
	if got := telemetry.CallClass(context.Background(), opaque); got != "other" {
		t.Errorf("live ctx = %q, want other", got)
	}
}

func TestSMTPClass(t *testing.T) {
	reply := fmt.Errorf("smtp: rcpt to alice@example.com: %w", &textproto.Error{Code: 550, Msg: "alice@example.com unknown"})
	if got := telemetry.SMTPClass(context.Background(), reply); got != "550" {
		t.Errorf("SMTPClass(reply) = %q, want 550", got)
	}
	dial := fmt.Errorf("smtp: dial: %w", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)})
	if got := telemetry.SMTPClass(context.Background(), dial); got != "connection" {
		t.Errorf("SMTPClass(dial) = %q, want connection", got)
	}
}

func TestTypeOfIsTheInnermostType(t *testing.T) {
	err := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", &textproto.Error{Code: 535, Msg: "secret"}))
	if got := telemetry.TypeOf(err); got != "*textproto.Error" {
		t.Errorf("TypeOf = %q", got)
	}
	if got := telemetry.TypeOf(errors.Join(io.EOF, context.Canceled)); got != "*errors.joinError" {
		t.Errorf("TypeOf(joined) = %q", got)
	}
}
