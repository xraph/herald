package contract

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xraph/grove"
	"github.com/xraph/grove/drivers/sqlitedriver"
	_ "github.com/xraph/grove/drivers/sqlitedriver/sqlitemigrate"

	"github.com/xraph/herald"
	"github.com/xraph/herald/provider"
	sqlitestore "github.com/xraph/herald/store/sqlite"
)

func sqliteDeps(t *testing.T) Deps {
	t.Helper()
	ctx := context.Background()
	sdb := sqlitedriver.New()
	if err := sdb.Open(ctx, filepath.Join(t.TempDir(), "herald.db")); err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	db, err := grove.Open(sdb)
	if err != nil {
		t.Fatalf("grove open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := sqlitestore.New(db)
	if err = s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	h, err := herald.New(herald.WithStore(s), herald.WithDriver(&fakeDriver{vendorID: "vendor-1"}))
	if err != nil {
		t.Fatal(err)
	}
	return Deps{Herald: h}
}

func TestSQLiteWrites(t *testing.T) {
	deps := sqliteDeps(t)
	p := &provider.Provider{
		AppID: appA, Name: "primary", Channel: "email", Driver: "fake",
		Credentials: map[string]string{"api_key": canary}, Settings: map[string]string{"from": "a@example.com"}, Enabled: true,
	}
	if err := deps.Herald.CreateProvider(bg, p); err != nil {
		t.Fatal(err)
	}

	t.Run("provider settings round-trip", func(t *testing.T) {
		if _, err := providersUpdateHandler(deps)(bg, providersUpdateRequest{
			ID: p.ID.String(), SetSettings: map[string]string{"from_name": "Ops"}, RemoveSettings: []string{"from"},
		}, as(appA)); err != nil {
			t.Fatal(err)
		}
		got, err := providersDetailHandler(deps)(bg, providersDetailRequest{ID: p.ID.String()}, as(appA))
		if err != nil {
			t.Fatal(err)
		}
		keys := map[string]bool{}
		for _, s := range got.Provider.Settings {
			keys[s.Key] = true
		}
		if !keys["from_name"] || keys["from"] {
			t.Errorf("settings = %+v", got.Provider.Settings)
		}
	})

	t.Run("opt-outs accumulate in one record", func(t *testing.T) {
		for _, ch := range []string{"email", "sms"} {
			if _, err := preferencesOptOutHandler(deps)(bg, preferencesOptOutRequest{UserID: "user-1", Type: "auth.welcome", Channel: ch}, as(appA)); err != nil {
				t.Fatal(err)
			}
		}
		got, err := preferencesGetHandler(deps)(bg, preferencesGetRequest{UserID: "user-1"}, as(appA))
		if err != nil || got.Preference == nil {
			t.Fatalf("preferences.get = %+v, %v", got, err)
		}
		cp := got.Preference.Overrides["auth.welcome"]
		if cp.Email == nil || *cp.Email || cp.SMS == nil || *cp.SMS {
			t.Errorf("overrides = %+v", got.Preference.Overrides)
		}
	})

	t.Run("a merged rule keeps its id", func(t *testing.T) {
		first, err := scopesSetHandler(deps)(bg, scopesSetRequest{Scope: "org", ScopeID: "org-1", EmailProviderID: ptr(p.ID.String())}, as(appA))
		if err != nil {
			t.Fatal(err)
		}
		second, err := scopesSetHandler(deps)(bg, scopesSetRequest{Scope: "org", ScopeID: "org-1", FromName: ptr("Org")}, as(appA))
		if err != nil {
			t.Fatal(err)
		}
		if second.Rule.ID != first.Rule.ID || second.Rule.Providers["email"] == nil {
			t.Errorf("first %+v, second %+v", first.Rule, second.Rule)
		}
		list, _ := scopesListHandler(deps)(bg, scopesListRequest{}, as(appA))
		if len(list.Rules) != 1 {
			t.Errorf("rules = %d, want 1", len(list.Rules))
		}
	})

	t.Run("template variables and duplicate locales", func(t *testing.T) {
		created, err := templatesCreateHandler(deps)(bg, templatesCreateRequest{
			Slug: "sqlite.test", Name: "SQLite", Channel: "email", Category: "transactional",
			Variables: []VariableWire{{Name: "user_name", Type: "string", Required: true}},
			Version:   &versionContent{Locale: "en", Subject: "Hi", Text: "Hi"},
		}, as(appA))
		if err != nil {
			t.Fatal(err)
		}
		detail, err := templatesDetailHandler(deps)(bg, templatesDetailRequest{ID: created.Template.ID}, as(appA))
		if err != nil || len(detail.Template.Variables) != 1 || !detail.Template.Variables[0].Required {
			t.Errorf("variables after a round-trip = %+v, %v", detail.Template.Variables, err)
		}
		if _, err := versionsCreateHandler(deps)(bg, versionsCreateRequest{TemplateID: created.Template.ID, Locale: "en", Text: "again"}, as(appA)); codeOf(err) != "CONFLICT" {
			t.Errorf("duplicate locale on SQLite: %v, want CONFLICT", err)
		}
	})

	t.Run("the delivery log pages newest first", func(t *testing.T) {
		for range 3 {
			if _, err := sendTestHandler(deps)(bg, sendTestRequest{Channel: "email", Recipient: "ada@example.com", Body: "Hello"}, as(appA)); err != nil {
				t.Fatal(err)
			}
		}
		first, err := messagesListHandler(deps)(bg, messagesListRequest{Limit: 2}, as(appA))
		if err != nil || len(first.Messages) != 2 || first.NextCursor == "" {
			t.Fatalf("page 1 = %+v, %v", first, err)
		}
		if first.Messages[0].CreatedAt.Before(first.Messages[1].CreatedAt) {
			t.Error("messages are not newest first")
		}
		second, err := messagesListHandler(deps)(bg, messagesListRequest{Limit: 2, Cursor: first.NextCursor}, as(appA))
		if err != nil || len(second.Messages) != 1 || second.NextCursor != "" {
			t.Errorf("page 2 = %+v, %v", second, err)
		}
	})
}
