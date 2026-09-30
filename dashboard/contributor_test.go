package dashboard

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xraph/forge/extensions/dashboard/contributor"

	"github.com/xraph/herald"
	"github.com/xraph/herald/credential"
	"github.com/xraph/herald/driver/email"
	"github.com/xraph/herald/store/memory"
)

func createForm(driverName string) contributor.Params {
	return contributor.Params{FormData: map[string]string{
		"action": "create_provider", "name": "mail", "channel": "email", "driver": driverName,
		"cred_key_0": "api_key", "cred_value_0": "sk_canary_dashboard",
	}}
}

func TestProviderCreateGoesThroughTheEngine(t *testing.T) {
	st := memory.New()
	h, err := herald.New(herald.WithStore(st), herald.WithDriver(&email.ResendDriver{}),
		herald.WithCredentialKey("k1", bytes.Repeat([]byte{3}, credential.KeySize)))
	if err != nil {
		t.Fatal(err)
	}
	c := New(nil, h)

	page, err := c.renderProviderCreate(t.Context(), "app_a", createForm("nope"))
	if err != nil {
		t.Fatal(err)
	}
	var html bytes.Buffer
	if err := page.Render(t.Context(), &html); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html.String(), "driver not found") || strings.Contains(html.String(), "sk_canary_dashboard") {
		t.Errorf("an invalid provider must show the validation error, and never the value, in the banner")
	}
	if all, _ := st.ListAllProviders(t.Context(), "app_a"); len(all) != 0 {
		t.Fatalf("an invalid provider was stored: %+v", all)
	}

	if _, err := c.renderProviderCreate(t.Context(), "app_a", createForm("resend")); err != nil {
		t.Fatal(err)
	}
	all, _ := st.ListAllProviders(t.Context(), "app_a")
	if len(all) != 1 {
		t.Fatalf("stored %d providers, want 1", len(all))
	}
	if v := all[0].Credentials["api_key"]; !strings.HasPrefix(v, "enc:v1:k1:") {
		t.Errorf("the dashboard stored api_key as %q, want it encrypted", v)
	}
}
