package dashboard

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/xraph/forge/extensions/dashboard/contributor"

	"github.com/xraph/herald"
	"github.com/xraph/herald/dashboard/pages"
	"github.com/xraph/herald/dashboard/settings"
	"github.com/xraph/herald/dashboard/widgets"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/template"
)

// Ensure Contributor implements the required interface at compile time.
var _ contributor.LocalContributor = (*Contributor)(nil)

// Contributor implements the dashboard LocalContributor interface for Herald.
type Contributor struct {
	manifest *contributor.Manifest
	h        *herald.Herald
}

// New creates a new herald dashboard contributor.
func New(manifest *contributor.Manifest, h *herald.Herald) *Contributor {
	return &Contributor{
		manifest: manifest,
		h:        h,
	}
}

// Manifest returns the contributor manifest.
func (c *Contributor) Manifest() *contributor.Manifest { return c.manifest }

// RenderPage renders a page for the given route.
func (c *Contributor) RenderPage(ctx context.Context, route string, params contributor.Params) (templ.Component, error) {
	appID := c.resolveAppID(params)

	switch route {
	case "/":
		return c.renderOverview(ctx, appID)
	case "/providers":
		return c.renderProviders(ctx, appID, params)
	case "/providers/create":
		return c.renderProviderCreate(ctx, appID, params)
	case "/providers/detail":
		return c.renderProviderDetail(ctx, appID, params)
	case "/templates":
		return c.renderTemplates(ctx, appID, params)
	case "/templates/create":
		return c.renderTemplateCreate(ctx, appID, params)
	case "/templates/detail":
		return c.renderTemplateDetail(ctx, appID, params)
	case "/templates/versions/create":
		return c.renderVersionCreate(ctx, params)
	case "/messages":
		return c.renderMessages(ctx, appID, params)
	case "/messages/detail":
		return c.renderMessageDetail(ctx, params)
	case "/inbox":
		return c.renderInbox(ctx, appID, params)
	case "/preferences":
		return c.renderPreferences(ctx, appID, params)
	case "/send-test":
		return c.renderSendTest(ctx, appID, params)
	default:
		return nil, contributor.ErrPageNotFound
	}
}

// RenderWidget renders a widget by ID.
func (c *Contributor) RenderWidget(ctx context.Context, widgetID string) (templ.Component, error) {
	appID := c.defaultAppID()

	switch widgetID {
	case "herald-stats":
		return c.renderStatsWidget(ctx, appID)
	case "herald-recent-messages":
		return c.renderRecentMessagesWidget(ctx, appID)
	case "herald-delivery-status":
		return c.renderDeliveryStatusWidget(ctx, appID)
	case "herald-channel-breakdown":
		return c.renderChannelBreakdownWidget(ctx, appID)
	default:
		return nil, contributor.ErrWidgetNotFound
	}
}

// RenderSettings renders a settings panel by ID.
func (c *Contributor) RenderSettings(_ context.Context, settingID string) (templ.Component, error) {
	switch settingID {
	case "herald-config":
		return c.renderSettings()
	default:
		return nil, contributor.ErrSettingNotFound
	}
}

// ─── Page Renderers ──────────────────────────────────────────────────────────

func (c *Contributor) renderOverview(ctx context.Context, appID string) (templ.Component, error) {
	providerCount, templateCount, _ := fetchOverviewStats(ctx, c.h.Store(), appID)
	activeProviders := fetchActiveProviderCount(ctx, c.h.Store(), appID)
	totalMessages := fetchTotalMessageCount(ctx, c.h.Store(), appID)
	_, failed, pending := fetchMessageCounts(ctx, c.h.Store(), appID)
	channelBreakdown := fetchChannelBreakdown(ctx, c.h.Store(), appID)
	recentMsgs, _ := fetchMessages(ctx, c.h.Store(), appID, "", "", 5) //nolint:errcheck // best-effort

	return pages.OverviewPage(pages.OverviewPageData{
		ProviderCount:       providerCount,
		ActiveProviderCount: activeProviders,
		TemplateCount:       templateCount,
		MessageCount:        totalMessages,
		FailedCount:         failed,
		PendingCount:        pending,
		ChannelBreakdown:    channelBreakdown,
		RecentMessages:      recentMsgs,
	}), nil
}

func (c *Contributor) renderProviders(ctx context.Context, appID string, params contributor.Params) (templ.Component, error) {
	channel := params.QueryParams["channel"]
	providers, err := fetchProviders(ctx, c.h.Store(), appID, channel)
	if err != nil {
		return nil, fmt.Errorf("dashboard: render providers: %w", err)
	}
	return pages.ProvidersPage(pages.ProvidersPageData{
		Providers:     providers,
		ChannelFilter: channel,
	}), nil
}

func (c *Contributor) renderProviderCreate(ctx context.Context, appID string, params contributor.Params) (templ.Component, error) {
	driverInfos := fetchDriverInfo(c.h.Drivers())
	drivers := make([]pages.ProviderDriverInfo, len(driverInfos))
	for i, d := range driverInfos {
		drivers[i] = pages.ProviderDriverInfo{Name: d.Name, Channel: d.Channel}
	}
	channels := channelStrings()

	// Handle form POST.
	if params.FormData["action"] == "create_provider" {
		p := &provider.Provider{
			ID:      id.NewProviderID(),
			AppID:   appID,
			Name:    strings.TrimSpace(params.FormData["name"]),
			Channel: strings.TrimSpace(params.FormData["channel"]),
			Driver:  strings.TrimSpace(params.FormData["driver"]),
			Enabled: true,
		}

		if pri := params.FormData["priority"]; pri != "" {
			if v, err := strconv.Atoi(pri); err == nil {
				p.Priority = v
			}
		}

		p.Credentials = parseKeyValueRows(params.FormData, "cred_key_", "cred_value_", 10)
		p.Settings = parseKeyValueRows(params.FormData, "setting_key_", "setting_value_", 10)
		p.CreatedAt = time.Now()
		p.UpdatedAt = time.Now()

		if err := c.h.Store().CreateProvider(ctx, p); err != nil {
			return pages.ProviderCreatePage(pages.ProviderCreateData{
				Drivers:  drivers,
				Channels: channels,
				Error:    err.Error(),
			}), nil
		}

		return c.renderProviders(ctx, appID, params)
	}

	return pages.ProviderCreatePage(pages.ProviderCreateData{
		Drivers:  drivers,
		Channels: channels,
	}), nil
}

func (c *Contributor) renderProviderDetail(ctx context.Context, _ string, params contributor.Params) (templ.Component, error) {
	idStr := params.PathParams["id"]
	if idStr == "" {
		idStr = params.QueryParams["id"]
	}
	if idStr == "" {
		return nil, contributor.ErrPageNotFound
	}
	pid, err := id.ParseProviderID(idStr)
	if err != nil {
		return nil, contributor.ErrPageNotFound
	}

	// Handle actions.
	if action := params.QueryParams["action"]; action != "" {
		switch action {
		case "enable":
			p, actionErr := c.h.Store().GetProvider(ctx, pid)
			if actionErr == nil {
				p.Enabled = true
				p.UpdatedAt = time.Now()
				_ = c.h.Store().UpdateProvider(ctx, p) //nolint:errcheck // best-effort action
			}
		case "disable":
			p, actionErr := c.h.Store().GetProvider(ctx, pid)
			if actionErr == nil {
				p.Enabled = false
				p.UpdatedAt = time.Now()
				_ = c.h.Store().UpdateProvider(ctx, p) //nolint:errcheck // best-effort action
			}
		case "delete":
			if delErr := c.h.Store().DeleteProvider(ctx, pid); delErr != nil {
				p, _ := c.h.Store().GetProvider(ctx, pid) //nolint:errcheck // best-effort for error display
				return pages.ProviderDetailPage(pages.ProviderDetailPageData{
					Provider: p,
					Error:    delErr.Error(),
				}), nil
			}
			return c.renderProviders(ctx, "", params)
		}
	}

	p, err := c.h.Store().GetProvider(ctx, pid)
	if err != nil {
		return nil, fmt.Errorf("dashboard: resolve provider: %w", err)
	}
	return pages.ProviderDetailPage(pages.ProviderDetailPageData{
		Provider: p,
	}), nil
}

func (c *Contributor) renderTemplates(ctx context.Context, appID string, params contributor.Params) (templ.Component, error) {
	channel := params.QueryParams["channel"]
	category := params.QueryParams["category"]

	templates, err := fetchTemplates(ctx, c.h.Store(), appID, channel)
	if err != nil {
		return nil, fmt.Errorf("dashboard: render templates: %w", err)
	}

	// Client-side category filtering.
	if category != "" {
		filtered := make([]*template.Template, 0, len(templates))
		for _, t := range templates {
			if t.Category == category {
				filtered = append(filtered, t)
			}
		}
		templates = filtered
	}

	return pages.TemplatesPage(pages.TemplatesPageData{
		Templates:      templates,
		ChannelFilter:  channel,
		CategoryFilter: category,
	}), nil
}

func (c *Contributor) renderTemplateCreate(ctx context.Context, appID string, params contributor.Params) (templ.Component, error) {
	channels := channelStrings()

	// Handle form POST.
	if params.FormData["action"] == "create_template" {
		t := &template.Template{
			ID:        id.NewTemplateID(),
			AppID:     appID,
			Name:      strings.TrimSpace(params.FormData["name"]),
			Slug:      strings.TrimSpace(params.FormData["slug"]),
			Channel:   strings.TrimSpace(params.FormData["channel"]),
			Category:  strings.TrimSpace(params.FormData["category"]),
			Enabled:   true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := c.h.Store().CreateTemplate(ctx, t); err != nil {
			return pages.TemplateCreatePage(pages.TemplateCreateData{
				Channels: channels,
				Error:    err.Error(),
			}), nil
		}

		// Create initial version if content was provided.
		locale := strings.TrimSpace(params.FormData["locale"])
		subject := strings.TrimSpace(params.FormData["subject"])
		title := strings.TrimSpace(params.FormData["title"])
		htmlBody := params.FormData["html_body"]
		textBody := params.FormData["text_body"]

		if locale != "" && (subject != "" || htmlBody != "" || textBody != "" || title != "") {
			v := &template.Version{
				ID:         id.NewTemplateVersionID(),
				TemplateID: t.ID,
				Locale:     locale,
				Subject:    subject,
				Title:      title,
				HTML:       htmlBody,
				Text:       textBody,
				Active:     true,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
			if vErr := c.h.Store().CreateVersion(ctx, v); vErr != nil {
				return pages.TemplateCreatePage(pages.TemplateCreateData{
					Channels: channels,
					Error:    "Template created but initial version failed: " + vErr.Error(),
				}), nil
			}
		}

		return c.renderTemplates(ctx, appID, params)
	}

	return pages.TemplateCreatePage(pages.TemplateCreateData{
		Channels: channels,
	}), nil
}

func (c *Contributor) renderTemplateDetail(ctx context.Context, _ string, params contributor.Params) (templ.Component, error) {
	idStr := params.PathParams["id"]
	if idStr == "" {
		idStr = params.QueryParams["id"]
	}
	if idStr == "" {
		return nil, contributor.ErrPageNotFound
	}
	tid, err := id.ParseTemplateID(idStr)
	if err != nil {
		return nil, contributor.ErrPageNotFound
	}

	// Handle actions.
	if action := params.QueryParams["action"]; action != "" {
		switch action {
		case "enable":
			t, actionErr := c.h.Store().GetTemplate(ctx, tid)
			if actionErr == nil {
				t.Enabled = true
				t.UpdatedAt = time.Now()
				_ = c.h.Store().UpdateTemplate(ctx, t) //nolint:errcheck // best-effort action
			}
		case "disable":
			t, actionErr := c.h.Store().GetTemplate(ctx, tid)
			if actionErr == nil {
				t.Enabled = false
				t.UpdatedAt = time.Now()
				_ = c.h.Store().UpdateTemplate(ctx, t) //nolint:errcheck // best-effort action
			}
		case "delete":
			if delErr := c.h.Store().DeleteTemplate(ctx, tid); delErr != nil {
				t, _ := c.h.Store().GetTemplate(ctx, tid)         //nolint:errcheck // best-effort for error display
				versions, _ := c.h.Store().ListVersions(ctx, tid) //nolint:errcheck // best-effort for error display
				return pages.TemplateDetailPage(pages.TemplateDetailPageData{
					Template: t,
					Versions: versions,
					Error:    delErr.Error(),
				}), nil
			}
			return c.renderTemplates(ctx, "", params)
		}
	}

	t, err := c.h.Store().GetTemplate(ctx, tid)
	if err != nil {
		return nil, fmt.Errorf("dashboard: resolve template: %w", err)
	}
	versions, _ := c.h.Store().ListVersions(ctx, tid) //nolint:errcheck // best-effort
	return pages.TemplateDetailPage(pages.TemplateDetailPageData{
		Template: t,
		Versions: versions,
	}), nil
}

func (c *Contributor) renderVersionCreate(ctx context.Context, params contributor.Params) (templ.Component, error) {
	templateID := params.QueryParams["template_id"]
	if templateID == "" {
		templateID = params.FormData["template_id"]
	}
	if templateID == "" {
		return nil, contributor.ErrPageNotFound
	}

	// Handle form POST.
	if params.FormData["action"] == "create_version" {
		tid, err := id.ParseTemplateID(templateID)
		if err != nil {
			return pages.VersionCreatePage(pages.VersionCreateData{
				TemplateID: templateID,
				Error:      "Invalid template ID",
			}), nil
		}

		v := &template.Version{
			ID:         id.NewTemplateVersionID(),
			TemplateID: tid,
			Locale:     strings.TrimSpace(params.FormData["locale"]),
			Subject:    strings.TrimSpace(params.FormData["subject"]),
			Title:      strings.TrimSpace(params.FormData["title"]),
			HTML:       params.FormData["html_body"],
			Text:       params.FormData["text_body"],
			Active:     true,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := c.h.Store().CreateVersion(ctx, v); err != nil {
			return pages.VersionCreatePage(pages.VersionCreateData{
				TemplateID: templateID,
				Error:      err.Error(),
			}), nil
		}

		// Redirect back to template detail.
		return c.renderTemplateDetail(ctx, "", params)
	}

	return pages.VersionCreatePage(pages.VersionCreateData{
		TemplateID: templateID,
	}), nil
}

func (c *Contributor) renderMessages(ctx context.Context, appID string, params contributor.Params) (templ.Component, error) {
	channel := params.QueryParams["channel"]
	status := params.QueryParams["status"]
	msgs, err := fetchMessages(ctx, c.h.Store(), appID, channel, status, 50)
	if err != nil {
		return nil, fmt.Errorf("dashboard: render messages: %w", err)
	}
	totalCount := fetchTotalMessageCount(ctx, c.h.Store(), appID)
	return pages.MessagesPage(pages.MessagesPageData{
		Messages:      msgs,
		StatusFilter:  status,
		ChannelFilter: channel,
		TotalCount:    totalCount,
	}), nil
}

func (c *Contributor) renderMessageDetail(ctx context.Context, params contributor.Params) (templ.Component, error) {
	idStr := params.PathParams["id"]
	if idStr == "" {
		idStr = params.QueryParams["id"]
	}
	if idStr == "" {
		return nil, contributor.ErrPageNotFound
	}
	mid, err := id.ParseMessageID(idStr)
	if err != nil {
		return nil, contributor.ErrPageNotFound
	}

	// Handle retry action.
	if params.QueryParams["action"] == "retry" {
		msg, getErr := c.h.Store().GetMessage(ctx, mid)
		if getErr == nil {
			_, sendErr := c.h.Send(ctx, &herald.SendRequest{
				AppID:   msg.AppID,
				Channel: msg.Channel,
				To:      []string{msg.Recipient},
				Subject: msg.Subject,
				Body:    msg.Body,
			})
			if sendErr != nil {
				return pages.MessageDetailPage(pages.MessageDetailPageData{
					Message: msg,
					Error:   "Retry failed: " + sendErr.Error(),
				}), nil
			}
			// Re-fetch to show updated state.
			msg, _ = c.h.Store().GetMessage(ctx, mid) //nolint:errcheck // best-effort re-fetch after retry
			return pages.MessageDetailPage(pages.MessageDetailPageData{
				Message: msg,
			}), nil
		}
	}

	msg, err := c.h.Store().GetMessage(ctx, mid)
	if err != nil {
		return nil, fmt.Errorf("dashboard: resolve message: %w", err)
	}
	return pages.MessageDetailPage(pages.MessageDetailPageData{
		Message: msg,
	}), nil
}

func (c *Contributor) renderInbox(ctx context.Context, appID string, params contributor.Params) (templ.Component, error) {
	userID := params.QueryParams["user_id"]

	if userID == "" {
		return pages.InboxPage(pages.InboxPageData{}), nil
	}

	// Handle actions.
	if action := params.QueryParams["action"]; action != "" {
		switch action {
		case "mark_all_read":
			_ = c.h.Store().MarkAllRead(ctx, appID, userID) //nolint:errcheck // best-effort action
		case "mark_read":
			if nid := params.QueryParams["notif_id"]; nid != "" {
				if parsed, err := id.ParseInboxID(nid); err == nil {
					_ = c.h.Store().MarkRead(ctx, parsed) //nolint:errcheck // best-effort action
				}
			}
		case "delete":
			if nid := params.QueryParams["notif_id"]; nid != "" {
				if parsed, err := id.ParseInboxID(nid); err == nil {
					_ = c.h.Store().DeleteNotification(ctx, parsed) //nolint:errcheck // best-effort action
				}
			}
		}
	}

	notifications, err := fetchInboxNotifications(ctx, c.h.Store(), appID, userID, 50, 0)
	if err != nil {
		return pages.InboxPage(pages.InboxPageData{
			UserID: userID,
			Error:  err.Error(),
		}), nil
	}

	unreadCount := fetchUnreadCount(ctx, c.h.Store(), appID, userID)

	return pages.InboxPage(pages.InboxPageData{
		Notifications: notifications,
		UserID:        userID,
		UnreadCount:   unreadCount,
	}), nil
}

func (c *Contributor) renderPreferences(ctx context.Context, appID string, params contributor.Params) (templ.Component, error) {
	userID := params.QueryParams["user_id"]

	if userID == "" {
		return pages.PreferencesPage(pages.PreferencesPageData{}), nil
	}

	pref, err := fetchUserPreference(ctx, c.h.Store(), appID, userID)
	if err != nil {
		return pages.PreferencesPage(pages.PreferencesPageData{
			UserID: userID,
		}), nil
	}

	return pages.PreferencesPage(pages.PreferencesPageData{
		Preference: pref,
		UserID:     userID,
	}), nil
}

func (c *Contributor) renderSendTest(ctx context.Context, appID string, params contributor.Params) (templ.Component, error) {
	channels := channelStrings()

	// Handle form POST.
	if params.FormData["action"] == "send_test" {
		req := &herald.SendRequest{
			AppID:    appID,
			Channel:  strings.TrimSpace(params.FormData["channel"]),
			To:       []string{strings.TrimSpace(params.FormData["recipient"])},
			Subject:  strings.TrimSpace(params.FormData["subject"]),
			Body:     strings.TrimSpace(params.FormData["body"]),
			Template: strings.TrimSpace(params.FormData["template_slug"]),
		}

		result, err := c.h.Send(ctx, req)
		if err != nil {
			return pages.SendTestPage(pages.SendTestPageData{
				Channels: channels,
				Error:    err.Error(),
			}), nil
		}

		return pages.SendTestPage(pages.SendTestPageData{
			Channels: channels,
			Result:   result,
			Success:  true,
		}), nil
	}

	return pages.SendTestPage(pages.SendTestPageData{
		Channels: channels,
	}), nil
}

// ─── Widget Render Helpers ───────────────────────────────────────────────────

func (c *Contributor) renderStatsWidget(ctx context.Context, appID string) (templ.Component, error) {
	providerCount, templateCount, messageCount := fetchOverviewStats(ctx, c.h.Store(), appID)
	activeProviders := fetchActiveProviderCount(ctx, c.h.Store(), appID)
	return widgets.StatsWidget(providerCount, activeProviders, templateCount, messageCount), nil
}

func (c *Contributor) renderRecentMessagesWidget(ctx context.Context, appID string) (templ.Component, error) {
	msgs, _ := fetchMessages(ctx, c.h.Store(), appID, "", "", 5) //nolint:errcheck // best-effort
	return widgets.RecentMessagesWidget(msgs), nil
}

func (c *Contributor) renderDeliveryStatusWidget(ctx context.Context, appID string) (templ.Component, error) {
	sent, failed, pending := fetchMessageCounts(ctx, c.h.Store(), appID)
	total := sent + failed + pending
	var successRate int
	if total > 0 {
		successRate = (sent * 100) / total
	}
	return widgets.DeliveryStatusWidget(sent, failed, pending, successRate), nil
}

func (c *Contributor) renderChannelBreakdownWidget(ctx context.Context, appID string) (templ.Component, error) {
	breakdown := fetchChannelBreakdown(ctx, c.h.Store(), appID)
	return widgets.ChannelBreakdownWidget(breakdown), nil
}

// ─── Settings Render Helper ──────────────────────────────────────────────────

func (c *Contributor) renderSettings() (templ.Component, error) {
	cfg := c.h.Config()
	driverNames := c.h.Drivers().Names()
	rawInfos := fetchDriverInfo(c.h.Drivers())
	driverInfos := make([]settings.ConfigDriverInfo, len(rawInfos))
	for i, d := range rawInfos {
		driverInfos[i] = settings.ConfigDriverInfo{Name: d.Name, Channel: d.Channel}
	}
	return settings.ConfigPanel(cfg, driverNames, driverInfos), nil
}

// ─── App ID Resolution ──────────────────────────────────────────────────────

func (c *Contributor) resolveAppID(params contributor.Params) string {
	if appID := params.QueryParams["app_id"]; appID != "" {
		return appID
	}
	return c.defaultAppID()
}

func (c *Contributor) defaultAppID() string {
	return ""
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// channelStrings returns channel names as string slice.
func channelStrings() []string {
	chs := herald.ValidChannels()
	out := make([]string, len(chs))
	for i, ch := range chs {
		out[i] = string(ch)
	}
	return out
}

// parseKeyValueRows extracts key-value pairs from indexed form fields.
func parseKeyValueRows(formData map[string]string, keyPrefix, valPrefix string, maxRows int) map[string]string {
	result := make(map[string]string)
	for i := 0; i < maxRows; i++ {
		key := strings.TrimSpace(formData[fmt.Sprintf("%s%d", keyPrefix, i)])
		val := strings.TrimSpace(formData[fmt.Sprintf("%s%d", valPrefix, i)])
		if key != "" && val != "" {
			result[key] = val
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
