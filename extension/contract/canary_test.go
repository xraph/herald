package contract

import (
	"encoding/json"
	"strings"
	"testing"

	dashcontract "github.com/xraph/forge/extensions/dashboard/contract"

	"github.com/xraph/herald"
)

// TestNoResponseCarriesACredential runs every declared query, and the
// commands that answer with a provider, with and without a credential key,
// and fails if a response or an error ever contains the canary credential or
// an encrypted value. A query added later without a row here fails too.
func TestNoResponseCarriesACredential(t *testing.T) {
	for name, opts := range map[string][]herald.Option{"plaintext": nil, "keyed": {withKey()}} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, opts...)
			d, intents := harness(t, e.deps)
			prov := e.provider(t, appA, "primary")
			tmpl := e.template(t, appA, "auth.welcome", "email", "en")
			pid, tid := prov.ID.String(), tmpl.ID.String()

			check := func(label string, data json.RawMessage, err error) {
				t.Helper()
				for _, s := range []string{string(data), errText(err)} {
					if strings.Contains(s, canary) || strings.Contains(s, "enc:v1:") {
						t.Errorf("%s leaked a credential: %s", label, s)
					}
				}
			}

			sent, err := call(d, as(appA), "send.test", true, map[string]any{"channel": "email", "recipient": "ada@example.com", "body": "Hello"})
			check("send.test", sent, err)
			var st struct {
				MessageID string `json:"messageId"`
			}
			if err = json.Unmarshal(sent, &st); err != nil || st.MessageID == "" {
				t.Fatalf("send.test: %s, %v", sent, err)
			}
			data, err := call(d, as(appA), "scopes.set", true, map[string]any{"scope": "app", "emailProviderId": pid})
			check("scopes.set", data, err)

			queries := map[string]map[string]any{
				"engine.info":       {},
				"overview.stats":    {"window": "30d"},
				"providers.list":    {},
				"providers.detail":  {"id": pid},
				"templates.list":    {},
				"templates.detail":  {"id": tid},
				"templates.resolve": {"id": tid, "locale": "en"},
				"templates.render":  {"templateId": tid, "content": map[string]any{"subject": "Hi {{.user_name}}"}, "data": map[string]any{"user_name": "Ada"}},
				"messages.list":     {},
				"messages.detail":   {"id": st.MessageID},
				"inbox.list":        {"userId": "user-1"},
				"preferences.get":   {"userId": "user-1"},
				"scopes.list":       {},
				"send.resolve":      {"channel": "email"},
			}
			for intent, cmd := range intents {
				if cmd {
					continue
				}
				params, ok := queries[intent]
				if !ok {
					t.Errorf("query %s has no row in the canary table", intent)
					continue
				}
				data, err := call(d, as(appA), intent, false, params)
				if err != nil {
					t.Errorf("%s: %v", intent, err)
				}
				check(intent, data, err)
			}

			commands := []struct {
				intent string
				body   map[string]any
				// refused is set for writes that must be turned away, so a
				// refusal that quietly becomes a success path fails.
				refused dashcontract.ErrorCode
			}{
				{"providers.create", map[string]any{"name": "second", "channel": "email", "driver": "fake", "enabled": true, "credentials": map[string]any{"api_key": canary}}, ""},
				{"providers.update", map[string]any{"id": pid, "setCredentials": map[string]any{"api_key": canary}}, ""},
				// Refused writes must not echo the value they refused.
				{"providers.update", map[string]any{"id": pid, "setCredentials": map[string]any{"base_url": canary}}, dashcontract.CodeBadRequest},
				{"providers.create", map[string]any{"name": "bad", "channel": "email", "driver": "fake", "credentials": map[string]any{"host": canary, "api_key": canary}}, dashcontract.CodeBadRequest},
			}
			if name == "keyed" {
				commands = append(commands, struct {
					intent  string
					body    map[string]any
					refused dashcontract.ErrorCode
				}{"providers.encryptStored", map[string]any{}, ""})
			}
			for _, c := range commands {
				data, err := call(d, as(appA), c.intent, true, c.body)
				if got := codeOf(err); got != c.refused {
					t.Errorf("%s %v: error code %q (%v), want %q", c.intent, c.body, got, err, c.refused)
				}
				check(c.intent, data, err)
			}
		})
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
