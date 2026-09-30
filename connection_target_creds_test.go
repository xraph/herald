package herald

import (
	"bytes"
	"errors"
	"log/slog"
	"maps"
	"strings"
	"testing"

	"github.com/xraph/herald/driver/email"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/store/memory"
)

// A connection target smuggled in through credentials must not move a
// provider either. Send merges credentials under settings, so a credential
// base_url or host reaches the driver whenever no setting shadows it.
func TestConnectionTargetInCredentialsCannotMoveAProvider(t *testing.T) {
	type setup struct {
		name, target string
		drivers      []Option
		input        func() *provider.Provider
		secrets      map[string]string
	}
	described := setup{
		name: "described driver, base_url", target: "base_url",
		drivers: []Option{WithDriver(newDescribed(""))},
		input:   func() *provider.Provider { return newProviderInput("app_a") },
		secrets: map[string]string{"api_key": "sk_new_value"},
	}
	noSchema := setup{
		name: "no-schema driver, host", target: "host",
		drivers: []Option{WithDriver(&recordingDriver{name: "rec", channel: "email"})},
		input: func() *provider.Provider {
			return &provider.Provider{AppID: "app_a", Name: "free-form", Channel: "email", Driver: "rec", Enabled: true,
				Credentials: map[string]string{"password": "sk_canary_value"}, Settings: map[string]string{}}
		},
		secrets: map[string]string{"password": "sk_new_value"},
	}
	smtp := setup{
		name: "smtp, host", target: "host",
		drivers: []Option{WithDriver(&email.SMTPDriver{})},
		input: func() *provider.Provider {
			return &provider.Provider{AppID: "app_a", Name: "mail", Channel: "email", Driver: "smtp", Enabled: true,
				Credentials: map[string]string{"username": "ops", "password": "sk_canary_value"},
				Settings:    map[string]string{"host": "smtp.vendor.example", "port": "587"}}
		},
		secrets: map[string]string{"password": "sk_new_value"},
	}

	for _, s := range []setup{described, noSchema, smtp} {
		for _, variant := range []string{"A: credential only", "B: remove the setting, set the credential"} {
			for _, reentered := range []bool{false, true} {
				name := s.name + "/" + variant
				if reentered {
					name += "/secrets re-entered"
				}
				t.Run(name, func(t *testing.T) {
					st := memory.New()
					h := newHerald(t, st, append(s.drivers, WithCredentialKey("k1", testKey(1)))...)
					p := s.input()
					if strings.HasPrefix(variant, "B") && p.Settings[s.target] == "" {
						p.Settings[s.target] = "vendor.example"
					}
					if err := h.CreateProvider(bg, p); err != nil {
						t.Fatalf("CreateProvider: %v", err)
					}
					before, _ := st.GetProvider(bg, p.ID)

					u := ProviderUpdate{SetCredentials: map[string]string{s.target: "attacker.example"}}
					if strings.HasPrefix(variant, "B") {
						u.RemoveSettings = []string{s.target}
					}
					if reentered {
						maps.Copy(u.SetCredentials, s.secrets)
					}
					_, err := h.UpdateProvider(bg, "app_a", p.ID, u)
					if !errors.Is(err, ErrInvalidProvider) {
						t.Fatalf("err = %v, want ErrInvalidProvider", err)
					}
					if !strings.Contains(err.Error(), s.target) || strings.Contains(err.Error(), "sk_") || strings.Contains(err.Error(), "attacker") {
						t.Errorf("the refusal must name %s and no value: %q", s.target, err)
					}
					after, _ := st.GetProvider(bg, p.ID)
					if !maps.Equal(after.Settings, before.Settings) || !maps.Equal(after.Credentials, before.Credentials) {
						t.Error("a refused update changed the stored row")
					}
				})
			}
		}
	}
}

func TestCreateRefusesSettingsInCredentials(t *testing.T) {
	h := newHerald(t, memory.New(), WithDriver(newDescribed("")), WithDriver(&email.SMTPDriver{}),
		WithDriver(&recordingDriver{name: "rec", channel: "email"}))
	cases := []struct {
		name, key string
		p         *provider.Provider
	}{
		{"base_url on a described driver", "base_url", func() *provider.Provider {
			p := newProviderInput("app_a")
			p.Credentials["base_url"] = "https://attacker.example"
			return p
		}()},
		{"host on a no-schema driver", "host", &provider.Provider{AppID: "app_a", Name: "x", Channel: "email", Driver: "rec",
			Credentials: map[string]string{"host": "attacker.example"}}},
		{"a setting-placed field of the schema", "port", &provider.Provider{AppID: "app_a", Name: "mail", Channel: "email", Driver: "smtp",
			Credentials: map[string]string{"port": "587"}, Settings: map[string]string{"host": "smtp.vendor.example"}}},
	}
	for _, c := range cases {
		err := h.CreateProvider(bg, c.p)
		if !errors.Is(err, ErrInvalidProvider) || !strings.Contains(err.Error(), c.key) {
			t.Errorf("%s: %v, want ErrInvalidProvider naming %s", c.name, err, c.key)
		}
	}
	if all, _ := h.Store().ListAllProviders(bg, "app_a"); len(all) != 0 {
		t.Errorf("refused creates stored %d providers", len(all))
	}
}

// A row written before this check, with base_url in credentials, can be
// moved to settings at the same value without re-entering secrets: the
// effective target doesn't change.
func TestLegacyTargetInCredentialsMovesToSettingsWithoutSecrets(t *testing.T) {
	st := memory.New()
	h := newHerald(t, st, WithDriver(newDescribed("")), WithCredentialKey("k1", testKey(1)))
	p := newProviderInput("app_a")
	p.ID = id.NewProviderID()
	p.Credentials["base_url"] = "https://api.vendor.example"
	sealed, err := h.seal(p.ID.String(), p.Credentials, []string{"api_key", "base_url", "username"})
	if err != nil {
		t.Fatal(err)
	}
	p.Credentials = sealed
	if err = st.CreateProvider(bg, p); err != nil {
		t.Fatal(err)
	}

	if _, err = h.UpdateProvider(bg, "app_a", p.ID, ProviderUpdate{
		RemoveCredentials: []string{"base_url"}, SetSettings: map[string]string{"base_url": "https://attacker.example"},
	}); !errors.Is(err, ErrInvalidProvider) || !strings.Contains(err.Error(), "api_key") {
		t.Errorf("moving a legacy credential target to a new server: %v, want a refusal naming api_key", err)
	}
	got, err := h.UpdateProvider(bg, "app_a", p.ID, ProviderUpdate{
		RemoveCredentials: []string{"base_url"}, SetSettings: map[string]string{"base_url": "https://api.vendor.example"},
	})
	if err != nil {
		t.Fatalf("moving it to settings at the same value: %v", err)
	}
	if _, ok := got.Credentials["base_url"]; ok || got.Settings["base_url"] != "https://api.vendor.example" {
		t.Errorf("credentials %v, settings %v", got.Credentials, got.Settings)
	}
}

func TestSeedingWarnsAboutATargetInCredentials(t *testing.T) {
	var logs bytes.Buffer
	st := memory.New()
	h := newHerald(t, st, WithDriver(&email.ResendDriver{}), WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))
	p := provider.Provider{ID: id.NewProviderID(), Name: "seeded", Channel: "email", Driver: "resend", Enabled: true,
		Credentials: map[string]string{"api_key": "sk_canary_value", "base_url": "https://api.vendor.example"}}
	if err := h.SeedConfiguredProviders(bg, []provider.Provider{p}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "failed validation") || !strings.Contains(logs.String(), "base_url") {
		t.Errorf("no warning about base_url in credentials: %s", logs.String())
	}
	if strings.Contains(logs.String(), "sk_canary_value") {
		t.Error("the warning leaked a credential value")
	}
	got, _ := st.GetProvider(bg, p.ID)
	if got.Credentials["base_url"] == "" || got.Settings["base_url"] != "" {
		t.Errorf("seeding must not move the key silently: credentials %v, settings %v", got.Credentials, got.Settings)
	}
}

// checkRetarget on its own, without ValidateProvider in front of it: the
// merged value decides, and a target set in credentials is always a move.
func TestCheckRetargetUsesTheMergedTarget(t *testing.T) {
	h := newHerald(t, memory.New(), WithDriver(newDescribed("")))
	existing := &provider.Provider{Driver: "described",
		Credentials: map[string]string{"api_key": "stored", "base_url": "https://legacy.example"},
		Settings:    map[string]string{"base_url": "https://api.vendor.example"}}
	before := existing.Credentials
	after := func(creds, settings map[string]string) *provider.Provider {
		return &provider.Provider{Driver: "described", Credentials: creds, Settings: settings}
	}

	// Removing the setting lets the credential copy through: a move.
	err := h.checkRetarget(existing, before, after(before, map[string]string{}), ProviderUpdate{RemoveSettings: []string{"base_url"}})
	if !errors.Is(err, ErrInvalidProvider) || !strings.Contains(err.Error(), "api_key") {
		t.Errorf("removing the shadowing setting: %v", err)
	}
	// A target set in credentials is a move even when a setting shadows it.
	smuggle := ProviderUpdate{SetCredentials: map[string]string{"base_url": "https://attacker.example"}}
	creds := map[string]string{"api_key": "stored", "base_url": "https://attacker.example"}
	if err := h.checkRetarget(existing, before, after(creds, existing.Settings), smuggle); !errors.Is(err, ErrInvalidProvider) {
		t.Errorf("a credential target under a setting: %v", err)
	}
	// Removing both leaves the vendor default: nothing to re-enter.
	if err := h.checkRetarget(existing, before, after(map[string]string{"api_key": "stored"}, map[string]string{}),
		ProviderUpdate{RemoveSettings: []string{"base_url"}, RemoveCredentials: []string{"base_url"}}); err != nil {
		t.Errorf("reverting to the vendor default: %v", err)
	}
}
