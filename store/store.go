// Package store defines the composite Store interface for all Herald persistence.
//
// The composite store follows the same pattern as Relay and Authsome:
// each subsystem defines its own store interface, and the aggregate Store
// composes them all.
package store

import (
	"context"

	"github.com/xraph/herald/inbox"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/preference"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/template"
)

// Store is the aggregate persistence interface for Herald.
type Store interface {
	provider.Store
	template.Store
	message.Store
	inbox.Store
	preference.Store
	scope.Store

	// Migrate runs all schema migrations.
	Migrate(ctx context.Context) error

	// Ping checks database connectivity.
	Ping(ctx context.Context) error

	// Close closes the store connection.
	Close() error
}
