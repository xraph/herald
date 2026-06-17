package herald

import (
	"context"
	"testing"

	"github.com/xraph/herald/driver/email"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/store/memory"
)

func newTestHerald(t *testing.T) (*Herald, *memory.Store) {
	t.Helper()
	st := memory.New()
	h, err := New(WithStore(st), WithDriver(&email.ResendDriver{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h, st
}

func TestSeedConfiguredProvidersCreates(t *testing.T) {
	h, st := newTestHerald(t)
	p := provider.Provider{ID: id.NewProviderID(), Name: "resend-main", Channel: "email", Driver: "resend", Enabled: true}

	if err := h.SeedConfiguredProviders(context.Background(), []provider.Provider{p}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, _ := st.ListAllProviders(context.Background(), "")
	if len(got) != 1 {
		t.Fatalf("provider count = %d, want 1", len(got))
	}
	if got[0].Name != "resend-main" {
		t.Errorf("name = %q", got[0].Name)
	}
}

func TestSeedConfiguredProvidersSkipsExisting(t *testing.T) {
	h, st := newTestHerald(t)
	p := provider.Provider{ID: id.NewProviderID(), Name: "resend-main", Channel: "email", Driver: "resend", Enabled: true}

	// First seed creates it.
	_ = h.SeedConfiguredProviders(context.Background(), []provider.Provider{p})
	// Second seed (different ID, same name) must be skipped.
	p2 := provider.Provider{ID: id.NewProviderID(), Name: "resend-main", Channel: "email", Driver: "resend", Enabled: true}
	if err := h.SeedConfiguredProviders(context.Background(), []provider.Provider{p2}); err != nil {
		t.Fatalf("seed 2: %v", err)
	}
	got, _ := st.ListAllProviders(context.Background(), "")
	if len(got) != 1 {
		t.Fatalf("provider count = %d, want 1 (no duplicate)", len(got))
	}
}

func TestSeedConfiguredProvidersUnregisteredDriverDoesNotFail(t *testing.T) {
	h, st := newTestHerald(t)
	p := provider.Provider{ID: id.NewProviderID(), Name: "ghost", Channel: "email", Driver: "no-such-driver", Enabled: true}
	// Unknown driver: warn, still create the record, never error.
	if err := h.SeedConfiguredProviders(context.Background(), []provider.Provider{p}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, _ := st.ListAllProviders(context.Background(), "")
	if len(got) != 1 {
		t.Errorf("provider count = %d, want 1", len(got))
	}
}
