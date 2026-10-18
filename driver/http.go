package driver

import (
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/trace"

	"github.com/xraph/herald/internal/telemetry"
)

// Endpoint describes a vendor call for the client span HTTPClient records.
// None of it is read from the request URL, which is why a span can't leak a
// token, a device token or an account ID that sits in the URL.
type Endpoint struct {
	// Driver is the driver's name, recorded as herald.driver.
	Driver string

	// URLTemplate is the request path with its variable parts named, such
	// as "/2010-04-01/Accounts/{account_sid}/Messages.json". It is recorded
	// as url.template and names the span. Leave it empty when the driver
	// can't know the path, such as a webhook URL the user supplies whole.
	URLTemplate string

	// HideHost keeps server.address and server.port off the span. Set it
	// when the user supplies the whole URL, since the host is then theirs
	// (a customer's endpoint) rather than the vendor's.
	HideHost bool
}

// HTTPClient returns the *http.Client a driver uses to call its vendor. It
// behaves like &http.Client{Timeout: timeout}: the same default transport,
// connection pool, HTTP/2 support and redirect policy, and the error a
// failed request returns is the transport's own, unwrapped.
//
// On top of that, every request it sends is an OpenTelemetry client span
// under the span in the request's context. The span is named after the
// method and ep.URLTemplate ("POST /v3/mail/send", or "POST" with no
// template) and carries http.request.method, server.address and server.port
// (unless ep.HideHost), url.template, herald.driver and
// http.response.status_code. It never carries the URL, its path or its
// query, or any header. A 4xx or 5xx response, or a transport failure, marks
// it failed with error.type set to the status code or to timeout, canceled,
// dns, tls, connection or other; the error's message never reaches it.
//
// The request goes out with exactly the headers the driver set: no
// traceparent, tracestate or baggage. Vendors are third parties, and the
// inbound request's baggage is not theirs to see.
func HTTPClient(timeout time.Duration, ep Endpoint) *http.Client {
	return &http.Client{Timeout: timeout, Transport: &vendorTransport{ep: ep}}
}

// vendorTransport records the client span and hands the request to
// http.DefaultTransport, looked up per request so it is exactly the
// transport a plain http.Client would use.
type vendorTransport struct {
	ep Endpoint
}

// RoundTrip implements http.RoundTripper.
func (t *vendorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	method := knownMethod(req.Method)
	name := method
	attrs := make([]attribute.KeyValue, 0, 6)
	attrs = append(attrs, telemetry.KeyHTTPMethod.String(method))
	if t.ep.URLTemplate != "" {
		name = method + " " + t.ep.URLTemplate
		attrs = append(attrs, telemetry.KeyURLTemplate.String(t.ep.URLTemplate))
	}
	if t.ep.Driver != "" {
		attrs = append(attrs, telemetry.KeyDriver.String(t.ep.Driver))
	}
	if !t.ep.HideHost && req.URL != nil {
		attrs = append(attrs, telemetry.KeyServerAddress.String(req.URL.Hostname()))
		if p, err := strconv.Atoi(req.URL.Port()); err == nil {
			attrs = append(attrs, telemetry.KeyServerPort.Int(p))
		}
	}

	// The span hangs off the caller's span, and the request goes on without
	// the caller's baggage, so nothing downstream of here can forward it.
	ctx, span := telemetry.Tracer().Start(req.Context(), name,
		trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attrs...))
	defer span.End()
	out := req.WithContext(baggage.ContextWithoutBaggage(ctx))

	resp, err := http.DefaultTransport.RoundTrip(out)
	if err != nil {
		class := telemetry.CallClass(out.Context(), err)
		span.SetAttributes(telemetry.KeyErrorType.String(class))
		telemetry.FailErr(span, class, err)
		return nil, err
	}
	span.SetAttributes(telemetry.KeyHTTPStatusCode.Int(resp.StatusCode))
	if resp.StatusCode >= http.StatusBadRequest {
		code := strconv.Itoa(resp.StatusCode)
		span.SetAttributes(telemetry.KeyErrorType.String(code))
		telemetry.Fail(span, code, "")
	}
	return resp, nil
}

// knownMethod returns m when it is a standard HTTP method and "_OTHER"
// otherwise, as the HTTP semantic conventions ask.
func knownMethod(m string) string {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace:
		return m
	default:
		return "_OTHER"
	}
}
