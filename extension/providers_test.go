package extension

import (
	"testing"
	"time"
)

func boolPtr(b bool) *bool { return &b }

func TestToProviderDefaults(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	pc := ProviderConfig{
		Name:        "cf-email",
		Channel:     "email",
		Driver:      "cloudflare",
		Credentials: map[string]string{"api_token": "t"},
		Priority:    5,
		// Enabled left nil -> should default to true
	}
	p := pc.toProvider(now)

	if p.Name != "cf-email" || p.Channel != "email" || p.Driver != "cloudflare" {
		t.Errorf("core fields wrong: %+v", p)
	}
	if p.Priority != 5 {
		t.Errorf("priority = %d, want 5", p.Priority)
	}
	if !p.Enabled {
		t.Error("Enabled should default to true when nil")
	}
	if p.Credentials["api_token"] != "t" {
		t.Errorf("credentials not mapped: %+v", p.Credentials)
	}
	if p.ID.String() == "" {
		t.Error("ID should be generated")
	}
	if !p.CreatedAt.Equal(now) || !p.UpdatedAt.Equal(now) {
		t.Error("timestamps should be set to now")
	}
}

func TestToProviderExplicitDisabled(t *testing.T) {
	p := ProviderConfig{Name: "x", Enabled: boolPtr(false)}.toProvider(time.Unix(0, 0))
	if p.Enabled {
		t.Error("Enabled should be false when explicitly set")
	}
}

func TestProvidersFromConfig(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	out := providersFromConfig([]ProviderConfig{{Name: "a"}, {Name: "b"}}, now)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].Name != "a" || out[1].Name != "b" {
		t.Errorf("names = %q,%q", out[0].Name, out[1].Name)
	}
}
