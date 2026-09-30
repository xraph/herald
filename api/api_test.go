package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xraph/forge"

	"github.com/xraph/herald"
	"github.com/xraph/herald/api"
	"github.com/xraph/herald/driver/email"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/store/memory"
)

const canary = "sk_canary_api_value"

type harness struct {
	handler http.Handler
	st      *memory.Store
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	st := memory.New()
	h, err := herald.New(herald.WithStore(st), herald.WithDriver(&email.ResendDriver{}),
		herald.WithCredentialKey("k1", bytes.Repeat([]byte{7}, 32)))
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
	x := newHarness(t)
	p := x.createProvider(t, "app_a")
	if len(p.Credentials) != 1 || p.Credentials[0].Key != "api_key" ||
		p.Credentials[0].Protection != herald.ProtectionAESGCM || p.Credentials[0].KeyID != "k1" {
		t.Errorf("credentials = %+v", p.Credentials)
	}
	// do() fails the test on any response containing the canary.
	x.do(t, http.MethodGet, "/v1/providers?app_id=app_a", nil)
	x.do(t, http.MethodGet, "/v1/providers/"+p.ID+"?app_id=app_a", nil)
	x.do(t, http.MethodPut, "/v1/providers/"+p.ID+"?app_id=app_a", map[string]any{"name": "renamed"})
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

func TestScopedConfigReturnsTheStoredRow(t *testing.T) {
	x := newHarness(t)
	body := map[string]any{"app_id": "app_a", "webhook_provider_id": "hpvd_w", "chat_provider_id": "hpvd_c"}
	first := decode[map[string]any](t, x.do(t, http.MethodPut, "/v1/config/app", body))
	second := decode[map[string]any](t, x.do(t, http.MethodPut, "/v1/config/app", body))
	if first["id"] != second["id"] {
		t.Errorf("upsert answered two different IDs: %v, %v", first["id"], second["id"])
	}
	if second["webhook_provider_id"] != "hpvd_w" || second["chat_provider_id"] != "hpvd_c" {
		t.Errorf("webhook/chat not stored: %+v", second)
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
