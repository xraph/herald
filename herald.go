// Package herald provides a unified, multi-channel notification delivery engine.
//
// Herald supports email, SMS, push notifications, and in-app notifications
// with pluggable provider drivers, a template system with i18n support,
// and scoped configuration overrides (app → org → user).
package herald

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/xraph/herald/bridge"
	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/inbox"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/scope"
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
	// ProviderID sends through this provider instead of resolving one. It must belong to AppID and handle Channel; a disabled provider is allowed, because naming it is the explicit act.
	ProviderID string `json:"provider_id,omitempty"`
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
	MessageID id.MessageID   `json:"message_id"`
	Status    message.Status `json:"status"`
	// ProviderID is always Herald's provider ID.
	ProviderID string `json:"provider_id,omitempty"`
	// ProviderMessageID is the vendor's ID for the message, when it gave one.
	ProviderMessageID string `json:"provider_message_id,omitempty"`
	Error             string `json:"error,omitempty"`
	// Logged is false when the message log couldn't be written. The send
	// still happened; it just won't appear in the log.
	Logged bool `json:"logged"`
}

// Send delivers a notification on a single channel.
func (h *Herald) Send(ctx context.Context, req *SendRequest) (*SendResult, error) {
	if req.UserID != "" && req.Template != "" {
		pref, _ := h.store.GetPreference(ctx, req.AppID, req.UserID) //nolint:errcheck // no preference means opted in
		if pref != nil && pref.IsOptedOut(req.Template, req.Channel) {
			return h.suppress(ctx, req), nil
		}
	}

	rendered, err := h.renderRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resolved, err := h.resolveForSend(ctx, req)
	if err != nil {
		return nil, err
	}
	prov := resolved.Provider

	drv, err := h.driverFor(prov)
	if err != nil {
		return nil, err
	}

	// A decryption failure is recorded on every message row below rather than
	// returned, so the log says why nothing was delivered.
	data, dataErr := h.driverData(prov)
	outbound := &driver.OutboundMessage{
		Subject: rendered.Subject,
		HTML:    rendered.HTML,
		Text:    rendered.Text,
		Title:   rendered.Title,
		Data:    data,
	}
	applyFrom(outbound, resolved, req.Channel, prov)

	now := time.Now().UTC()
	results := make([]*SendResult, 0, len(req.To))
	for _, recipient := range req.To {
		msg := &message.Message{
			ID:         id.NewMessageID(),
			AppID:      req.AppID,
			EnvID:      req.EnvID,
			TemplateID: req.Template,
			ProviderID: prov.ID.String(),
			Channel:    req.Channel,
			Recipient:  recipient,
			Subject:    rendered.Subject,
			Body:       truncate(rendered.Text, h.config.TruncateBodyAt),
			Status:     message.StatusSending,
			Metadata:   req.Metadata,
			Async:      req.Async,
			Attempts:   1,
			CreatedAt:  now,
		}
		logged := h.logMessage(ctx, msg)

		var d message.Delivery
		if dataErr != nil {
			d = message.Delivery{Status: message.StatusFailed, Error: dataErr.Error()}
		} else {
			outbound.To = recipient
			result, sendErr := drv.Send(ctx, outbound)
			if sendErr != nil {
				d = message.Delivery{Status: message.StatusFailed, Error: sendErr.Error()}
			} else {
				sentAt := time.Now().UTC()
				d = message.Delivery{Status: message.StatusSent, SentAt: &sentAt}
				if result != nil {
					d.ProviderMessageID = result.ProviderMessageID
				}
			}
		}
		if logged {
			if err := h.store.RecordDelivery(ctx, msg.ID, d); err != nil {
				h.logger.Warn("herald: failed to record delivery", "message_id", msg.ID.String(), "error", err)
			}
		}

		if d.Status == message.StatusSent && req.Channel == string(ChannelInApp) && req.UserID != "" {
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

		results = append(results, &SendResult{
			MessageID:         msg.ID,
			Status:            d.Status,
			ProviderID:        prov.ID.String(),
			ProviderMessageID: d.ProviderMessageID,
			Error:             d.Error,
			Logged:            logged,
		})
	}

	if len(results) == 0 {
		return &SendResult{Status: message.StatusFailed, Error: "no recipients"}, nil
	}

	r := results[0]
	outcome := bridge.OutcomeSuccess
	if r.Status == message.StatusFailed {
		outcome = bridge.OutcomeFailure
	}
	h.Audit(ctx, bridge.SeverityInfo, outcome, "notification.send", "message", r.MessageID.String(), req.UserID, req.AppID, "notification", map[string]string{
		"channel":  req.Channel,
		"provider": prov.ID.String(),
		"via":      resolved.Via,
		"template": req.Template,
		"status":   string(r.Status),
	})
	return r, nil
}

// ResolveProvider reports which provider would send on channel for this
// app, org and user, and why. It returns nil, nil when nothing handles it.
func (h *Herald) ResolveProvider(ctx context.Context, appID, orgID, userID, channel string) (*scope.ResolveResult, error) {
	return h.resolver.ResolveProvider(ctx, appID, orgID, userID, channel)
}

// suppress records an opted-out send without calling any driver.
func (h *Herald) suppress(ctx context.Context, req *SendRequest) *SendResult {
	const reason = "user opted out"
	now := time.Now().UTC()
	var first *SendResult
	for _, recipient := range req.To {
		msg := &message.Message{
			ID:         id.NewMessageID(),
			AppID:      req.AppID,
			EnvID:      req.EnvID,
			TemplateID: req.Template,
			Channel:    req.Channel,
			Recipient:  recipient,
			Status:     message.StatusSuppressed,
			Error:      reason,
			Metadata:   req.Metadata,
			Async:      req.Async,
			CreatedAt:  now,
		}
		logged := h.logMessage(ctx, msg)
		if first == nil {
			first = &SendResult{MessageID: msg.ID, Status: message.StatusSuppressed, Error: reason, Logged: logged}
		}
	}
	if first == nil {
		first = &SendResult{Status: message.StatusSuppressed, Error: reason}
	}
	h.Audit(ctx, bridge.SeverityInfo, bridge.OutcomeSuccess, "notification.suppressed", "message", first.MessageID.String(), req.UserID, req.AppID, "notification", map[string]string{
		"channel": req.Channel, "template": req.Template,
	})
	return first
}

// logMessage writes the message row and reports whether it worked. A failed
// write never blocks the send.
func (h *Herald) logMessage(ctx context.Context, msg *message.Message) bool {
	if err := h.store.CreateMessage(ctx, msg); err != nil {
		h.logger.Warn("herald: could not write the message log; sending anyway",
			"channel", msg.Channel, "status", string(msg.Status), "error", err)
		return false
	}
	return true
}

// renderRequest renders the template, or uses the raw subject and body. A
// template is used only when Template is set and Body is empty.
func (h *Herald) renderRequest(ctx context.Context, req *SendRequest) (*template.RenderedContent, error) {
	if req.Template == "" || req.Body != "" {
		return &template.RenderedContent{Subject: req.Subject, Text: req.Body}, nil
	}
	tmpl, err := h.store.GetTemplateBySlug(ctx, req.AppID, req.Template, req.Channel)
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) {
			return nil, fmt.Errorf("%w: %s on %s", ErrTemplateNotFound, req.Template, req.Channel)
		}
		return nil, fmt.Errorf("herald: load template %q: %w", req.Template, err)
	}
	if !tmpl.Enabled {
		return nil, ErrTemplateDisabled
	}
	locale := req.Locale
	if locale == "" {
		locale = h.config.DefaultLocale
	}
	return h.renderer.Render(tmpl, locale, req.Data)
}

// resolveForSend returns the chosen provider when the request names one, and
// otherwise runs the scope resolver.
func (h *Herald) resolveForSend(ctx context.Context, req *SendRequest) (*scope.ResolveResult, error) {
	if req.ProviderID == "" {
		res, err := h.resolver.ResolveProvider(ctx, req.AppID, req.OrgID, req.UserID, req.Channel)
		if err != nil {
			return nil, err
		}
		if res == nil {
			return nil, fmt.Errorf("%w: channel=%s", ErrNoProviderConfigured, req.Channel)
		}
		return res, nil
	}
	pid, err := id.ParseProviderID(req.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrProviderNotFound, req.ProviderID)
	}
	p, err := h.store.GetProvider(ctx, pid)
	if err != nil {
		return nil, err
	}
	if p.AppID != req.AppID {
		return nil, ErrProviderNotFound
	}
	if p.Channel != req.Channel {
		return nil, fmt.Errorf("%w: provider %s sends %s, not %s", ErrInvalidChannel, p.ID, p.Channel, req.Channel)
	}
	cfg, _ := h.store.GetScopedConfig(ctx, req.AppID, scope.ScopeApp, req.AppID) //nolint:errcheck // no app rule means provider settings supply From
	return &scope.ResolveResult{Provider: p, Config: cfg, Via: scope.ViaChosen}, nil
}

// driverFor returns the registered driver for p, or ErrDriverNotFound. Send
// and PreviewSend both call it, so a preview refuses what a send would.
func (h *Herald) driverFor(p *provider.Provider) (driver.Driver, error) {
	drv, err := h.drivers.Get(p.Driver)
	if err != nil {
		return nil, fmt.Errorf("%w: driver=%s", ErrDriverNotFound, p.Driver)
	}
	return drv, nil
}

// driverData is the map a driver reads: credentials decrypted, then settings,
// with settings winning a key collision, which is how Send has always merged
// them. A credential encrypted under a key that isn't configured fails here.
func (h *Herald) driverData(p *provider.Provider) (map[string]string, error) {
	creds, err := h.open(p)
	if err != nil {
		return nil, err
	}
	data := make(map[string]string, len(creds)+len(p.Settings))
	maps.Copy(data, creds)
	maps.Copy(data, p.Settings)
	return data, nil
}

// applyFrom sets the sender from the routing rule, falling back to the
// provider's own from settings.
func applyFrom(out *driver.OutboundMessage, res *scope.ResolveResult, channel string, prov *provider.Provider) {
	if res.Config != nil {
		out.From = res.Config.FromEmail
		out.FromName = res.Config.FromName
		if channel == string(ChannelSMS) {
			out.From = res.Config.FromPhone
		}
	}
	if out.From == "" {
		out.From = prov.Settings["from"]
	}
	if out.FromName == "" {
		out.FromName = prov.Settings["from_name"]
	}
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

// SeedConfiguredProviders persists providers declared in configuration into
// the store, seed-if-absent: a provider is created only when no provider of
// the same name exists for its app. Existing records (including dashboard
// edits) are left untouched. A missing driver or failed Validate is logged as
// a warning, never an error.
func (h *Herald) SeedConfiguredProviders(ctx context.Context, providers []provider.Provider) error {
	for i := range providers {
		p := providers[i]

		existing, _ := h.store.ListAllProviders(ctx, p.AppID) //nolint:errcheck // skip lookup failures
		duplicate := false
		for _, e := range existing {
			if e.Name == p.Name {
				duplicate = true
				break
			}
		}
		if duplicate {
			h.logger.Info("herald: configured provider already exists, skipping",
				"name", p.Name, "app_id", p.AppID)
			continue
		}

		if vErr := h.ValidateProvider(&p); vErr != nil {
			h.logger.Warn("herald: configured provider failed validation; seeding it anyway",
				"name", p.Name, "driver", p.Driver, "error", vErr)
		}
		if p.ID.IsNil() {
			p.ID = id.NewProviderID() // the credential cipher binds to the ID
		}
		sealed, err := h.seal(p.ID.String(), p.Credentials, slices.Collect(maps.Keys(p.Credentials)))
		if err != nil {
			h.logger.Warn("herald: failed to encrypt configured provider credentials", "name", p.Name, "error", err)
			continue
		}
		p.Credentials = sealed

		if err := h.store.CreateProvider(ctx, &p); err != nil {
			h.logger.Warn("herald: failed to seed configured provider",
				"name", p.Name, "error", err)
			continue
		}
		h.logger.Info("herald: seeded configured provider",
			"name", p.Name, "channel", p.Channel, "driver", p.Driver, "provider_id", p.ID.String())
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

// truncate shortens s to at most maxLen bytes without splitting a UTF-8
// character. maxLen <= 0 means no limit.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	cut := maxLen
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
