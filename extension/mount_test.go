package extension

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xraph/forge"

	"github.com/xraph/herald"
	"github.com/xraph/herald/api"
	"github.com/xraph/herald/store/memory"
)

// TestMiddlewareGuardsEveryRoute proves the middleware runs, on a route from
// every group. A metadata-only option would pass a test that only checked
// the option was set.
func TestMiddlewareGuardsEveryRoute(t *testing.T) {
	h, err := herald.New(herald.WithStore(memory.New()))
	if err != nil {
		t.Fatal(err)
	}
	router := forge.NewRouter()
	deny := func(forge.Handler) forge.Handler {
		return func(forge.Context) error { return forge.NewHTTPError(http.StatusUnauthorized, "denied") }
	}
	if !mountAPI(router, api.NewForgeAPI(h.Store(), h, forge.NewNoopLogger()), "/herald", []forge.Middleware{deny}, nil) {
		t.Fatal("mountAPI reported unprotected with middleware given")
	}
	for _, r := range []struct{ method, path string }{
		{http.MethodGet, "/herald/v1/providers"},
		{http.MethodGet, "/herald/v1/templates"},
		{http.MethodPost, "/herald/v1/send"},
		{http.MethodGet, "/herald/v1/messages"},
		{http.MethodGet, "/herald/v1/inbox"},
		{http.MethodPut, "/herald/v1/preferences"},
		{http.MethodGet, "/herald/v1/config"},
	} {
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), r.method, r.path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", r.method, r.path, rec.Code)
		}
	}
}

func TestNoMiddlewareIsReportedAsUnprotected(t *testing.T) {
	h, _ := herald.New(herald.WithStore(memory.New()))
	if mountAPI(forge.NewRouter(), api.NewForgeAPI(h.Store(), h, forge.NewNoopLogger()), "/herald", nil, nil) {
		t.Error("no middleware must report unprotected")
	}
}
