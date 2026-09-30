package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xraph/forge"

	"github.com/xraph/herald"
	"github.com/xraph/herald/api"
	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/email"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/store/memory"
)

const canary = "sk_canary_api_value"

type harness struct {
	handler http.Handler
	st      *memory.Store
}

// stubDriver stands in for the webhook and chat drivers, which live in their
// own modules.
type stubDriver struct{ name, channel string }

func (d stubDriver) Name() string                          { return d.name }
func (d stubDriver) Channel() string                       { return d.channel }
func (d stubDriver) Validate(_, _ map[string]string) error { return nil }
func (d stubDriver) Send(context.Context, *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	return &driver.DeliveryResult{Status: message.StatusSent}, nil
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return buildHarness(t, herald.WithCredentialKey("k1", bytes.Repeat([]byte{7}, 32)))
}

func buildHarness(t *testing.T, opts ...herald.Option) *harness {
	t.Helper()
	st := memory.New()
	h, err := herald.New(append([]herald.Option{herald.WithStore(st), herald.WithDriver(&email.ResendDriver{}),
		herald.WithDriver(stubDriver{"hook", "webhook"}), herald.WithDriver(stubDriver{"talk", "chat"})}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	router := forge.NewRouter()
	api.NewForgeAPI(st, h, forge.NewNoopLogger()).RegisterRoutes(router)
	return &harness{handler: router.Handler(), st: st}
}

func (x *harness) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == nil {
		r = httptest.NewRequestWithContext(t.Context(), method, path, nil)
	} else {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	x.handler.ServeHTTP(rec, r)
	if strings.Contains(rec.Body.String(), canary) {
		t.Fatalf("%s %s leaked a credential value: %s", method, path, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "enc:v1:") {
		t.Fatalf("%s %s leaked an encrypted credential: %s", method, path, rec.Body)
	}
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func (x *harness) createProvider(t *testing.T, appID string) api.ProviderResponse {
	t.Helper()
	rec := x.do(t, http.MethodPost, "/v1/providers", map[string]any{
		"app_id": appID, "name": "resend", "channel": "email", "driver": "resend",
		"credentials": map[string]string{"api_key": canary}, "enabled": true,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create provider: %d %s", rec.Code, rec.Body)
	}
	return decode[api.ProviderResponse](t, rec)
}

func TestCredentialValuesNeverLeave(t *testing.T) {
	for _, keyed := range []bool{true, false} {
		name := "plaintext store"
		if keyed {
			name = "encrypted store"
		}
		t.Run(name, func(t *testing.T) {
			x := buildHarness(t)
			protection, keyID := herald.ProtectionPlaintext, ""
			if keyed {
				x = newHarness(t)
				protection, keyID = herald.ProtectionAESGCM, "k1"
			}
			p := x.createProvider(t, "app_a")
			if len(p.Credentials) != 1 || p.Credentials[0].Key != "api_key" ||
				p.Credentials[0].Protection != protection || p.Credentials[0].KeyID != keyID {
				t.Errorf("credentials = %+v", p.Credentials)
			}
			// do() fails the test on any response containing the canary or
			// an encrypted value. Without a key the store holds the canary
			// itself, so a leak can't hide behind ciphertext.
			for _, r := range []struct {
				method, path string
				body         any
			}{
				{http.MethodGet, "/v1/providers?app_id=app_a", nil},
				{http.MethodGet, "/v1/providers?app_id=app_a&channel=email", nil},
				{http.MethodGet, "/v1/providers/" + p.ID + "?app_id=app_a", nil},
				{http.MethodPut, "/v1/providers/" + p.ID + "?app_id=app_a", map[string]any{"name": "renamed"}},
			} {
				if rec := x.do(t, r.method, r.path, r.body); rec.Code != http.StatusOK {
					t.Errorf("%s %s = %d %s", r.method, r.path, rec.Code, rec.Body)
				}
			}
		})
	}
}

func TestListRoutesNeedOnlyTheAppID(t *testing.T) {
	x := newHarness(t)
	for _, path := range []string{
		"/v1/providers?app_id=app_a",
		"/v1/templates?app_id=app_a",
		"/v1/messages?app_id=app_a",
		"/v1/messages?app_id=app_a&channel=email&status=sent&offset=0&limit=10",
		"/v1/inbox?app_id=app_a&user_id=u1",
		"/v1/inbox?app_id=app_a&user_id=u1&offset=0&limit=10",
	} {
		if rec := x.do(t, http.MethodGet, path, nil); rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d %s, want 200", path, rec.Code, rec.Body)
		}
	}
	for _, path := range []string{"/v1/providers", "/v1/templates", "/v1/messages"} {
		if rec := x.do(t, http.MethodGet, path, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s without app_id = %d, want 400", path, rec.Code)
		}
	}
}

func TestUpdateWithoutEnabledKeepsItEnabled(t *testing.T) {
	x := newHarness(t)
	p := x.createProvider(t, "app_a")
	rec := x.do(t, http.MethodPut, "/v1/providers/"+p.ID+"?app_id=app_a", map[string]any{"name": "renamed"})
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	got := decode[api.ProviderResponse](t, x.do(t, http.MethodGet, "/v1/providers/"+p.ID+"?app_id=app_a", nil))
	if !got.Enabled || got.Name != "renamed" {
		t.Errorf("after a PUT without enabled: %+v", got)
	}
	rec = x.do(t, http.MethodPut, "/v1/providers/"+p.ID+"?app_id=app_a", map[string]any{"priority": 0, "enabled": false})
	got = decode[api.ProviderResponse](t, rec)
	if got.Enabled || got.Priority != 0 {
		t.Errorf("explicit enabled=false and priority=0 must apply: %+v", got)
	}
}

func TestByIDRoutesCheckTheApp(t *testing.T) {
	x := newHarness(t)
	p := x.createProvider(t, "app_a")
	for _, q := range []string{"?app_id=app_b", ""} {
		if rec := x.do(t, http.MethodGet, "/v1/providers/"+p.ID+q, nil); rec.Code != http.StatusNotFound {
			t.Errorf("GET provider%s = %d, want 404", q, rec.Code)
		}
	}
	if rec := x.do(t, http.MethodDelete, "/v1/providers/"+p.ID+"?app_id=app_b", nil); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE from another app = %d, want 404", rec.Code)
	}
	if rec := x.do(t, http.MethodGet, "/v1/providers/"+p.ID+"?app_id=app_a", nil); rec.Code != http.StatusOK {
		t.Errorf("GET from its own app = %d", rec.Code)
	}
}

func TestErrorCodes(t *testing.T) {
	x := newHarness(t)
	if rec := x.do(t, http.MethodGet, "/v1/providers/"+id.NewProviderID().String(), nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing provider = %d, want 404", rec.Code)
	}
	if rec := x.do(t, http.MethodGet, "/v1/providers/not-an-id", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad ID = %d, want 400", rec.Code)
	}
	rec := x.do(t, http.MethodPost, "/v1/providers", map[string]any{
		"app_id": "app_a", "name": "x", "channel": "sms", "driver": "resend", "credentials": map[string]string{"api_key": canary},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("driver on the wrong channel = %d, want 400", rec.Code)
	}
	tmpl := map[string]any{"app_id": "app_a", "slug": "welcome", "name": "W", "channel": "email", "enabled": true}
	if rec := x.do(t, http.MethodPost, "/v1/templates", tmpl); rec.Code != http.StatusCreated {
		t.Fatalf("create template: %d %s", rec.Code, rec.Body)
	}
	if rec := x.do(t, http.MethodPost, "/v1/templates", tmpl); rec.Code != http.StatusConflict {
		t.Errorf("duplicate slug = %d, want 409", rec.Code)
	}
	if rec := x.do(t, http.MethodDelete, "/v1/config/org/org_1?app_id=app_a", nil); rec.Code != http.StatusNotFound {
		t.Errorf("deleting a missing org config = %d, want 404 (it used to panic)", rec.Code)
	}
}

func TestVersionMustBelongToTheTemplateInThePath(t *testing.T) {
	x := newHarness(t)
	mk := func(slug string) string {
		rec := x.do(t, http.MethodPost, "/v1/templates", map[string]any{"app_id": "app_a", "slug": slug, "name": slug, "channel": "email", "enabled": true})
		return decode[map[string]any](t, rec)["id"].(string)
	}
	t1, t2 := mk("one"), mk("two")
	rec := x.do(t, http.MethodPost, "/v1/templates/"+t1+"/versions?app_id=app_a", map[string]any{"locale": "en", "text": "hi"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create version: %d %s", rec.Code, rec.Body)
	}
	v := decode[map[string]any](t, rec)["id"].(string)
	if rec := x.do(t, http.MethodPut, "/v1/templates/"+t2+"/versions/"+v+"?app_id=app_a", map[string]any{"text": "x"}); rec.Code != http.StatusNotFound {
		t.Errorf("version through the wrong template = %d, want 404", rec.Code)
	}
	if rec := x.do(t, http.MethodPost, "/v1/templates/"+t1+"/versions?app_id=app_a", map[string]any{"locale": "en"}); rec.Code != http.StatusConflict {
		t.Errorf("duplicate locale = %d, want 409", rec.Code)
	}
}

func (x *harness) createStubProvider(t *testing.T, appID, channel, driverName string) string {
	t.Helper()
	rec := x.do(t, http.MethodPost, "/v1/providers", map[string]any{
		"app_id": appID, "name": driverName, "channel": channel, "driver": driverName, "enabled": true,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s provider: %d %s", channel, rec.Code, rec.Body)
	}
	return decode[api.ProviderResponse](t, rec).ID
}

func TestScopedConfigReturnsTheStoredRow(t *testing.T) {
	x := newHarness(t)
	hook := x.createStubProvider(t, "app_a", "webhook", "hook")
	talk := x.createStubProvider(t, "app_a", "chat", "talk")
	body := map[string]any{"app_id": "app_a", "webhook_provider_id": hook, "chat_provider_id": talk}
	first := x.do(t, http.MethodPut, "/v1/config/app", body)
	if first.Code != http.StatusOK {
		t.Fatalf("set config: %d %s", first.Code, first.Body)
	}
	second := decode[map[string]any](t, x.do(t, http.MethodPut, "/v1/config/app", body))
	if decode[map[string]any](t, first)["id"] != second["id"] {
		t.Errorf("upsert answered two different IDs: %s, %v", first.Body, second["id"])
	}
	if second["webhook_provider_id"] != hook || second["chat_provider_id"] != talk {
		t.Errorf("webhook/chat not stored: %+v", second)
	}
}

func TestScopedConfigRefusesProvidersItCannotUse(t *testing.T) {
	x := newHarness(t)
	theirs := x.createProvider(t, "app_b").ID
	mine := x.createProvider(t, "app_a").ID
	cases := []struct {
		name, path string
		body       map[string]any
	}{
		{"another app's provider", "/v1/config/app", map[string]any{"app_id": "app_a", "email_provider_id": theirs}},
		{"another app's provider on an org rule", "/v1/config/org/org_1", map[string]any{"app_id": "app_a", "email_provider_id": theirs}},
		{"another app's provider on a user rule", "/v1/config/user/u1", map[string]any{"app_id": "app_a", "email_provider_id": theirs}},
		{"an email provider in the sms slot", "/v1/config/app", map[string]any{"app_id": "app_a", "sms_provider_id": mine}},
		{"a provider that doesn't exist", "/v1/config/app", map[string]any{"app_id": "app_a", "chat_provider_id": id.NewProviderID().String()}},
		{"a malformed ID", "/v1/config/app", map[string]any{"app_id": "app_a", "push_provider_id": "hpvd_w"}},
	}
	for _, c := range cases {
		if rec := x.do(t, http.MethodPut, c.path, c.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", c.name, rec.Code, rec.Body)
		}
	}
	if configs, err := x.st.ListScopedConfigs(t.Context(), "app_a"); err != nil || len(configs) != 0 {
		t.Errorf("refused writes stored %d configs (err %v)", len(configs), err)
	}
	if rec := x.do(t, http.MethodPut, "/v1/config/app", map[string]any{"app_id": "app_a", "email_provider_id": mine}); rec.Code != http.StatusOK {
		t.Errorf("app_a's own email provider: %d %s", rec.Code, rec.Body)
	}
}

func TestEncryptRoute(t *testing.T) {
	x := newHarness(t)
	if err := x.st.CreateProvider(t.Context(), &provider.Provider{
		ID: id.NewProviderID(), AppID: "app_a", Name: "legacy", Channel: "email", Driver: "resend",
		Credentials: map[string]string{"api_key": canary}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	rec := x.do(t, http.MethodPost, "/v1/providers/encrypt?app_id=app_a", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("encrypt: %d %s", rec.Code, rec.Body)
	}
	if rep := decode[herald.EncryptReport](t, rec); rep.Providers != 1 || rep.ValuesEncrypted != 1 {
		t.Errorf("report = %+v", rep)
	}
}
