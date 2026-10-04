package contract

import (
	"testing"

	dashcontract "github.com/xraph/forge/extensions/dashboard/contract"
)

func TestResolveApp(t *testing.T) {
	deps := Deps{DefaultAppID: "configured"}
	cases := []struct {
		name   string
		claims map[string]any
		want   string
		denied bool
	}{
		{"a usable claim wins", map[string]any{"app_id": "app_x"}, "app_x", false},
		{"no claims uses the configured app", nil, "configured", false},
		{"other claims only uses the configured app", map[string]any{"org_id": "o"}, "configured", false},
		{"an empty claim is refused", map[string]any{"app_id": ""}, "", true},
		{"a blank claim is refused", map[string]any{"app_id": "  "}, "", true},
		{"a number is refused", map[string]any{"app_id": 42}, "", true},
		{"nil is refused", map[string]any{"app_id": nil}, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolveApp(dashcontract.Principal{Claims: c.claims}, deps)
			if c.denied {
				if codeOf(err) != dashcontract.CodePermissionDenied || got != "" {
					t.Fatalf("got %q, %v; want PERMISSION_DENIED and no app", got, err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("got %q, %v; want %q", got, err, c.want)
			}
		})
	}

	// With nothing configured, an absent claim means the "" app, which is an
	// exact match in every store (pinned by the store conformance suite).
	got, err := resolveApp(dashcontract.Principal{}, Deps{})
	if err != nil || got != "" {
		t.Fatalf("no claim, no config: got %q, %v; want the \"\" app", got, err)
	}
}

func TestActorFrom(t *testing.T) {
	if actorFrom(as(appA)) != "operator-1" {
		t.Error("the subject should be the actor")
	}
	if actorFrom(dashcontract.Principal{}) != "" {
		t.Error("no user means no actor")
	}
}
