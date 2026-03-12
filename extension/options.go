package extension

import (
	"github.com/xraph/herald"
	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/store"
)

// ExtOption configures the Herald Forge extension.
type ExtOption func(*Extension)

// WithStore sets the persistence backend via a herald option.
func WithStore(s store.Store) ExtOption {
	return func(e *Extension) {
		e.opts = append(e.opts, herald.WithStore(s))
	}
}

// WithBasePath sets the URL prefix for all herald routes.
func WithBasePath(path string) ExtOption {
	return func(e *Extension) {
		e.config.BasePath = path
	}
}

// WithConfig sets the extension configuration directly.
func WithConfig(cfg Config) ExtOption {
	return func(e *Extension) {
		e.config = cfg
	}
}

// WithHeraldOption appends a raw herald.Option to the extension.
func WithHeraldOption(opt herald.Option) ExtOption {
	return func(e *Extension) {
		e.opts = append(e.opts, opt)
	}
}

// WithDriver registers a notification driver with the Herald instance.
func WithDriver(d driver.Driver) ExtOption {
	return func(e *Extension) {
		e.opts = append(e.opts, herald.WithDriver(d))
	}
}

// WithDisableRoutes disables automatic route registration.
func WithDisableRoutes() ExtOption {
	return func(e *Extension) {
		e.config.DisableRoutes = true
	}
}

// WithDisableMigrate disables automatic database migration on Register.
func WithDisableMigrate() ExtOption {
	return func(e *Extension) {
		e.config.DisableMigrate = true
	}
}

// WithRequireConfig requires configuration to be present in YAML files.
// If true and no config is found, Register returns an error.
func WithRequireConfig(require bool) ExtOption {
	return func(e *Extension) {
		e.config.RequireConfig = require
	}
}

// WithGroveDatabase sets the name of the grove.DB to resolve from the DI container.
// The extension will auto-construct the appropriate store backend (postgres/sqlite/mongo)
// based on the grove driver type. Pass an empty string to use the default (unnamed) grove.DB.
func WithGroveDatabase(name string) ExtOption {
	return func(e *Extension) {
		e.config.GroveDatabase = name
		e.useGrove = true
	}
}
