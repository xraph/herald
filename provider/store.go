package provider

import (
	"context"

	"github.com/xraph/herald/id"
)

// Store defines persistence operations for providers.
type Store interface {
	CreateProvider(ctx context.Context, p *Provider) error
	GetProvider(ctx context.Context, providerID id.ProviderID) (*Provider, error)
	UpdateProvider(ctx context.Context, p *Provider) error
	DeleteProvider(ctx context.Context, providerID id.ProviderID) error
	ListProviders(ctx context.Context, appID string, channel string) ([]*Provider, error)
	ListAllProviders(ctx context.Context, appID string) ([]*Provider, error)
}
