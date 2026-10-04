package contract

import (
	"context"
	"strings"

	"github.com/xraph/forge/extensions/dashboard/contract"

	"github.com/xraph/herald/bridge"
)

// actorFrom is the operator a command's audit event names: the session's
// subject, trimmed, or "" when there is none. An unattributed write shows no
// actor rather than a wrong one. Only commands call it.
func actorFrom(p contract.Principal) string {
	if p.User == nil {
		return ""
	}
	return strings.TrimSpace(p.User.Subject)
}

// audit records a dashboard command with the operator as actor and the
// resolved app as tenant. meta must never carry a credential value.
//
//nolint:unparam // resource is "provider" until the template, scope and message commands land.
func audit(ctx context.Context, deps Deps, p contract.Principal, appID, action, resource, resourceID string, meta map[string]string) {
	deps.Herald.Audit(ctx, bridge.SeverityInfo, bridge.OutcomeSuccess, "dashboard."+action, resource, resourceID, actorFrom(p), appID, "dashboard", meta)
}
