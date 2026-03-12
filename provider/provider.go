// Package provider defines the notification provider entity and its store interface.
package provider

import (
	"time"

	"github.com/xraph/herald/id"
)

// Provider represents a configured notification delivery provider.
// Each provider is scoped to an app and handles a single channel type.
type Provider struct {
	ID          id.ProviderID     `json:"id"`
	AppID       string            `json:"app_id"`
	Name        string            `json:"name"`
	Channel     string            `json:"channel"`
	Driver      string            `json:"driver"`
	Credentials map[string]string `json:"credentials,omitempty"`
	Settings    map[string]string `json:"settings,omitempty"`
	Priority    int               `json:"priority"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}
