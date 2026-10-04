package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	dashcontract "github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"
	"github.com/xraph/forge/extensions/dashboard/contract/loader"
	"github.com/xraph/forge/extensions/dashboard/contract/transport"
)

// TestCommandsOverTheWireCarryTheirInvalidates posts one real command per
// area through forge's transport and checks the response tells the client
// exactly what the manifest says to refetch. forge v1.10.0 dropped these on
// the floor, which is why Task 1 moves to v1.11.2.
func TestCommandsOverTheWireCarryTheirInvalidates(t *testing.T) {
	e := newEnv(t, withKey())
	e.deps.DefaultAppID = appA // the transport passes no claims
	e.provider(t, appA, "primary")

	reg, wreg, d := dashcontract.NewRegistry(), dashcontract.NewWardenRegistry(), dispatcher.New(nil)
	if err := Register(d, reg, wreg, e.deps); err != nil {
		t.Fatal(err)
	}
	h := transport.NewHandler(reg, wreg, d, nil)
	m, err := loader.Load(bytes.NewReader(manifestYAML), "herald/contract/manifest.yaml")
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string][]string{}
	for _, in := range m.Intents {
		declared[in.Name] = in.Invalidates
	}

	commands := []struct {
		intent  string
		payload string
	}{
		{"providers.create", `{"name":"second","channel":"email","driver":"fake","enabled":true,"credentials":{"api_key":"k"},"settings":{}}`},
		{"providers.encryptStored", `{}`},
		{"templates.create", `{"slug":"wire.test","name":"Wire","channel":"email","category":"transactional"}`},
		{"send.test", `{"channel":"email","recipient":"ada@example.com","body":"Hello"}`},
		{"inbox.markAllRead", `{"userId":"user-1"}`},
		{"preferences.optOut", `{"userId":"user-1","type":"auth.welcome","channel":"email"}`},
		{"scopes.set", `{"scope":"app","fromName":"Wire"}`},
	}
	for _, c := range commands {
		body := `{"envelope":"v1","kind":"command","contributor":"herald","intent":"` + c.intent +
			`","intentVersion":1,"csrf":"test","idempotencyKey":"` + c.intent + `","payload":` + c.payload + `}`
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/dashboard/v1", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		var resp dashcontract.Response
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || !resp.OK {
			t.Errorf("%s: %s", c.intent, rec.Body)
			continue
		}
		if !reflect.DeepEqual(resp.Meta.Invalidates, declared[c.intent]) {
			t.Errorf("%s: meta.invalidates = %v, manifest says %v", c.intent, resp.Meta.Invalidates, declared[c.intent])
		}
	}
}
