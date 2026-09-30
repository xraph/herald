package storetest

import (
	"testing"
	"time"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/preference"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/template"
)

func testListTemplatesCarryVersions(t *testing.T, s store.Store) {
	tmpl := newTemplate("app_a", "welcome", "email", Base)
	must(t, "create template", s.CreateTemplate(ctx, tmpl))
	must(t, "create fr", s.CreateVersion(ctx, newVersion(tmpl.ID, "fr")))
	must(t, "create en", s.CreateVersion(ctx, newVersion(tmpl.ID, "en")))
	bare := newTemplate("app_a", "bare", "sms", Base.Add(time.Minute))
	must(t, "create bare", s.CreateTemplate(ctx, bare))

	all, err := s.ListTemplates(ctx, "app_a")
	must(t, "list templates", err)
	byChannel, err := s.ListTemplatesByChannel(ctx, "app_a", "email")
	must(t, "list templates by channel", err)

	check := func(what string, got []string) {
		t.Helper()
		sameOrder(t, what, got, []string{"en", "fr"})
	}
	for _, x := range all {
		if x.ID.String() == tmpl.ID.String() {
			check("ListTemplates versions", localesOf(x.Versions))
		}
		if x.ID.String() == bare.ID.String() && len(x.Versions) != 0 {
			t.Errorf("template with no versions came back with %d", len(x.Versions))
		}
	}
	if len(byChannel) != 1 {
		t.Fatalf("ListTemplatesByChannel: got %d templates, want 1", len(byChannel))
	}
	check("ListTemplatesByChannel versions", localesOf(byChannel[0].Versions))
}

func localesOf(versions []template.Version) []string {
	locales := make([]string, 0, len(versions))
	for _, v := range versions {
		locales = append(locales, v.Locale)
	}
	return locales
}

func testScopedConfigUpsert(t *testing.T, s store.Store) {
	first := &scope.Config{
		ID: id.NewScopedConfigID(), AppID: "app_a", Scope: scope.ScopeApp, ScopeID: "app_a",
		EmailProviderID: "hpvd_e1", WebhookProviderID: "hpvd_w1", ChatProviderID: "hpvd_c1",
		CreatedAt: Base, UpdatedAt: Base,
	}
	must(t, "insert", s.SetScopedConfig(ctx, first))

	second := &scope.Config{
		ID: id.NewScopedConfigID(), AppID: "app_a", Scope: scope.ScopeApp, ScopeID: "app_a",
		EmailProviderID: "hpvd_e2", WebhookProviderID: "hpvd_w2", ChatProviderID: "hpvd_c2",
		CreatedAt: Base.Add(time.Hour), UpdatedAt: Base.Add(time.Hour),
	}
	must(t, "upsert", s.SetScopedConfig(ctx, second))

	got, err := s.GetScopedConfig(ctx, "app_a", scope.ScopeApp, "app_a")
	must(t, "get", err)
	if got.EmailProviderID != "hpvd_e2" || got.WebhookProviderID != "hpvd_w2" || got.ChatProviderID != "hpvd_c2" {
		t.Errorf("upsert did not update every channel: email=%q webhook=%q chat=%q",
			got.EmailProviderID, got.WebhookProviderID, got.ChatProviderID)
	}
	if got.ID.String() != first.ID.String() {
		t.Errorf("upsert replaced the row's ID: got %s, want %s", got.ID, first.ID)
	}
	if !got.CreatedAt.Equal(Base) {
		t.Errorf("upsert replaced created_at: got %v, want %v", got.CreatedAt, Base)
	}
}

func testPreferenceUpsert(t *testing.T, s store.Store) {
	off := false
	first := &preference.Preference{
		ID: id.NewPreferenceID(), AppID: "app_a", UserID: "u1",
		Overrides: map[string]preference.ChannelPreference{"a": {Email: &off}},
		CreatedAt: Base, UpdatedAt: Base,
	}
	must(t, "insert", s.SetPreference(ctx, first))
	second := &preference.Preference{
		ID: id.NewPreferenceID(), AppID: "app_a", UserID: "u1",
		Overrides: map[string]preference.ChannelPreference{"b": {SMS: &off}},
		CreatedAt: Base.Add(time.Hour), UpdatedAt: Base.Add(time.Hour),
	}
	must(t, "upsert", s.SetPreference(ctx, second))

	got, err := s.GetPreference(ctx, "app_a", "u1")
	must(t, "get", err)
	if _, ok := got.Overrides["b"]; !ok || len(got.Overrides) != 1 {
		t.Errorf("upsert should replace overrides with the new set: got %+v", got.Overrides)
	}
	if got.ID.String() != first.ID.String() || !got.CreatedAt.Equal(Base) {
		t.Errorf("upsert replaced identity: id=%s created=%v", got.ID, got.CreatedAt)
	}
}
