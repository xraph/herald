package inbox

import (
	"context"

	"github.com/xraph/herald/id"
)

// Store defines persistence operations for in-app notifications.
type Store interface {
	CreateNotification(ctx context.Context, n *Notification) error
	GetNotification(ctx context.Context, notifID id.InboxID) (*Notification, error)
	DeleteNotification(ctx context.Context, notifID id.InboxID) error
	MarkRead(ctx context.Context, notifID id.InboxID) error
	MarkAllRead(ctx context.Context, appID string, userID string) error
	UnreadCount(ctx context.Context, appID string, userID string) (int, error)
	ListNotifications(ctx context.Context, appID string, userID string, limit, offset int) ([]*Notification, error)
}
