// Package telemetry holds what herald's spans and metrics share: the tracer,
// the attribute keys, the error classes and a small label vector over a
// go-utils metrics sink.
//
// Nothing here ever reads an error's message. A driver error can quote up to
// 1KB of the vendor's response, and an SMTP error can name the recipient, so
// a failed span carries a class and the error's Go type, never its text.
package telemetry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ScopeName is the instrumentation scope every herald span carries.
const ScopeName = "github.com/xraph/herald"

// Tracer returns herald's tracer from the global provider, which forge.New
// installs. It looks the provider up on every call, so a provider set after
// herald starts (or swapped in a test) is the one that gets the spans.
func Tracer() trace.Tracer { return otel.Tracer(ScopeName) }

// Attribute keys herald sets. The semconv ones use the OpenTelemetry names.
const (
	KeyChannel        = attribute.Key("herald.channel")
	KeyTemplate       = attribute.Key("herald.template")
	KeyDriver         = attribute.Key("herald.driver")
	KeyProviderID     = attribute.Key("herald.provider.id")
	KeyResolveVia     = attribute.Key("herald.resolve.via")
	KeyRecipientCount = attribute.Key("herald.recipient.count")
	KeySent           = attribute.Key("herald.delivery.sent")
	KeyFailed         = attribute.Key("herald.delivery.failed")
	KeySuppressed     = attribute.Key("herald.suppressed")
	KeyChannelCount   = attribute.Key("herald.channel.count")
	KeyChannelsFailed = attribute.Key("herald.channel.failed")
	KeyLocale         = attribute.Key("herald.template.locale")
	KeyLocaleMatch    = attribute.Key("herald.template.locale_match")

	KeyServerAddress  = attribute.Key("server.address")
	KeyServerPort     = attribute.Key("server.port")
	KeyHTTPMethod     = attribute.Key("http.request.method")
	KeyHTTPStatusCode = attribute.Key("http.response.status_code")
	KeyURLTemplate    = attribute.Key("url.template")
	KeyErrorType      = attribute.Key("error.type")
	KeyExceptionType  = attribute.Key("exception.type")
)

// Fail marks span as failed with class as the status description, and adds
// an exception event carrying only errType as exception.type. Both must be a
// class or a type name from a fixed set, never an error's message.
func Fail(span trace.Span, class, errType string) {
	span.SetStatus(codes.Error, class)
	if errType != "" {
		span.AddEvent("exception", trace.WithAttributes(KeyExceptionType.String(errType)))
	}
}

// FailErr is Fail with the Go type of err's innermost error. err's message is
// never read.
func FailErr(span trace.Span, class string, err error) {
	errType := ""
	if err != nil {
		errType = TypeOf(err)
	}
	Fail(span, class, errType)
}

// TypeOf is the Go type of the innermost error in err's chain, such as
// *net.OpError or syscall.Errno. It follows single-error Unwrap only, so a
// joined error reports its own type.
func TypeOf(err error) string {
	for {
		next := errors.Unwrap(err)
		if next == nil {
			return fmt.Sprintf("%T", err)
		}
		err = next
	}
}

// Network failure classes. These are the only values NetClass returns.
const (
	ClassTimeout    = "timeout"
	ClassCanceled   = "canceled"
	ClassDNS        = "dns"
	ClassTLS        = "tls"
	ClassConnection = "connection"
	ClassOther      = "other"
)

// NetClass sorts a transport failure into timeout, canceled, dns, tls,
// connection or other.
func NetClass(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return ClassCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return ClassTimeout
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsTimeout {
			return ClassTimeout
		}
		return ClassDNS
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ClassTimeout
	}
	if isTLS(err) {
		return ClassTLS
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) ||
		errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return ClassConnection
	}
	return ClassOther
}

// CallClass is NetClass for a call made under ctx. A deadline that has
// passed or a canceled ctx decides the class whatever err says, since
// http.Client's own timeout can surface as an unexported "request canceled"
// error that is neither a context error nor a net.Error.
func CallClass(ctx context.Context, err error) string {
	if dl, ok := ctx.Deadline(); ok && !time.Now().Before(dl) {
		return ClassTimeout
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return ClassCanceled
	}
	return NetClass(err)
}

func isTLS(err error) bool {
	var (
		verifyErr    *tls.CertificateVerificationError
		recordErr    tls.RecordHeaderError
		alertErr     tls.AlertError
		unknownAuth  x509.UnknownAuthorityError
		hostnameErr  x509.HostnameError
		invalidCert  x509.CertificateInvalidError
		echRejection *tls.ECHRejectionError
	)
	return errors.As(err, &verifyErr) || errors.As(err, &recordErr) || errors.As(err, &alertErr) ||
		errors.As(err, &unknownAuth) || errors.As(err, &hostnameErr) || errors.As(err, &invalidCert) ||
		errors.As(err, &echRejection)
}

// SMTPClass is the class of a failed SMTP conversation under ctx: the
// server's reply code ("535", "550") when it answered with an error,
// otherwise CallClass.
func SMTPClass(ctx context.Context, err error) string {
	var reply *textproto.Error
	if errors.As(err, &reply) && reply.Code >= 100 && reply.Code <= 599 {
		return strconv.Itoa(reply.Code)
	}
	return CallClass(ctx, err)
}
