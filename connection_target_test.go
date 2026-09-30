package herald

import (
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/store/memory"
)

// withTarget creates a described-driver provider that already points at a
// custom server.
func withTarget(t *testing.T, h *Herald) *provider.Provider {
	t.Helper()
	p := newProviderInput("app_a")
	p.Settings["base_url"] = "https://api.vendor.example"
	p.Settings["host"] = "smtp.vendor.example"
	if err := h.CreateProvider(bg, p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestChangingAConnectionTargetNeedsTheSecretsAgain(t *testing.T) {
	for _, setting := range []string{"base_url", "host"} {
		t.Run(setting, func(t *testing.T) {
			st := memory.New()
			h := newHerald(t, st, WithDriver(newDescribed("")), WithCredentialKey("k1", testKey(1)))
			p := withTarget(t, h)
			before, _ := st.GetProvider(bg, p.ID)

			_, err := h.UpdateProvider(bg, "app_a", p.ID, ProviderUpdate{
				SetSettings: map[string]string{setting: "attacker.example"},
			})
			if !errors.Is(err, ErrInvalidProvider) {
				t.Fatalf("err = %v, want ErrInvalidProvider", err)
			}
			msg := err.Error()
			if !strings.Contains(msg, setting) || !strings.Contains(msg, "api_key") || strings.Contains(msg, "username") {
				t.Errorf("the refusal must name %s and the secret api_key, and only secrets: %q", setting, msg)
			}
			if strings.Contains(msg, "sk_canary_value") {
				t.Errorf("the refusal leaked a credential value: %q", msg)
			}
			after, _ := st.GetProvider(bg, p.ID)
			if !maps.Equal(after.Settings, before.Settings) || !maps.Equal(after.Credentials, before.Credentials) {
				t.Error("a refused update changed the stored row")
			}

			got, err := h.UpdateProvider(bg, "app_a", p.ID, ProviderUpdate{
				SetSettings:    map[string]string{setting: "new.vendor.example"},
				SetCredentials: map[string]string{"api_key": "sk_new_value"},
			})
			if err != nil {
				t.Fatalf("with the secret re-sent: %v", err)
			}
			stored, _ := st.GetProvider(bg, p.ID)
			if stored.Settings[setting] != "new.vendor.example" || got.Settings[setting] != "new.vendor.example" {
				t.Errorf("%s = %q, want the new target", setting, stored.Settings[setting])
			}
			if v := stored.Credentials["api_key"]; !strings.HasPrefix(v, "enc:v1:k1:") || v == before.Credentials["api_key"] {
				t.Errorf("the re-sent secret must be stored freshly encrypted: %q", v)
			}
			plain, err := h.open(stored)
			if err != nil || plain["api_key"] != "sk_new_value" {
				t.Errorf("decrypted api_key = %q, %v", plain["api_key"], err)
			}
		})
	}
}

func TestRevertingOrKeepingAConnectionTargetNeedsNoSecrets(t *testing.T) {
	st := memory.New()
	h := newHerald(t, st, WithDriver(newDescribed("")), WithCredentialKey("k1", testKey(1)))
	p := withTarget(t, h)

	if _, err := h.UpdateProvider(bg, "app_a", p.ID, ProviderUpdate{
		SetSettings: map[string]string{"base_url": "https://api.vendor.example", "host": "smtp.vendor.example"},
	}); err != nil {
		t.Errorf("setting the targets to their current values: %v", err)
	}
	got, err := h.UpdateProvider(bg, "app_a", p.ID, ProviderUpdate{RemoveSettings: []string{"base_url", "host"}})
	if err != nil {
		t.Fatalf("removing the targets: %v", err)
	}
	if _, ok := got.Settings["base_url"]; ok {
		t.Errorf("base_url was not removed: %+v", got.Settings)
	}
}

func TestChangingAConnectionTargetWithoutASchemaNeedsEveryCredential(t *testing.T) {
	st := memory.New()
	h := newHerald(t, st, WithDriver(&recordingDriver{name: "rec", channel: "email"}))
	p := &provider.Provider{
		ID: id.NewProviderID(), AppID: "app_a", Name: "free-form", Channel: "email", Driver: "rec", Enabled: true,
		Credentials: map[string]string{"api_key": "sk_canary_value", "username": "ops"},
		Settings:    map[string]string{"base_url": "https://api.vendor.example"},
	}
	if err := h.CreateProvider(bg, p); err != nil {
		t.Fatal(err)
	}
	move := map[string]string{"base_url": "https://attacker.example"}

	_, err := h.UpdateProvider(bg, "app_a", p.ID, ProviderUpdate{
		SetSettings: move, SetCredentials: map[string]string{"api_key": "sk_new_value"},
	})
	if !errors.Is(err, ErrInvalidProvider) || !strings.Contains(err.Error(), "username") || strings.Contains(err.Error(), "ops") {
		t.Fatalf("with one of two credentials re-sent: %v; want a refusal naming username", err)
	}
	if _, err := h.UpdateProvider(bg, "app_a", p.ID, ProviderUpdate{
		SetSettings: move, SetCredentials: map[string]string{"api_key": "sk_new_value", "username": "ops2"},
	}); err != nil {
		t.Errorf("with every credential re-sent: %v", err)
	}
}
