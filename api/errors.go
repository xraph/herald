package api

import (
	"errors"
	"net/http"

	"github.com/xraph/forge"

	"github.com/xraph/herald"
	"github.com/xraph/herald/store"
)

// mapError turns a domain error into its HTTP status. Only errors with no
// domain meaning become a 500.
func mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case isAny(err, store.ErrProviderNotFound, store.ErrTemplateNotFound, store.ErrVersionNotFound,
		store.ErrMessageNotFound, store.ErrNotificationNotFound, store.ErrPreferenceNotFound, store.ErrScopedConfigNotFound):
		return forge.NotFound(err.Error())
	case isAny(err, store.ErrDuplicateSlug, store.ErrDuplicateLocale):
		return forge.NewHTTPError(http.StatusConflict, err.Error())
	case isAny(err, herald.ErrInvalidProvider, herald.ErrInvalidChannel, herald.ErrDriverNotFound,
		herald.ErrNoProviderConfigured, herald.ErrTemplateDisabled, herald.ErrNoVersionForLocale,
		herald.ErrMissingRequiredVariable, herald.ErrTemplateRenderFailed,
		herald.ErrNoCredentialKey, herald.ErrCredentialKeyUnavailable):
		return forge.BadRequest(err.Error())
	default:
		return forge.InternalError(err)
	}
}

func isAny(err error, targets ...error) bool {
	for _, t := range targets {
		if errors.Is(err, t) {
			return true
		}
	}
	return false
}
