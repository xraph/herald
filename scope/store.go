package scope

import (
	"context"

	"github.com/xraph/herald/id"
)

// Store defines persistence operations for scoped configurations.
type Store interface {
	GetScopedConfig(ctx context.Context, appID string, scopeType ScopeType, scopeID string) (*Config, error)
	SetScopedConfig(ctx context.Context, cfg *Config) error
	DeleteScopedConfig(ctx context.Context, configID id.ScopedConfigID) error
	ListScopedConfigs(ctx context.Context, appID string) ([]*Config, error)
}
