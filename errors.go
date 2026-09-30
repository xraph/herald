package herald

import (
	"errors"

	"github.com/xraph/herald/store"
	"github.com/xraph/herald/template"
)

// Sentinel errors returned by Herald operations.
var (
	// ErrNoStore is returned when a Herald instance is created without a store.
	ErrNoStore = errors.New("herald: store is required")

	// ErrProviderNotFound is returned when a provider cannot be found.
	ErrProviderNotFound = store.ErrProviderNotFound

	// ErrProviderDisabled is returned when the resolved provider is disabled.
	ErrProviderDisabled = errors.New("herald: provider is disabled")

	// ErrNoProviderConfigured is returned when no provider is configured for the requested channel.
	ErrNoProviderConfigured = errors.New("herald: no provider configured for channel")

	// ErrDriverNotFound is returned when a driver is not registered.
	ErrDriverNotFound = errors.New("herald: driver not found")

	// ErrTemplateNotFound is returned when a template cannot be found.
	ErrTemplateNotFound = store.ErrTemplateNotFound

	// ErrVersionNotFound is returned when a template version cannot be found.
	ErrVersionNotFound = store.ErrVersionNotFound

	// ErrTemplateDisabled is returned when the resolved template is disabled.
	ErrTemplateDisabled = errors.New("herald: template is disabled")

	// ErrNoVersionForLocale is returned when no template version matches the requested locale.
	ErrNoVersionForLocale = template.ErrNoVersionForLocale

	// ErrTemplateRenderFailed is returned when template rendering fails.
	ErrTemplateRenderFailed = template.ErrTemplateRenderFailed

	// ErrMissingRequiredVariable is returned when a required template variable is not provided.
	ErrMissingRequiredVariable = template.ErrMissingRequiredVariable

	// ErrMessageNotFound is returned when a message cannot be found.
	ErrMessageNotFound = store.ErrMessageNotFound

	// ErrInboxNotFound is returned when an in-app notification cannot be found.
	ErrInboxNotFound = store.ErrNotificationNotFound

	// ErrPreferenceNotFound is returned when user preferences cannot be found.
	ErrPreferenceNotFound = store.ErrPreferenceNotFound

	// ErrScopedConfigNotFound is returned when no scoped config is found.
	ErrScopedConfigNotFound = store.ErrScopedConfigNotFound

	// ErrInvalidChannel is returned when an unsupported channel type is specified.
	ErrInvalidChannel = errors.New("herald: invalid channel type")

	// ErrSendFailed is returned when notification delivery fails.
	ErrSendFailed = errors.New("herald: send failed")

	// ErrOptedOut is returned when the user has opted out of this notification type.
	ErrOptedOut = errors.New("herald: user opted out")

	// ErrStoreClosed is returned when a store operation is attempted after close.
	ErrStoreClosed = errors.New("herald: store is closed")

	// ErrMigrationFailed is returned when a database migration fails.
	ErrMigrationFailed = errors.New("herald: migration failed")

	// ErrDuplicateSlug is returned when a template slug+channel+app combination already exists.
	ErrDuplicateSlug = store.ErrDuplicateSlug

	// ErrDuplicateLocale is returned when a template version for the same locale already exists.
	ErrDuplicateLocale = store.ErrDuplicateLocale
)
