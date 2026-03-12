// Package preference defines user notification preference entities.
package preference

import (
	"time"

	"github.com/xraph/herald/id"
)

// ChannelPreference holds per-channel opt-in/opt-out settings.
// nil means "use default" (opted-in).
type ChannelPreference struct {
	Email *bool `json:"email,omitempty"`
	SMS   *bool `json:"sms,omitempty"`
	Push  *bool `json:"push,omitempty"`
	InApp *bool `json:"inapp,omitempty"`
}

// Preference represents a user's notification preferences for an app.
// Overrides is keyed by notification type slug (e.g., "auth.welcome").
type Preference struct {
	ID        id.PreferenceID              `json:"id"`
	AppID     string                       `json:"app_id"`
	UserID    string                       `json:"user_id"`
	Overrides map[string]ChannelPreference `json:"overrides"`
	CreatedAt time.Time                    `json:"created_at"`
	UpdatedAt time.Time                    `json:"updated_at"`
}

// IsOptedOut checks whether the user has explicitly opted out of a
// specific notification type on a specific channel. Returns false
// (not opted out) if no preference is set.
func (p *Preference) IsOptedOut(notifType, channel string) bool {
	if p == nil || p.Overrides == nil {
		return false
	}

	cp, ok := p.Overrides[notifType]
	if !ok {
		return false
	}

	switch channel {
	case "email":
		return cp.Email != nil && !*cp.Email
	case "sms":
		return cp.SMS != nil && !*cp.SMS
	case "push":
		return cp.Push != nil && !*cp.Push
	case "inapp":
		return cp.InApp != nil && !*cp.InApp
	default:
		return false
	}
}
