package extension

import (
	"github.com/xraph/forge"

	"github.com/xraph/herald/api"
)

// mountAPI registers the REST API under basePath behind the host's
// middleware and reports whether anything protects it.
//
// This deliberately doesn't offer forge's WithGroupAuth: at forge v1.10.0 and
// v1.11.2 it only writes auth metadata for the OpenAPI generator, and nothing
// enforces it, so it would make the API document auth while serving every
// request. Middleware is the only thing that actually runs.
func mountAPI(router forge.Router, a *api.ForgeAPI, basePath string, mw []forge.Middleware, logger forge.Logger) bool {
	group := router.Group(basePath)
	if len(mw) > 0 {
		// Use, not WithGroupMiddleware: forge's sub-groups copy the parent's
		// Use chain but not its group-option middleware, and RegisterRoutes
		// opens a sub-group per resource. The option would guard nothing.
		group.Use(mw...)
	}
	a.RegisterRoutes(group)
	if len(mw) == 0 && logger != nil {
		logger.Warn("herald: the REST API is mounted with no authentication; anyone who can reach it can read and change notification data. Protect it with extension.WithAPIMiddleware.",
			forge.F("base_path", basePath))
	}
	return len(mw) > 0
}
