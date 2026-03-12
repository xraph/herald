package dashboard

import (
	"errors"
	"time"

	"github.com/xraph/forgeui/bridge"

	"github.com/xraph/herald"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/inbox"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/preference"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/template"
)

// RegisterBridge registers all Herald bridge functions on the given bridge instance.
func RegisterBridge(b *bridge.Bridge, heraldFn func() *herald.Herald) error {
	reg := &bridgeRegistry{heraldFn: heraldFn}

	// Overview / Stats
	if err := b.Register("herald.getOverview", reg.getOverview,
		bridge.WithDescription("Get notification overview stats"),
		bridge.WithFunctionCache(5*time.Second),
	); err != nil {
		return err
	}

	if err := b.Register("herald.getMessageCounts", reg.getMessageCounts,
		bridge.WithDescription("Get message counts by status"),
		bridge.WithFunctionCache(10*time.Second),
	); err != nil {
		return err
	}

	// Providers
	if err := b.Register("herald.getProviders", reg.getProviders,
		bridge.WithDescription("List notification providers"),
		bridge.WithFunctionCache(5*time.Second),
	); err != nil {
		return err
	}

	if err := b.Register("herald.getProvider", reg.getProvider,
		bridge.WithDescription("Get a single provider by ID"),
	); err != nil {
		return err
	}

	if err := b.Register("herald.createProvider", reg.createProvider,
		bridge.WithDescription("Create a new notification provider"),
	); err != nil {
		return err
	}

	if err := b.Register("herald.updateProvider", reg.updateProvider,
		bridge.WithDescription("Update an existing provider"),
	); err != nil {
		return err
	}

	if err := b.Register("herald.deleteProvider", reg.deleteProvider,
		bridge.WithDescription("Delete a provider"),
	); err != nil {
		return err
	}

	if err := b.Register("herald.toggleProvider", reg.toggleProvider,
		bridge.WithDescription("Enable or disable a provider"),
	); err != nil {
		return err
	}

	// Templates
	if err := b.Register("herald.getTemplates", reg.getTemplates,
		bridge.WithDescription("List notification templates"),
		bridge.WithFunctionCache(5*time.Second),
	); err != nil {
		return err
	}

	if err := b.Register("herald.getTemplate", reg.getTemplate,
		bridge.WithDescription("Get a single template with versions"),
	); err != nil {
		return err
	}

	if err := b.Register("herald.createTemplate", reg.createTemplate,
		bridge.WithDescription("Create a new notification template"),
	); err != nil {
		return err
	}

	if err := b.Register("herald.updateTemplate", reg.updateTemplate,
		bridge.WithDescription("Update an existing template"),
	); err != nil {
		return err
	}

	if err := b.Register("herald.deleteTemplate", reg.deleteTemplate,
		bridge.WithDescription("Delete a template"),
	); err != nil {
		return err
	}

	if err := b.Register("herald.createVersion", reg.createVersion,
		bridge.WithDescription("Create a new template version"),
	); err != nil {
		return err
	}

	// Messages
	if err := b.Register("herald.getMessages", reg.getMessages,
		bridge.WithDescription("List notification messages"),
		bridge.WithFunctionCache(5*time.Second),
	); err != nil {
		return err
	}

	if err := b.Register("herald.getMessage", reg.getMessage,
		bridge.WithDescription("Get a single message by ID"),
	); err != nil {
		return err
	}

	// Inbox
	if err := b.Register("herald.getInbox", reg.getInbox,
		bridge.WithDescription("Get in-app notifications for a user"),
	); err != nil {
		return err
	}

	if err := b.Register("herald.markRead", reg.markRead,
		bridge.WithDescription("Mark a notification as read"),
	); err != nil {
		return err
	}

	if err := b.Register("herald.markAllRead", reg.markAllRead,
		bridge.WithDescription("Mark all notifications as read for a user"),
	); err != nil {
		return err
	}

	if err := b.Register("herald.deleteNotification", reg.deleteNotification,
		bridge.WithDescription("Delete an inbox notification"),
	); err != nil {
		return err
	}

	// Preferences
	if err := b.Register("herald.getPreferences", reg.getPreferences,
		bridge.WithDescription("Get user notification preferences"),
	); err != nil {
		return err
	}

	if err := b.Register("herald.updatePreferences", reg.updatePreferences,
		bridge.WithDescription("Update user notification preferences"),
	); err != nil {
		return err
	}

	// Send test
	if err := b.Register("herald.sendTest", reg.sendTest,
		bridge.WithDescription("Send a test notification"),
	); err != nil {
		return err
	}

	// Config
	return b.Register("herald.getConfig", reg.getConfig,
		bridge.WithDescription("Get Herald configuration"),
		bridge.WithFunctionCache(30*time.Second),
	)
}

// bridgeRegistry holds resolver functions for bridge function handlers.
type bridgeRegistry struct {
	heraldFn func() *herald.Herald
}

func (r *bridgeRegistry) resolveHerald() (*herald.Herald, error) {
	h := r.heraldFn()
	if h == nil {
		return nil, errors.New("herald not initialized")
	}
	return h, nil
}

func (r *bridgeRegistry) resolveStore() (store.Store, error) {
	h, err := r.resolveHerald()
	if err != nil {
		return nil, err
	}
	return h.Store(), nil
}

// ── Parameter types ─────────────────────────────────────────────────────────

type emptyParams struct{}

type idParams struct {
	ID string `json:"id"`
}

type channelFilterParams struct {
	Channel string `json:"channel"`
}

type templateFilterParams struct {
	Channel  string `json:"channel"`
	Category string `json:"category"`
}

type messageFilterParams struct {
	Channel string `json:"channel"`
	Status  string `json:"status"`
	Limit   int    `json:"limit"`
}

type inboxParams struct {
	UserID string `json:"user_id"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

type markAllReadParams struct {
	UserID string `json:"user_id"`
}

type createProviderParams struct {
	AppID       string            `json:"app_id"`
	Name        string            `json:"name"`
	Channel     string            `json:"channel"`
	Driver      string            `json:"driver"`
	Priority    int               `json:"priority"`
	Enabled     bool              `json:"enabled"`
	Credentials map[string]string `json:"credentials"`
	Settings    map[string]string `json:"settings"`
}

type updateProviderParams struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Channel     string            `json:"channel"`
	Driver      string            `json:"driver"`
	Priority    int               `json:"priority"`
	Enabled     bool              `json:"enabled"`
	Credentials map[string]string `json:"credentials"`
	Settings    map[string]string `json:"settings"`
}

type toggleProviderParams struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

type createTemplateParams struct {
	AppID    string `json:"app_id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Channel  string `json:"channel"`
	Category string `json:"category"`
	Enabled  bool   `json:"enabled"`
}

type updateTemplateParams struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Channel  string `json:"channel"`
	Category string `json:"category"`
	Enabled  bool   `json:"enabled"`
}

type createVersionParams struct {
	TemplateID string `json:"template_id"`
	Locale     string `json:"locale"`
	Subject    string `json:"subject"`
	HTML       string `json:"html"`
	Text       string `json:"text"`
	Active     bool   `json:"active"`
}

type preferencesParams struct {
	UserID string `json:"user_id"`
}

type updatePreferencesParams struct {
	AppID     string                                  `json:"app_id"`
	UserID    string                                  `json:"user_id"`
	Overrides map[string]preference.ChannelPreference `json:"overrides"`
}

type sendTestParams struct {
	AppID     string `json:"app_id"`
	Channel   string `json:"channel"`
	Recipient string `json:"recipient"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	Template  string `json:"template"`
}

// ── Response types ──────────────────────────────────────────────────────────

type overviewResponse struct {
	ProviderCount       int            `json:"provider_count"`
	ActiveProviderCount int            `json:"active_provider_count"`
	TemplateCount       int            `json:"template_count"`
	MessageCount        int            `json:"message_count"`
	FailedCount         int            `json:"failed_count"`
	PendingCount        int            `json:"pending_count"`
	ChannelBreakdown    map[string]int `json:"channel_breakdown"`
}

type messageCountsResponse struct {
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
	Pending int `json:"pending"`
	Total   int `json:"total"`
}

type templateDetailResponse struct {
	Template *template.Template  `json:"template"`
	Versions []*template.Version `json:"versions"`
}

type inboxResponse struct {
	Notifications []*inbox.Notification `json:"notifications"`
	UnreadCount   int                   `json:"unread_count"`
}

type configResponse struct {
	DefaultLocale     string       `json:"default_locale"`
	MaxBatchSize      int          `json:"max_batch_size"`
	TruncateBodyAt    int          `json:"truncate_body_at"`
	DriverNames       []string     `json:"driver_names"`
	Drivers           []DriverInfo `json:"drivers"`
	SupportedChannels []string     `json:"supported_channels"`
}

type actionResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ── Handler implementations ─────────────────────────────────────────────────

func (r *bridgeRegistry) getOverview(ctx bridge.Context, _ emptyParams) (*overviewResponse, error) {
	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	goCtx := ctx.Context()

	provs, _ := s.ListAllProviders(goCtx, "")                              //nolint:errcheck // best-effort display data
	tmpls, _ := s.ListTemplates(goCtx, "")                                 //nolint:errcheck // best-effort display data
	msgs, _ := s.ListMessages(goCtx, "", message.ListOptions{Limit: 1000}) //nolint:errcheck // best-effort display data

	var activeCount, failedCount, pendingCount int
	channelBreakdown := make(map[string]int)

	for _, p := range provs {
		if p.Enabled {
			activeCount++
		}
	}

	for _, m := range msgs {
		channelBreakdown[m.Channel]++
		switch m.Status {
		case message.StatusFailed:
			failedCount++
		case message.StatusQueued, message.StatusSending:
			pendingCount++
		}
	}

	return &overviewResponse{
		ProviderCount:       len(provs),
		ActiveProviderCount: activeCount,
		TemplateCount:       len(tmpls),
		MessageCount:        len(msgs),
		FailedCount:         failedCount,
		PendingCount:        pendingCount,
		ChannelBreakdown:    channelBreakdown,
	}, nil
}

func (r *bridgeRegistry) getMessageCounts(ctx bridge.Context, _ emptyParams) (*messageCountsResponse, error) {
	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	msgs, _ := s.ListMessages(ctx.Context(), "", message.ListOptions{Limit: 1000}) //nolint:errcheck // best-effort display data

	var sent, failed, pending int
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

	return &messageCountsResponse{
		Sent:    sent,
		Failed:  failed,
		Pending: pending,
		Total:   sent + failed + pending,
	}, nil
}

func (r *bridgeRegistry) getProviders(ctx bridge.Context, params channelFilterParams) ([]*provider.Provider, error) {
	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	if params.Channel != "" {
		return s.ListProviders(ctx.Context(), "", params.Channel)
	}
	return s.ListAllProviders(ctx.Context(), "")
}

func (r *bridgeRegistry) getProvider(ctx bridge.Context, params idParams) (*provider.Provider, error) {
	if params.ID == "" {
		return nil, errors.New("id is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	pid, parseErr := id.ParseProviderID(params.ID)
	if parseErr != nil {
		return nil, parseErr
	}

	return s.GetProvider(ctx.Context(), pid)
}

func (r *bridgeRegistry) createProvider(ctx bridge.Context, params createProviderParams) (*provider.Provider, error) {
	if params.Name == "" {
		return nil, errors.New("name is required")
	}
	if params.Channel == "" {
		return nil, errors.New("channel is required")
	}
	if params.Driver == "" {
		return nil, errors.New("driver is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	p := &provider.Provider{
		ID:          id.NewProviderID(),
		AppID:       params.AppID,
		Name:        params.Name,
		Channel:     params.Channel,
		Driver:      params.Driver,
		Priority:    params.Priority,
		Enabled:     params.Enabled,
		Credentials: params.Credentials,
		Settings:    params.Settings,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	if createErr := s.CreateProvider(ctx.Context(), p); createErr != nil {
		return nil, createErr
	}
	return p, nil
}

func (r *bridgeRegistry) updateProvider(ctx bridge.Context, params updateProviderParams) (*provider.Provider, error) {
	if params.ID == "" {
		return nil, errors.New("id is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	pid, parseErr := id.ParseProviderID(params.ID)
	if parseErr != nil {
		return nil, parseErr
	}

	p, getErr := s.GetProvider(ctx.Context(), pid)
	if getErr != nil {
		return nil, getErr
	}

	if params.Name != "" {
		p.Name = params.Name
	}
	if params.Channel != "" {
		p.Channel = params.Channel
	}
	if params.Driver != "" {
		p.Driver = params.Driver
	}
	p.Priority = params.Priority
	p.Enabled = params.Enabled
	if params.Credentials != nil {
		p.Credentials = params.Credentials
	}
	if params.Settings != nil {
		p.Settings = params.Settings
	}
	p.UpdatedAt = time.Now().UTC()

	if updateErr := s.UpdateProvider(ctx.Context(), p); updateErr != nil {
		return nil, updateErr
	}
	return p, nil
}

func (r *bridgeRegistry) deleteProvider(ctx bridge.Context, params idParams) (*actionResult, error) {
	if params.ID == "" {
		return nil, errors.New("id is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	pid, parseErr := id.ParseProviderID(params.ID)
	if parseErr != nil {
		return nil, parseErr
	}

	if delErr := s.DeleteProvider(ctx.Context(), pid); delErr != nil {
		return nil, delErr
	}

	return &actionResult{Success: true, Message: "Provider deleted"}, nil
}

func (r *bridgeRegistry) toggleProvider(ctx bridge.Context, params toggleProviderParams) (*actionResult, error) {
	if params.ID == "" {
		return nil, errors.New("id is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	pid, parseErr := id.ParseProviderID(params.ID)
	if parseErr != nil {
		return nil, parseErr
	}

	p, getErr := s.GetProvider(ctx.Context(), pid)
	if getErr != nil {
		return nil, getErr
	}

	p.Enabled = params.Enabled
	p.UpdatedAt = time.Now().UTC()

	if updateErr := s.UpdateProvider(ctx.Context(), p); updateErr != nil {
		return nil, updateErr
	}

	status := "enabled"
	if !params.Enabled {
		status = "disabled"
	}
	return &actionResult{Success: true, Message: "Provider " + status}, nil
}

func (r *bridgeRegistry) getTemplates(ctx bridge.Context, params templateFilterParams) ([]*template.Template, error) {
	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	if params.Channel != "" {
		return s.ListTemplatesByChannel(ctx.Context(), "", params.Channel)
	}
	return s.ListTemplates(ctx.Context(), "")
}

func (r *bridgeRegistry) getTemplate(ctx bridge.Context, params idParams) (*templateDetailResponse, error) {
	if params.ID == "" {
		return nil, errors.New("id is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	tid, parseErr := id.ParseTemplateID(params.ID)
	if parseErr != nil {
		return nil, parseErr
	}

	t, getErr := s.GetTemplate(ctx.Context(), tid)
	if getErr != nil {
		return nil, getErr
	}

	versions, _ := s.ListVersions(ctx.Context(), tid) //nolint:errcheck // best-effort display data

	return &templateDetailResponse{
		Template: t,
		Versions: versions,
	}, nil
}

func (r *bridgeRegistry) createTemplate(ctx bridge.Context, params createTemplateParams) (*template.Template, error) {
	if params.Name == "" {
		return nil, errors.New("name is required")
	}
	if params.Slug == "" {
		return nil, errors.New("slug is required")
	}
	if params.Channel == "" {
		return nil, errors.New("channel is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	t := &template.Template{
		ID:        id.NewTemplateID(),
		AppID:     params.AppID,
		Name:      params.Name,
		Slug:      params.Slug,
		Channel:   params.Channel,
		Category:  params.Category,
		Enabled:   params.Enabled,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if createErr := s.CreateTemplate(ctx.Context(), t); createErr != nil {
		return nil, createErr
	}
	return t, nil
}

func (r *bridgeRegistry) updateTemplate(ctx bridge.Context, params updateTemplateParams) (*template.Template, error) {
	if params.ID == "" {
		return nil, errors.New("id is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	tid, parseErr := id.ParseTemplateID(params.ID)
	if parseErr != nil {
		return nil, parseErr
	}

	t, getErr := s.GetTemplate(ctx.Context(), tid)
	if getErr != nil {
		return nil, getErr
	}

	if params.Name != "" {
		t.Name = params.Name
	}
	if params.Slug != "" {
		t.Slug = params.Slug
	}
	if params.Channel != "" {
		t.Channel = params.Channel
	}
	if params.Category != "" {
		t.Category = params.Category
	}
	t.Enabled = params.Enabled
	t.UpdatedAt = time.Now().UTC()

	if updateErr := s.UpdateTemplate(ctx.Context(), t); updateErr != nil {
		return nil, updateErr
	}
	return t, nil
}

func (r *bridgeRegistry) deleteTemplate(ctx bridge.Context, params idParams) (*actionResult, error) {
	if params.ID == "" {
		return nil, errors.New("id is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	tid, parseErr := id.ParseTemplateID(params.ID)
	if parseErr != nil {
		return nil, parseErr
	}

	if delErr := s.DeleteTemplate(ctx.Context(), tid); delErr != nil {
		return nil, delErr
	}
	return &actionResult{Success: true, Message: "Template deleted"}, nil
}

func (r *bridgeRegistry) createVersion(ctx bridge.Context, params createVersionParams) (*template.Version, error) {
	if params.TemplateID == "" {
		return nil, errors.New("template_id is required")
	}
	if params.Locale == "" {
		return nil, errors.New("locale is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	tid, parseErr := id.ParseTemplateID(params.TemplateID)
	if parseErr != nil {
		return nil, parseErr
	}

	v := &template.Version{
		ID:         id.NewTemplateVersionID(),
		TemplateID: tid,
		Locale:     params.Locale,
		Subject:    params.Subject,
		HTML:       params.HTML,
		Text:       params.Text,
		Active:     params.Active,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	if createErr := s.CreateVersion(ctx.Context(), v); createErr != nil {
		return nil, createErr
	}
	return v, nil
}

func (r *bridgeRegistry) getMessages(ctx bridge.Context, params messageFilterParams) ([]*message.Message, error) {
	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}

	return s.ListMessages(ctx.Context(), "", message.ListOptions{
		Channel: params.Channel,
		Status:  message.Status(params.Status),
		Limit:   limit,
	})
}

func (r *bridgeRegistry) getMessage(ctx bridge.Context, params idParams) (*message.Message, error) {
	if params.ID == "" {
		return nil, errors.New("id is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	mid, parseErr := id.ParseMessageID(params.ID)
	if parseErr != nil {
		return nil, parseErr
	}

	return s.GetMessage(ctx.Context(), mid)
}

func (r *bridgeRegistry) getInbox(ctx bridge.Context, params inboxParams) (*inboxResponse, error) {
	if params.UserID == "" {
		return nil, errors.New("user_id is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}

	notifications, listErr := s.ListNotifications(ctx.Context(), "", params.UserID, limit, params.Offset)
	if listErr != nil {
		return nil, listErr
	}

	unread, _ := s.UnreadCount(ctx.Context(), "", params.UserID) //nolint:errcheck // best-effort display data

	return &inboxResponse{
		Notifications: notifications,
		UnreadCount:   unread,
	}, nil
}

func (r *bridgeRegistry) markRead(ctx bridge.Context, params idParams) (*actionResult, error) {
	if params.ID == "" {
		return nil, errors.New("id is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	nid, parseErr := id.ParseInboxID(params.ID)
	if parseErr != nil {
		return nil, parseErr
	}

	if markErr := s.MarkRead(ctx.Context(), nid); markErr != nil {
		return nil, markErr
	}
	return &actionResult{Success: true, Message: "Notification marked as read"}, nil
}

func (r *bridgeRegistry) markAllRead(ctx bridge.Context, params markAllReadParams) (*actionResult, error) {
	if params.UserID == "" {
		return nil, errors.New("user_id is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	if markErr := s.MarkAllRead(ctx.Context(), "", params.UserID); markErr != nil {
		return nil, markErr
	}
	return &actionResult{Success: true, Message: "All notifications marked as read"}, nil
}

func (r *bridgeRegistry) deleteNotification(ctx bridge.Context, params idParams) (*actionResult, error) {
	if params.ID == "" {
		return nil, errors.New("id is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	nid, parseErr := id.ParseInboxID(params.ID)
	if parseErr != nil {
		return nil, parseErr
	}

	if delErr := s.DeleteNotification(ctx.Context(), nid); delErr != nil {
		return nil, delErr
	}
	return &actionResult{Success: true, Message: "Notification deleted"}, nil
}

func (r *bridgeRegistry) getPreferences(ctx bridge.Context, params preferencesParams) (*preference.Preference, error) {
	if params.UserID == "" {
		return nil, errors.New("user_id is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	return s.GetPreference(ctx.Context(), "", params.UserID)
}

func (r *bridgeRegistry) updatePreferences(ctx bridge.Context, params updatePreferencesParams) (*actionResult, error) {
	if params.UserID == "" {
		return nil, errors.New("user_id is required")
	}

	s, err := r.resolveStore()
	if err != nil {
		return nil, err
	}

	pref := &preference.Preference{
		AppID:     params.AppID,
		UserID:    params.UserID,
		Overrides: params.Overrides,
	}

	if setErr := s.SetPreference(ctx.Context(), pref); setErr != nil {
		return nil, setErr
	}
	return &actionResult{Success: true, Message: "Preferences updated"}, nil
}

func (r *bridgeRegistry) sendTest(ctx bridge.Context, params sendTestParams) (*herald.SendResult, error) {
	if params.Channel == "" {
		return nil, errors.New("channel is required")
	}
	if params.Recipient == "" {
		return nil, errors.New("recipient is required")
	}

	h, err := r.resolveHerald()
	if err != nil {
		return nil, err
	}

	req := &herald.SendRequest{
		AppID:   params.AppID,
		Channel: params.Channel,
		To:      []string{params.Recipient},
		Subject: params.Subject,
		Body:    params.Body,
	}
	if params.Template != "" {
		req.Template = params.Template
	}

	return h.Send(ctx.Context(), req)
}

func (r *bridgeRegistry) getConfig(_ bridge.Context, _ emptyParams) (*configResponse, error) {
	h, err := r.resolveHerald()
	if err != nil {
		return nil, err
	}

	cfg := h.Config()
	drivers := h.Drivers()
	driverNames := drivers.Names()

	var driverInfos []DriverInfo
	channelSet := make(map[string]bool)
	for _, name := range driverNames {
		d, dErr := drivers.Get(name)
		if dErr != nil {
			continue
		}
		ch := d.Channel()
		driverInfos = append(driverInfos, DriverInfo{Name: name, Channel: ch})
		channelSet[ch] = true
	}

	var channels []string
	for ch := range channelSet {
		channels = append(channels, ch)
	}

	return &configResponse{
		DefaultLocale:     cfg.DefaultLocale,
		MaxBatchSize:      cfg.MaxBatchSize,
		TruncateBodyAt:    cfg.TruncateBodyAt,
		DriverNames:       driverNames,
		Drivers:           driverInfos,
		SupportedChannels: channels,
	}, nil
}
