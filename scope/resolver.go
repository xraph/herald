package scope

import (
	"context"
	"log/slog"
	"sort"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/provider"
)

// Resolver resolves the appropriate notification provider for a given
// app/org/user scope using the fallback chain: user → org → app → default.
type Resolver struct {
	scopeStore    Store
	providerStore provider.Store
	logger        *slog.Logger
}

// NewResolver creates a new scoped provider resolver.
func NewResolver(scopeStore Store, providerStore provider.Store, logger *slog.Logger) *Resolver {
	return &Resolver{
		scopeStore:    scopeStore,
		providerStore: providerStore,
		logger:        logger,
	}
}

// ResolveResult holds the resolved provider and scoped configuration.
type ResolveResult struct {
	Provider *provider.Provider
	Config   *Config
}

// ResolveProvider resolves the best provider for a given channel through the
// scope chain: user → org → app → first enabled provider.
func (r *Resolver) ResolveProvider(
	ctx context.Context,
	appID, orgID, userID string,
	channel string,
) (*ResolveResult, error) {
	// 1. User-scoped override
	if userID != "" {
		if result := r.tryScope(ctx, appID, ScopeUser, userID, channel); result != nil {
			return result, nil
		}
	}

	// 2. Org-scoped override
	if orgID != "" {
		if result := r.tryScope(ctx, appID, ScopeOrg, orgID, channel); result != nil {
			return result, nil
		}
	}

	// 3. App-scoped default
	if result := r.tryScope(ctx, appID, ScopeApp, appID, channel); result != nil {
		return result, nil
	}

	// 4. Fallback: first enabled provider for channel, sorted by priority
	providers, err := r.providerStore.ListProviders(ctx, appID, channel)
	if err != nil {
		return nil, err
	}

	// Sort by priority (lower = higher priority)
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Priority < providers[j].Priority
	})

	for _, p := range providers {
		if p.Enabled {
			r.logger.Debug("herald: resolved provider via fallback",
				"provider", p.Name,
				"channel", channel,
				"app_id", appID,
			)
			return &ResolveResult{Provider: p}, nil
		}
	}

	return nil, nil
}

// tryScope attempts to resolve a provider from a specific scope level.
func (r *Resolver) tryScope(ctx context.Context, appID string, scopeType ScopeType, scopeID, channel string) *ResolveResult {
	cfg, err := r.scopeStore.GetScopedConfig(ctx, appID, scopeType, scopeID)
	if err != nil || cfg == nil {
		return nil
	}

	pidStr := cfg.ProviderIDFor(channel)
	if pidStr == "" {
		return nil
	}

	pid, err := id.ParseProviderID(pidStr)
	if err != nil {
		r.logger.Warn("herald: invalid provider ID in scoped config",
			"provider_id", pidStr,
			"scope", scopeType,
			"scope_id", scopeID,
		)
		return nil
	}

	prov, err := r.providerStore.GetProvider(ctx, pid)
	if err != nil || !prov.Enabled {
		return nil
	}

	return &ResolveResult{Provider: prov, Config: cfg}
}
