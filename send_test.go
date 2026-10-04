package herald

import (
	"context"
	"errors"
	"maps"
	"testing"
	"unicode/utf8"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/preference"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/store/memory"
)

var bg = context.Background()

// recordingDriver captures every outbound message. It copies Data so a test
// can check exactly what the driver was handed.
type recordingDriver struct {
	name, channel string
	vendorID      string
	err           error
	sent          []*driver.OutboundMessage
}

func (d *recordingDriver) Name() string                          { return d.name }
func (d *recordingDriver) Channel() string                       { return d.channel }
func (d *recordingDriver) Validate(_, _ map[string]string) error { return nil }
func (d *recordingDriver) Send(_ context.Context, m *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	c := *m
	c.Data = maps.Clone(m.Data)
	d.sent = append(d.sent, &c)
	if d.err != nil {
		return nil, d.err
	}
	return &driver.DeliveryResult{ProviderMessageID: d.vendorID, Status: message.StatusSent}, nil
}

func newHerald(t *testing.T, st store.Store, opts ...Option) *Herald {
	t.Helper()
	h, err := New(append([]Option{WithStore(st)}, opts...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h
}

func seedProvider(t *testing.T, h *Herald, appID, name, driverName string, priority int, enabled bool) *provider.Provider {
	t.Helper()
	p := &provider.Provider{
		ID: id.NewProviderID(), AppID: appID, Name: name, Channel: "email", Driver: driverName,
		Credentials: map[string]string{"api_key": "secret-" + name},
		Settings:    map[string]string{"from": "no-reply@example.com"},
		Priority:    priority, Enabled: enabled,
	}
	if err := h.Store().CreateProvider(bg, p); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return p
}

func TestSendRecordsTheWholeDelivery(t *testing.T) {
	st := memory.New()
	rec := &recordingDriver{name: "rec", channel: "email", vendorID: "pm_1"}
	h := newHerald(t, st, WithDriver(rec))
	p := seedProvider(t, h, "app_a", "primary", "rec", 0, true)

	res, err := h.Send(bg, &SendRequest{AppID: "app_a", Channel: "email", To: []string{"ada@example.com"}, Subject: "Hi", Body: "Hello"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderID != p.ID.String() || res.ProviderMessageID != "pm_1" || !res.Logged {
		t.Errorf("result = %+v", res)
	}
	got, err := st.GetMessage(bg, res.MessageID)
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if got.Status != message.StatusSent || got.ProviderMessageID != "pm_1" || got.SentAt == nil || got.ProviderID != p.ID.String() {
		t.Errorf("stored message = %+v", got)
	}
}

func TestSendFailureIsRecordedNotReturned(t *testing.T) {
	st := memory.New()
	rec := &recordingDriver{name: "rec", channel: "email", err: errors.New("rec: API error 401: bad key")}
	h := newHerald(t, st, WithDriver(rec))
	seedProvider(t, h, "app_a", "primary", "rec", 0, true)

	res, err := h.Send(bg, &SendRequest{AppID: "app_a", Channel: "email", To: []string{"ada@example.com"}, Body: "Hello"})
	if err != nil {
		t.Fatalf("a provider failure is a result, not a Go error: %v", err)
	}
	if res.Status != message.StatusFailed || res.Error != "rec: API error 401: bad key" {
		t.Errorf("result = %+v", res)
	}
	got, _ := st.GetMessage(bg, res.MessageID)
	if got.Status != message.StatusFailed || got.SentAt != nil {
		t.Errorf("stored = %+v", got)
	}
}

func TestOptedOutIsSuppressedAndLogged(t *testing.T) {
	st := memory.New()
	rec := &recordingDriver{name: "rec", channel: "email"}
	h := newHerald(t, st, WithDriver(rec))
	seedProvider(t, h, "app_a", "primary", "rec", 0, true)
	off := false
	if err := st.SetPreference(bg, &preference.Preference{
		ID: id.NewPreferenceID(), AppID: "app_a", UserID: "u1",
		Overrides: map[string]preference.ChannelPreference{"auth.welcome": {Email: &off}},
	}); err != nil {
		t.Fatal(err)
	}

	res, err := h.Send(bg, &SendRequest{AppID: "app_a", UserID: "u1", Channel: "email", Template: "auth.welcome", To: []string{"ada@example.com"}})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSuppressed || res.MessageID.IsNil() || res.Error != "user opted out" {
		t.Errorf("result = %+v, want suppressed with a message ID", res)
	}
	if len(rec.sent) != 0 {
		t.Errorf("driver was called %d times for an opted-out user", len(rec.sent))
	}
	got, err := st.GetMessage(bg, res.MessageID)
	if err != nil || got.Status != message.StatusSuppressed {
		t.Errorf("stored = %+v, %v", got, err)
	}
}

func TestChosenProviderSkipsTheResolver(t *testing.T) {
	st := memory.New()
	rec := &recordingDriver{name: "rec", channel: "email"}
	h := newHerald(t, st, WithDriver(rec))
	seedProvider(t, h, "app_a", "preferred", "rec", 0, true)
	chosen := seedProvider(t, h, "app_a", "disabled-one", "rec", 5, false)

	res, err := h.Send(bg, &SendRequest{AppID: "app_a", Channel: "email", ProviderID: chosen.ID.String(), To: []string{"a@x"}, Body: "b"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.ProviderID != chosen.ID.String() {
		t.Errorf("sent through %s, want the chosen (disabled) provider %s", res.ProviderID, chosen.ID)
	}
}

func TestChosenProviderFromAnotherAppIsRefused(t *testing.T) {
	st := memory.New()
	rec := &recordingDriver{name: "rec", channel: "email"}
	h := newHerald(t, st, WithDriver(rec))
	seedProvider(t, h, "app_a", "mine", "rec", 0, true)
	theirs := seedProvider(t, h, "app_b", "theirs", "rec", 0, true)

	_, err := h.Send(bg, &SendRequest{AppID: "app_a", Channel: "email", ProviderID: theirs.ID.String(), To: []string{"a@x"}, Body: "b"})
	if !errors.Is(err, ErrProviderNotFound) {
		t.Errorf("err = %v, want ErrProviderNotFound", err)
	}
	if len(rec.sent) != 0 {
		t.Error("a provider from another app was used")
	}
	if msgs, _ := st.ListMessages(bg, "app_a", message.ListOptions{}); len(msgs) != 0 {
		t.Errorf("a refused send left %d message rows", len(msgs))
	}
}

func TestChosenProviderOnTheWrongChannelIsRefused(t *testing.T) {
	st := memory.New()
	h := newHerald(t, st, WithDriver(&recordingDriver{name: "rec", channel: "email"}))
	p := seedProvider(t, h, "app_a", "mail", "rec", 0, true)
	_, err := h.Send(bg, &SendRequest{AppID: "app_a", Channel: "sms", ProviderID: p.ID.String(), To: []string{"+1"}, Body: "b"})
	if !errors.Is(err, ErrInvalidChannel) {
		t.Errorf("err = %v, want ErrInvalidChannel", err)
	}
}

// brokenLog fails every CreateMessage, to prove a broken log never blocks a send.
type brokenLog struct{ *memory.Store }

func (brokenLog) CreateMessage(context.Context, *message.Message) error {
	return errors.New("disk full")
}

func TestLogFailureStillSends(t *testing.T) {
	st := brokenLog{memory.New()}
	rec := &recordingDriver{name: "rec", channel: "email"}
	h := newHerald(t, st, WithDriver(rec))
	seedProvider(t, h, "app_a", "primary", "rec", 0, true)

	res, err := h.Send(bg, &SendRequest{AppID: "app_a", Channel: "email", To: []string{"a@x"}, Body: "b"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.Logged || len(rec.sent) != 1 {
		t.Errorf("result = %+v, sends = %d; want sent, not logged, one send", res, len(rec.sent))
	}
}

func TestResolveProviderSaysWhy(t *testing.T) {
	st := memory.New()
	h := newHerald(t, st, WithDriver(&recordingDriver{name: "rec", channel: "email"}))
	fallback := seedProvider(t, h, "app_a", "fallback", "rec", 0, true)
	routed := seedProvider(t, h, "app_a", "routed", "rec", 9, true)

	res, err := h.ResolveProvider(bg, "app_a", "", "", "email")
	if err != nil || res.Provider.ID.String() != fallback.ID.String() || res.Via != scope.ViaFallback {
		t.Fatalf("no routing: %+v, %v", res, err)
	}
	if err = st.SetScopedConfig(bg, &scope.Config{
		ID: id.NewScopedConfigID(), AppID: "app_a", Scope: scope.ScopeApp, ScopeID: "app_a", EmailProviderID: routed.ID.String(),
	}); err != nil {
		t.Fatal(err)
	}
	res, err = h.ResolveProvider(bg, "app_a", "", "", "email")
	if err != nil || res.Provider.ID.String() != routed.ID.String() || res.Via != scope.ViaApp {
		t.Fatalf("app routing: %+v, %v", res, err)
	}
	none, err := h.ResolveProvider(bg, "app_a", "", "", "sms")
	if err != nil || none != nil {
		t.Errorf("nothing handles sms: got %+v, %v", none, err)
	}
}

func TestTruncateIsRuneSafe(t *testing.T) {
	got := truncate("héllo", 2) // é is bytes 1 and 2
	if got != "h" || !utf8.ValidString(got) {
		t.Errorf("truncate = %q", got)
	}
	if truncate("abc", 0) != "abc" || truncate("abc", 5) != "abc" {
		t.Error("truncate changed a string it should leave alone")
	}
}
