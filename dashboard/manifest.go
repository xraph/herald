package dashboard

import (
	"github.com/xraph/forge/extensions/dashboard/contributor"
)

// NewManifest builds a contributor.Manifest for the herald dashboard.
func NewManifest() *contributor.Manifest {
	return &contributor.Manifest{
		Name:        "herald",
		DisplayName: "Herald",
		Icon:        "bell",
		Version:     "1.0.0",
		Layout:      "extension",
		ShowSidebar: boolPtr(true),
		TopbarConfig: &contributor.TopbarConfig{
			Title:       "Herald",
			LogoIcon:    "bell",
			AccentColor: "#f59e0b",
			ShowSearch:  true,
			Actions: []contributor.TopbarAction{
				{Label: "Send Test", Icon: "send", Href: "/send-test", Variant: "outline"},
			},
		},
		Nav:      baseNav(),
		Widgets:  baseWidgets(),
		Settings: baseSettings(),
		Capabilities: []string{
			"searchable",
		},
	}
}

// baseNav returns the core navigation items for the herald dashboard.
func baseNav() []contributor.NavItem {
	return []contributor.NavItem{
		{Label: "Overview", Path: "/", Icon: "layout-dashboard", Group: "Notifications", Priority: 0},
		{Label: "Providers", Path: "/providers", Icon: "server", Group: "Notifications", Priority: 1},
		{Label: "Templates", Path: "/templates", Icon: "file-text", Group: "Notifications", Priority: 2},
		{Label: "Messages", Path: "/messages", Icon: "send", Group: "Notifications", Priority: 3},
		{Label: "Inbox", Path: "/inbox", Icon: "inbox", Group: "Delivery", Priority: 0},
		{Label: "Preferences", Path: "/preferences", Icon: "users", Group: "Configuration", Priority: 0},
	}
}

// baseWidgets returns the core widget descriptors for the herald dashboard.
func baseWidgets() []contributor.WidgetDescriptor {
	return []contributor.WidgetDescriptor{
		{
			ID:          "herald-stats",
			Title:       "Notification Stats",
			Description: "Provider, template, and message counts",
			Size:        "md",
			RefreshSec:  60,
			Group:       "Notifications",
		},
		{
			ID:          "herald-recent-messages",
			Title:       "Recent Messages",
			Description: "Latest deliveries",
			Size:        "md",
			RefreshSec:  30,
			Group:       "Notifications",
		},
		{
			ID:          "herald-delivery-status",
			Title:       "Delivery Status",
			Description: "Success and failure breakdown",
			Size:        "lg",
			RefreshSec:  60,
			Group:       "Notifications",
		},
		{
			ID:          "herald-channel-breakdown",
			Title:       "Channel Breakdown",
			Description: "Messages per channel type",
			Size:        "md",
			RefreshSec:  60,
			Group:       "Notifications",
		},
	}
}

// baseSettings returns the settings descriptors for the herald dashboard.
func baseSettings() []contributor.SettingsDescriptor {
	return []contributor.SettingsDescriptor{
		{
			ID:          "herald-config",
			Title:       "Notification Settings",
			Description: "Configure notification engine behavior",
			Group:       "Notifications",
			Icon:        "bell",
		},
	}
}

func boolPtr(b bool) *bool { return &b }
