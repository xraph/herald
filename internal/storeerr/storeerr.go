// Package storeerr holds the not-found sentinels that packages below store
// need to match. store imports scope, so scope can't import store; both point
// at the values here, which keeps errors.Is working across either name.
package storeerr

import "errors"

var (
	// ErrProviderNotFound is store.ErrProviderNotFound.
	ErrProviderNotFound = errors.New("herald: provider not found")
	// ErrScopedConfigNotFound is store.ErrScopedConfigNotFound.
	ErrScopedConfigNotFound = errors.New("herald: scoped config not found")
)
