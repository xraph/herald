package herald

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/internal/telemetry/telemetrytest"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/preference"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/store/memory"
	"github.com/xraph/herald/template"
)

// Values that must never reach a span, an event, a status or a label.
const (
	leakRecipient = "alice@example.com"
	leakData      = "data-SECRET"
	leakMeta      = "meta-SECRET"
	leakCred      = "cred-SECRET"
	leakSubject   = "subject-SECRET"
	leakBody      = "body-SECRET"
	leakVendor    = "vendor said alice@example.com is over quota"
)

// httpDriver calls a vendor through driver.HTTPClient, as the real HTTP
// drivers do, so a test can see the client span under the send span.
type httpDriver struct {
	url string
}

func (d *httpDriver) Name() string                          { return "vendor" }
func (d *httpDriver) Channel() string                       { return "email" }
func (d *httpDriver) Validate(_, _ map[string]string) error { return nil }
func (d *httpDriver) Send(ctx context.Context, m *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	c := driver.HTTPClient(10*time.Second, driver.Endpoint{Driver: "vendor", URLTemplate: "/send"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url+"/send?to="+m.To, strings.NewReader(m.Text))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.Data["api_key"])
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	return &driver.DeliveryResult{Status: message.StatusSent}, nil
}

func seedTemplate(t *testing.T, st store.Store, appID, slug, channel string) {
	t.Helper()
	tmpl := &template.Template{ID: id.NewTemplateID(), AppID: appID, Slug: slug, Name: slug, Channel: channel, Enabled: true}
	if err := st.CreateTemplate(bg, tmpl); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	v := &template.Version{ID: id.NewTemplateVersionID(), TemplateID: tmpl.ID, Locale: "en", Active: true,
		Subject: leakSubject + " {{.code}}", Text: leakBody + " {{.code}}"}
	if err := st.CreateVersion(bg, v); err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}
}

// assertNoLabelLeaks fails if any value appears in any label on sink.
func assertNoLabelLeaks(t *testing.T, sink *telemetrytest.Sink, values ...string) {
	t.Helper()
	for _, name := range sink.Names() {
		for _, sr := range sink.All(name) {
			for k, lv := range sr.Labels {
				for _, v := range values {
					if strings.Contains(lv, v) {
						t.Errorf("%q reached label %s on %s", v, k, name)
					}
				}
			}
		}
	}
}

func assertLabelKeys(t *testing.T, sink *telemetrytest.Sink, name string, keys ...string) {
	t.Helper()
	want := slices.Clone(keys)
	slices.Sort(want)
	for _, sr := range sink.All(name) {
		got := slices.Sorted(maps.Keys(sr.Labels))
		if !slices.Equal(got, want) {
			t.Errorf("%s has labels %q, want %q", name, got, want)
		}
	}
}

func TestSendTraceAndMetrics(t *testing.T) {
	rec := telemetrytest.Spans(t)
	sink := telemetrytest.NewSink()

	vendor := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer vendor.Close()

	st := memory.New()
	h := newHerald(t, st, WithDriver(&httpDriver{url: vendor.URL}), WithMetrics(sink))
	p := &provider.Provider{
		ID: id.NewProviderID(), AppID: "app_a", Name: "vendor", Channel: "email", Driver: "vendor",
		Credentials: map[string]string{"api_key": leakCred}, Enabled: true,
	}
	if err := st.CreateProvider(bg, p); err != nil {
		t.Fatal(err)
	}
	seedTemplate(t, st, "app_a", "auth.welcome", "email")

	ctx, parent := otel.Tracer("test").Start(bg, "request")
	res, err := h.Send(ctx, &SendRequest{
		AppID: "app_a", UserID: "user-SECRET", Channel: "email", Template: "auth.welcome", Locale: "en-GB",
		To:       []string{leakRecipient, "bob@example.com"},
		Data:     map[string]any{"code": leakData},
		Metadata: map[string]string{"k": leakMeta},
	})
	parent.End()
	if err != nil || res.Status != message.StatusSent {
		t.Fatalf("Send = %+v, %v", res, err)
	}

	send := telemetrytest.One(t, rec, "send notification")
	if send.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Error("send notification isn't a child of the caller's span")
	}
	if send.SpanKind() != trace.SpanKindInternal {
		t.Errorf("send kind = %v", send.SpanKind())
	}
	a := telemetrytest.Attrs(send)
	for k, v := range map[string]string{
		"herald.channel":         "email",
		"herald.template":        "auth.welcome",
		"herald.recipient.count": "2",
		"herald.driver":          "vendor",
		"herald.provider.id":     p.ID.String(),
		"herald.delivery.sent":   "2",
		"herald.delivery.failed": "0",
	} {
		if a[k] != v {
			t.Errorf("send %s = %q, want %q", k, a[k], v)
		}
	}
	if send.Status().Code == codes.Error {
		t.Errorf("send status = %+v", send.Status())
	}

	render := telemetrytest.One(t, rec, "render template")
	ra := telemetrytest.Attrs(render)
	if render.Parent().SpanID() != send.SpanContext().SpanID() ||
		ra["herald.template"] != "auth.welcome" || ra["herald.template.locale"] != "en" || ra["herald.template.locale_match"] != "language" {
		t.Errorf("render span parent/attrs = %v", ra)
	}

	resolve := telemetrytest.One(t, rec, "resolve provider")
	va := telemetrytest.Attrs(resolve)
	if resolve.Parent().SpanID() != send.SpanContext().SpanID() ||
		va["herald.channel"] != "email" || va["herald.resolve.via"] != "fallback" || va["herald.provider.id"] != p.ID.String() {
		t.Errorf("resolve span parent/attrs = %v", va)
	}

	calls := telemetrytest.Named(rec, "POST /send")
	if len(calls) != 2 {
		t.Fatalf("want a client span per recipient, got %d", len(calls))
	}
	for _, c := range calls {
		if c.Parent().SpanID() != send.SpanContext().SpanID() || c.SpanKind() != trace.SpanKindClient {
			t.Error("a vendor call isn't a client span under send notification")
		}
	}

	for _, v := range []string{leakRecipient, "bob@example.com", leakData, leakMeta, leakCred, leakSubject, leakBody, "en-GB", "user-SECRET"} {
		if leaks := telemetrytest.Leaks(rec, v); len(leaks) > 0 {
			t.Errorf("%q reached telemetry: %v", v, leaks)
		}
	}
	assertNoLabelLeaks(t, sink, leakRecipient, leakData, leakMeta, leakCred, "app_a", "user-SECRET", "auth.welcome", p.ID.String())

	if got := sink.Get(metricDeliveriesAttempted, "channel", "email", "driver", "vendor", "result", "ok").Value(); got != 2 {
		t.Errorf("deliveries ok = %v, want 2", got)
	}
	dur := sink.Get(metricDeliveryDuration, "channel", "email", "driver", "vendor", "result", "ok")
	if dur.Value() != 2 || dur.Unit != "s" || !slices.Equal(dur.Buckets, deliveryBuckets) {
		t.Errorf("duration series = %+v", dur)
	}
	assertLabelKeys(t, sink, metricDeliveriesAttempted, "channel", "driver", "result")
	assertLabelKeys(t, sink, metricDeliveryDuration, "channel", "driver", "result")
}

func TestSendRejectionTelemetry(t *testing.T) {
	cases := []struct {
		name    string
		req     *SendRequest
		reason  string
		errType string
		child   string // the child span that fails too, if any
		channel string
	}{
		{
			name:    "template missing",
			req:     &SendRequest{AppID: "app_a", Channel: "email", Template: "nope", To: []string{leakRecipient}},
			reason:  "template_not_found",
			errType: "herald.ErrTemplateNotFound",
			child:   "render template",
			channel: "email",
		},
		{
			name:    "no provider for the channel",
			req:     &SendRequest{AppID: "app_a", Channel: "sms", Body: leakBody, To: []string{leakRecipient}},
			reason:  "no_provider",
			errType: "herald.ErrNoProviderConfigured",
			child:   "resolve provider",
			channel: "sms",
		},
		{
			name:    "a channel the caller made up",
			req:     &SendRequest{AppID: "app_a", Channel: "pigeon-" + leakMeta, Body: leakBody, To: []string{leakRecipient}},
			reason:  "no_provider",
			errType: "herald.ErrNoProviderConfigured",
			child:   "resolve provider",
			channel: "other",
		},
		{
			name:    "driver not registered",
			req:     &SendRequest{AppID: "app_b", Channel: "email", Body: leakBody, To: []string{leakRecipient}},
			reason:  "driver_not_found",
			errType: "herald.ErrDriverNotFound",
			channel: "email",
		},
		{
			name:    "chosen provider unparseable",
			req:     &SendRequest{AppID: "app_a", Channel: "email", Body: leakBody, ProviderID: "bogus-" + leakMeta, To: []string{leakRecipient}},
			reason:  "provider_not_found",
			errType: "herald.ErrProviderNotFound",
			child:   "resolve provider",
			channel: "email",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := telemetrytest.Spans(t)
			sink := telemetrytest.NewSink()
			st := memory.New()
			h := newHerald(t, st, WithDriver(&recordingDriver{name: "rec", channel: "email"}), WithMetrics(sink))
			seedProvider(t, h, "app_a", "primary", "rec", 0, true)
			seedProvider(t, h, "app_b", "gone", "unregistered", 0, true)

			if _, err := h.Send(bg, tc.req); err == nil {
				t.Fatal("Send succeeded")
			}
			send := telemetrytest.One(t, rec, "send notification")
			if send.Status().Code != codes.Error || send.Status().Description != tc.reason {
				t.Errorf("send status = %+v, want %q", send.Status(), tc.reason)
			}
			if types := telemetrytest.ExceptionTypes(send); len(types) != 1 || types[0] != tc.errType {
				t.Errorf("exception types = %q, want %q", types, tc.errType)
			}
			if got := telemetrytest.Attrs(send)["herald.channel"]; got != tc.channel {
				t.Errorf("herald.channel = %q, want %q", got, tc.channel)
			}
			if tc.child != "" {
				if c := telemetrytest.One(t, rec, tc.child); c.Status().Description != tc.reason {
					t.Errorf("%s status = %+v", tc.child, c.Status())
				}
			}
			if got := sink.Get(metricSendsRejected, "channel", tc.channel, "reason", tc.reason).Value(); got != 1 {
				t.Errorf("rejections{%s,%s} = %v, want 1", tc.channel, tc.reason, got)
			}
			assertLabelKeys(t, sink, metricSendsRejected, "channel", "reason")
			for _, v := range []string{leakRecipient, leakMeta, leakBody, "nope on email"} {
				if leaks := telemetrytest.Leaks(rec, v); len(leaks) > 0 {
					t.Errorf("%q reached telemetry: %v", v, leaks)
				}
			}
			assertNoLabelLeaks(t, sink, leakMeta, "pigeon")
		})
	}
}

func TestSendDeliveryFailureTelemetry(t *testing.T) {
	rec := telemetrytest.Spans(t)
	sink := telemetrytest.NewSink()
	st := memory.New()
	h := newHerald(t, st, WithDriver(&recordingDriver{name: "rec", channel: "email", err: errors.New(leakVendor)}), WithMetrics(sink))
	seedProvider(t, h, "app_a", "primary", "rec", 0, true)

	res, err := h.Send(bg, &SendRequest{AppID: "app_a", Channel: "email", Body: "hi", To: []string{leakRecipient}})
	if err != nil || res.Status != message.StatusFailed {
		t.Fatalf("Send = %+v, %v", res, err)
	}
	send := telemetrytest.One(t, rec, "send notification")
	if send.Status().Code != codes.Error || send.Status().Description != "delivery_failed" {
		t.Errorf("status = %+v", send.Status())
	}
	if a := telemetrytest.Attrs(send); a["herald.delivery.sent"] != "0" || a["herald.delivery.failed"] != "1" {
		t.Errorf("attrs = %v", a)
	}
	if types := telemetrytest.ExceptionTypes(send); len(types) != 1 || types[0] != "*errors.errorString" {
		t.Errorf("exception types = %q", types)
	}
	if leaks := telemetrytest.Leaks(rec, leakRecipient); len(leaks) > 0 {
		t.Errorf("the driver's error reached telemetry: %v", leaks)
	}
	if got := sink.Get(metricDeliveriesAttempted, "channel", "email", "driver", "rec", "result", "error").Value(); got != 1 {
		t.Errorf("deliveries error = %v, want 1", got)
	}
	if got := sink.Get(metricDeliveryDuration, "channel", "email", "driver", "rec", "result", "error").Value(); got != 1 {
		t.Errorf("duration error count = %v, want 1", got)
	}
}

func TestSendSuppressedTelemetry(t *testing.T) {
	rec := telemetrytest.Spans(t)
	sink := telemetrytest.NewSink()
	st := memory.New()
	drv := &recordingDriver{name: "rec", channel: "email"}
	h := newHerald(t, st, WithDriver(drv), WithMetrics(sink))
	seedProvider(t, h, "app_a", "primary", "rec", 0, true)
	off := false
	if err := st.SetPreference(bg, &preference.Preference{
		ID: id.NewPreferenceID(), AppID: "app_a", UserID: "u1",
		Overrides: map[string]preference.ChannelPreference{"auth.welcome": {Email: &off}},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := h.Send(bg, &SendRequest{AppID: "app_a", UserID: "u1", Channel: "email", Template: "auth.welcome",
		To: []string{leakRecipient, "bob@example.com"}}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	send := telemetrytest.One(t, rec, "send notification")
	a := telemetrytest.Attrs(send)
	if a["herald.suppressed"] != "true" || a["herald.driver"] != "" {
		t.Errorf("attrs = %v", a)
	}
	if send.Status().Code == codes.Error {
		t.Errorf("an opted-out user isn't a failure: %+v", send.Status())
	}
	if got := sink.Get(metricMessagesSuppressed, "channel", "email").Value(); got != 2 {
		t.Errorf("suppressed = %v, want 2", got)
	}
	if n := len(sink.All(metricDeliveriesAttempted)); n != 0 {
		t.Errorf("a suppressed send recorded %d delivery series", n)
	}
}

func TestNotifyTelemetry(t *testing.T) {
	rec := telemetrytest.Spans(t)
	st := memory.New()
	h := newHerald(t, st, WithDriver(&recordingDriver{name: "rec", channel: "email"}))
	seedProvider(t, h, "app_a", "primary", "rec", 0, true)
	seedTemplate(t, st, "app_a", "auth.welcome", "email")
	seedTemplate(t, st, "app_a", "auth.welcome", "sms")

	results, err := h.Notify(bg, &NotifyRequest{AppID: "app_a", Template: "auth.welcome", Channels: []string{"email", "sms"},
		To: []string{leakRecipient}, Data: map[string]any{"code": leakData}})
	if err != nil || len(results) != 2 {
		t.Fatalf("Notify = %v, %v", results, err)
	}
	notify := telemetrytest.One(t, rec, "notify channels")
	a := telemetrytest.Attrs(notify)
	if a["herald.channel.count"] != "2" || a["herald.channel.failed"] != "1" || a["herald.template"] != "auth.welcome" || a["herald.recipient.count"] != "1" {
		t.Errorf("notify attrs = %v", a)
	}
	if notify.Status().Code != codes.Error || notify.Status().Description != "channel_failed" {
		t.Errorf("notify status = %+v", notify.Status())
	}
	sends := telemetrytest.Named(rec, "send notification")
	if len(sends) != 2 {
		t.Fatalf("want a send span per channel, got %d", len(sends))
	}
	for _, s := range sends {
		if s.Parent().SpanID() != notify.SpanContext().SpanID() {
			t.Error("a send span isn't a child of notify channels")
		}
	}
	for _, v := range []string{leakRecipient, leakData} {
		if leaks := telemetrytest.Leaks(rec, v); len(leaks) > 0 {
			t.Errorf("%q reached telemetry: %v", v, leaks)
		}
	}
}

// failingLog fails message log writes, the way a database outage would.
type failingLog struct {
	store.Store
	create, record bool
}

func (s *failingLog) CreateMessage(ctx context.Context, m *message.Message) error {
	if s.create {
		return errors.New("insert into herald_messages: " + leakRecipient)
	}
	return s.Store.CreateMessage(ctx, m)
}

func (s *failingLog) RecordDelivery(ctx context.Context, msgID id.MessageID, d message.Delivery) error {
	if s.record {
		return errors.New("update herald_messages failed")
	}
	return s.Store.RecordDelivery(ctx, msgID, d)
}

func TestMessageLogFailureMetric(t *testing.T) {
	for _, tc := range []struct {
		name           string
		create, record bool
		op             string
	}{
		{"create fails", true, false, "create"},
		{"record fails", false, true, "record_delivery"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink := telemetrytest.NewSink()
			st := &failingLog{Store: memory.New(), create: tc.create, record: tc.record}
			h := newHerald(t, st, WithDriver(&recordingDriver{name: "rec", channel: "email"}), WithMetrics(sink))
			seedProvider(t, h, "app_a", "primary", "rec", 0, true)

			if _, err := h.Send(bg, &SendRequest{AppID: "app_a", Channel: "email", Body: "hi", To: []string{leakRecipient, "bob@example.com"}}); err != nil {
				t.Fatalf("Send: %v", err)
			}
			if got := sink.Get(metricLogWritesFailed, "op", tc.op).Value(); got != 2 {
				t.Errorf("log failures{op=%s} = %v, want 2", tc.op, got)
			}
			assertLabelKeys(t, sink, metricLogWritesFailed, "op")
		})
	}
}

func TestChannelLabelIsClamped(t *testing.T) {
	for in, want := range map[string]string{
		"email": "email", "sms": "sms", "push": "push", "inapp": "inapp", "webhook": "webhook", "chat": "chat",
		"": "other", "EMAIL": "other", "fax": "other", leakRecipient: "other",
	} {
		if got := channelLabel(in); got != want {
			t.Errorf("channelLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
