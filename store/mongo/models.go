package mongo

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

	ID          string            `grove:"id,pk"       bson:"_id"`
	AppID       string            `grove:"app_id"      bson:"app_id"`
	Name        string            `grove:"name"        bson:"name"`
	Channel     string            `grove:"channel"     bson:"channel"`
	Driver      string            `grove:"driver"      bson:"driver"`
	Credentials map[string]string `grove:"credentials" bson:"credentials,omitempty"`
	Settings    map[string]string `grove:"settings"    bson:"settings,omitempty"`
	Priority    int               `grove:"priority"    bson:"priority"`
	Enabled     bool              `grove:"enabled"     bson:"enabled"`
	CreatedAt   time.Time         `grove:"created_at"  bson:"created_at"`
	UpdatedAt   time.Time         `grove:"updated_at"  bson:"updated_at"`
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

	ID        string          `grove:"id,pk"       bson:"_id"`
	AppID     string          `grove:"app_id"      bson:"app_id"`
	Slug      string          `grove:"slug"        bson:"slug"`
	Name      string          `grove:"name"        bson:"name"`
	Channel   string          `grove:"channel"     bson:"channel"`
	Category  string          `grove:"category"    bson:"category"`
	Variables json.RawMessage `grove:"variables"   bson:"variables,omitempty"`
	IsSystem  bool            `grove:"is_system"   bson:"is_system"`
	Enabled   bool            `grove:"enabled"     bson:"enabled"`
	CreatedAt time.Time       `grove:"created_at"  bson:"created_at"`
	UpdatedAt time.Time       `grove:"updated_at"  bson:"updated_at"`
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

	ID         string    `grove:"id,pk"        bson:"_id"`
	TemplateID string    `grove:"template_id"  bson:"template_id"`
	Locale     string    `grove:"locale"       bson:"locale"`
	Subject    string    `grove:"subject"      bson:"subject"`
	HTML       string    `grove:"html"         bson:"html"`
	Text       string    `grove:"text"         bson:"text"`
	Title      string    `grove:"title"        bson:"title"`
	Active     bool      `grove:"active"       bson:"active"`
	CreatedAt  time.Time `grove:"created_at"   bson:"created_at"`
	UpdatedAt  time.Time `grove:"updated_at"   bson:"updated_at"`
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

	ID          string            `grove:"id,pk"        bson:"_id"`
	AppID       string            `grove:"app_id"       bson:"app_id"`
	EnvID       string            `grove:"env_id"       bson:"env_id"`
	TemplateID  string            `grove:"template_id"  bson:"template_id"`
	ProviderID  string            `grove:"provider_id"  bson:"provider_id"`
	Channel     string            `grove:"channel"      bson:"channel"`
	Recipient   string            `grove:"recipient"    bson:"recipient"`
	Subject     string            `grove:"subject"      bson:"subject"`
	Body        string            `grove:"body"         bson:"body"`
	Status      string            `grove:"status"       bson:"status"`
	Error       string            `grove:"error"        bson:"error"`
	Metadata    map[string]string `grove:"metadata"     bson:"metadata,omitempty"`
	Async       bool              `grove:"async"        bson:"async"`
	Attempts    int               `grove:"attempts"     bson:"attempts"`
	SentAt      *time.Time        `grove:"sent_at"      bson:"sent_at,omitempty"`
	DeliveredAt *time.Time        `grove:"delivered_at" bson:"delivered_at,omitempty"`
	CreatedAt   time.Time         `grove:"created_at"   bson:"created_at"`
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

	ID        string            `grove:"id,pk"       bson:"_id"`
	AppID     string            `grove:"app_id"      bson:"app_id"`
	EnvID     string            `grove:"env_id"      bson:"env_id"`
	UserID    string            `grove:"user_id"     bson:"user_id"`
	Type      string            `grove:"type"        bson:"type"`
	Title     string            `grove:"title"       bson:"title"`
	Body      string            `grove:"body"        bson:"body"`
	ActionURL string            `grove:"action_url"  bson:"action_url"`
	ImageURL  string            `grove:"image_url"   bson:"image_url"`
	Read      bool              `grove:"read"        bson:"read"`
	ReadAt    *time.Time        `grove:"read_at"     bson:"read_at,omitempty"`
	Metadata  map[string]string `grove:"metadata"    bson:"metadata,omitempty"`
	ExpiresAt *time.Time        `grove:"expires_at"  bson:"expires_at,omitempty"`
	CreatedAt time.Time         `grove:"created_at"  bson:"created_at"`
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

	ID        string          `grove:"id,pk"       bson:"_id"`
	AppID     string          `grove:"app_id"      bson:"app_id"`
	UserID    string          `grove:"user_id"     bson:"user_id"`
	Overrides json.RawMessage `grove:"overrides"   bson:"overrides,omitempty"`
	CreatedAt time.Time       `grove:"created_at"  bson:"created_at"`
	UpdatedAt time.Time       `grove:"updated_at"  bson:"updated_at"`
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

	ID                string    `grove:"id,pk"              bson:"_id"`
	AppID             string    `grove:"app_id"             bson:"app_id"`
	Scope             string    `grove:"scope"              bson:"scope"`
	ScopeID           string    `grove:"scope_id"           bson:"scope_id"`
	EmailProviderID   string    `grove:"email_provider_id"  bson:"email_provider_id"`
	SMSProviderID     string    `grove:"sms_provider_id"    bson:"sms_provider_id"`
	PushProviderID    string    `grove:"push_provider_id"      bson:"push_provider_id"`
	WebhookProviderID string    `grove:"webhook_provider_id"   bson:"webhook_provider_id"`
	ChatProviderID    string    `grove:"chat_provider_id"      bson:"chat_provider_id"`
	FromEmail         string    `grove:"from_email"            bson:"from_email"`
	FromName          string    `grove:"from_name"          bson:"from_name"`
	FromPhone         string    `grove:"from_phone"         bson:"from_phone"`
	DefaultLocale     string    `grove:"default_locale"     bson:"default_locale"`
	CreatedAt         time.Time `grove:"created_at"         bson:"created_at"`
	UpdatedAt         time.Time `grove:"updated_at"         bson:"updated_at"`
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
