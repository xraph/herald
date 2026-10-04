package extension

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/xraph/forge"
	dashauth "github.com/xraph/forge/extensions/dashboard/auth"
	dashcontract "github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"

	"github.com/xraph/herald/store/memory"
)

func TestContractContributorNeedsAnInitialisedHerald(t *testing.T) {
	// An extension that was never initialised skips registration quietly
	// rather than panicking the dashboard.
	var e Extension
	if err := e.RegisterContractContributor(dispatcher.New(nil), dashcontract.NewRegistry(), dashcontract.NewWardenRegistry()); err != nil {
		t.Fatalf("uninitialised extension: %v", err)
	}
}

// TestDashboardAppIDReachesTheContract registers a real extension, registers
// its contributor and asks engine.info, with no app claim, which app is in
// view. It covers DashboardAppID set in code and set in the config file.
func TestDashboardAppIDReachesTheContract(t *testing.T) {
	for name, tc := range map[string]struct {
		opts []ExtOption
		yaml map[string]any
		want string
	}{
		"programmatic": {
			opts: []ExtOption{WithConfig(Config{DashboardAppID: "app_code", DisableRoutes: true})},
			want: "app_code",
		},
		"config file": {
			yaml: map[string]any{"dashboard_app_id": "app_yaml", "disable_routes": true},
			want: "app_yaml",
		},
		"code fills a gap in the config file": {
			opts: []ExtOption{WithConfig(Config{DashboardAppID: "app_code"})},
			yaml: map[string]any{"disable_routes": true},
			want: "app_code",
		},
		"config file wins over code": {
			opts: []ExtOption{WithConfig(Config{DashboardAppID: "app_code"})},
			yaml: map[string]any{"dashboard_app_id": "app_yaml", "disable_routes": true},
			want: "app_yaml",
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := forge.DefaultAppConfig()
			cfg.Name = "herald-wiring-test"
			cfg.Logger = forge.NewNoopLogger()
			cfg.EnableConfigAutoDiscovery = false
			cfg.EnableEnvConfig = false
			app := forge.NewApp(cfg)
			if tc.yaml != nil {
				app.Config().Set("extensions.herald", tc.yaml)
			}
			e := New(append([]ExtOption{WithStore(memory.New())}, tc.opts...)...)
			if err := e.Register(app); err != nil {
				t.Fatalf("Register: %v", err)
			}

			d := dispatcher.New(nil)
			if err := e.RegisterContractContributor(d, dashcontract.NewRegistry(), dashcontract.NewWardenRegistry()); err != nil {
				t.Fatalf("RegisterContractContributor: %v", err)
			}
			data, _, err := d.Dispatch(context.Background(), dashcontract.Request{
				Envelope: "v1", Contributor: "herald", Intent: "engine.info", IntentVersion: 1,
				Kind: dashcontract.KindQuery, Params: map[string]any{},
			}, dashcontract.Principal{User: &dashauth.UserInfo{Subject: "operator-1"}})
			if err != nil {
				t.Fatalf("engine.info: %v", err)
			}
			var info struct {
				App struct {
					ID string `json:"id"`
				} `json:"app"`
			}
			if err := json.Unmarshal(data, &info); err != nil {
				t.Fatalf("decode %s: %v", data, err)
			}
			if info.App.ID != tc.want {
				t.Errorf("engine.info app = %q, want %q", info.App.ID, tc.want)
			}
		})
	}
}
