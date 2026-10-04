package contract

import (
	"errors"

	"github.com/xraph/forge"
	"github.com/xraph/forge/extensions/dashboard/contract"

	"github.com/xraph/herald"
	"github.com/xraph/herald/credential"
	"github.com/xraph/herald/store"
)

// mapError translates a Herald error into a *contract.Error. An error with no
// domain meaning becomes INTERNAL with a fixed message: its own text never
// reaches the client, because a store or driver error can carry a connection
// string. The domain errors passed through as BAD_REQUEST below name keys and
// IDs only, never a credential value (Herald's hardening pins that).
func mapError(err error) error {
	if err == nil {
		return nil
	}
	var ce *contract.Error
	if errors.As(err, &ce) {
		return ce
	}
	switch {
	case errors.Is(err, store.ErrProviderNotFound):
		return notFound("provider not found")
	case errors.Is(err, store.ErrTemplateNotFound):
		return notFound("template not found")
	case errors.Is(err, store.ErrVersionNotFound):
		return notFound("template version not found")
	case errors.Is(err, store.ErrMessageNotFound):
		return notFound("message not found")
	case errors.Is(err, store.ErrNotificationNotFound):
		return notFound("notification not found")
	case errors.Is(err, store.ErrScopedConfigNotFound):
		return notFound("routing rule not found")
	case errors.Is(err, store.ErrDuplicateSlug):
		return conflict("a template with this slug already exists on this channel")
	case errors.Is(err, store.ErrDuplicateLocale):
		return conflict("this template already has a version for that locale")
	case errors.Is(err, herald.ErrCredentialKeyUnavailable):
		return &contract.Error{Code: contract.CodeUnavailable, Message: "this provider's credentials are encrypted under a key this server doesn't have"}
	case isAny(err, herald.ErrInvalidProvider, herald.ErrInvalidChannel, herald.ErrDriverNotFound,
		herald.ErrNoProviderConfigured, herald.ErrTemplateDisabled, herald.ErrNoVersionForLocale,
		herald.ErrMissingRequiredVariable, herald.ErrTemplateRenderFailed, herald.ErrNoCredentialKey,
		credential.ErrMalformed):
		return badRequest(err.Error())
	default:
		return &contract.Error{Code: contract.CodeInternal, Message: "an internal error occurred"}
	}
}

// mapError maps err and, when the result is INTERNAL and a logger is set,
// logs the underlying error with the intent that hit it, the one case an
// operator can't diagnose from what the client sees.
func (d Deps) mapError(intent string, err error) error {
	mapped := mapError(err)
	if mapped == nil || d.Logger == nil {
		return mapped
	}
	var ce *contract.Error
	if errors.As(mapped, &ce) && ce.Code == contract.CodeInternal {
		d.Logger.Error("herald/contract: internal error answering intent",
			forge.F("intent", intent),
			forge.F("error", err),
		)
	}
	return mapped
}

func isAny(err error, targets ...error) bool {
	for _, t := range targets {
		if errors.Is(err, t) {
			return true
		}
	}
	return false
}

func badRequest(msg string) error {
	return &contract.Error{Code: contract.CodeBadRequest, Message: msg}
}

func notFound(msg string) error {
	return &contract.Error{Code: contract.CodeNotFound, Message: msg}
}

func conflict(msg string) error {
	return &contract.Error{Code: contract.CodeConflict, Message: msg}
}
