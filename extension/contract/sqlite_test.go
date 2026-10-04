package contract

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xraph/grove"
	"github.com/xraph/grove/drivers/sqlitedriver"
	_ "github.com/xraph/grove/drivers/sqlitedriver/sqlitemigrate"

	"github.com/xraph/herald"
	"github.com/xraph/herald/provider"
	sqlitestore "github.com/xraph/herald/store/sqlite"
)

// sqliteStore opens a fresh SQLite store in a temp directory.
func sqliteStore(t *testing.T) *sqlitestore.Store {
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
	return s
}

// sqliteDepsOn is an engine over s. Two engines over one store let a test
// write plaintext with one and read it back through a keyed one.
func sqliteDepsOn(t *testing.T, s *sqlitestore.Store, opts ...herald.Option) Deps {
	t.Helper()
	h, err := herald.New(append([]herald.Option{herald.WithStore(s), herald.WithDriver(&fakeDriver{vendorID: "vendor-1"})}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return Deps{Herald: h}
}

func sqliteDeps(t *testing.T) Deps { return sqliteDepsOn(t, sqliteStore(t)) }

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
		list, err := scopesListHandler(deps)(bg, scopesListRequest{}, as(appA))
		if err != nil {
			t.Fatalf("scopes.list: %v", err)
		}
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
			t.Fatalf("page 2 = %+v, %v", second, err)
		}
		all := append(append([]MessageSummary{}, first.Messages...), second.Messages...)
		seen := map[string]bool{}
		for i, m := range all {
			seen[m.ID] = true
			if i > 0 && m.CreatedAt.After(all[i-1].CreatedAt) {
				t.Errorf("message %d is newer than the one before it across the pages", i)
			}
		}
		if len(seen) != 3 {
			t.Errorf("distinct messages over both pages = %d, want 3", len(seen))
		}
	})
}

// TestSQLiteKeyedCredentials runs the credential-protection path on a real
// database: a plaintext row written before a key existed, one rewritten with
// the key, and the migration that encrypts what is left.
func TestSQLiteKeyedCredentials(t *testing.T) {
	s := sqliteStore(t)
	plain := sqliteDepsOn(t, s)
	keyed := sqliteDepsOn(t, s, withKey())
	mk := func(name string) *provider.Provider {
		p := &provider.Provider{
			AppID: appA, Name: name, Channel: "email", Driver: "fake",
			Credentials: map[string]string{"api_key": canary}, Settings: map[string]string{}, Enabled: true,
		}
		if err := plain.Herald.CreateProvider(bg, p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	updated, stored := mk("updated"), mk("stored")

	check := func(label string, v any, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		raw, merr := json.Marshal(v)
		if merr != nil {
			t.Fatal(merr)
		}
		if strings.Contains(string(raw), canary) || strings.Contains(string(raw), "enc:v1:") {
			t.Errorf("%s leaked a credential: %s", label, raw)
		}
	}
	protection := func(id string) string {
		t.Helper()
		got, err := providersDetailHandler(keyed)(bg, providersDetailRequest{ID: id}, as(appA))
		check("providers.detail", got, err)
		if len(got.Provider.Credentials) != 1 || got.Provider.Credentials[0].Key != "api_key" {
			t.Fatalf("credentials = %+v", got.Provider.Credentials)
		}
		return got.Provider.Credentials[0].Protection
	}
	if got := protection(stored.ID.String()); got == "aes-256-gcm" {
		t.Fatalf("a row written without a key reads as %q before encryptStored", got)
	}

	upd, err := providersUpdateHandler(keyed)(bg, providersUpdateRequest{
		ID: updated.ID.String(), SetCredentials: map[string]string{"api_key": canary + "_2"},
	}, as(appA))
	check("providers.update", upd, err)
	enc, err := providersEncryptStoredHandler(keyed)(bg, providersEncryptStoredRequest{}, as(appA))
	check("providers.encryptStored", enc, err)
	if enc.ValuesEncrypted != 1 {
		t.Errorf("encryptStored = %+v, want exactly the one plaintext value", enc)
	}
	for name, p := range map[string]*provider.Provider{"updated": updated, "stored": stored} {
		if got := protection(p.ID.String()); got != "aes-256-gcm" {
			t.Errorf("%s provider's credential protection = %q, want aes-256-gcm", name, got)
		}
	}
	list, err := providersListHandler(keyed)(bg, providersListRequest{}, as(appA))
	check("providers.list", list, err)
}
