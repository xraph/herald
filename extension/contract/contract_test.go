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
