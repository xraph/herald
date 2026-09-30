package store

import (
	"errors"

	"github.com/xraph/herald/internal/storeerr"
)

// Sentinel errors every Store implementation returns. The root herald package
// re-exports them, so callers can match either name with errors.Is.
var (
	ErrProviderNotFound     = storeerr.ErrProviderNotFound
	ErrTemplateNotFound     = errors.New("herald: template not found")
	ErrVersionNotFound      = errors.New("herald: template version not found")
	ErrMessageNotFound      = errors.New("herald: message not found")
	ErrNotificationNotFound = errors.New("herald: in-app notification not found")
	ErrPreferenceNotFound   = errors.New("herald: user preference not found")
	ErrScopedConfigNotFound = storeerr.ErrScopedConfigNotFound

	// ErrDuplicateSlug is returned when (app, slug, channel) already exists.
	ErrDuplicateSlug = errors.New("herald: duplicate template slug")
	// ErrDuplicateLocale is returned when (template, locale) already exists.
	ErrDuplicateLocale = errors.New("herald: duplicate locale version")
)
