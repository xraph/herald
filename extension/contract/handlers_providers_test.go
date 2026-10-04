package contract

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/scope"
)

func TestProvidersListAndDetail(t *testing.T) {
	e := newEnv(t, withKey())
	mine := e.provider(t, appA, "primary")
	e.provider(t, appB, "theirs")

	list, err := providersListHandler(e.deps)(bg, providersListRequest{}, as(appA))
	if err != nil {
		t.Fatalf("providers.list: %v", err)
	}
	if len(list.Providers) != 1 || list.Providers[0].ID != mine.ID.String() {
		t.Fatalf("list = %+v, want only app_a's provider", list.Providers)
	}
	cred := list.Providers[0].Credentials
	if len(cred) != 1 || cred[0].Key != "api_key" || cred[0].Protection != "aes-256-gcm" || cred[0].KeyID != "k1" {
		t.Errorf("credentials = %+v", cred)
	}

	err = e.st.SetScopedConfig(bg, &scope.Config{
		ID: id.NewScopedConfigID(), AppID: appA, Scope: scope.ScopeOrg, ScopeID: "org_1", EmailProviderID: mine.ID.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := providersDetailHandler(e.deps)(bg, providersDetailRequest{ID: mine.ID.String()}, as(appA))
	if err != nil {
		t.Fatalf("providers.detail: %v", err)
	}
	if len(detail.Provider.UsedBy) != 1 || detail.Provider.UsedBy[0].ScopeID != "org_1" || detail.Provider.UsedBy[0].Channel != "email" {
		t.Errorf("usedBy = %+v", detail.Provider.UsedBy)
	}
	if len(detail.Provider.Settings) != 1 || detail.Provider.Settings[0].Key != "from" || detail.Provider.Settings[0].Value == nil {
		t.Errorf("settings = %+v", detail.Provider.Settings)
	}

	raw, _ := json.Marshal(detail)
	if strings.Contains(string(raw), canary) || strings.Contains(string(raw), "enc:v1:") {
		t.Fatalf("providers.detail leaked a credential: %s", raw)
	}
	rawList, _ := json.Marshal(list)
	if strings.Contains(string(rawList), canary) || strings.Contains(string(rawList), "enc:v1:") {
		t.Fatalf("providers.list leaked a credential: %s", rawList)
	}
}

func TestProvidersDetailOwnership(t *testing.T) {
	e := newEnv(t)
	theirs := e.provider(t, appB, "theirs")
	_, err := providersDetailHandler(e.deps)(bg, providersDetailRequest{ID: theirs.ID.String()}, as(appA))
	if codeOf(err) != "NOT_FOUND" {
		t.Errorf("another app's provider: %v, want NOT_FOUND", err)
	}
	_, err = providersDetailHandler(e.deps)(bg, providersDetailRequest{ID: "nope"}, as(appA))
	if codeOf(err) != "BAD_REQUEST" {
		t.Errorf("malformed id: %v, want BAD_REQUEST", err)
	}
}

func TestProvidersDetailHidesLegacySecretSettings(t *testing.T) {
	e := newEnv(t)
	p := e.provider(t, appA, "legacy")
	// A row written before placement rules existed, with a secret-schema key
	// in settings, straight into the store.
	stored, _ := e.st.GetProvider(bg, p.ID)
	stored.Settings["api_key"] = canary
	if err := e.st.UpdateProvider(bg, stored); err != nil {
		t.Fatal(err)
	}
	detail, err := providersDetailHandler(e.deps)(bg, providersDetailRequest{ID: p.ID.String()}, as(appA))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range detail.Provider.Settings {
		if s.Key == "api_key" {
			found = true
			if s.Value != nil || !s.Secret {
				t.Errorf("a secret setting came back with its value: %+v", s)
			}
		}
	}
	if !found {
		t.Errorf("the legacy api_key setting should be listed (valueless), got %+v", detail.Provider.Settings)
	}
	raw, _ := json.Marshal(detail)
	if strings.Contains(string(raw), canary) {
		t.Fatalf("providers.detail leaked a legacy secret setting: %s", raw)
	}
}

func TestProvidersListFiltersByChannelAndScopesToTheSession(t *testing.T) {
	e := newEnv(t)
	e.provider(t, appA, "mail")
	e.provider(t, appB, "other")

	got, err := providersListHandler(e.deps)(bg, providersListRequest{Channel: " email "}, as(appA))
	if err != nil || len(got.Providers) != 1 {
		t.Fatalf("channel=email: %+v, %v; want app_a's one provider", got.Providers, err)
	}
	got, err = providersListHandler(e.deps)(bg, providersListRequest{Channel: "sms"}, as(appA))
	if err != nil || got.Providers == nil || len(got.Providers) != 0 {
		t.Fatalf("channel=sms: %#v, %v; want an empty, non-nil list", got.Providers, err)
	}
	_, err = providersListHandler(e.deps)(bg, providersListRequest{}, as(""))
	if codeOf(err) != "PERMISSION_DENIED" {
		t.Errorf("unusable app claim: %v, want PERMISSION_DENIED", err)
	}
}

func TestProvidersReportPlaintextCredentialsWithoutValues(t *testing.T) {
	e := newEnv(t) // no credential key: the credential is stored as given
	p := e.provider(t, appA, "plain")

	list, err := providersListHandler(e.deps)(bg, providersListRequest{}, as(appA))
	if err != nil {
		t.Fatal(err)
	}
	cred := list.Providers[0].Credentials
	if len(cred) != 1 || cred[0].Key != "api_key" || cred[0].Protection == "aes-256-gcm" {
		t.Errorf("credentials = %+v, want one plaintext api_key", cred)
	}
	detail, err := providersDetailHandler(e.deps)(bg, providersDetailRequest{ID: " " + p.ID.String() + " "}, as(appA))
	if err != nil {
		t.Fatal(err)
	}
	if detail.Provider.UsedBy == nil {
		t.Error("usedBy is null, want an empty list")
	}
	raw, _ := json.Marshal([]any{list, detail})
	if strings.Contains(string(raw), canary) {
		t.Fatalf("a plaintext credential reached the wire: %s", raw)
	}
}
