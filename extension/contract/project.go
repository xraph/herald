package contract

import (
	"maps"
	"slices"
	"time"

	"github.com/xraph/herald"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/template"
)

// Wire types are camelCase and live in this package. Handlers never return
// a Herald domain struct, so a field added to one can't leak by accident.

// AppRef names the app a session is looking at.
type AppRef struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// FieldInfo describes one value a driver reads.
type FieldInfo struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Help      string `json:"help,omitempty"`
	Required  bool   `json:"required"`
	Secret    bool   `json:"secret"`
	Placement string `json:"placement"`
}

// DriverInfo is a registered driver. Fields is null for a driver that
// doesn't describe itself, so the form falls back to key/value rows, and
// an empty list for one that needs nothing.
type DriverInfo struct {
	Name    string      `json:"name"`
	Channel string      `json:"channel"`
	Fields  []FieldInfo `json:"fields"`
}

// hasFallback reports whether a template has an active "" version, which
// answers any locale it doesn't list.
func hasFallback(t *template.Template) bool {
	for _, v := range t.Versions {
		if v.Active && v.Locale == "" {
			return true
		}
	}
	return false
}

// CredentialStatus is how one stored credential is protected. It never
// carries the value.
type CredentialStatus struct {
	Key        string `json:"key"`
	Protection string `json:"protection"`
	KeyID      string `json:"keyId,omitempty"`
}

// ProviderSummary is a provider as list pages show it.
type ProviderSummary struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Channel     string             `json:"channel"`
	Driver      string             `json:"driver"`
	Priority    int                `json:"priority"`
	Enabled     bool               `json:"enabled"`
	Credentials []CredentialStatus `json:"credentials"`
	CreatedAt   time.Time          `json:"createdAt"`
	UpdatedAt   time.Time          `json:"updatedAt"`
}

// SettingEntry is one setting. Value is omitted for a key the driver marks
// secret, which only a row written before placement rules can hold.
type SettingEntry struct {
	Key    string  `json:"key"`
	Value  *string `json:"value,omitempty"`
	Secret bool    `json:"secret"`
}

// RouteUse is a routing rule that sends one channel through a provider.
type RouteUse struct {
	Scope   string `json:"scope"`
	ScopeID string `json:"scopeId"`
	Channel string `json:"channel"`
}

// ProviderDetail is a provider with its settings and the routing rules that
// point at it, so deleting it can warn about what will dangle.
type ProviderDetail struct {
	ProviderSummary
	Settings []SettingEntry `json:"settings"`
	UsedBy   []RouteUse     `json:"usedBy"`
}

func projectProvider(h *herald.Herald, p *provider.Provider) ProviderSummary {
	states := h.CredentialStatus(p)
	creds := make([]CredentialStatus, 0, len(states))
	for _, s := range states {
		creds = append(creds, CredentialStatus{Key: s.Key, Protection: s.Protection, KeyID: s.KeyID})
	}
	return ProviderSummary{
		ID: p.ID.String(), Name: p.Name, Channel: p.Channel, Driver: p.Driver,
		Priority: p.Priority, Enabled: p.Enabled, Credentials: creds,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func projectSettings(h *herald.Herald, p *provider.Provider) []SettingEntry {
	secret := map[string]bool{}
	if fields, ok := h.Drivers().Describe(p.Driver); ok {
		for _, f := range fields {
			if f.Secret {
				secret[f.Key] = true
			}
		}
	}
	keys := slices.Sorted(maps.Keys(p.Settings))
	out := make([]SettingEntry, 0, len(keys))
	for _, k := range keys {
		entry := SettingEntry{Key: k, Secret: secret[k]}
		if !entry.Secret {
			v := p.Settings[k]
			entry.Value = &v
		}
		out = append(out, entry)
	}
	return out
}
