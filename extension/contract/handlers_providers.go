package contract

import (
	"context"
	"strconv"
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
	if err := query(d, "providers.detail", providersDetailHandler(deps)); err != nil {
		return err
	}
	if err := command(d, "providers.create", providersCreateHandler(deps)); err != nil {
		return err
	}
	if err := command(d, "providers.update", providersUpdateHandler(deps)); err != nil {
		return err
	}
	if err := command(d, "providers.delete", providersDeleteHandler(deps)); err != nil {
		return err
	}
	return command(d, "providers.encryptStored", providersEncryptStoredHandler(deps))
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
// NOT_FOUND exactly like a missing one. intent labels the server-side log
// when the lookup fails for a reason with no domain meaning.
func ownedProvider(ctx context.Context, deps Deps, appID, raw, intent string) (*provider.Provider, error) {
	pid, err := parseProviderID(raw)
	if err != nil {
		return nil, err
	}
	p, err := deps.Herald.GetProvider(ctx, appID, pid)
	if err != nil {
		return nil, deps.mapError(intent, err)
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
		prov, err := ownedProvider(ctx, deps, appID, in.ID, "providers.detail")
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

type providersCreateRequest struct {
	Name        string            `json:"name"`
	Channel     string            `json:"channel"`
	Driver      string            `json:"driver"`
	Priority    int               `json:"priority"`
	Enabled     bool              `json:"enabled"`
	Credentials map[string]string `json:"credentials"`
	Settings    map[string]string `json:"settings"`
}

type providerResponse struct {
	Provider ProviderSummary `json:"provider"`
}

// providersCreateHandler creates a provider through the engine, which
// validates it and encrypts its credentials when a key is configured.
func providersCreateHandler(deps Deps) func(context.Context, providersCreateRequest, contract.Principal) (providerResponse, error) {
	return func(ctx context.Context, in providersCreateRequest, p contract.Principal) (providerResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return providerResponse{}, err
		}
		prov := &provider.Provider{
			AppID: appID, Name: in.Name, Channel: strings.TrimSpace(in.Channel), Driver: strings.TrimSpace(in.Driver),
			Priority: in.Priority, Enabled: in.Enabled, Credentials: in.Credentials, Settings: in.Settings,
		}
		if err := deps.Herald.CreateProvider(ctx, prov); err != nil {
			return providerResponse{}, deps.mapError("providers.create", err)
		}
		audit(ctx, deps, p, appID, "providers.create", "provider", prov.ID.String(), map[string]string{
			"name": prov.Name, "channel": prov.Channel, "driver": prov.Driver,
		})
		return providerResponse{Provider: projectProvider(deps.Herald, prov)}, nil
	}
}

type providersUpdateRequest struct {
	ID                string            `json:"id"`
	Name              *string           `json:"name"`
	Priority          *int              `json:"priority"`
	Enabled           *bool             `json:"enabled"`
	SetCredentials    map[string]string `json:"setCredentials"`
	RemoveCredentials []string          `json:"removeCredentials"`
	SetSettings       map[string]string `json:"setSettings"`
	RemoveSettings    []string          `json:"removeSettings"`
}

// providersUpdateHandler changes only what the request names. Moving
// base_url or host to a new server requires re-entering the provider's
// secrets in the same request (the engine enforces it).
func providersUpdateHandler(deps Deps) func(context.Context, providersUpdateRequest, contract.Principal) (providerResponse, error) {
	return func(ctx context.Context, in providersUpdateRequest, p contract.Principal) (providerResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return providerResponse{}, err
		}
		pid, err := parseProviderID(in.ID)
		if err != nil {
			return providerResponse{}, err
		}
		prov, err := deps.Herald.UpdateProvider(ctx, appID, pid, herald.ProviderUpdate{
			Name: in.Name, Priority: in.Priority, Enabled: in.Enabled,
			SetCredentials: in.SetCredentials, RemoveCredentials: in.RemoveCredentials,
			SetSettings: in.SetSettings, RemoveSettings: in.RemoveSettings,
		})
		if err != nil {
			return providerResponse{}, deps.mapError("providers.update", err)
		}
		audit(ctx, deps, p, appID, "providers.update", "provider", prov.ID.String(), map[string]string{"name": prov.Name})
		return providerResponse{Provider: projectProvider(deps.Herald, prov)}, nil
	}
}

type providersDeleteRequest struct {
	ID string `json:"id"`
}

type deleteResponse struct {
	OK bool   `json:"ok"`
	ID string `json:"id"`
}

func providersDeleteHandler(deps Deps) func(context.Context, providersDeleteRequest, contract.Principal) (deleteResponse, error) {
	return func(ctx context.Context, in providersDeleteRequest, p contract.Principal) (deleteResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return deleteResponse{}, err
		}
		pid, err := parseProviderID(in.ID)
		if err != nil {
			return deleteResponse{}, err
		}
		if err := deps.Herald.DeleteProvider(ctx, appID, pid); err != nil {
			return deleteResponse{}, deps.mapError("providers.delete", err)
		}
		audit(ctx, deps, p, appID, "providers.delete", "provider", pid.String(), nil)
		return deleteResponse{OK: true, ID: pid.String()}, nil
	}
}

type providersEncryptStoredRequest struct{}

type providersEncryptStoredResponse struct {
	Providers        int `json:"providers"`
	ValuesEncrypted  int `json:"valuesEncrypted"`
	AlreadyEncrypted int `json:"alreadyEncrypted"`
}

func providersEncryptStoredHandler(deps Deps) func(context.Context, providersEncryptStoredRequest, contract.Principal) (providersEncryptStoredResponse, error) {
	return func(ctx context.Context, _ providersEncryptStoredRequest, p contract.Principal) (providersEncryptStoredResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return providersEncryptStoredResponse{}, err
		}
		rep, err := deps.Herald.EncryptStoredCredentials(ctx, appID)
		if err != nil {
			return providersEncryptStoredResponse{}, deps.mapError("providers.encryptStored", err)
		}
		audit(ctx, deps, p, appID, "providers.encryptStored", "provider", "", map[string]string{
			"providers": strconv.Itoa(rep.Providers), "values_encrypted": strconv.Itoa(rep.ValuesEncrypted),
		})
		return providersEncryptStoredResponse{Providers: rep.Providers, ValuesEncrypted: rep.ValuesEncrypted, AlreadyEncrypted: rep.AlreadyEncrypted}, nil
	}
}
