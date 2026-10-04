package contract

import (
	"context"
	"testing"

	"github.com/xraph/herald"
	"github.com/xraph/herald/bridge"
)

func ptr[T any](v T) *T { return &v }

func TestScopesSetMergesAndList(t *testing.T) {
	e := newEnv(t)
	p := e.provider(t, appA, "primary")

	if _, err := scopesSetHandler(e.deps)(bg, scopesSetRequest{
		Scope: "org", ScopeID: "org-1", EmailProviderID: ptr(p.ID.String()), FromEmail: ptr("org@example.com"),
	}, as(appA)); err != nil {
		t.Fatal(err)
	}
	// A second write sends only fromName; the provider and fromEmail stay.
	got, err := scopesSetHandler(e.deps)(bg, scopesSetRequest{Scope: "org", ScopeID: "org-1", FromName: ptr("Org")}, as(appA))
	if err != nil {
		t.Fatal(err)
	}
	if got.Rule.Providers["email"] == nil || got.Rule.Providers["email"].ID != p.ID.String() || got.Rule.FromEmail != "org@example.com" || got.Rule.FromName != "Org" {
		t.Errorf("merged rule = %+v", got.Rule)
	}
	// An empty string clears a slot.
	got, err = scopesSetHandler(e.deps)(bg, scopesSetRequest{Scope: "org", ScopeID: "org-1", EmailProviderID: ptr("")}, as(appA))
	if err != nil {
		t.Fatalf("clearing a slot: %v", err)
	}
	if got.Rule.Providers["email"] != nil {
		t.Errorf("cleared slot = %+v", got.Rule.Providers["email"])
	}

	// The app rule's scopeId is always the app, whatever the client sends.
	app, err := scopesSetHandler(e.deps)(bg, scopesSetRequest{Scope: "app", ScopeID: "something-else", EmailProviderID: ptr(p.ID.String())}, as(appA))
	if err != nil || app.Rule.ScopeID != appA {
		t.Errorf("app rule = %+v, %v", app.Rule, err)
	}

	if err = e.h.DeleteProvider(bg, appA, p.ID); err != nil {
		t.Fatal(err)
	}
	list, err := scopesListHandler(e.deps)(bg, scopesListRequest{}, as(appA))
	if err != nil || len(list.Rules) != 2 {
		t.Fatalf("scopes.list = %+v, %v", list, err)
	}
	for _, r := range list.Rules {
		if r.Scope == "app" && (r.Providers["email"] == nil || !r.Providers["email"].Dangling) {
			t.Errorf("a deleted provider must show as dangling: %+v", r.Providers["email"])
		}
		if !r.DefaultLocaleUnused {
			t.Error("defaultLocaleUnused must be true: Send never reads a rule's default locale")
		}
	}
	theirs, err := scopesListHandler(e.deps)(bg, scopesListRequest{}, as(appB))
	if err != nil {
		t.Fatalf("app_b scopes.list: %v", err)
	}
	if len(theirs.Rules) != 0 {
		t.Errorf("app_b sees %d of app_a's rules", len(theirs.Rules))
	}
}

func TestScopesRefusals(t *testing.T) {
	e := newEnv(t)
	theirs := e.provider(t, appB, "theirs")
	mine := e.provider(t, appA, "mine")
	cases := map[string]scopesSetRequest{
		"unknown scope":          {Scope: "team", ScopeID: "t"},
		"org with no id":         {Scope: "org"},
		"another app's provider": {Scope: "org", ScopeID: "o", EmailProviderID: ptr(theirs.ID.String())},
		"email provider on sms":  {Scope: "org", ScopeID: "o", SMSProviderID: ptr(mine.ID.String())},
	}
	for name, in := range cases {
		if _, err := scopesSetHandler(e.deps)(bg, in, as(appA)); codeOf(err) != "BAD_REQUEST" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := scopesDeleteHandler(e.deps)(bg, scopesDeleteRequest{Scope: "org", ScopeID: "nobody"}, as(appA)); codeOf(err) != "NOT_FOUND" {
		t.Errorf("delete a rule that isn't there: %v", err)
	}
}

func TestScopesDelete(t *testing.T) {
	e := newEnv(t)
	if _, err := scopesSetHandler(e.deps)(bg, scopesSetRequest{Scope: "user", ScopeID: "u-1", FromName: ptr("U")}, as(appA)); err != nil {
		t.Fatal(err)
	}
	if _, err := scopesDeleteHandler(e.deps)(bg, scopesDeleteRequest{Scope: "user", ScopeID: "u-1"}, as(appB)); codeOf(err) != "NOT_FOUND" {
		t.Errorf("app_b deleting app_a's rule: %v", err)
	}
	if _, err := scopesDeleteHandler(e.deps)(bg, scopesDeleteRequest{Scope: "user", ScopeID: "u-1"}, as(appA)); err != nil {
		t.Fatal(err)
	}
	list, err := scopesListHandler(e.deps)(bg, scopesListRequest{}, as(appA))
	if err != nil {
		t.Fatalf("scopes.list after delete: %v", err)
	}
	if len(list.Rules) != 0 {
		t.Errorf("rules after delete = %+v", list.Rules)
	}
}

func TestScopesAudit(t *testing.T) {
	var events []*bridge.AuditEvent
	rec := bridge.ChronicleFunc(func(_ context.Context, ev *bridge.AuditEvent) error {
		events = append(events, ev)
		return nil
	})
	e := newEnv(t, herald.WithChronicle(rec))

	if _, err := scopesListHandler(e.deps)(bg, scopesListRequest{}, as(appA)); err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("scopes.list wrote %d audit events, want none", len(events))
	}
	if _, err := scopesSetHandler(e.deps)(bg, scopesSetRequest{Scope: "org", ScopeID: "org-1", FromName: ptr("Org")}, as(appA)); err != nil {
		t.Fatal(err)
	}
	if _, err := scopesDeleteHandler(e.deps)(bg, scopesDeleteRequest{Scope: "org", ScopeID: "org-1"}, as(appA)); err != nil {
		t.Fatal(err)
	}
	want := []string{"dashboard.scopes.set", "dashboard.scopes.delete"}
	if len(events) != len(want) {
		t.Fatalf("got %d audit events, want %d: %+v", len(events), len(want), events)
	}
	for i, ev := range events {
		if ev.Action != want[i] || ev.ActorID != "operator-1" || ev.Tenant != appA {
			t.Errorf("event %d = %+v", i, ev)
		}
		if ev.Metadata["scope"] != "org" || ev.Metadata["scope_id"] != "org-1" {
			t.Errorf("event %d metadata = %+v", i, ev.Metadata)
		}
	}
}
