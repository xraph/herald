// Package scope defines scoped provider configuration for app/org/user overrides.
package scope

import (
	"time"

	"github.com/xraph/herald/id"
)

// ScopeType identifies the level at which configuration is applied.
type ScopeType string //nolint:revive // renaming would break the public API

// Scope type constants.
const (
	ScopeApp  ScopeType = "app"
	ScopeOrg  ScopeType = "org"
	ScopeUser ScopeType = "user"
)

// Config represents a scoped provider configuration override.
type Config struct {
	ID                id.ScopedConfigID `json:"id"`
	AppID             string            `json:"app_id"`
	Scope             ScopeType         `json:"scope"`
	ScopeID           string            `json:"scope_id"`
	EmailProviderID   string            `json:"email_provider_id,omitempty"`
	SMSProviderID     string            `json:"sms_provider_id,omitempty"`
	PushProviderID    string            `json:"push_provider_id,omitempty"`
	WebhookProviderID string            `json:"webhook_provider_id,omitempty"`
	ChatProviderID    string            `json:"chat_provider_id,omitempty"`
	FromEmail         string            `json:"from_email,omitempty"`
	FromName          string            `json:"from_name,omitempty"`
	FromPhone         string            `json:"from_phone,omitempty"`
	DefaultLocale     string            `json:"default_locale,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// ProviderIDFor returns the provider ID override for the given channel.
func (c *Config) ProviderIDFor(channel string) string {
	if c == nil {
		return ""
	}

	switch channel {
	case "email":
		return c.EmailProviderID
	case "sms":
		return c.SMSProviderID
	case "push":
		return c.PushProviderID
	case "webhook":
		return c.WebhookProviderID
	case "chat":
		return c.ChatProviderID
	default:
		return ""
	}
}
