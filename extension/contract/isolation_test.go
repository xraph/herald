package contract

import (
	"bytes"
	"encoding/json"
	"testing"

	dashauth "github.com/xraph/forge/extensions/dashboard/auth"
	dashcontract "github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"
	"github.com/xraph/forge/extensions/dashboard/contract/loader"
)

// harness registers the contributor and returns the dispatcher plus every
// declared intent, mapped to whether it's a command.
func harness(t *testing.T, deps Deps) (*dispatcher.Dispatcher, map[string]bool) {
	t.Helper()
	d := dispatcher.New(nil)
	if err := Register(d, dashcontract.NewRegistry(), dashcontract.NewWardenRegistry(), deps); err != nil {
		t.Fatalf("Register: %v", err)
	}
	m, err := loader.Load(bytes.NewReader(manifestYAML), "herald/contract/manifest.yaml")
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	intents := map[string]bool{}
	for _, in := range m.Intents {
		intents[in.Name] = in.Kind == dashcontract.IntentKindCommand
	}
	return d, intents
}

// call dispatches one intent the way the transport would: params for a
// query, a JSON payload for a command.
func call(d *dispatcher.Dispatcher, p dashcontract.Principal, intent string, cmd bool, body map[string]any) (json.RawMessage, error) {
	if body == nil {
		body = map[string]any{}
	}
	req := dashcontract.Request{
		Envelope: "v1", Contributor: ContributorName, Intent: intent, IntentVersion: 1,
		Kind: dashcontract.KindQuery, Params: body,
	}
	if cmd {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		req.Kind, req.Params, req.Payload = dashcontract.KindCommand, nil, raw
	}
	data, _, err := d.Dispatch(bg, req, p)
	return data, err
}

// TestByIDIntentsNeverCrossApps aims every by-ID intent at app_b's rows from
// an app_a session. Each must answer NOT_FOUND, the same as for a row that
// doesn't exist, and leave app_b's data as it was.
func TestByIDIntentsNeverCrossApps(t *testing.T) {
	e := newEnv(t)
	d, intents := harness(t, e.deps)
	prov := e.provider(t, appB, "theirs")
	tmpl := e.template(t, appB, "auth.welcome", "email", "en")
	versions, err := e.st.ListVersions(bg, tmpl.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %v, %v", versions, err)
	}
	msg := seedMessages(t, e, appB, 1, prov.ID.String())[0]
	note := seedInbox(t, e, appB, "user-1", 1)[0]
	pid, tid, vid := prov.ID.String(), tmpl.ID.String(), versions[0].ID.String()

	cases := []struct {
		intent string
		body   map[string]any
	}{
		{"providers.detail", map[string]any{"id": pid}},
		{"providers.update", map[string]any{"id": pid, "name": "renamed"}},
		{"providers.delete", map[string]any{"id": pid}},
		{"templates.detail", map[string]any{"id": tid}},
		{"templates.resolve", map[string]any{"id": tid, "locale": "en"}},
		{"templates.render", map[string]any{"templateId": tid, "content": map[string]any{"subject": "x"}}},
		{"templates.update", map[string]any{"id": tid, "name": "renamed"}},
		{"templates.delete", map[string]any{"id": tid}},
		{"versions.create", map[string]any{"templateId": tid, "locale": "fr", "text": "x"}},
		{"versions.update", map[string]any{"templateId": tid, "versionId": vid, "text": "x"}},
		{"versions.delete", map[string]any{"templateId": tid, "versionId": vid}},
		{"messages.detail", map[string]any{"id": msg.ID.String()}},
		{"inbox.markRead", map[string]any{"id": note.ID.String()}},
		{"inbox.delete", map[string]any{"id": note.ID.String()}},
		{"send.resolve", map[string]any{"channel": "email", "providerId": pid}},
		{"send.test", map[string]any{"channel": "email", "recipient": "a@example.com", "body": "x", "providerId": pid}},
	}
	// Every manifest intent is either a by-ID case above or named here as
	// not by-ID. An intent in neither fails, so a new by-ID intent can't
	// slip past this test.
	notByID := map[string]bool{
		"engine.info": true, "overview.stats": true,
		"providers.list": true, "providers.create": true, "providers.encryptStored": true,
		"templates.list": true, "templates.create": true, "templates.resetDefaults": true,
		"messages.list": true, "inbox.list": true, "inbox.markAllRead": true,
		"preferences.get": true, "preferences.optOut": true,
		"scopes.list": true, "scopes.set": true, "scopes.delete": true,
	}
	byID := map[string]bool{}
	for _, c := range cases {
		byID[c.intent] = true
		if notByID[c.intent] {
			t.Errorf("%s is both a by-ID case and on the not-by-ID list", c.intent)
		}
	}
	for intent := range intents {
		if !byID[intent] && !notByID[intent] {
			t.Errorf("%s is in neither the by-ID table nor the not-by-ID list", intent)
		}
	}
	for intent := range notByID {
		if _, declared := intents[intent]; !declared {
			t.Errorf("%s is on the not-by-ID list but not in the manifest", intent)
		}
	}

	for _, c := range cases {
		cmd, declared := intents[c.intent]
		if !declared {
			t.Errorf("%s is in this table but not in the manifest", c.intent)
			continue
		}
		if _, err := call(d, as(appA), c.intent, cmd, c.body); codeOf(err) != dashcontract.CodeNotFound {
			t.Errorf("%s on app_b's row from app_a: %v, want NOT_FOUND", c.intent, err)
		}
	}

	if got, err := e.st.GetProvider(bg, prov.ID); err != nil || got.Name != "theirs" {
		t.Errorf("app_b's provider afterwards: %+v, %v", got, err)
	}
	if got, err := e.st.GetTemplate(bg, tmpl.ID); err != nil || got.Name != tmpl.Name {
		t.Errorf("app_b's template afterwards: %+v, %v", got, err)
	}
	if got, err := e.st.ListVersions(bg, tmpl.ID); err != nil || len(got) != 1 || got[0].Text != versions[0].Text {
		t.Errorf("app_b's versions afterwards: %+v, %v", got, err)
	}
	if got, err := e.st.GetNotification(bg, note.ID); err != nil || got.Read {
		t.Errorf("app_b's notification afterwards: %+v, %v", got, err)
	}
	if len(e.drv.sent) != 0 {
		t.Errorf("send.test through app_b's provider reached the driver %d times", len(e.drv.sent))
	}
}

// TestUnusableClaimIsRefusedEverywhere sends each malformed app claim to
// every declared intent. A claim that is present but unusable must never
// fall back to the configured or "" app.
func TestUnusableClaimIsRefusedEverywhere(t *testing.T) {
	e := newEnv(t)
	e.deps.DefaultAppID = appA
	d, intents := harness(t, e.deps)
	for name, claim := range map[string]any{"empty": "", "blank": "   ", "number": 42, "null": nil, "list": []string{appA}} {
		p := dashcontract.Principal{User: &dashauth.UserInfo{Subject: "operator-1"}, Claims: map[string]any{"app_id": claim}}
		for intent, cmd := range intents {
			if _, err := call(d, p, intent, cmd, nil); codeOf(err) != dashcontract.CodePermissionDenied {
				t.Errorf("%s claim on %s: %v, want PERMISSION_DENIED", name, intent, err)
			}
		}
	}
}
