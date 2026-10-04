package contract

import (
	"context"
	"strings"

	"github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"

	"github.com/xraph/herald"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/provider"
)

func registerProviders(d *dispatcher.Dispatcher, deps Deps) error {
	if err := query(d, "providers.list", providersListHandler(deps)); err != nil {
		return err
	}
	return query(d, "providers.detail", providersDetailHandler(deps))
}

// routedChannels are the scoped-config slots that can name a provider.
var routedChannels = []string{"email", "sms", "push", "webhook", "chat"}

func parseProviderID(raw string) (id.ProviderID, error) {
	pid, err := id.ParseProviderID(strings.TrimSpace(raw))
	if err != nil {
		return id.Nil, badRequest("id is not a provider id")
	}
	return pid, nil
}

// ownedProvider loads a provider of appID. Another app's provider answers
// NOT_FOUND exactly like a missing one.
func ownedProvider(ctx context.Context, deps Deps, appID, raw string) (*provider.Provider, error) {
	pid, err := parseProviderID(raw)
	if err != nil {
		return nil, err
	}
	p, err := deps.Herald.GetProvider(ctx, appID, pid)
	if err != nil {
		return nil, deps.mapError("provider lookup", err)
	}
	return p, nil
}

type providersListRequest struct {
	Channel string `json:"channel"`
}

type providersListResponse struct {
	Providers []ProviderSummary `json:"providers"`
}

func providersListHandler(deps Deps) func(context.Context, providersListRequest, contract.Principal) (providersListResponse, error) {
	return func(ctx context.Context, in providersListRequest, p contract.Principal) (providersListResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return providersListResponse{}, err
		}
		var list []*provider.Provider
		if ch := strings.TrimSpace(in.Channel); ch != "" {
			list, err = deps.Herald.Store().ListProviders(ctx, appID, ch)
		} else {
			list, err = deps.Herald.Store().ListAllProviders(ctx, appID)
		}
		if err != nil {
			return providersListResponse{}, deps.mapError("providers.list", err)
		}
		out := providersListResponse{Providers: make([]ProviderSummary, 0, len(list))}
		for _, prov := range list {
			out.Providers = append(out.Providers, projectProvider(deps.Herald, prov))
		}
		return out, nil
	}
}

type providersDetailRequest struct {
	ID string `json:"id"`
}

type providersDetailResponse struct {
	Provider ProviderDetail `json:"provider"`
}

func providersDetailHandler(deps Deps) func(context.Context, providersDetailRequest, contract.Principal) (providersDetailResponse, error) {
	return func(ctx context.Context, in providersDetailRequest, p contract.Principal) (providersDetailResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return providersDetailResponse{}, err
		}
		prov, err := ownedProvider(ctx, deps, appID, in.ID)
		if err != nil {
			return providersDetailResponse{}, err
		}
		usedBy, err := routesUsing(ctx, deps.Herald, appID, prov.ID.String())
		if err != nil {
			return providersDetailResponse{}, deps.mapError("providers.detail", err)
		}
		return providersDetailResponse{Provider: ProviderDetail{
			ProviderSummary: projectProvider(deps.Herald, prov),
			Settings:        projectSettings(deps.Herald, prov),
			UsedBy:          usedBy,
		}}, nil
	}
}

// routesUsing lists the routing rules in appID that send a channel through
// the provider with providerID.
func routesUsing(ctx context.Context, h *herald.Herald, appID, providerID string) ([]RouteUse, error) {
	cfgs, err := h.Store().ListScopedConfigs(ctx, appID)
	if err != nil {
		return nil, err
	}
	out := []RouteUse{}
	for _, c := range cfgs {
		for _, ch := range routedChannels {
			if c.ProviderIDFor(ch) == providerID {
				out = append(out, RouteUse{Scope: string(c.Scope), ScopeID: c.ScopeID, Channel: ch})
			}
		}
	}
	return out, nil
}
