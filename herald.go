// Package herald provides a unified, multi-channel notification delivery engine.
//
// Herald supports email, SMS, push notifications, and in-app notifications
// with pluggable provider drivers, a template system with i18n support,
// and scoped configuration overrides (app → org → user).
package herald

import (
	"context"
	"fmt"
	"time"

	"github.com/xraph/herald/bridge"
	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/inbox"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/template"
)

// SendRequest describes a single notification to send.
type SendRequest struct {
	AppID    string            `json:"app_id"`
	EnvID    string            `json:"env_id,omitempty"`
	OrgID    string            `json:"org_id,omitempty"`
	UserID   string            `json:"user_id,omitempty"`
	Channel  string            `json:"channel"`
	Template string            `json:"template,omitempty"`
	Locale   string            `json:"locale,omitempty"`
	To       []string          `json:"to"`
	Data     map[string]any    `json:"data,omitempty"`
	Subject  string            `json:"subject,omitempty"`
	Body     string            `json:"body,omitempty"`
	Async    bool              `json:"async,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// NotifyRequest sends a notification across multiple channels using a template.
type NotifyRequest struct {
	AppID    string            `json:"app_id"`
	EnvID    string            `json:"env_id,omitempty"`
	OrgID    string            `json:"org_id,omitempty"`
	UserID   string            `json:"user_id,omitempty"`
	Template string            `json:"template"`
	Locale   string            `json:"locale,omitempty"`
	To       []string          `json:"to"`
	Data     map[string]any    `json:"data,omitempty"`
	Channels []string          `json:"channels"`
	Async    bool              `json:"async,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// SendResult contains the outcome of a send operation.
type SendResult struct {
	MessageID  id.MessageID   `json:"message_id"`
	Status     message.Status `json:"status"`
	ProviderID string         `json:"provider_id,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// Send delivers a notification on a single channel.
func (h *Herald) Send(ctx context.Context, req *SendRequest) (*SendResult, error) {
	channel := req.Channel

	// Check user preferences if user ID is provided
	if req.UserID != "" && req.Template != "" {
		pref, _ := h.store.GetPreference(ctx, req.AppID, req.UserID) //nolint:errcheck // preference lookup is optional
		if pref != nil && pref.IsOptedOut(req.Template, channel) {
			h.logger.Debug("herald: user opted out",
				"user_id", req.UserID,
				"template", req.Template,
				"channel", channel,
			)
			return &SendResult{Status: message.StatusSent, Error: "user opted out"}, nil
		}
	}

	// Resolve template and render content
	var rendered *template.RenderedContent
	if req.Template != "" && req.Body == "" {
		tmpl, err := h.store.GetTemplateBySlug(ctx, req.AppID, req.Template, channel)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrTemplateNotFound, err)
		}
		if !tmpl.Enabled {
			return nil, ErrTemplateDisabled
		}

		locale := req.Locale
		if locale == "" {
			locale = h.config.DefaultLocale
		}

		rendered, err = h.renderer.Render(tmpl, locale, req.Data)
		if err != nil {
			return nil, err
		}
	} else {
		rendered = &template.RenderedContent{
			Subject: req.Subject,
			Text:    req.Body,
		}
	}

	// Resolve provider via scope chain
	resolved, err := h.resolver.ResolveProvider(ctx, req.AppID, req.OrgID, req.UserID, channel)
	if err != nil || resolved == nil {
		return nil, fmt.Errorf("%w: channel=%s", ErrNoProviderConfigured, channel)
	}

	prov := resolved.Provider

	// Get the driver for this provider
	drv, err := h.drivers.Get(prov.Driver)
	if err != nil {
		return nil, fmt.Errorf("%w: driver=%s", ErrDriverNotFound, prov.Driver)
	}

	// Build outbound message with provider credentials injected into Data
	driverData := make(map[string]string)
	for k, v := range prov.Credentials {
		driverData[k] = v
	}
	for k, v := range prov.Settings {
		driverData[k] = v
	}

	outbound := &driver.OutboundMessage{
		Subject: rendered.Subject,
		HTML:    rendered.HTML,
		Text:    rendered.Text,
		Title:   rendered.Title,
		Data:    driverData,
	}

	// Set From fields from scoped config or provider settings
	if resolved.Config != nil {
		outbound.From = resolved.Config.FromEmail
		outbound.FromName = resolved.Config.FromName
		if channel == "sms" {
			outbound.From = resolved.Config.FromPhone
		}
	}
	if outbound.From == "" {
		outbound.From = prov.Settings["from"]
	}
	if outbound.FromName == "" {
		outbound.FromName = prov.Settings["from_name"]
	}

	// Create message log entry
	now := time.Now().UTC()
	results := make([]*SendResult, 0, len(req.To))

	for _, recipient := range req.To {
		msg := &message.Message{
			ID:         id.NewMessageID(),
			AppID:      req.AppID,
			EnvID:      req.EnvID,
			TemplateID: req.Template,
			ProviderID: prov.ID.String(),
			Channel:    channel,
			Recipient:  recipient,
			Subject:    rendered.Subject,
			Body:       truncate(rendered.Text, h.config.TruncateBodyAt),
			Status:     message.StatusSending,
			Metadata:   req.Metadata,
			Async:      req.Async,
			Attempts:   1,
			CreatedAt:  now,
		}

		_ = h.store.CreateMessage(ctx, msg) //nolint:errcheck // best-effort delivery log

		// Send via driver
		outbound.To = recipient
		result, sendErr := drv.Send(ctx, outbound)

		if sendErr != nil {
			msg.Status = message.StatusFailed
			msg.Error = sendErr.Error()
			_ = h.store.UpdateMessageStatus(ctx, msg.ID, message.StatusFailed, sendErr.Error()) //nolint:errcheck // best-effort status update

			results = append(results, &SendResult{
				MessageID:  msg.ID,
				Status:     message.StatusFailed,
				ProviderID: prov.ID.String(),
				Error:      sendErr.Error(),
			})
			continue
		}

		sentAt := time.Now().UTC()
		msg.Status = message.StatusSent
		msg.SentAt = &sentAt
		_ = h.store.UpdateMessageStatus(ctx, msg.ID, message.StatusSent, "") //nolint:errcheck // best-effort status update

		// For in-app channel, also create inbox entry
		if channel == string(ChannelInApp) && req.UserID != "" {
			_ = h.store.CreateNotification(ctx, &inbox.Notification{ //nolint:errcheck // best-effort inbox entry
				ID:        id.NewInboxID(),
				AppID:     req.AppID,
				EnvID:     req.EnvID,
				UserID:    req.UserID,
				Type:      req.Template,
				Title:     rendered.Title,
				Body:      rendered.Text,
				Metadata:  req.Metadata,
				CreatedAt: now,
			})
		}

		sr := &SendResult{
			MessageID:  msg.ID,
			Status:     message.StatusSent,
			ProviderID: prov.ID.String(),
		}
		if result != nil && result.ProviderMessageID != "" {
			sr.ProviderID = result.ProviderMessageID
		}
		results = append(results, sr)
	}

	if len(results) == 0 {
		return &SendResult{Status: message.StatusFailed, Error: "no recipients"}, nil
	}

	// Audit the send operation.
	r := results[0]
	outcome := bridge.OutcomeSuccess
	if r.Status == message.StatusFailed {
		outcome = bridge.OutcomeFailure
	}
	h.Audit(ctx, bridge.SeverityInfo, outcome, "notification.send", "message", r.MessageID.String(), req.UserID, req.AppID, "notification", map[string]string{
		"channel":  channel,
		"provider": prov.ID.String(),
		"template": req.Template,
		"status":   string(r.Status),
	})

	return results[0], nil
}

// Notify sends a notification across multiple channels using a template.
func (h *Herald) Notify(ctx context.Context, req *NotifyRequest) ([]*SendResult, error) {
	var results []*SendResult

	for _, ch := range req.Channels {
		sendReq := &SendRequest{
			AppID:    req.AppID,
			EnvID:    req.EnvID,
			OrgID:    req.OrgID,
			UserID:   req.UserID,
			Channel:  ch,
			Template: req.Template,
			Locale:   req.Locale,
			To:       req.To,
			Data:     req.Data,
			Async:    req.Async,
			Metadata: req.Metadata,
		}

		result, err := h.Send(ctx, sendReq)
		if err != nil {
			h.logger.Warn("herald: notify channel failed",
				"channel", ch,
				"template", req.Template,
				"error", err,
			)
			results = append(results, &SendResult{
				Status: message.StatusFailed,
				Error:  err.Error(),
			})
			continue
		}

		results = append(results, result)
	}

	// Audit the multi-channel notify.
	outcome := bridge.OutcomeSuccess
	for _, r := range results {
		if r.Status == message.StatusFailed {
			outcome = bridge.OutcomeFailure
			break
		}
	}
	h.Audit(ctx, bridge.SeverityInfo, outcome, "notification.notify", "message", "", req.UserID, req.AppID, "notification", map[string]string{
		"template":       req.Template,
		"channels_count": fmt.Sprintf("%d", len(req.Channels)),
		"results_count":  fmt.Sprintf("%d", len(results)),
	})

	return results, nil
}

// SeedDefaultTemplates creates default notification templates for an app.
func (h *Herald) SeedDefaultTemplates(ctx context.Context, appID string) error {
	defaults := template.DefaultTemplates(appID)
	now := time.Now()

	for _, tmpl := range defaults {
		existing, _ := h.store.GetTemplateBySlug(ctx, appID, tmpl.Slug, tmpl.Channel) //nolint:errcheck // skip if lookup fails
		if existing != nil {
			continue // already seeded
		}

		tmpl.CreatedAt = now
		tmpl.UpdatedAt = now

		if err := h.store.CreateTemplate(ctx, tmpl); err != nil {
			h.logger.Warn("herald: failed to seed template",
				"slug", tmpl.Slug,
				"channel", tmpl.Channel,
				"error", err,
			)
			continue
		}

		for i := range tmpl.Versions {
			v := &tmpl.Versions[i]
			v.TemplateID = tmpl.ID
			v.CreatedAt = now
			v.UpdatedAt = now
			if err := h.store.CreateVersion(ctx, v); err != nil {
				h.logger.Warn("herald: failed to seed template version",
					"slug", tmpl.Slug,
					"locale", v.Locale,
					"error", err,
				)
			}
		}
	}

	return nil
}

// SeedDefaultProviders creates default providers for built-in zero-config
// drivers (e.g. inapp). This ensures channels work out of the box without
// requiring manual provider setup via the dashboard. Providers that need
// credentials (email, sms, push) are not seeded.
func (h *Herald) SeedDefaultProviders(ctx context.Context, appID string) error {
	// zeroConfigDrivers are drivers that work without credentials.
	zeroConfigDrivers := map[string]bool{
		"inapp": true,
	}

	for _, name := range h.drivers.Names() {
		if !zeroConfigDrivers[name] {
			continue
		}

		drv, err := h.drivers.Get(name)
		if err != nil {
			continue
		}

		channel := drv.Channel()

		existing, _ := h.store.ListProviders(ctx, appID, channel) //nolint:errcheck // skip if lookup fails
		if len(existing) > 0 {
			continue // already has a provider for this channel
		}

		now := time.Now()
		p := &provider.Provider{
			ID:        id.NewProviderID(),
			AppID:     appID,
			Name:      name + " (default)",
			Channel:   channel,
			Driver:    name,
			Priority:  0,
			Enabled:   true,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := h.store.CreateProvider(ctx, p); err != nil {
			h.logger.Warn("herald: failed to seed default provider",
				"channel", channel,
				"driver", name,
				"error", err,
			)
			continue
		}

		h.logger.Info("herald: seeded default provider",
			"channel", channel,
			"driver", name,
			"provider_id", p.ID.String(),
		)
	}

	return nil
}

// ResetDefaultTemplates deletes all system templates for an app and re-seeds
// the factory defaults. Custom (non-system) templates are preserved.
func (h *Herald) ResetDefaultTemplates(ctx context.Context, appID string) error {
	templates, err := h.store.ListTemplates(ctx, appID)
	if err != nil {
		return fmt.Errorf("herald: list templates for reset: %w", err)
	}

	for _, t := range templates {
		if !t.IsSystem {
			continue
		}
		// Delete versions first, then the template.
		versions, _ := h.store.ListVersions(ctx, t.ID) //nolint:errcheck // best-effort cleanup
		for _, v := range versions {
			_ = h.store.DeleteVersion(ctx, v.ID) //nolint:errcheck // best-effort cleanup
		}
		if err := h.store.DeleteTemplate(ctx, t.ID); err != nil {
			h.logger.Warn("herald: failed to delete system template during reset",
				"slug", t.Slug,
				"channel", t.Channel,
				"error", err,
			)
		}
	}

	return h.SeedDefaultTemplates(ctx, appID)
}

// Start is a no-op for Herald (no background workers needed currently).
// This exists for interface compatibility with Forge extensions.
func (h *Herald) Start(_ context.Context) {}

// Stop is a no-op for Herald.
func (h *Herald) Stop(_ context.Context) {}

// Health checks the health of Herald by pinging its store.
func (h *Herald) Health(ctx context.Context) error {
	return h.store.Ping(ctx)
}

// Audit records an audit event if a Chronicle backend is configured.
// This is used by both core Herald operations and the API layer.
func (h *Herald) Audit(ctx context.Context, severity, outcome, action, resource, resourceID, actorID, tenant, category string, metadata map[string]string) {
	if h.chronicle == nil {
		return
	}
	if err := h.chronicle.Record(ctx, &bridge.AuditEvent{
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		ActorID:    actorID,
		Tenant:     tenant,
		Outcome:    outcome,
		Severity:   severity,
		Category:   category,
		Metadata:   metadata,
	}); err != nil {
		h.logger.Warn("herald: audit record failed",
			"action", action,
			"error", err,
		)
	}
}

// truncate shortens a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
