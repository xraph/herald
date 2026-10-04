package contract

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/store"
)

func registerScopes(d *dispatcher.Dispatcher, deps Deps) error {
	if err := query(d, "scopes.list", scopesListHandler(deps)); err != nil {
		return err
	}
	if err := command(d, "scopes.set", scopesSetHandler(deps)); err != nil {
		return err
	}
	return command(d, "scopes.delete", scopesDeleteHandler(deps))
}

// RoutedProvider is a provider a rule points at. Dangling means the rule
// names an ID this app no longer has, so a send on that channel fails
// instead of falling back.
type RoutedProvider struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Dangling bool   `json:"dangling"`
}

// ScopeRule is one routing rule. Providers is keyed by channel and holds
// only the channels the rule sets.
type ScopeRule struct {
	ID                  string                     `json:"id"`
	Scope               string                     `json:"scope"`
	ScopeID             string                     `json:"scopeId"`
	Providers           map[string]*RoutedProvider `json:"providers"`
	FromEmail           string                     `json:"fromEmail,omitempty"`
	FromName            string                     `json:"fromName,omitempty"`
	FromPhone           string                     `json:"fromPhone,omitempty"`
	DefaultLocale       string                     `json:"defaultLocale,omitempty"`
	DefaultLocaleUnused bool                       `json:"defaultLocaleUnused"`
	UpdatedAt           time.Time                  `json:"updatedAt"`
}

func projectRule(c *scope.Config, providers map[string]*provider.Provider) ScopeRule {
	r := ScopeRule{
		ID: c.ID.String(), Scope: string(c.Scope), ScopeID: c.ScopeID, Providers: map[string]*RoutedProvider{},
		FromEmail: c.FromEmail, FromName: c.FromName, FromPhone: c.FromPhone,
		// Stored and shown, but Send uses the engine's default locale; the
		// page says so rather than let an operator think this does something.
		DefaultLocale: c.DefaultLocale, DefaultLocaleUnused: true, UpdatedAt: c.UpdatedAt,
	}
	for _, ch := range []string{"email", "sms", "push", "webhook", "chat"} {
		raw := c.ProviderIDFor(ch)
		if raw == "" {
			continue
		}
		if p, ok := providers[raw]; ok {
			r.Providers[ch] = &RoutedProvider{ID: raw, Name: p.Name}
		} else {
			r.Providers[ch] = &RoutedProvider{ID: raw, Dangling: true}
		}
	}
	return r
}

type scopesListRequest struct{}

type scopesListResponse struct {
	Rules []ScopeRule `json:"rules"`
}

func scopesListHandler(deps Deps) func(context.Context, scopesListRequest, contract.Principal) (scopesListResponse, error) {
	return func(ctx context.Context, _ scopesListRequest, p contract.Principal) (scopesListResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return scopesListResponse{}, err
		}
		configs, err := deps.Herald.Store().ListScopedConfigs(ctx, appID)
		if err != nil {
			return scopesListResponse{}, deps.mapError("scopes.list", err)
		}
		providers, err := providerNames(ctx, deps, appID)
		if err != nil {
			return scopesListResponse{}, deps.mapError("scopes.list", err)
		}
		out := scopesListResponse{Rules: make([]ScopeRule, 0, len(configs))}
		for _, c := range configs {
			out.Rules = append(out.Rules, projectRule(c, providers))
		}
		return out, nil
	}
}

// scopeKey validates a scope and its ID. The app rule's ID is always the app
// itself, so a client can't write a rule under another app's name.
func scopeKey(appID, rawScope, rawID string) (scope.ScopeType, string, error) {
	switch st := scope.ScopeType(strings.TrimSpace(rawScope)); st {
	case scope.ScopeApp:
		return st, appID, nil
	case scope.ScopeOrg, scope.ScopeUser:
		sid := strings.TrimSpace(rawID)
		if sid == "" {
			return "", "", badRequest("scopeId is required for an org or user rule")
		}
		return st, sid, nil
	default:
		return "", "", badRequest("scope must be app, org or user")
	}
}

type scopesSetRequest struct {
	Scope             string  `json:"scope"`
	ScopeID           string  `json:"scopeId"`
	EmailProviderID   *string `json:"emailProviderId"`
	SMSProviderID     *string `json:"smsProviderId"`
	PushProviderID    *string `json:"pushProviderId"`
	WebhookProviderID *string `json:"webhookProviderId"`
	ChatProviderID    *string `json:"chatProviderId"`
	FromEmail         *string `json:"fromEmail"`
	FromName          *string `json:"fromName"`
	FromPhone         *string `json:"fromPhone"`
}

type scopesSetResponse struct {
	Rule ScopeRule `json:"rule"`
}

// scopesSetHandler applies only the fields sent to the existing rule, or to
// a new one, checks every provider it names through the engine's
// CheckRouting, and saves. An empty string clears a field.
func scopesSetHandler(deps Deps) func(context.Context, scopesSetRequest, contract.Principal) (scopesSetResponse, error) {
	return func(ctx context.Context, in scopesSetRequest, p contract.Principal) (scopesSetResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return scopesSetResponse{}, err
		}
		st, sid, err := scopeKey(appID, in.Scope, in.ScopeID)
		if err != nil {
			return scopesSetResponse{}, err
		}
		now := time.Now().UTC()
		cfg, err := deps.Herald.Store().GetScopedConfig(ctx, appID, st, sid)
		switch {
		case errors.Is(err, store.ErrScopedConfigNotFound):
			cfg = &scope.Config{ID: id.NewScopedConfigID(), AppID: appID, Scope: st, ScopeID: sid, CreatedAt: now}
		case err != nil:
			return scopesSetResponse{}, deps.mapError("scopes.set", err)
		}
		for _, f := range []struct {
			in  *string
			dst *string
		}{
			{in.EmailProviderID, &cfg.EmailProviderID}, {in.SMSProviderID, &cfg.SMSProviderID},
			{in.PushProviderID, &cfg.PushProviderID}, {in.WebhookProviderID, &cfg.WebhookProviderID},
			{in.ChatProviderID, &cfg.ChatProviderID}, {in.FromEmail, &cfg.FromEmail},
			{in.FromName, &cfg.FromName}, {in.FromPhone, &cfg.FromPhone},
		} {
			if f.in != nil {
				*f.dst = strings.TrimSpace(*f.in)
			}
		}
		cfg.UpdatedAt = now
		if err = deps.Herald.CheckRouting(ctx, cfg); err != nil {
			return scopesSetResponse{}, deps.mapError("scopes.set", err)
		}
		if err = deps.Herald.Store().SetScopedConfig(ctx, cfg); err != nil {
			return scopesSetResponse{}, deps.mapError("scopes.set", err)
		}
		saved, err := deps.Herald.Store().GetScopedConfig(ctx, appID, st, sid)
		if err != nil {
			return scopesSetResponse{}, deps.mapError("scopes.set", err)
		}
		providers, err := providerNames(ctx, deps, appID)
		if err != nil {
			return scopesSetResponse{}, deps.mapError("scopes.set", err)
		}
		audit(ctx, deps, p, appID, "scopes.set", "scoped_config", saved.ID.String(), map[string]string{"scope": string(st), "scope_id": sid})
		return scopesSetResponse{Rule: projectRule(saved, providers)}, nil
	}
}

type scopesDeleteRequest struct {
	Scope   string `json:"scope"`
	ScopeID string `json:"scopeId"`
}

func scopesDeleteHandler(deps Deps) func(context.Context, scopesDeleteRequest, contract.Principal) (deleteResponse, error) {
	return func(ctx context.Context, in scopesDeleteRequest, p contract.Principal) (deleteResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return deleteResponse{}, err
		}
		st, sid, err := scopeKey(appID, in.Scope, in.ScopeID)
		if err != nil {
			return deleteResponse{}, err
		}
		cfg, err := deps.Herald.Store().GetScopedConfig(ctx, appID, st, sid)
		if err != nil {
			return deleteResponse{}, deps.mapError("scopes.delete", err)
		}
		if err := deps.Herald.Store().DeleteScopedConfig(ctx, cfg.ID); err != nil {
			return deleteResponse{}, deps.mapError("scopes.delete", err)
		}
		audit(ctx, deps, p, appID, "scopes.delete", "scoped_config", cfg.ID.String(), map[string]string{"scope": string(st), "scope_id": sid})
		return deleteResponse{OK: true, ID: cfg.ID.String()}, nil
	}
}
