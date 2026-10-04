package contract

import (
	"bytes"
	"strings"
	"testing"

	dashcontract "github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/loader"
)

// TestEveryCommandAuditsOnceAndNoQueryAudits dispatches every manifest intent
// once, in manifest order, with a payload that succeeds. Each command must
// write exactly one dashboard.<intent> event for operator-1 on app_a, and each
// query must write no event at all. A command with no payload row fails, so a
// new command can't skip its audit unnoticed.
func TestEveryCommandAuditsOnceAndNoQueryAudits(t *testing.T) {
	e := newEnv(t, withKey())
	d, _ := harness(t, e.deps)

	kept := e.provider(t, appA, "kept")
	doomed := e.provider(t, appA, "doomed")
	tmpl := e.template(t, appA, "auth.welcome", "email", "en", "de")
	gone := e.template(t, appA, "auth.gone", "email", "en")
	versions, err := e.st.ListVersions(bg, tmpl.ID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions: %v, %v", versions, err)
	}
	var en, de string
	for _, v := range versions {
		if v.Locale == "en" {
			en = v.ID.String()
		} else {
			de = v.ID.String()
		}
	}
	msg := seedMessages(t, e, appA, 1, kept.ID.String())[0]
	notes := seedInbox(t, e, appA, "user-1", 2)
	pid, tid := kept.ID.String(), tmpl.ID.String()

	queries := map[string]map[string]any{
		"engine.info":       {},
		"overview.stats":    {},
		"providers.list":    {},
		"providers.detail":  {"id": pid},
		"templates.list":    {},
		"templates.detail":  {"id": tid},
		"templates.resolve": {"id": tid, "locale": "en"},
		"templates.render":  {"templateId": tid, "content": map[string]any{"subject": "Hi {{.user_name}}"}, "data": map[string]any{"user_name": "Ada"}},
		"messages.list":     {},
		"messages.detail":   {"id": msg.ID.String()},
		"inbox.list":        {"userId": "user-1"},
		"preferences.get":   {"userId": "user-1"},
		"scopes.list":       {},
		"send.resolve":      {"channel": "email"},
	}
	commands := map[string]map[string]any{
		"providers.create": {
			"name": "created", "channel": "email", "driver": "fake", "enabled": true,
			"credentials": map[string]any{"api_key": canary},
		},
		"providers.update":        {"id": pid, "name": "renamed"},
		"providers.delete":        {"id": doomed.ID.String()},
		"providers.encryptStored": {},
		"templates.create":        {"slug": "billing.receipt", "name": "Receipt", "channel": "email"},
		"templates.update":        {"id": tid, "name": "Renamed"},
		"templates.delete":        {"id": gone.ID.String()},
		"templates.resetDefaults": {},
		"versions.create":         {"templateId": tid, "locale": "fr", "text": "Salut"},
		"versions.update":         {"templateId": tid, "versionId": en, "text": "Hello again"},
		"versions.delete":         {"templateId": tid, "versionId": de},
		"send.test":               {"channel": "email", "recipient": "ada@example.com", "body": "Hello"},
		"inbox.markRead":          {"id": notes[0].ID.String()},
		"inbox.markAllRead":       {"userId": "user-1"},
		"inbox.delete":            {"id": notes[1].ID.String()},
		"preferences.optOut":      {"userId": "user-1", "type": "auth.welcome", "channel": "sms"},
		"scopes.set":              {"scope": "org", "scopeId": "org-1", "fromName": "Org"},
		"scopes.delete":           {"scope": "org", "scopeId": "org-1"},
	}

	m, err := loader.Load(bytes.NewReader(manifestYAML), "herald/contract/manifest.yaml")
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	seen := map[string]bool{}
	for _, in := range m.Intents {
		seen[in.Name] = true
		cmd := in.Kind == dashcontract.IntentKindCommand
		body, ok := queries[in.Name]
		if cmd {
			body, ok = commands[in.Name]
		}
		if !ok {
			t.Errorf("%s has no payload row in this test", in.Name)
			continue
		}
		e.audits.reset()
		if _, err := call(d, as(appA), in.Name, cmd, body); err != nil {
			t.Errorf("%s: %v", in.Name, err)
			continue
		}
		events := e.audits.all()
		if !cmd {
			if len(events) != 0 {
				t.Errorf("query %s wrote %d audit events: %+v", in.Name, len(events), events)
			}
			continue
		}
		var dash int
		for _, ev := range events {
			if !strings.HasPrefix(ev.Action, "dashboard.") {
				continue // Herald's own events, such as notification.send
			}
			dash++
			if ev.Action != "dashboard."+in.Name || ev.ActorID != "operator-1" || ev.Tenant != appA {
				t.Errorf("%s wrote %+v", in.Name, ev)
			}
		}
		if dash != 1 {
			t.Errorf("%s wrote %d dashboard audit events, want 1: %+v", in.Name, dash, events)
		}
	}
	for _, table := range []map[string]map[string]any{queries, commands} {
		for name := range table {
			if !seen[name] {
				t.Errorf("%s has a payload row but is not in the manifest", name)
			}
		}
	}
}
