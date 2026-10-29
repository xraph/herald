package extension

import (
	"testing"

	"github.com/xraph/forge"

	"github.com/xraph/herald/store/memory"
)

func TestExtensionRegistersSealedDriver(t *testing.T) {
	cfg := forge.DefaultAppConfig()
	cfg.Name = "herald-sealed-test"
	cfg.Logger = forge.NewNoopLogger()
	cfg.EnableConfigAutoDiscovery = false
	cfg.EnableEnvConfig = false
	app := forge.NewApp(cfg)

	e := New(WithStore(memory.New()), WithDisableRoutes())
	if err := e.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	d, err := e.Herald().Drivers().Get("sealed")
	if err != nil {
		t.Fatalf("Get(sealed): %v", err)
	}
	if d.Channel() != "push" {
		t.Errorf("Channel = %q", d.Channel())
	}
}
