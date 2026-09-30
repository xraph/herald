package store

import "errors"

// Sentinel errors every Store implementation returns. The root herald package
// re-exports them, so callers can match either name with errors.Is.
var (
	ErrProviderNotFound     = errors.New("herald: provider not found")
	ErrTemplateNotFound     = errors.New("herald: template not found")
	ErrVersionNotFound      = errors.New("herald: template version not found")
	ErrMessageNotFound      = errors.New("herald: message not found")
	ErrNotificationNotFound = errors.New("herald: in-app notification not found")
	ErrPreferenceNotFound   = errors.New("herald: user preference not found")
	ErrScopedConfigNotFound = errors.New("herald: scoped config not found")

	// ErrDuplicateSlug is returned when (app, slug, channel) already exists.
	ErrDuplicateSlug = errors.New("herald: duplicate template slug")
	// ErrDuplicateLocale is returned when (template, locale) already exists.
	ErrDuplicateLocale = errors.New("herald: duplicate locale version")
)
