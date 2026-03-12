package postgres

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/xraph/grove"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/inbox"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/preference"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/template"
)

// --- Provider models ---

type providerModel struct {
	grove.BaseModel `grove:"table:herald_providers"`

	ID          string            `grove:"id,pk"`
	AppID       string            `grove:"app_id"`
	Name        string            `grove:"name"`
	Channel     string            `grove:"channel"`
	Driver      string            `grove:"driver"`
	Credentials map[string]string `grove:"credentials,type:jsonb"`
	Settings    map[string]string `grove:"settings,type:jsonb"`
	Priority    int               `grove:"priority"`
	Enabled     bool              `grove:"enabled"`
	CreatedAt   time.Time         `grove:"created_at"`
	UpdatedAt   time.Time         `grove:"updated_at"`
}

func toProviderModel(p *provider.Provider) *providerModel {
	return &providerModel{
		ID:          p.ID.String(),
		AppID:       p.AppID,
		Name:        p.Name,
		Channel:     p.Channel,
		Driver:      p.Driver,
		Credentials: p.Credentials,
		Settings:    p.Settings,
		Priority:    p.Priority,
		Enabled:     p.Enabled,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func fromProviderModel(m *providerModel) (*provider.Provider, error) {
	pid, err := id.ParseProviderID(m.ID)
	if err != nil {
		return nil, fmt.Errorf("parse provider ID %q: %w", m.ID, err)
	}
	return &provider.Provider{
		ID:          pid,
		AppID:       m.AppID,
		Name:        m.Name,
		Channel:     m.Channel,
		Driver:      m.Driver,
		Credentials: m.Credentials,
		Settings:    m.Settings,
		Priority:    m.Priority,
		Enabled:     m.Enabled,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}, nil
}

// --- Template models ---

type templateModel struct {
	grove.BaseModel `grove:"table:herald_templates"`

	ID        string          `grove:"id,pk"`
	AppID     string          `grove:"app_id"`
	Slug      string          `grove:"slug"`
	Name      string          `grove:"name"`
	Channel   string          `grove:"channel"`
	Category  string          `grove:"category"`
	Variables json.RawMessage `grove:"variables,type:jsonb"`
	IsSystem  bool            `grove:"is_system"`
	Enabled   bool            `grove:"enabled"`
	CreatedAt time.Time       `grove:"created_at"`
	UpdatedAt time.Time       `grove:"updated_at"`
}

func toTemplateModel(t *template.Template) *templateModel {
	vars, _ := json.Marshal(t.Variables) //nolint:errcheck // best-effort
	return &templateModel{
		ID:        t.ID.String(),
		AppID:     t.AppID,
		Slug:      t.Slug,
		Name:      t.Name,
		Channel:   t.Channel,
		Category:  t.Category,
		Variables: vars,
		IsSystem:  t.IsSystem,
		Enabled:   t.Enabled,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

func fromTemplateModel(m *templateModel) (*template.Template, error) {
	tid, err := id.ParseTemplateID(m.ID)
	if err != nil {
		return nil, fmt.Errorf("parse template ID %q: %w", m.ID, err)
	}
	var vars []template.Variable
	if len(m.Variables) > 0 {
		_ = json.Unmarshal(m.Variables, &vars) //nolint:errcheck // best-effort
	}
	return &template.Template{
		ID:        tid,
		AppID:     m.AppID,
		Slug:      m.Slug,
		Name:      m.Name,
		Channel:   m.Channel,
		Category:  m.Category,
		Variables: vars,
		IsSystem:  m.IsSystem,
		Enabled:   m.Enabled,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}, nil
}

// --- Template Version models ---

type templateVersionModel struct {
	grove.BaseModel `grove:"table:herald_template_versions"`

	ID         string    `grove:"id,pk"`
	TemplateID string    `grove:"template_id"`
	Locale     string    `grove:"locale"`
	Subject    string    `grove:"subject"`
	HTML       string    `grove:"html"`
	Text       string    `grove:"text"`
	Title      string    `grove:"title"`
	Active     bool      `grove:"active"`
	CreatedAt  time.Time `grove:"created_at"`
	UpdatedAt  time.Time `grove:"updated_at"`
}

func toVersionModel(v *template.Version) *templateVersionModel {
	return &templateVersionModel{
		ID:         v.ID.String(),
		TemplateID: v.TemplateID.String(),
		Locale:     v.Locale,
		Subject:    v.Subject,
		HTML:       v.HTML,
		Text:       v.Text,
		Title:      v.Title,
		Active:     v.Active,
		CreatedAt:  v.CreatedAt,
		UpdatedAt:  v.UpdatedAt,
	}
}

func fromVersionModel(m *templateVersionModel) (*template.Version, error) {
	vid, err := id.ParseTemplateVersionID(m.ID)
	if err != nil {
		return nil, fmt.Errorf("parse template version ID %q: %w", m.ID, err)
	}
	tid, err := id.ParseTemplateID(m.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("parse template ID %q: %w", m.TemplateID, err)
	}
	return &template.Version{
		ID:         vid,
		TemplateID: tid,
		Locale:     m.Locale,
		Subject:    m.Subject,
		HTML:       m.HTML,
		Text:       m.Text,
		Title:      m.Title,
		Active:     m.Active,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}, nil
}

// --- Message models ---

type messageModel struct {
	grove.BaseModel `grove:"table:herald_messages"`

	ID          string            `grove:"id,pk"`
	AppID       string            `grove:"app_id"`
	EnvID       string            `grove:"env_id"`
	TemplateID  string            `grove:"template_id"`
	ProviderID  string            `grove:"provider_id"`
	Channel     string            `grove:"channel"`
	Recipient   string            `grove:"recipient"`
	Subject     string            `grove:"subject"`
	Body        string            `grove:"body"`
	Status      string            `grove:"status"`
	Error       string            `grove:"error"`
	Metadata    map[string]string `grove:"metadata,type:jsonb"`
	Async       bool              `grove:"async"`
	Attempts    int               `grove:"attempts"`
	SentAt      *time.Time        `grove:"sent_at"`
	DeliveredAt *time.Time        `grove:"delivered_at"`
	CreatedAt   time.Time         `grove:"created_at"`
}

func toMessageModel(m *message.Message) *messageModel {
	return &messageModel{
		ID:          m.ID.String(),
		AppID:       m.AppID,
		EnvID:       m.EnvID,
		TemplateID:  m.TemplateID,
		ProviderID:  m.ProviderID,
		Channel:     m.Channel,
		Recipient:   m.Recipient,
		Subject:     m.Subject,
		Body:        m.Body,
		Status:      string(m.Status),
		Error:       m.Error,
		Metadata:    m.Metadata,
		Async:       m.Async,
		Attempts:    m.Attempts,
		SentAt:      m.SentAt,
		DeliveredAt: m.DeliveredAt,
		CreatedAt:   m.CreatedAt,
	}
}

func fromMessageModel(m *messageModel) (*message.Message, error) {
	mid, err := id.ParseMessageID(m.ID)
	if err != nil {
		return nil, fmt.Errorf("parse message ID %q: %w", m.ID, err)
	}
	return &message.Message{
		ID:          mid,
		AppID:       m.AppID,
		EnvID:       m.EnvID,
		TemplateID:  m.TemplateID,
		ProviderID:  m.ProviderID,
		Channel:     m.Channel,
		Recipient:   m.Recipient,
		Subject:     m.Subject,
		Body:        m.Body,
		Status:      message.Status(m.Status),
		Error:       m.Error,
		Metadata:    m.Metadata,
		Async:       m.Async,
		Attempts:    m.Attempts,
		SentAt:      m.SentAt,
		DeliveredAt: m.DeliveredAt,
		CreatedAt:   m.CreatedAt,
	}, nil
}

// --- Inbox Notification models ---

type notificationModel struct {
	grove.BaseModel `grove:"table:herald_inbox"`

	ID        string            `grove:"id,pk"`
	AppID     string            `grove:"app_id"`
	EnvID     string            `grove:"env_id"`
	UserID    string            `grove:"user_id"`
	Type      string            `grove:"type"`
	Title     string            `grove:"title"`
	Body      string            `grove:"body"`
	ActionURL string            `grove:"action_url"`
	ImageURL  string            `grove:"image_url"`
	Read      bool              `grove:"read"`
	ReadAt    *time.Time        `grove:"read_at"`
	Metadata  map[string]string `grove:"metadata,type:jsonb"`
	ExpiresAt *time.Time        `grove:"expires_at"`
	CreatedAt time.Time         `grove:"created_at"`
}

func toNotificationModel(n *inbox.Notification) *notificationModel {
	return &notificationModel{
		ID:        n.ID.String(),
		AppID:     n.AppID,
		EnvID:     n.EnvID,
		UserID:    n.UserID,
		Type:      n.Type,
		Title:     n.Title,
		Body:      n.Body,
		ActionURL: n.ActionURL,
		ImageURL:  n.ImageURL,
		Read:      n.Read,
		ReadAt:    n.ReadAt,
		Metadata:  n.Metadata,
		ExpiresAt: n.ExpiresAt,
		CreatedAt: n.CreatedAt,
	}
}

func fromNotificationModel(m *notificationModel) (*inbox.Notification, error) {
	nid, err := id.ParseInboxID(m.ID)
	if err != nil {
		return nil, fmt.Errorf("parse inbox ID %q: %w", m.ID, err)
	}
	return &inbox.Notification{
		ID:        nid,
		AppID:     m.AppID,
		EnvID:     m.EnvID,
		UserID:    m.UserID,
		Type:      m.Type,
		Title:     m.Title,
		Body:      m.Body,
		ActionURL: m.ActionURL,
		ImageURL:  m.ImageURL,
		Read:      m.Read,
		ReadAt:    m.ReadAt,
		Metadata:  m.Metadata,
		ExpiresAt: m.ExpiresAt,
		CreatedAt: m.CreatedAt,
	}, nil
}

// --- Preference models ---

type preferenceModel struct {
	grove.BaseModel `grove:"table:herald_preferences"`

	ID        string          `grove:"id,pk"`
	AppID     string          `grove:"app_id"`
	UserID    string          `grove:"user_id"`
	Overrides json.RawMessage `grove:"overrides,type:jsonb"`
	CreatedAt time.Time       `grove:"created_at"`
	UpdatedAt time.Time       `grove:"updated_at"`
}

func toPreferenceModel(p *preference.Preference) *preferenceModel {
	overrides, _ := json.Marshal(p.Overrides) //nolint:errcheck // best-effort
	return &preferenceModel{
		ID:        p.ID.String(),
		AppID:     p.AppID,
		UserID:    p.UserID,
		Overrides: overrides,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

func fromPreferenceModel(m *preferenceModel) (*preference.Preference, error) {
	pid, err := id.ParsePreferenceID(m.ID)
	if err != nil {
		return nil, fmt.Errorf("parse preference ID %q: %w", m.ID, err)
	}
	var overrides map[string]preference.ChannelPreference
	if len(m.Overrides) > 0 {
		_ = json.Unmarshal(m.Overrides, &overrides) //nolint:errcheck // best-effort
	}
	return &preference.Preference{
		ID:        pid,
		AppID:     m.AppID,
		UserID:    m.UserID,
		Overrides: overrides,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}, nil
}

// --- ScopedConfig models ---

type scopedConfigModel struct {
	grove.BaseModel `grove:"table:herald_scoped_configs"`

	ID                string    `grove:"id,pk"`
	AppID             string    `grove:"app_id"`
	Scope             string    `grove:"scope"`
	ScopeID           string    `grove:"scope_id"`
	EmailProviderID   string    `grove:"email_provider_id"`
	SMSProviderID     string    `grove:"sms_provider_id"`
	PushProviderID    string    `grove:"push_provider_id"`
	WebhookProviderID string    `grove:"webhook_provider_id"`
	ChatProviderID    string    `grove:"chat_provider_id"`
	FromEmail         string    `grove:"from_email"`
	FromName          string    `grove:"from_name"`
	FromPhone         string    `grove:"from_phone"`
	DefaultLocale     string    `grove:"default_locale"`
	CreatedAt         time.Time `grove:"created_at"`
	UpdatedAt         time.Time `grove:"updated_at"`
}

func toScopedConfigModel(c *scope.Config) *scopedConfigModel {
	return &scopedConfigModel{
		ID:                c.ID.String(),
		AppID:             c.AppID,
		Scope:             string(c.Scope),
		ScopeID:           c.ScopeID,
		EmailProviderID:   c.EmailProviderID,
		SMSProviderID:     c.SMSProviderID,
		PushProviderID:    c.PushProviderID,
		WebhookProviderID: c.WebhookProviderID,
		ChatProviderID:    c.ChatProviderID,
		FromEmail:         c.FromEmail,
		FromName:          c.FromName,
		FromPhone:         c.FromPhone,
		DefaultLocale:     c.DefaultLocale,
		CreatedAt:         c.CreatedAt,
		UpdatedAt:         c.UpdatedAt,
	}
}

func fromScopedConfigModel(m *scopedConfigModel) (*scope.Config, error) {
	cid, err := id.ParseScopedConfigID(m.ID)
	if err != nil {
		return nil, fmt.Errorf("parse scoped config ID %q: %w", m.ID, err)
	}
	return &scope.Config{
		ID:                cid,
		AppID:             m.AppID,
		Scope:             scope.ScopeType(m.Scope),
		ScopeID:           m.ScopeID,
		EmailProviderID:   m.EmailProviderID,
		SMSProviderID:     m.SMSProviderID,
		PushProviderID:    m.PushProviderID,
		WebhookProviderID: m.WebhookProviderID,
		ChatProviderID:    m.ChatProviderID,
		FromEmail:         m.FromEmail,
		FromName:          m.FromName,
		FromPhone:         m.FromPhone,
		DefaultLocale:     m.DefaultLocale,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}, nil
}
