// Package inbox defines the in-app notification entity for the inbox channel.
package inbox

import (
	"time"

	"github.com/xraph/herald/id"
)

// Notification represents an in-app notification stored in a user's inbox.
type Notification struct {
	ID        id.InboxID        `json:"id"`
	AppID     string            `json:"app_id"`
	EnvID     string            `json:"env_id,omitempty"`
	UserID    string            `json:"user_id"`
	Type      string            `json:"type"`
	Title     string            `json:"title"`
	Body      string            `json:"body,omitempty"`
	ActionURL string            `json:"action_url,omitempty"`
	ImageURL  string            `json:"image_url,omitempty"`
	Read      bool              `json:"read"`
	ReadAt    *time.Time        `json:"read_at,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	ExpiresAt *time.Time        `json:"expires_at,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}
