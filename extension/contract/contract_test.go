package contract

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	dashcontract "github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"
	"github.com/xraph/forge/extensions/dashboard/contract/loader"
)

func TestRegisterNeedsHerald(t *testing.T) {
	if err := Register(dispatcher.New(nil), dashcontract.NewRegistry(), dashcontract.NewWardenRegistry(), Deps{}); err == nil {
		t.Fatal("Register with no Herald: want an error")
	}
}

// TestEveryDeclaredIntentIsRegistered dispatches every intent the manifest
// declares and fails if the dispatcher doesn't know one. A handler's own
// NOT_FOUND or BAD_REQUEST is fine; "not registered" never is, because in the
// browser it only shows up as a 404.
func TestEveryDeclaredIntentIsRegistered(t *testing.T) {
	e := newEnv(t)
	d := dispatcher.New(nil)
	if err := Register(d, dashcontract.NewRegistry(), dashcontract.NewWardenRegistry(), e.deps); err != nil {
		t.Fatalf("Register: %v", err)
	}
	m, err := loader.Load(bytes.NewReader(manifestYAML), "herald/contract/manifest.yaml")
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if len(m.Intents) == 0 {
		t.Fatal("manifest declares no intents")
	}
	for _, intent := range m.Intents {
		req := dashcontract.Request{
			Envelope: "v1", Contributor: ContributorName, Intent: intent.Name, IntentVersion: 1,
			Kind: dashcontract.KindQuery, Params: map[string]any{},
		}
		if intent.Kind == dashcontract.IntentKindCommand {
			req.Kind = dashcontract.KindCommand
			req.Params = nil
			req.Payload = json.RawMessage(`{}`)
		}
		_, _, err := d.Dispatch(bg, req, noClaims())
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "not registered") {
			t.Errorf("%s is declared but not registered: %v", intent.Name, err)
		}
	}
}

// TestEveryCommandDeclaresInvalidates catches a command added without
// telling the client what to refetch, which reads in the browser as a write
// that silently failed.
func TestEveryCommandDeclaresInvalidates(t *testing.T) {
	m, err := loader.Load(bytes.NewReader(manifestYAML), "herald/contract/manifest.yaml")
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	for _, intent := range m.Intents {
		if intent.Kind == dashcontract.IntentKindCommand && len(intent.Invalidates) == 0 {
			t.Errorf("command %s declares no invalidates", intent.Name)
		}
	}
}

// specInvalidates is the spec's command table, written out on purpose and
// not read from the manifest: the manifest is what it checks.
var specInvalidates = map[string][]string{
	"providers.create":        {"providers.list", "providers.detail", "overview.stats", "send.resolve", "scopes.list"},
	"providers.update":        {"providers.list", "providers.detail", "overview.stats", "send.resolve", "scopes.list"},
	"providers.delete":        {"providers.list", "providers.detail", "overview.stats", "send.resolve", "scopes.list"},
	"providers.encryptStored": {"providers.list", "providers.detail", "overview.stats"},
	"templates.create":        {"templates.list", "templates.detail", "templates.resolve", "overview.stats", "preferences.get"},
	"templates.update":        {"templates.list", "templates.detail", "templates.resolve", "overview.stats", "preferences.get"},
	"templates.delete":        {"templates.list", "templates.detail", "templates.resolve", "overview.stats", "preferences.get"},
	"templates.resetDefaults": {"templates.list", "templates.detail", "templates.resolve", "overview.stats", "preferences.get"},
	"versions.create":         {"templates.list", "templates.detail", "templates.resolve", "overview.stats"},
	"versions.update":         {"templates.list", "templates.detail", "templates.resolve", "overview.stats"},
	"versions.delete":         {"templates.list", "templates.detail", "templates.resolve", "overview.stats"},
	"send.test":               {"messages.list", "messages.detail", "overview.stats", "inbox.list"},
	"inbox.markRead":          {"inbox.list"},
	"inbox.markAllRead":       {"inbox.list"},
	"inbox.delete":            {"inbox.list"},
	"preferences.optOut":      {"preferences.get"},
	"scopes.set":              {"scopes.list", "send.resolve", "providers.detail"},
	"scopes.delete":           {"scopes.list", "send.resolve", "providers.detail"},
}

// TestInvalidatesMatchTheSpecTable pins every command's invalidates to the
// spec's table, as sets, and every target to a declared query. A command with
// no row in the table fails, so adding one means deciding what it refreshes.
func TestInvalidatesMatchTheSpecTable(t *testing.T) {
	m, err := loader.Load(bytes.NewReader(manifestYAML), "herald/contract/manifest.yaml")
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	queries, commands := map[string]bool{}, map[string]bool{}
	for _, in := range m.Intents {
		if in.Kind == dashcontract.IntentKindCommand {
			commands[in.Name] = true
		} else {
			queries[in.Name] = true
		}
	}
	for _, in := range m.Intents {
		if in.Kind != dashcontract.IntentKindCommand {
			continue
		}
		want, ok := specInvalidates[in.Name]
		if !ok {
			t.Errorf("command %s has no row in the spec table", in.Name)
			continue
		}
		got := map[string]bool{}
		for _, target := range in.Invalidates {
			if got[target] {
				t.Errorf("%s lists %s twice", in.Name, target)
			}
			got[target] = true
			if !queries[target] {
				t.Errorf("%s invalidates %s, which is not a declared query", in.Name, target)
			}
		}
		wantSet := map[string]bool{}
		for _, target := range want {
			wantSet[target] = true
			if !got[target] {
				t.Errorf("%s is missing %s from its invalidates", in.Name, target)
			}
		}
		for target := range got {
			if !wantSet[target] {
				t.Errorf("%s invalidates %s, which the spec table does not list", in.Name, target)
			}
		}
	}
	for name := range specInvalidates {
		if !commands[name] {
			t.Errorf("spec table row %s is not a declared command", name)
		}
	}
}
