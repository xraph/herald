package contract

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"maps"
	"sync"
	"testing"

	dashauth "github.com/xraph/forge/extensions/dashboard/auth"
	dashcontract "github.com/xraph/forge/extensions/dashboard/contract"

	"github.com/xraph/herald"
	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/store/memory"
	"github.com/xraph/herald/template"
)

const (
	appA = "app_a"
	appB = "app_b"
	// canary is a credential value that must never leave the server.
	canary = "sk_canary_contract_value"
)

var bg = context.Background()

// fakeDriver is an email driver with a schema: a required secret api_key and
// a base_url setting. It records what it was asked to send.
type fakeDriver struct {
	mu       sync.Mutex
	sent     []*driver.OutboundMessage
	vendorID string
	err      error
}

func (d *fakeDriver) Name() string    { return "fake" }
func (d *fakeDriver) Channel() string { return "email" }

func (d *fakeDriver) Validate(creds, _ map[string]string) error {
	if creds["api_key"] == "" {
		return errors.New("fake: api_key is required")
	}
	return nil
}

func (d *fakeDriver) Fields() []driver.Field {
	return []driver.Field{
		{Key: "api_key", Label: "API key", Required: true, Secret: true, Placement: driver.PlacementCredential},
		{Key: "base_url", Label: "API base URL", Placement: driver.PlacementSetting},
		{Key: "from", Label: "From address", Placement: driver.PlacementSetting},
	}
}

func (d *fakeDriver) Send(_ context.Context, m *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	c := *m
	c.Data = maps.Clone(m.Data)
	d.sent = append(d.sent, &c)
	if d.err != nil {
		return nil, d.err
	}
	return &driver.DeliveryResult{ProviderMessageID: d.vendorID, Status: message.StatusSent}, nil
}

type env struct {
	h    *herald.Herald
	st   *memory.Store
	drv  *fakeDriver
	deps Deps
}

func newEnv(t *testing.T, opts ...herald.Option) *env {
	t.Helper()
	st := memory.New()
	drv := &fakeDriver{vendorID: "vendor-1"}
	h, err := herald.New(append([]herald.Option{herald.WithStore(st), herald.WithDriver(drv)}, opts...)...)
	if err != nil {
		t.Fatalf("herald.New: %v", err)
	}
	return &env{h: h, st: st, drv: drv, deps: Deps{Herald: h}}
}

func withKey() herald.Option {
	return herald.WithCredentialKey("k1", bytes.Repeat([]byte{3}, 32))
}

// as is a session for appID with an operator subject.
func as(appID string) dashcontract.Principal {
	return dashcontract.Principal{
		User:   &dashauth.UserInfo{Subject: "operator-1"},
		Claims: map[string]any{"app_id": appID},
	}
}

// noClaims is a session with an operator and no app claim at all.
func noClaims() dashcontract.Principal {
	return dashcontract.Principal{User: &dashauth.UserInfo{Subject: "operator-1"}}
}

// encodeRaw base64url-encodes s without padding, for building bad cursors.
func encodeRaw(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

func codeOf(err error) dashcontract.ErrorCode {
	var ce *dashcontract.Error
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func (e *env) provider(t *testing.T, appID, name string) *provider.Provider {
	t.Helper()
	p := &provider.Provider{
		AppID: appID, Name: name, Channel: "email", Driver: "fake",
		Credentials: map[string]string{"api_key": canary},
		Settings:    map[string]string{"from": "no-reply@example.com"},
		Enabled:     true,
	}
	if err := e.h.CreateProvider(bg, p); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	return p
}

func (e *env) template(t *testing.T, appID, slug, channel string, locales ...string) *template.Template {
	t.Helper()
	tmpl := &template.Template{
		ID: id.NewTemplateID(), AppID: appID, Slug: slug, Name: "Template " + slug,
		Channel: channel, Category: template.CategoryTransactional, Enabled: true,
		Variables: []template.Variable{{Name: "user_name", Type: "string", Required: true}},
	}
	if err := e.st.CreateTemplate(bg, tmpl); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	for _, loc := range locales {
		v := &template.Version{
			ID: id.NewTemplateVersionID(), TemplateID: tmpl.ID, Locale: loc,
			Subject: "Hello {{.user_name}}", Text: "Hi {{.user_name}}", Active: true,
		}
		if err := e.st.CreateVersion(bg, v); err != nil {
			t.Fatalf("CreateVersion: %v", err)
		}
	}
	got, err := e.st.GetTemplate(bg, tmpl.ID)
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	return got
}
