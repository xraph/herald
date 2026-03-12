package preference

import (
	"context"
)

// Store defines persistence operations for user notification preferences.
type Store interface {
	GetPreference(ctx context.Context, appID string, userID string) (*Preference, error)
	SetPreference(ctx context.Context, p *Preference) error
	DeletePreference(ctx context.Context, appID string, userID string) error
}
