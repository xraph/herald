package herald

import (
	"errors"
	"testing"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/store/memory"
)

func TestPreviewSendMatchesSend(t *testing.T) {
	st := memory.New()
	h := newHerald(t, st, WithDriver(&recordingDriver{name: "rec", channel: "email"}))
	fallback := seedProvider(t, h, "app_a", "fallback", "rec", 0, true)

	got, err := h.PreviewSend(bg, &SendRequest{AppID: "app_a", Channel: "email"})
	if err != nil || got.Provider.ID.String() != fallback.ID.String() || got.Via != scope.ViaFallback {
		t.Fatalf("fallback = %+v, %v", got, err)
	}
	if got.From != "no-reply@example.com" {
		t.Errorf("from = %q, want the provider's from setting", got.From)
	}

	if err = st.SetScopedConfig(bg, &scope.Config{
		ID: id.NewScopedConfigID(), AppID: "app_a", Scope: scope.ScopeApp, ScopeID: "app_a",
		EmailProviderID: fallback.ID.String(), FromEmail: "routed@example.com", FromName: "Routed",
	}); err != nil {
		t.Fatal(err)
	}
	got, err = h.PreviewSend(bg, &SendRequest{AppID: "app_a", Channel: "email"})
	if err != nil || got.Via != scope.ViaApp || got.From != "routed@example.com" || got.FromName != "Routed" {
		t.Fatalf("routed = %+v, %v", got, err)
	}

	if _, err := h.PreviewSend(bg, &SendRequest{AppID: "app_a", Channel: "sms"}); !errors.Is(err, ErrNoProviderConfigured) {
		t.Errorf("nothing handles sms: %v", err)
	}
	other := seedProvider(t, h, "app_b", "theirs", "rec", 0, true)
	if _, err := h.PreviewSend(bg, &SendRequest{AppID: "app_a", Channel: "email", ProviderID: other.ID.String()}); !errors.Is(err, ErrProviderNotFound) {
		t.Errorf("a chosen provider from another app: %v", err)
	}
}

// SMS drivers keep their sender under their own setting key and fall back to
// it when OutboundMessage.From is empty, so the preview has to read the same
// key or it reports no sender for a provider that will send from one.
func TestPreviewSendReportsTheSMSDriversOwnSender(t *testing.T) {
	cases := []struct {
		name     string
		settings map[string]string
		want     string
	}{
		{"from_number, as twilio and vonage keep it", map[string]string{"from_number": "+15550100"}, "+15550100"},
		{"originator, as messagebird keeps it", map[string]string{"originator": "Acme"}, "Acme"},
		{"a plain from setting", map[string]string{"from": "+15550111"}, "+15550111"},
		{"from_number before a stray from", map[string]string{"from": "ignored", "from_number": "+15550100"}, "+15550100"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := memory.New()
			h := newHerald(t, st, WithDriver(&recordingDriver{name: "rec", channel: "sms"}))
			p := &provider.Provider{
				ID: id.NewProviderID(), AppID: "app_a", Name: "texts", Channel: "sms", Driver: "rec",
				Credentials: map[string]string{"auth_token": "secret"}, Settings: tc.settings, Enabled: true,
			}
			if err := st.CreateProvider(bg, p); err != nil {
				t.Fatal(err)
			}
			got, err := h.PreviewSend(bg, &SendRequest{AppID: "app_a", Channel: "sms"})
			if err != nil || got.From != tc.want {
				t.Fatalf("from = %q, %v; want %q", got.From, err, tc.want)
			}
		})
	}

	t.Run("a rule's from phone still wins", func(t *testing.T) {
		st := memory.New()
		h := newHerald(t, st, WithDriver(&recordingDriver{name: "rec", channel: "sms"}))
		p := &provider.Provider{
			ID: id.NewProviderID(), AppID: "app_a", Name: "texts", Channel: "sms", Driver: "rec",
			Settings: map[string]string{"from_number": "+15550100"}, Enabled: true,
		}
		if err := st.CreateProvider(bg, p); err != nil {
			t.Fatal(err)
		}
		if err := st.SetScopedConfig(bg, &scope.Config{
			ID: id.NewScopedConfigID(), AppID: "app_a", Scope: scope.ScopeApp, ScopeID: "app_a",
			SMSProviderID: p.ID.String(), FromPhone: "+15550199",
		}); err != nil {
			t.Fatal(err)
		}
		got, err := h.PreviewSend(bg, &SendRequest{AppID: "app_a", Channel: "sms"})
		if err != nil || got.From != "+15550199" {
			t.Fatalf("from = %q, %v; want the rule's from phone", got.From, err)
		}
	})
}

func TestPreviewSendRefusesAnUnregisteredDriverLikeSend(t *testing.T) {
	st := memory.New()
	h := newHerald(t, st, WithDriver(&recordingDriver{name: "rec", channel: "email"}))
	seedProvider(t, h, "app_a", "ghost", "not-registered", 0, true)

	_, sendErr := h.Send(bg, &SendRequest{AppID: "app_a", Channel: "email", To: []string{"ada@example.com"}, Body: "Hi"})
	if !errors.Is(sendErr, ErrDriverNotFound) {
		t.Fatalf("Send = %v, want ErrDriverNotFound", sendErr)
	}
	_, err := h.PreviewSend(bg, &SendRequest{AppID: "app_a", Channel: "email"})
	if !errors.Is(err, ErrDriverNotFound) || err.Error() != sendErr.Error() {
		t.Errorf("PreviewSend = %v, want the error Send returns: %v", err, sendErr)
	}
}
