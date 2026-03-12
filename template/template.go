// Package template defines the notification template entity with i18n support.
package template

import (
	"time"

	"github.com/xraph/herald/id"
)

// Template represents a notification template with locale-specific versions.
type Template struct {
	ID        id.TemplateID `json:"id"`
	AppID     string        `json:"app_id"`
	Slug      string        `json:"slug"`
	Name      string        `json:"name"`
	Channel   string        `json:"channel"`
	Category  string        `json:"category"`
	Variables []Variable    `json:"variables,omitempty"`
	Versions  []Version     `json:"versions,omitempty"`
	IsSystem  bool          `json:"is_system"`
	Enabled   bool          `json:"enabled"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// Version represents a locale-specific content version of a template.
type Version struct {
	ID         id.TemplateVersionID `json:"id"`
	TemplateID id.TemplateID        `json:"template_id"`
	Locale     string               `json:"locale"`
	Subject    string               `json:"subject,omitempty"`
	HTML       string               `json:"html,omitempty"`
	Text       string               `json:"text,omitempty"`
	Title      string               `json:"title,omitempty"`
	Active     bool                 `json:"active"`
	CreatedAt  time.Time            `json:"created_at"`
	UpdatedAt  time.Time            `json:"updated_at"`
}

// Variable describes an expected template variable.
type Variable struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

// Category constants for template organization.
const (
	CategoryAuth          = "auth"
	CategoryTransactional = "transactional"
	CategoryMarketing     = "marketing"
	CategorySystem        = "system"
)
