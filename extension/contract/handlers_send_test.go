package contract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/xraph/forge"

	"github.com/xraph/herald"
	"github.com/xraph/herald/bridge"
	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/provider"
)

func TestSendResolve(t *testing.T) {
	e := newEnv(t)
	p := e.provider(t, appA, "primary")

	got, err := sendResolveHandler(e.deps)(bg, sendResolveRequest{Channel: "email"}, as(appA))
	if err != nil || got.Provider == nil || got.Provider.ID != p.ID.String() || got.Via != "fallback" {
		t.Fatalf("send.resolve = %+v, %v", got, err)
	}
	if got.From.Email != "no-reply@example.com" {
		t.Errorf("from = %+v", got.From)
	}
	none, err := sendResolveHandler(e.deps)(bg, sendResolveRequest{Channel: "sms"}, as(appA))
	if err != nil || none.Provider != nil || none.Via != "none" {
		t.Errorf("nothing handles sms = %+v, %v; want a null provider, not an error", none, err)
	}
	theirs := e.provider(t, appB, "theirs")
	if _, err := sendResolveHandler(e.deps)(bg, sendResolveRequest{Channel: "email", ProviderID: theirs.ID.String()}, as(appA)); codeOf(err) != "NOT_FOUND" {
		t.Errorf("a chosen provider from another app: %v", err)
	}
}

func TestSendTestReportsEachOutcomeHonestly(t *testing.T) {
	e := newEnv(t)
	p := e.provider(t, appA, "primary")

	sent, err := sendTestHandler(e.deps)(bg, sendTestRequest{Channel: "email", Recipient: "ada@example.com", Subject: "Hi", Body: "Hello"}, as(appA))
	if err != nil {
		t.Fatalf("send.test: %v", err)
	}
	if sent.Status != "sent" || sent.ProviderMessageID != "vendor-1" || sent.Provider == nil || sent.Provider.ID != p.ID.String() || !sent.Logged {
		t.Errorf("sent = %+v", sent)
	}
	if len(e.drv.sent) != 1 || e.drv.sent[0].To != "ada@example.com" {
		t.Errorf("driver saw %d sends", len(e.drv.sent))
	}

	// A provider failure is a normal response, so the page can show exactly
	// what the provider said.
	e.drv.err = errors.New("fake: API error 401: bad key")
	failed, err := sendTestHandler(e.deps)(bg, sendTestRequest{Channel: "email", Recipient: "ada@example.com", Body: "Hello"}, as(appA))
	if err != nil {
		t.Fatalf("a provider failure must not be a contract error: %v", err)
	}
	if failed.Status != "failed" || !strings.Contains(failed.Error, "401") {
		t.Errorf("failed = %+v", failed)
	}
}

func TestSendTestRefusals(t *testing.T) {
	e := newEnv(t)
	e.provider(t, appA, "primary")
	cases := []struct {
		name string
		in   sendTestRequest
		code string
	}{
		{"no recipient", sendTestRequest{Channel: "email", Body: "x"}, "BAD_REQUEST"},
		{"unknown channel", sendTestRequest{Channel: "fax", Recipient: "a", Body: "x"}, "BAD_REQUEST"},
		{"nothing to send", sendTestRequest{Channel: "email", Recipient: "a"}, "BAD_REQUEST"},
		{"no provider for the channel", sendTestRequest{Channel: "sms", Recipient: "+1", Body: "x"}, "BAD_REQUEST"},
		{"template that doesn't exist", sendTestRequest{Channel: "email", Recipient: "a", Template: "nope"}, "NOT_FOUND"},
	}
	for _, c := range cases {
		if _, err := sendTestHandler(e.deps)(bg, c.in, as(appA)); string(codeOf(err)) != c.code {
			t.Errorf("%s: %v, want %s", c.name, err, c.code)
		}
	}
	if len(e.drv.sent) != 0 {
		t.Errorf("a refused test reached the driver %d times", len(e.drv.sent))
	}
}

// hookDriver runs after delegating to fakeDriver, so a test can change the
// world between the send and the handler's lookup of the provider.
type hookDriver struct {
	*fakeDriver
	after func()
}

func (d *hookDriver) Name() string { return "hook" }

func (d *hookDriver) Send(ctx context.Context, m *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	res, err := d.fakeDriver.Send(ctx, m)
	if d.after != nil {
		d.after()
	}
	return res, err
}

type warnLogger struct {
	forge.Logger
	warns []string
}

func (l *warnLogger) Warn(msg string, fields ...forge.Field) {
	l.warns = append(l.warns, msg)
	for _, f := range fields {
		l.warns = append(l.warns, fmt.Sprint(f))
	}
}

func TestSendTestNamesTheProviderEvenWhenTheLookupFails(t *testing.T) {
	log := &warnLogger{}
	hook := &hookDriver{fakeDriver: &fakeDriver{vendorID: "vendor-1"}}
	e := newEnv(t, herald.WithDriver(hook))
	e.deps.Logger = log
	p := &provider.Provider{
		AppID: appA, Name: "doomed", Channel: "email", Driver: "hook", Enabled: true,
		Credentials: map[string]string{"api_key": canary},
	}
	if err := e.h.CreateProvider(bg, p); err != nil {
		t.Fatal(err)
	}
	// The provider disappears after the driver delivered but before the
	// handler looks it up for its name.
	hook.after = func() {
		if err := e.h.DeleteProvider(bg, appA, p.ID); err != nil {
			t.Errorf("delete in hook: %v", err)
		}
	}

	got, err := sendTestHandler(e.deps)(bg, sendTestRequest{Channel: "email", Recipient: "ada@example.com", Body: "Hello"}, as(appA))
	if err != nil {
		t.Fatalf("a lookup failure must not be a contract error: %v", err)
	}
	if got.Status != "sent" || got.Provider == nil || got.Provider.ID != p.ID.String() {
		t.Fatalf("got = %+v, want status sent and the provider id", got)
	}
	if got.Provider.Name != "" || got.Provider.Driver != "" {
		t.Errorf("provider = %+v, want only the id when the lookup failed", got.Provider)
	}
	joined := strings.Join(log.warns, " ")
	if !strings.Contains(joined, p.ID.String()) || strings.Contains(joined, canary) {
		t.Errorf("warn log = %q, want the provider id and no credential", joined)
	}
}

func TestSendRefusesAnUnregisteredDriverTheSameWay(t *testing.T) {
	e := newEnv(t)
	ghost := &provider.Provider{
		ID: id.NewProviderID(), AppID: appA, Name: "ghost", Channel: "email", Driver: "not-registered", Enabled: true,
	}
	if err := e.st.CreateProvider(bg, ghost); err != nil {
		t.Fatal(err)
	}
	_, rerr := sendResolveHandler(e.deps)(bg, sendResolveRequest{Channel: "email"}, as(appA))
	_, terr := sendTestHandler(e.deps)(bg, sendTestRequest{Channel: "email", Recipient: "a", Body: "x"}, as(appA))
	if codeOf(rerr) != "BAD_REQUEST" || codeOf(terr) != "BAD_REQUEST" || rerr.Error() != terr.Error() {
		t.Errorf("send.resolve = %v, send.test = %v; want the same BAD_REQUEST", rerr, terr)
	}
}

func TestSendTestBodyAndTemplate(t *testing.T) {
	e := newEnv(t)
	e.provider(t, appA, "primary")
	e.template(t, appA, "welcome", "email", "")

	// A blank body is no body: the template is what goes out.
	got, err := sendTestHandler(e.deps)(bg, sendTestRequest{
		Channel: "email", Recipient: "ada@example.com", Template: "welcome", Body: "  ", Data: map[string]any{"user_name": "Ada"},
	}, as(appA))
	if err != nil || got.Status != "sent" {
		t.Fatalf("template with a blank body = %+v, %v", got, err)
	}
	if len(e.drv.sent) != 1 || e.drv.sent[0].Text != "Hi Ada" {
		t.Fatalf("driver saw %+v, want the rendered template", e.drv.sent)
	}

	// A real body next to a template is ambiguous, so it is refused.
	_, err = sendTestHandler(e.deps)(bg, sendTestRequest{
		Channel: "email", Recipient: "ada@example.com", Template: "welcome", Body: "Hello", Data: map[string]any{"user_name": "Ada"},
	}, as(appA))
	if codeOf(err) != "BAD_REQUEST" || !strings.Contains(err.Error(), "not both") {
		t.Errorf("template and body = %v, want BAD_REQUEST", err)
	}
	if len(e.drv.sent) != 1 {
		t.Errorf("the refusal reached the driver: %d sends", len(e.drv.sent))
	}

	// The body goes out trimmed.
	if _, err := sendTestHandler(e.deps)(bg, sendTestRequest{Channel: "email", Recipient: "ada@example.com", Body: "  Hello  "}, as(appA)); err != nil {
		t.Fatal(err)
	}
	if got := e.drv.sent[len(e.drv.sent)-1].Text; got != "Hello" {
		t.Errorf("text = %q, want the trimmed body", got)
	}
}

func TestSendTestOtherAppsProviderIsNotFound(t *testing.T) {
	e := newEnv(t)
	e.provider(t, appA, "primary")
	theirs := e.provider(t, appB, "theirs")
	_, err := sendTestHandler(e.deps)(bg, sendTestRequest{
		Channel: "email", Recipient: "ada@example.com", Body: "Hello", ProviderID: theirs.ID.String(),
	}, as(appA))
	if codeOf(err) != "NOT_FOUND" {
		t.Errorf("another app's provider: %v, want NOT_FOUND", err)
	}
	if len(e.drv.sent) != 0 {
		t.Errorf("reached the driver %d times", len(e.drv.sent))
	}
}

func TestSendAudit(t *testing.T) {
	var events []*bridge.AuditEvent
	rec := bridge.ChronicleFunc(func(_ context.Context, ev *bridge.AuditEvent) error {
		events = append(events, ev)
		return nil
	})
	e := newEnv(t, withKey(), herald.WithChronicle(rec))
	e.provider(t, appA, "primary")

	if _, err := sendResolveHandler(e.deps)(bg, sendResolveRequest{Channel: "email"}, as(appA)); err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("send.resolve wrote %d audit events, want none", len(events))
	}

	got, err := sendTestHandler(e.deps)(bg, sendTestRequest{Channel: "email", Recipient: "ada@example.com", Body: "Hello"}, as(appA))
	if err != nil {
		t.Fatal(err)
	}
	var sends []*bridge.AuditEvent
	for _, ev := range events {
		if ev.Action == "dashboard.send.test" {
			sends = append(sends, ev)
		}
	}
	if len(sends) != 1 || sends[0].ActorID != "operator-1" || sends[0].Tenant != appA {
		t.Fatalf("send.test audit events = %+v", sends)
	}
	for name, v := range map[string]any{"response": got, "audit event": sends[0]} {
		raw, _ := json.Marshal(v)
		if strings.Contains(string(raw), canary) || strings.Contains(string(raw), "enc:v1:") {
			t.Errorf("%s carries a credential value: %s", name, raw)
		}
	}
}
