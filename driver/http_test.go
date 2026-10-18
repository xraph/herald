package driver_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/internal/telemetry/telemetrytest"
)

// The token, device token and account ID a vendor URL can carry. None of
// them may reach a span.
const (
	secretPath  = "tok-SECRET"
	secretQuery = "qry-SECRET"
)

// withPropagators installs forge's default propagators (tracecontext and
// baggage) for the test, so a client that injected anything would show it.
func withPropagators(t *testing.T) {
	t.Helper()
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })
}

// parentCtx returns a ctx carrying a recorded parent span and some inbound
// baggage, the way a request ctx does in a Forge app.
func parentCtx(t *testing.T) (context.Context, trace.Span) {
	t.Helper()
	member, err := baggage.NewMember("tenant", "acme")
	if err != nil {
		t.Fatal(err)
	}
	bag, err := baggage.New(member)
	if err != nil {
		t.Fatal(err)
	}
	ctx, span := otel.Tracer("test").Start(baggage.ContextWithBaggage(context.Background(), bag), "parent")
	t.Cleanup(func() { span.End() })
	return ctx, span
}

func post(ctx context.Context, t *testing.T, c *http.Client, rawURL string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(`{"to":"alice@example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer sk-SECRET")
	return c.Do(req)
}

func TestHTTPClientRecordsAClientSpanWithoutTheURL(t *testing.T) {
	rec := telemetrytest.Spans(t)
	withPropagators(t)

	var header http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Clone()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	ctx, parent := parentCtx(t)
	c := driver.HTTPClient(10*time.Second, driver.Endpoint{Driver: "fcm", URLTemplate: "/v1/projects/{project_id}/messages:send"})
	resp, err := post(ctx, t, c, srv.URL+"/v1/projects/"+secretPath+"/messages:send?key="+secretQuery)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	parent.End()

	span := telemetrytest.One(t, rec, "POST /v1/projects/{project_id}/messages:send")
	if span.SpanKind() != trace.SpanKindClient {
		t.Errorf("kind = %v, want client", span.SpanKind())
	}
	if span.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Error("the client span isn't a child of the caller's span")
	}
	u, _ := url.Parse(srv.URL)
	a := telemetrytest.Attrs(span)
	want := map[string]string{
		"http.request.method":       "POST",
		"server.address":            u.Hostname(),
		"server.port":               u.Port(),
		"url.template":              "/v1/projects/{project_id}/messages:send",
		"herald.driver":             "fcm",
		"http.response.status_code": "202",
	}
	for k, v := range want {
		if a[k] != v {
			t.Errorf("%s = %q, want %q", k, a[k], v)
		}
	}
	for _, k := range []string{"url.full", "url.path", "url.query", "error.type"} {
		if _, ok := a[k]; ok {
			t.Errorf("span has %s", k)
		}
	}
	if span.Status().Code == codes.Error {
		t.Errorf("a 202 marked the span failed: %v", span.Status())
	}
	for _, v := range []string{secretPath, secretQuery, "sk-SECRET", "alice@example.com"} {
		if leaks := telemetrytest.Leaks(rec, v); len(leaks) > 0 {
			t.Errorf("%q reached telemetry: %v", v, leaks)
		}
	}

	// A vendor is a third party: no trace context and none of the inbound
	// baggage goes with the request.
	for _, h := range []string{"Traceparent", "Tracestate", "Baggage"} {
		if v := header.Get(h); v != "" {
			t.Errorf("vendor got %s: %q", h, v)
		}
	}
	if header.Get("Authorization") != "Bearer sk-SECRET" {
		t.Error("the driver's own headers didn't arrive")
	}
}

func TestHTTPClientHidesAHostTheUserChose(t *testing.T) {
	rec := telemetrytest.Spans(t)
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	c := driver.HTTPClient(10*time.Second, driver.Endpoint{Driver: "webhook", HideHost: true})
	resp, err := post(context.Background(), t, c, srv.URL+"/hooks/"+secretPath)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()

	span := telemetrytest.One(t, rec, "POST")
	a := telemetrytest.Attrs(span)
	for _, k := range []string{"server.address", "server.port", "url.template"} {
		if _, ok := a[k]; ok {
			t.Errorf("span has %s = %q", k, a[k])
		}
	}
	if a["herald.driver"] != "webhook" || a["http.response.status_code"] != "200" {
		t.Errorf("attrs = %v", a)
	}
	u, _ := url.Parse(srv.URL)
	for _, v := range []string{secretPath, u.Host} {
		if leaks := telemetrytest.Leaks(rec, v); len(leaks) > 0 {
			t.Errorf("%q reached telemetry: %v", v, leaks)
		}
	}
}

func TestHTTPClientMarksAVendorErrorWithItsStatusOnly(t *testing.T) {
	rec := telemetrytest.Spans(t)
	const vendorText = "invalid recipient alice@example.com"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, vendorText)
	}))
	defer srv.Close()

	c := driver.HTTPClient(10*time.Second, driver.Endpoint{Driver: "postmark", URLTemplate: "/email"})
	resp, err := post(context.Background(), t, c, srv.URL+"/email")
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != vendorText {
		t.Errorf("the driver lost the vendor's body: %q", body)
	}

	span := telemetrytest.One(t, rec, "POST /email")
	if span.Status().Code != codes.Error || span.Status().Description != "422" {
		t.Errorf("status = %+v, want error 422", span.Status())
	}
	if a := telemetrytest.Attrs(span); a["error.type"] != "422" || a["http.response.status_code"] != "422" {
		t.Errorf("attrs = %v", a)
	}
	if leaks := telemetrytest.Leaks(rec, "alice@example.com"); len(leaks) > 0 {
		t.Errorf("vendor text reached telemetry: %v", leaks)
	}
}

func TestHTTPClientClassifiesTransportFailures(t *testing.T) {
	rec := telemetrytest.Spans(t)

	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()

	release := make(chan struct{})
	arrived := make(chan struct{}, 1)
	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case arrived <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer slow.Close()
	defer close(release)

	cases := []struct {
		name   string
		client *http.Client
		url    string
		ctx    func() (context.Context, context.CancelFunc)
		class  string
	}{
		{
			name:   "connection refused",
			client: driver.HTTPClient(10*time.Second, driver.Endpoint{Driver: "slack", HideHost: true}),
			url:    closedURL + "/services/" + secretPath,
			class:  "connection",
		},
		{
			name:   "client timeout",
			client: driver.HTTPClient(50*time.Millisecond, driver.Endpoint{Driver: "ses", URLTemplate: "/v2/email/outbound-emails"}),
			url:    slow.URL + "/v2/email/outbound-emails?" + secretQuery,
			class:  "timeout",
		},
		{
			name:   "caller canceled",
			client: driver.HTTPClient(10*time.Second, driver.Endpoint{Driver: "ses", URLTemplate: "/v2/email/outbound-emails"}),
			url:    slow.URL + "/v2/email/outbound-emails?" + secretQuery,
			ctx: func() (context.Context, context.CancelFunc) {
				select { // drop the previous case's arrival
				case <-arrived:
				default:
				}
				ctx, cancel := context.WithCancel(context.Background())
				go func() {
					<-arrived
					cancel()
				}()
				return ctx, cancel
			},
			class: "canceled",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := len(rec.Ended())
			ctx, cancel := context.Background(), context.CancelFunc(func() {})
			if tc.ctx != nil {
				ctx, cancel = tc.ctx()
			}
			defer cancel()
			resp, err := post(ctx, t, tc.client, tc.url)
			if resp != nil {
				resp.Body.Close()
			}
			if err == nil {
				t.Fatal("no error")
			}
			// The client still wraps the transport's error in a *url.Error,
			// exactly as a plain http.Client does.
			var ue *url.Error
			if !errors.As(err, &ue) {
				t.Errorf("error %T isn't a *url.Error", err)
			}
			ended := rec.Ended()[before:]
			if len(ended) != 1 {
				t.Fatalf("want one span, got %d", len(ended))
			}
			span := ended[0]
			if span.Status().Code != codes.Error || span.Status().Description != tc.class {
				t.Errorf("status = %+v, want error %q", span.Status(), tc.class)
			}
			if got := telemetrytest.Attrs(span)["error.type"]; got != tc.class {
				t.Errorf("error.type = %q, want %q", got, tc.class)
			}
			if types := telemetrytest.ExceptionTypes(span); len(types) != 1 || types[0] == "" {
				t.Errorf("exception types = %q", types)
			}
		})
	}
	for _, v := range []string{secretPath, secretQuery} {
		if leaks := telemetrytest.Leaks(rec, v); len(leaks) > 0 {
			t.Errorf("%q reached telemetry: %v", v, leaks)
		}
	}
}

func TestHTTPClientKeepsTheTimeout(t *testing.T) {
	c := driver.HTTPClient(7*time.Second, driver.Endpoint{})
	if c.Timeout != 7*time.Second {
		t.Errorf("Timeout = %v", c.Timeout)
	}
	if c.Jar != nil || c.CheckRedirect != nil {
		t.Error("the client should keep http.Client's default jar and redirect policy")
	}
}

func TestHTTPClientNamesUnknownMethodsOther(t *testing.T) {
	rec := telemetrytest.Spans(t)
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), "PURGE", srv.URL, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := driver.HTTPClient(time.Second, driver.Endpoint{URLTemplate: "/"}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	span := telemetrytest.One(t, rec, "_OTHER /")
	if m := telemetrytest.Attrs(span)["http.request.method"]; m != "_OTHER" {
		t.Errorf("method = %q", m)
	}
	if names := spanNames(rec.Ended()); slices.Contains(names, "PURGE /") {
		t.Errorf("a made-up method named a span: %q", names)
	}
}

func spanNames[T interface{ Name() string }](spans []T) []string {
	out := make([]string, 0, len(spans))
	for _, s := range spans {
		out = append(out, s.Name())
	}
	return out
}
