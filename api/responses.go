package api

import (
	"maps"
	"time"

	"github.com/xraph/herald"
	"github.com/xraph/herald/provider"
)

// ProviderResponse is a provider as the API shows it. Credentials are key
// names and how each is stored, never values. Settings the driver marks as
// secret are left out too, for rows written before that rule existed.
type ProviderResponse struct {
	ID          string                   `json:"id"`
	AppID       string                   `json:"app_id"`
	Name        string                   `json:"name"`
	Channel     string                   `json:"channel"`
	Driver      string                   `json:"driver"`
	Credentials []herald.CredentialState `json:"credentials"`
	Settings    map[string]string        `json:"settings,omitempty"`
	Priority    int                      `json:"priority"`
	Enabled     bool                     `json:"enabled"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
}

// EncryptProvidersRequest names the app whose credentials to encrypt.
type EncryptProvidersRequest struct {
	AppID string `description:"Application ID" query:"app_id"`
}

func (a *ForgeAPI) providerResponse(p *provider.Provider) *ProviderResponse {
	settings := maps.Clone(p.Settings)
	if fields, ok := a.herald.Drivers().Describe(p.Driver); ok {
		for _, f := range fields {
			if f.Secret {
				delete(settings, f.Key)
			}
		}
	}
	return &ProviderResponse{
		ID: p.ID.String(), AppID: p.AppID, Name: p.Name, Channel: p.Channel, Driver: p.Driver,
		Credentials: a.herald.CredentialStatus(p), Settings: settings,
		Priority: p.Priority, Enabled: p.Enabled, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}
