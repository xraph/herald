// Package contract wires Herald into the Forge dashboard's contract path. It
// registers the `herald` contributor with the dashboard's contract registry
// and answers its intents from the live Herald engine.
//
// This package is the surface the React dashboard shell reads. Every handler
// resolves the app from the session (scope.go) before it reads or writes
// anything, and no request field ever names an app.
package contract

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"

	"github.com/xraph/forge"
	"github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"
	"github.com/xraph/forge/extensions/dashboard/contract/loader"

	"github.com/xraph/herald"
)

//go:embed manifest.yaml
var manifestYAML []byte

// ContributorName is the join key with packages/plugin-herald's `extension`
// field. A mismatch hides the React plugin with no error anywhere, because
// that is what an uninstalled extension looks like.
const ContributorName = "herald"

// Deps bundles what the handlers need.
type Deps struct {
	// Herald is the engine. Required.
	Herald *herald.Herald

	// Logger receives an Error-level entry for every error a handler maps to
	// INTERNAL. Optional.
	Logger forge.Logger

	// DefaultAppID is the app a session with no app_id claim reads and
	// writes. Empty means the "" app, where standalone installs keep their
	// data. It is never used for a session whose claim is present but
	// unusable: that session is refused.
	DefaultAppID string

	// APIProtected reports whether the REST API was mounted behind host
	// middleware. Optional; nil reads as false. It's a func because the API is
	// mounted after the contract may be registered.
	APIProtected func() bool
}

// registrars binds each group of intents. Each handlers_*.go file contributes
// one; Register runs them all.
var registrars = []func(*dispatcher.Dispatcher, Deps) error{
	registerEngine,
	registerOverview,
	registerProviders,
	registerTemplates,
	registerVersions,
	registerMessages,
	registerSend,
}

// Register loads and validates the embedded manifest, registers the `herald`
// contributor with reg, and binds every handler against deps.
func Register(d *dispatcher.Dispatcher, reg contract.Registry, wreg contract.WardenRegistry, deps Deps) error {
	if deps.Herald == nil {
		return fmt.Errorf("herald/contract: Herald is required")
	}
	m, err := loader.Load(bytes.NewReader(manifestYAML), "herald/contract/manifest.yaml")
	if err != nil {
		return fmt.Errorf("herald/contract: load manifest: %w", err)
	}
	if err := loader.Validate(m, wreg); err != nil {
		return fmt.Errorf("herald/contract: validate manifest: %w", err)
	}
	if err := reg.Register(m); err != nil {
		return fmt.Errorf("herald/contract: register manifest: %w", err)
	}
	for _, bind := range registrars {
		if err := bind(d, deps); err != nil {
			return fmt.Errorf("herald/contract: %w", err)
		}
	}
	return nil
}

// query binds a typed query handler for intent version 1.
func query[I, O any](d *dispatcher.Dispatcher, intent string, fn func(ctx context.Context, in I, p contract.Principal) (O, error)) error {
	if err := dispatcher.RegisterQuery(d, ContributorName, intent, 1, fn); err != nil {
		return fmt.Errorf("register %s: %w", intent, err)
	}
	return nil
}

// command binds a typed command handler for intent version 1.
func command[I, O any](d *dispatcher.Dispatcher, intent string, fn func(ctx context.Context, in I, p contract.Principal) (O, error)) error {
	if err := dispatcher.RegisterCommand(d, ContributorName, intent, 1, fn); err != nil {
		return fmt.Errorf("register %s: %w", intent, err)
	}
	return nil
}
