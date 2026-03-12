package dashboard

import (
	"context"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/inbox"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/preference"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/template"
)

// DriverInfo holds driver name and channel mapping for display.
type DriverInfo struct {
	Name    string
	Channel string
}

// fetchOverviewStats returns counts for providers, templates, and messages.
func fetchOverviewStats(ctx context.Context, s store.Store, appID string) (providers, templates, messages int) {
	provs, err := s.ListAllProviders(ctx, appID)
	if err == nil {
		providers = len(provs)
	}
	tmpls, err := s.ListTemplates(ctx, appID)
	if err == nil {
		templates = len(tmpls)
	}
	msgs, err := s.ListMessages(ctx, appID, message.ListOptions{Limit: 1})
	if err == nil {
		messages = len(msgs)
	}
	return
}

// fetchActiveProviderCount returns the count of enabled providers.
func fetchActiveProviderCount(ctx context.Context, s store.Store, appID string) int {
	provs, err := s.ListAllProviders(ctx, appID)
	if err != nil {
		return 0
	}
	count := 0
	for _, p := range provs {
		if p.Enabled {
			count++
		}
	}
	return count
}

// fetchProviders returns providers for the given app, optionally filtered by channel.
func fetchProviders(ctx context.Context, s store.Store, appID, channel string) ([]*provider.Provider, error) {
	if channel != "" {
		return s.ListProviders(ctx, appID, channel)
	}
	return s.ListAllProviders(ctx, appID)
}

// fetchTemplates returns templates for the given app, optionally filtered by channel.
func fetchTemplates(ctx context.Context, s store.Store, appID, channel string) ([]*template.Template, error) {
	if channel != "" {
		return s.ListTemplatesByChannel(ctx, appID, channel)
	}
	return s.ListTemplates(ctx, appID)
}

// fetchMessages returns messages for the given app with optional filters.
func fetchMessages(ctx context.Context, s store.Store, appID, channel, status string, limit int) ([]*message.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.ListMessages(ctx, appID, message.ListOptions{
		Channel: channel,
		Status:  message.Status(status),
		Limit:   limit,
	})
}

// fetchMessageCounts returns the count of messages by status.
func fetchMessageCounts(ctx context.Context, s store.Store, appID string) (sent, failed, pending int) {
	msgs, err := s.ListMessages(ctx, appID, message.ListOptions{Limit: 1000})
	if err != nil {
		return 0, 0, 0
	}
	for _, m := range msgs {
		switch m.Status {
		case message.StatusSent, message.StatusDelivered:
			sent++
		case message.StatusFailed:
			failed++
		default:
			pending++
		}
	}
	return
}

// fetchTotalMessageCount returns the total number of messages.
func fetchTotalMessageCount(ctx context.Context, s store.Store, appID string) int {
	msgs, err := s.ListMessages(ctx, appID, message.ListOptions{Limit: 1000})
	if err != nil {
		return 0
	}
	return len(msgs)
}

// fetchChannelBreakdown returns message counts grouped by channel.
func fetchChannelBreakdown(ctx context.Context, s store.Store, appID string) map[string]int {
	msgs, err := s.ListMessages(ctx, appID, message.ListOptions{Limit: 1000})
	if err != nil {
		return nil
	}
	breakdown := make(map[string]int)
	for _, m := range msgs {
		breakdown[m.Channel]++
	}
	return breakdown
}

// fetchInboxNotifications returns in-app notifications for a user.
func fetchInboxNotifications(ctx context.Context, s store.Store, appID, userID string, limit, offset int) ([]*inbox.Notification, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.ListNotifications(ctx, appID, userID, limit, offset)
}

// fetchUnreadCount returns the unread inbox count for a user.
func fetchUnreadCount(ctx context.Context, s store.Store, appID, userID string) int {
	count, err := s.UnreadCount(ctx, appID, userID)
	if err != nil {
		return 0
	}
	return count
}

// fetchUserPreference returns a user's notification preferences.
func fetchUserPreference(ctx context.Context, s store.Store, appID, userID string) (*preference.Preference, error) {
	return s.GetPreference(ctx, appID, userID)
}

// fetchDriverInfo returns driver name and channel mapping from the registry.
func fetchDriverInfo(reg *driver.Registry) []DriverInfo {
	names := reg.Names()
	infos := make([]DriverInfo, 0, len(names))
	for _, name := range names {
		d, err := reg.Get(name)
		if err != nil {
			continue
		}
		infos = append(infos, DriverInfo{
			Name:    name,
			Channel: d.Channel(),
		})
	}
	return infos
}
