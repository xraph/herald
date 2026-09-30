package herald

import (
	"context"
	"errors"
	"testing"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/store/memory"
)

func routeApp(t *testing.T, st *memory.Store, appID, emailProviderID string) {
	t.Helper()
	if err := st.SetScopedConfig(bg, &scope.Config{
		ID: id.NewScopedConfigID(), AppID: appID, Scope: scope.ScopeApp, ScopeID: appID, EmailProviderID: emailProviderID,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingRuleNamingAnotherAppsProviderIsIgnored(t *testing.T) {
	st := memory.New()
	h := newHerald(t, st, WithDriver(&recordingDriver{name: "rec", channel: "email"}))
	mine := seedProvider(t, h, "app_a", "mine", "rec", 0, true)
	theirs := seedProvider(t, h, "app_b", "theirs", "rec", 0, true)
	routeApp(t, st, "app_a", theirs.ID.String())

	res, err := h.ResolveProvider(bg, "app_a", "", "", "email")
	if err != nil {
		t.Fatalf("ResolveProvider: %v", err)
	}
	if res.Provider.ID.String() != mine.ID.String() || res.Via != scope.ViaFallback {
		t.Errorf("resolved %s via %s, want app_a's own provider %s via fallback", res.Provider.ID, res.Via, mine.ID)
	}
}

func TestRoutingRuleNamingAProviderOnAnotherChannelIsIgnored(t *testing.T) {
	st := memory.New()
	h := newHerald(t, st, WithDriver(&recordingDriver{name: "rec", channel: "email"}),
		WithDriver(&recordingDriver{name: "recsms", channel: "sms"}))
	mail := seedProvider(t, h, "app_a", "mail", "rec", 0, true)
	sms := &provider.Provider{
		ID: id.NewProviderID(), AppID: "app_a", Name: "texts", Channel: "sms", Driver: "recsms", Enabled: true,
	}
	if err := st.CreateProvider(bg, sms); err != nil {
		t.Fatal(err)
	}
	routeApp(t, st, "app_a", sms.ID.String())

	res, err := h.ResolveProvider(bg, "app_a", "", "", "email")
	if err != nil {
		t.Fatalf("ResolveProvider: %v", err)
	}
	if res.Provider.ID.String() != mail.ID.String() || res.Via != scope.ViaFallback {
		t.Errorf("resolved %s via %s, want the email provider %s via fallback", res.Provider.ID, res.Via, mail.ID)
	}
}

// failingLookups fails the lookups the resolver makes, with errors that are
// not "not found".
type failingLookups struct {
	*memory.Store
	scopeErr, providerErr error
}

func (s failingLookups) GetScopedConfig(ctx context.Context, appID string, st scope.ScopeType, scopeID string) (*scope.Config, error) {
	if s.scopeErr != nil {
		return nil, s.scopeErr
	}
	return s.Store.GetScopedConfig(ctx, appID, st, scopeID)
}

func (s failingLookups) GetProvider(ctx context.Context, pid id.ProviderID) (*provider.Provider, error) {
	if s.providerErr != nil {
		return nil, s.providerErr
	}
	return s.Store.GetProvider(ctx, pid)
}

func TestResolveProviderReturnsStoreFailures(t *testing.T) {
	down := errors.New("connection reset")

	mem := memory.New()
	h := newHerald(t, failingLookups{Store: mem, scopeErr: down}, WithDriver(&recordingDriver{name: "rec", channel: "email"}))
	seedProvider(t, h, "app_a", "fallback", "rec", 0, true)
	if res, err := h.ResolveProvider(bg, "app_a", "org_1", "u1", "email"); !errors.Is(err, down) {
		t.Errorf("a failed routing-rule lookup: got %+v, %v; want the store error", res, err)
	}

	mem = memory.New()
	h = newHerald(t, failingLookups{Store: mem, providerErr: down}, WithDriver(&recordingDriver{name: "rec", channel: "email"}))
	routed := seedProvider(t, h, "app_a", "routed", "rec", 0, true)
	routeApp(t, mem, "app_a", routed.ID.String())
	if res, err := h.ResolveProvider(bg, "app_a", "", "", "email"); !errors.Is(err, down) {
		t.Errorf("a failed provider lookup: got %+v, %v; want the store error", res, err)
	}
}
