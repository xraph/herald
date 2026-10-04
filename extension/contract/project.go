package contract

import (
	"maps"
	"slices"
	"strings"
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
// secret, which only a row written before placement rules can hold, and for
// every key when the driver isn't registered or has no schema.
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

// projectSettings lists a provider's settings, hiding the value of every key
// its driver marks secret. When the driver isn't registered or has no schema,
// nothing says which keys are secret, so every value is hidden: a legacy row
// can hold a secret in settings.
func projectSettings(h *herald.Herald, p *provider.Provider) []SettingEntry {
	fields, ok := h.Drivers().Describe(p.Driver)
	unknown := !ok || len(fields) == 0
	secret := map[string]bool{}
	for _, f := range fields {
		if f.Secret {
			secret[f.Key] = true
		}
	}
	keys := slices.Sorted(maps.Keys(p.Settings))
	out := make([]SettingEntry, 0, len(keys))
	for _, k := range keys {
		entry := SettingEntry{Key: k, Secret: unknown || secret[k]}
		if !entry.Secret {
			v := p.Settings[k]
			entry.Value = &v
		}
		out = append(out, entry)
	}
	return out
}

// LocaleState is one version's locale and whether it's live.
type LocaleState struct {
	Locale string `json:"locale"`
	Active bool   `json:"active"`
}

// TemplateSummary is a template as list pages show it.
type TemplateSummary struct {
	ID          string        `json:"id"`
	Slug        string        `json:"slug"`
	Name        string        `json:"name"`
	Channel     string        `json:"channel"`
	Category    string        `json:"category"`
	IsSystem    bool          `json:"isSystem"`
	Enabled     bool          `json:"enabled"`
	Locales     []LocaleState `json:"locales"`
	HasFallback bool          `json:"hasFallback"`
	UpdatedAt   time.Time     `json:"updatedAt"`
}

// VariableWire is a declared template variable.
type VariableWire struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

// VersionWire is one locale's content.
type VersionWire struct {
	ID        string    `json:"id"`
	Locale    string    `json:"locale"`
	Subject   string    `json:"subject"`
	HTML      string    `json:"html"`
	Text      string    `json:"text"`
	Title     string    `json:"title"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ResolutionEntry says which version answers a locale and how.
type ResolutionEntry struct {
	Locale    string  `json:"locale"`
	VersionID *string `json:"versionId"`
	Match     string  `json:"match"`
}

// TemplateDetail is a template with its variables and versions.
type TemplateDetail struct {
	TemplateSummary
	Variables []VariableWire `json:"variables"`
	Versions  []VersionWire  `json:"versions"`
}

func projectTemplate(t *template.Template) TemplateSummary {
	locales := make([]LocaleState, 0, len(t.Versions))
	for _, v := range t.Versions {
		locales = append(locales, LocaleState{Locale: v.Locale, Active: v.Active})
	}
	return TemplateSummary{
		ID: t.ID.String(), Slug: t.Slug, Name: t.Name, Channel: t.Channel, Category: t.Category,
		IsSystem: t.IsSystem, Enabled: t.Enabled, Locales: locales, HasFallback: hasFallback(t), UpdatedAt: t.UpdatedAt,
	}
}

func projectVersion(v *template.Version) VersionWire {
	return VersionWire{
		ID: v.ID.String(), Locale: v.Locale, Subject: v.Subject, HTML: v.HTML, Text: v.Text, Title: v.Title,
		Active: v.Active, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

func projectTemplateDetail(t *template.Template) TemplateDetail {
	vars := make([]VariableWire, 0, len(t.Variables))
	for _, v := range t.Variables {
		vars = append(vars, VariableWire{Name: v.Name, Type: v.Type, Required: v.Required, Default: v.Default, Description: v.Description})
	}
	versions := make([]VersionWire, 0, len(t.Versions))
	for i := range t.Versions {
		versions = append(versions, projectVersion(&t.Versions[i]))
	}
	return TemplateDetail{TemplateSummary: projectTemplate(t), Variables: vars, Versions: versions}
}

func variablesFromWire(in []VariableWire) []template.Variable {
	out := make([]template.Variable, 0, len(in))
	for _, v := range in {
		typ := strings.TrimSpace(v.Type)
		if typ == "" {
			typ = "string"
		}
		out = append(out, template.Variable{
			Name: strings.TrimSpace(v.Name), Type: typ, Required: v.Required, Default: v.Default, Description: v.Description,
		})
	}
	return out
}

// ProviderRef points at a provider. Driver and Enabled are set where a page
// needs them.
type ProviderRef struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Driver  string `json:"driver,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// MessageSummary is a delivery-log row.
type MessageSummary struct {
	ID           string       `json:"id"`
	Recipient    string       `json:"recipient"`
	Channel      string       `json:"channel"`
	Status       string       `json:"status"`
	TemplateSlug string       `json:"templateSlug,omitempty"`
	Provider     *ProviderRef `json:"provider"`
	Error        string       `json:"error,omitempty"`
	CreatedAt    time.Time    `json:"createdAt"`
	SentAt       *time.Time   `json:"sentAt,omitempty"`
}

// MessageDetail is every logged field of a message. Body holds the text part
// only, truncated at the engine's truncateBodyAt; HTML bodies aren't logged.
type MessageDetail struct {
	MessageSummary
	Subject           string            `json:"subject,omitempty"`
	Body              string            `json:"body"`
	Metadata          map[string]string `json:"metadata"`
	Attempts          int               `json:"attempts"`
	Async             bool              `json:"async"`
	EnvID             string            `json:"envId,omitempty"`
	ProviderMessageID string            `json:"providerMessageId,omitempty"`
	Template          *TemplateRef      `json:"template"`
}
