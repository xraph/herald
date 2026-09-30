package scope

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/internal/storeerr"
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

// How a provider was chosen, reported in ResolveResult.Via.
const (
	ViaUser     = "user"     // a user-level routing rule
	ViaOrg      = "org"      // an org-level routing rule
	ViaApp      = "app"      // the app-level routing rule
	ViaFallback = "fallback" // no rule: the first enabled provider by priority
	ViaChosen   = "chosen"   // the caller named the provider
)

// ResolveResult holds the resolved provider and scoped configuration.
type ResolveResult struct {
	Provider *provider.Provider
	Config   *Config
	Via      string
}

// ResolveProvider resolves the best provider for a given channel through the
// scope chain: user → org → app → first enabled provider. A rule naming a
// provider of another app or channel is skipped. A store failure other than
// "not found" is returned. A nil result with a nil error means nothing
// handles the channel.
func (r *Resolver) ResolveProvider(
	ctx context.Context,
	appID, orgID, userID string,
	channel string,
) (*ResolveResult, error) {
	// 1. User-scoped override, 2. org-scoped override, 3. app-scoped default.
	levels := []struct {
		scopeType ScopeType
		scopeID   string
	}{{ScopeUser, userID}, {ScopeOrg, orgID}, {ScopeApp, appID}}
	for _, l := range levels {
		if l.scopeType != ScopeApp && l.scopeID == "" {
			continue
		}
		result, err := r.tryScope(ctx, appID, l.scopeType, l.scopeID, channel)
		if err != nil {
			return nil, err
		}
		if result != nil {
			return result, nil
		}
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
			return &ResolveResult{Provider: p, Via: ViaFallback}, nil
		}
	}

	return nil, nil
}

// tryScope resolves a provider from one scope level. A nil result with a nil
// error means the level has no usable rule and the chain moves on. A rule that
// names a provider from another app, or one on a different channel, is not
// usable: it is logged and skipped, so a routing rule can never send one
// app's messages through another app's provider. Store failures other than
// "not found" are returned, not treated as a missing rule.
func (r *Resolver) tryScope(ctx context.Context, appID string, scopeType ScopeType, scopeID, channel string) (*ResolveResult, error) {
	cfg, err := r.scopeStore.GetScopedConfig(ctx, appID, scopeType, scopeID)
	if errors.Is(err, storeerr.ErrScopedConfigNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("herald: load %s routing rule %q: %w", scopeType, scopeID, err)
	}

	pidStr := cfg.ProviderIDFor(channel)
	if pidStr == "" {
		return nil, nil
	}

	pid, err := id.ParseProviderID(pidStr)
	if err != nil {
		r.logger.Warn("herald: invalid provider ID in scoped config",
			"app_id", appID,
			"provider_id", pidStr,
			"scope", scopeType,
			"scope_id", scopeID,
		)
		return nil, nil
	}

	prov, err := r.providerStore.GetProvider(ctx, pid)
	if errors.Is(err, storeerr.ErrProviderNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("herald: load provider %s named by %s routing rule %q: %w", pidStr, scopeType, scopeID, err)
	}
	if prov.AppID != appID || prov.Channel != channel {
		reason := "provider belongs to another app"
		if prov.AppID == appID {
			reason = "provider sends " + prov.Channel + ", not " + channel
		}
		r.logger.Warn("herald: scoped config names a provider it can't use; ignoring the rule",
			"app_id", appID,
			"scope", scopeType,
			"scope_id", scopeID,
			"provider_id", pidStr,
			"reason", reason,
		)
		return nil, nil
	}
	if !prov.Enabled {
		return nil, nil
	}

	return &ResolveResult{Provider: prov, Config: cfg, Via: string(scopeType)}, nil
}
