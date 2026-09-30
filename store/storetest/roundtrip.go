package storetest

import (
	"reflect"
	"testing"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/preference"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/store"
)

func testRoundTrips(t *testing.T, s store.Store) {
	p := newProvider("app_a", "primary", "email", 2, Base)
	must(t, "create provider", s.CreateProvider(ctx, p))
	gotP, err := s.GetProvider(ctx, p.ID)
	must(t, "get provider", err)
	if !reflect.DeepEqual(gotP.Credentials, p.Credentials) {
		t.Errorf("provider credentials: got %v, want %v", gotP.Credentials, p.Credentials)
	}
	if !reflect.DeepEqual(gotP.Settings, p.Settings) {
		t.Errorf("provider settings: got %v, want %v", gotP.Settings, p.Settings)
	}
	if gotP.Priority != 2 || !gotP.Enabled || !gotP.CreatedAt.Equal(Base) {
		t.Errorf("provider scalars: got priority=%d enabled=%v created=%v", gotP.Priority, gotP.Enabled, gotP.CreatedAt)
	}

	tmpl := newTemplate("app_a", "welcome", "email", Base)
	must(t, "create template", s.CreateTemplate(ctx, tmpl))
	en := newVersion(tmpl.ID, "en")
	must(t, "create version", s.CreateVersion(ctx, en))
	gotT, err := s.GetTemplate(ctx, tmpl.ID)
	must(t, "get template", err)
	if !reflect.DeepEqual(gotT.Variables, tmpl.Variables) {
		t.Errorf("template variables: got %+v, want %+v", gotT.Variables, tmpl.Variables)
	}
	if len(gotT.Versions) != 1 || gotT.Versions[0].HTML != en.HTML || gotT.Versions[0].Title != en.Title || !gotT.Versions[0].Active {
		t.Errorf("template versions: got %+v", gotT.Versions)
	}

	m := newMessage("app_a", "email", "sent", Base)
	must(t, "create message", s.CreateMessage(ctx, m))
	gotM, err := s.GetMessage(ctx, m.ID)
	must(t, "get message", err)
	if !reflect.DeepEqual(gotM.Metadata, m.Metadata) || gotM.EnvID != "env_1" || gotM.Attempts != 1 {
		t.Errorf("message: got metadata=%v env=%q attempts=%d", gotM.Metadata, gotM.EnvID, gotM.Attempts)
	}

	n := newNotification("app_a", "user_1", Base)
	must(t, "create notification", s.CreateNotification(ctx, n))
	gotN, err := s.GetNotification(ctx, n.ID)
	must(t, "get notification", err)
	if !reflect.DeepEqual(gotN.Metadata, n.Metadata) || gotN.ActionURL != n.ActionURL {
		t.Errorf("notification: got metadata=%v action=%q", gotN.Metadata, gotN.ActionURL)
	}

	off, on := false, true
	pref := &preference.Preference{
		ID: id.NewPreferenceID(), AppID: "app_a", UserID: "user_1",
		Overrides: map[string]preference.ChannelPreference{
			"auth.welcome":  {Email: &off, SMS: &on},
			"billing.alert": {Push: &off},
		},
		CreatedAt: Base, UpdatedAt: Base,
	}
	must(t, "set preference", s.SetPreference(ctx, pref))
	gotPref, err := s.GetPreference(ctx, "app_a", "user_1")
	must(t, "get preference", err)
	if !gotPref.IsOptedOut("auth.welcome", "email") || gotPref.IsOptedOut("auth.welcome", "sms") ||
		!gotPref.IsOptedOut("billing.alert", "push") || gotPref.Overrides["auth.welcome"].Push != nil {
		t.Errorf("preference overrides did not survive: %+v", gotPref.Overrides)
	}

	cfg := &scope.Config{
		ID: id.NewScopedConfigID(), AppID: "app_a", Scope: scope.ScopeOrg, ScopeID: "org_1",
		EmailProviderID: "hpvd_e", SMSProviderID: "hpvd_s", PushProviderID: "hpvd_p",
		WebhookProviderID: "hpvd_w", ChatProviderID: "hpvd_c",
		FromEmail: "no-reply@example.com", FromName: "Example", FromPhone: "+15550100", DefaultLocale: "fr",
		CreatedAt: Base, UpdatedAt: Base,
	}
	must(t, "set scoped config", s.SetScopedConfig(ctx, cfg))
	gotCfg, err := s.GetScopedConfig(ctx, "app_a", scope.ScopeOrg, "org_1")
	must(t, "get scoped config", err)
	for _, ch := range []string{"email", "sms", "push", "webhook", "chat"} {
		if gotCfg.ProviderIDFor(ch) != cfg.ProviderIDFor(ch) {
			t.Errorf("scoped config %s provider: got %q, want %q", ch, gotCfg.ProviderIDFor(ch), cfg.ProviderIDFor(ch))
		}
	}
	if gotCfg.FromPhone != cfg.FromPhone || gotCfg.DefaultLocale != "fr" {
		t.Errorf("scoped config from fields: got %+v", gotCfg)
	}
}

// testReadsAreCopies pins that a caller mutating what a store returned does
// not change what the store holds. SQL and Mongo get this for free; memory
// used to hand out its own pointers.
func testReadsAreCopies(t *testing.T, s store.Store) {
	p := newProvider("app_a", "primary", "email", 0, Base)
	must(t, "create provider", s.CreateProvider(ctx, p))
	p.Credentials["password"] = "changed-after-create"

	got, err := s.GetProvider(ctx, p.ID)
	must(t, "get provider", err)
	if got.Credentials["password"] != "pw-primary" {
		t.Errorf("mutating the created struct changed the store: %q", got.Credentials["password"])
	}
	got.Credentials["password"] = "changed-after-read"
	again, err := s.GetProvider(ctx, p.ID)
	must(t, "get provider again", err)
	if again.Credentials["password"] != "pw-primary" {
		t.Errorf("mutating a read changed the store: %q", again.Credentials["password"])
	}
}
