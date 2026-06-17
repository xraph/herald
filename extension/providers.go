package extension

import (
	"time"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/provider"
)

// ProviderConfig declares a notification provider in config.yaml. It is
// seeded into the store on startup (see SeedConfiguredProviders).
type ProviderConfig struct {
	Name        string            `json:"name" yaml:"name" mapstructure:"name"`
	Channel     string            `json:"channel" yaml:"channel" mapstructure:"channel"`
	Driver      string            `json:"driver" yaml:"driver" mapstructure:"driver"`
	AppID       string            `json:"app_id" yaml:"app_id" mapstructure:"app_id"`
	Credentials map[string]string `json:"credentials" yaml:"credentials" mapstructure:"credentials"`
	Settings    map[string]string `json:"settings" yaml:"settings" mapstructure:"settings"`
	Priority    int               `json:"priority" yaml:"priority" mapstructure:"priority"`
	Enabled     *bool             `json:"enabled" yaml:"enabled" mapstructure:"enabled"`
}

// toProvider maps the config entry to a provider.Provider. Enabled defaults to
// true when unset; ID and timestamps are assigned here.
func (pc ProviderConfig) toProvider(now time.Time) provider.Provider {
	enabled := true
	if pc.Enabled != nil {
		enabled = *pc.Enabled
	}
	return provider.Provider{
		ID:          id.NewProviderID(),
		AppID:       pc.AppID,
		Name:        pc.Name,
		Channel:     pc.Channel,
		Driver:      pc.Driver,
		Credentials: pc.Credentials,
		Settings:    pc.Settings,
		Priority:    pc.Priority,
		Enabled:     enabled,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func providersFromConfig(pcs []ProviderConfig, now time.Time) []provider.Provider {
	out := make([]provider.Provider, 0, len(pcs))
	for _, pc := range pcs {
		out = append(out, pc.toProvider(now))
	}
	return out
}
