package herald

import (
	"bytes"
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/xraph/herald/credential"
	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/store/memory"
)

func testKey(b byte) []byte { return bytes.Repeat([]byte{b}, credential.KeySize) }

// describedDriver declares api_key as a required secret and refuses a
// missing one, like a real API-key driver.
type describedDriver struct{ recordingDriver }

func (d *describedDriver) Fields() []driver.Field {
	return []driver.Field{{Key: "api_key", Label: "API key", Required: true, Secret: true, Placement: driver.PlacementCredential}}
}

func (d *describedDriver) Validate(creds, _ map[string]string) error {
	if creds["api_key"] == "" {
		return errors.New("described: missing api_key")
	}
	return nil
}

func newDescribed(vendorID string) *describedDriver {
	return &describedDriver{recordingDriver{name: "described", channel: "email", vendorID: vendorID}}
}

func newProviderInput(appID string) *provider.Provider {
	return &provider.Provider{
		AppID: appID, Name: "primary", Channel: "email", Driver: "described",
		Credentials: map[string]string{"api_key": "sk_canary_value", "username": "ops"},
		Settings:    map[string]string{"from": "no-reply@example.com"},
		Enabled:     true,
	}
}

func TestCreateEncryptsWithAKeyAndSendDecrypts(t *testing.T) {
	st := memory.New()
	drv := newDescribed("pm_9")
	h := newHerald(t, st, WithDriver(drv), WithCredentialKey("k1", testKey(1)))

	p := newProviderInput("app_a")
	if err := h.CreateProvider(bg, p); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	stored, _ := st.GetProvider(bg, p.ID)
	for k, v := range stored.Credentials {
		if !strings.HasPrefix(v, "enc:v1:k1:") {
			t.Errorf("credential %q stored as %q", k, v)
		}
	}
	for _, s := range h.CredentialStatus(stored) {
		if s.Protection != ProtectionAESGCM || s.KeyID != "k1" {
			t.Errorf("status = %+v", s)
		}
	}

	res, err := h.Send(bg, &SendRequest{AppID: "app_a", Channel: "email", To: []string{"a@x"}, Body: "b"})
	if err != nil || res.Status != message.StatusSent {
		t.Fatalf("Send = %+v, %v", res, err)
	}
	if got := drv.sent[0].Data["api_key"]; got != "sk_canary_value" {
		t.Errorf("driver received %q, want the decrypted value", got)
	}
}

func TestCreateWithoutAKeyStoresPlaintextAndSaysSo(t *testing.T) {
	st := memory.New()
	h := newHerald(t, st, WithDriver(newDescribed("")))
	p := newProviderInput("app_a")
	if err := h.CreateProvider(bg, p); err != nil {
		t.Fatal(err)
	}
	stored, _ := st.GetProvider(bg, p.ID)
	if stored.Credentials["api_key"] != "sk_canary_value" {
		t.Errorf("stored = %q", stored.Credentials["api_key"])
	}
	for _, s := range h.CredentialStatus(stored) {
		if s.Protection != ProtectionPlaintext || s.KeyID != "" {
			t.Errorf("status = %+v, want plaintext", s)
		}
	}
}

func TestValidateProvider(t *testing.T) {
	h := newHerald(t, memory.New(), WithDriver(newDescribed("")))

	wrongChannel := newProviderInput("a")
	wrongChannel.Channel = "sms"
	if err := h.ValidateProvider(wrongChannel); !errors.Is(err, ErrInvalidChannel) {
		t.Errorf("wrong channel: %v", err)
	}
	unknown := newProviderInput("a")
	unknown.Driver = "nope"
	if err := h.ValidateProvider(unknown); !errors.Is(err, ErrDriverNotFound) {
		t.Errorf("unknown driver: %v", err)
	}
	secretInSettings := newProviderInput("a")
	delete(secretInSettings.Credentials, "api_key")
	secretInSettings.Settings["api_key"] = "sk_canary_value"
	err := h.ValidateProvider(secretInSettings)
	if !errors.Is(err, ErrInvalidProvider) || strings.Contains(err.Error(), "sk_canary_value") {
		t.Errorf("secret in settings: %v", err)
	}
	missing := newProviderInput("a")
	delete(missing.Credentials, "api_key")
	if err := h.ValidateProvider(missing); !errors.Is(err, ErrInvalidProvider) {
		t.Errorf("driver refusal: %v", err)
	}
	noName := newProviderInput("a")
	noName.Name = "  "
	if err := h.ValidateProvider(noName); !errors.Is(err, ErrInvalidProvider) {
		t.Errorf("blank name: %v", err)
	}
}

func TestUpdateTouchesOnlyWhatItNames(t *testing.T) {
	st := memory.New()
	h := newHerald(t, st, WithDriver(newDescribed("")), WithCredentialKey("k1", testKey(1)))
	p := newProviderInput("app_a")
	if err := h.CreateProvider(bg, p); err != nil {
		t.Fatal(err)
	}
	before, _ := st.GetProvider(bg, p.ID)

	name := "renamed"
	got, err := h.UpdateProvider(bg, "app_a", p.ID, ProviderUpdate{
		Name:           &name,
		SetCredentials: map[string]string{"username": "new-ops"},
	})
	if err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	if got.Name != "renamed" || !got.Enabled {
		t.Errorf("name=%q enabled=%v; leaving Enabled nil must not disable", got.Name, got.Enabled)
	}
	if got.Credentials["api_key"] != before.Credentials["api_key"] {
		t.Error("an untouched credential was rewritten")
	}
	if got.Credentials["username"] == before.Credentials["username"] || !strings.HasPrefix(got.Credentials["username"], "enc:v1:k1:") {
		t.Errorf("a replaced credential must be re-encrypted: %q", got.Credentials["username"])
	}

	got, err = h.UpdateProvider(bg, "app_a", p.ID, ProviderUpdate{RemoveCredentials: []string{"username"}})
	if err != nil || len(got.Credentials) != 1 {
		t.Errorf("remove: %v, %v", got.Credentials, err)
	}
}

func TestUpdateRefusesWhenTheKeyIsGone(t *testing.T) {
	st := memory.New()
	old := newHerald(t, st, WithDriver(newDescribed("")), WithCredentialKey("k0", testKey(9)))
	p := newProviderInput("app_a")
	if err := old.CreateProvider(bg, p); err != nil {
		t.Fatal(err)
	}
	before, _ := st.GetProvider(bg, p.ID)

	rotated := newHerald(t, st, WithDriver(newDescribed("")), WithCredentialKey("k1", testKey(1)))
	name := "x"
	_, err := rotated.UpdateProvider(bg, "app_a", p.ID, ProviderUpdate{Name: &name})
	if !errors.Is(err, ErrCredentialKeyUnavailable) {
		t.Fatalf("err = %v, want ErrCredentialKeyUnavailable", err)
	}
	after, _ := st.GetProvider(bg, p.ID)
	if after.Name != before.Name || !maps.Equal(after.Credentials, before.Credentials) {
		t.Error("a refused update changed the stored row")
	}
}

func TestProviderOwnership(t *testing.T) {
	h := newHerald(t, memory.New(), WithDriver(newDescribed("")))
	p := newProviderInput("app_a")
	if err := h.CreateProvider(bg, p); err != nil {
		t.Fatal(err)
	}
	name := "x"
	if _, err := h.UpdateProvider(bg, "app_b", p.ID, ProviderUpdate{Name: &name}); !errors.Is(err, ErrProviderNotFound) {
		t.Errorf("update from another app: %v", err)
	}
	if err := h.DeleteProvider(bg, "app_b", p.ID); !errors.Is(err, ErrProviderNotFound) {
		t.Errorf("delete from another app: %v", err)
	}
	if _, err := h.GetProvider(bg, "app_b", p.ID); !errors.Is(err, ErrProviderNotFound) {
		t.Errorf("get from another app: %v", err)
	}
	if err := h.DeleteProvider(bg, "app_a", p.ID); err != nil {
		t.Errorf("delete from its own app: %v", err)
	}
}

func TestEncryptStoredCredentials(t *testing.T) {
	st := memory.New()
	plain := newHerald(t, st, WithDriver(newDescribed("")))
	p := newProviderInput("app_a")
	if err := plain.CreateProvider(bg, p); err != nil {
		t.Fatal(err)
	}
	if _, err := plain.EncryptStoredCredentials(bg, "app_a"); !errors.Is(err, ErrNoCredentialKey) {
		t.Errorf("no key: %v", err)
	}

	drv := newDescribed("")
	keyed := newHerald(t, st, WithDriver(drv), WithCredentialKey("k1", testKey(1)))
	rep, err := keyed.EncryptStoredCredentials(bg, "app_a")
	if err != nil || rep != (EncryptReport{Providers: 1, ValuesEncrypted: 2}) {
		t.Fatalf("first run = %+v, %v", rep, err)
	}
	rep, err = keyed.EncryptStoredCredentials(bg, "app_a")
	if err != nil || rep != (EncryptReport{AlreadyEncrypted: 2}) {
		t.Fatalf("second run = %+v, %v; it must be idempotent", rep, err)
	}
	if _, err := keyed.Send(bg, &SendRequest{AppID: "app_a", Channel: "email", To: []string{"a@x"}, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if drv.sent[0].Data["api_key"] != "sk_canary_value" {
		t.Error("send after encrypting did not decrypt")
	}
}

func TestSendWithAMissingKeyFailsOnTheRecord(t *testing.T) {
	st := memory.New()
	old := newHerald(t, st, WithDriver(newDescribed("")), WithCredentialKey("k0", testKey(9)))
	if err := old.CreateProvider(bg, newProviderInput("app_a")); err != nil {
		t.Fatal(err)
	}
	drv := newDescribed("")
	rotated := newHerald(t, st, WithDriver(drv))
	res, err := rotated.Send(bg, &SendRequest{AppID: "app_a", Channel: "email", To: []string{"a@x"}, Body: "b"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusFailed || !strings.Contains(res.Error, "k0") || len(drv.sent) != 0 {
		t.Errorf("result = %+v, sends = %d", res, len(drv.sent))
	}
	stored, _ := st.GetMessage(bg, res.MessageID)
	if !strings.Contains(stored.Error, "k0") {
		t.Errorf("the log must say which key is missing: %q", stored.Error)
	}
}

func TestSeededProvidersAreEncryptedToo(t *testing.T) {
	st := memory.New()
	h := newHerald(t, st, WithDriver(newDescribed("")), WithCredentialKey("k1", testKey(1)))
	p := provider.Provider{ID: id.NewProviderID(), AppID: "app_a", Name: "seeded", Channel: "email", Driver: "described",
		Credentials: map[string]string{"api_key": "sk_canary_value"}, Enabled: true}
	if err := h.SeedConfiguredProviders(bg, []provider.Provider{p}); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetProvider(bg, p.ID)
	if !credential.IsEncrypted(got.Credentials["api_key"]) {
		t.Errorf("seeded credential stored as %q", got.Credentials["api_key"])
	}
}

func TestPreviousKeysNeedACurrentOne(t *testing.T) {
	_, err := New(WithStore(memory.New()), WithPreviousCredentialKey("k0", testKey(9)))
	if err == nil {
		t.Error("a previous key with no current key was accepted")
	}
	_, err = New(WithStore(memory.New()), WithCredentialKey("k1", make([]byte, 16)))
	if err == nil {
		t.Error("a 16-byte key was accepted")
	}
}
