package contract

import (
	"strings"

	"github.com/xraph/forge/extensions/dashboard/contract"
)

// resolveApp decides which Herald app a dashboard request reads and writes.
//
// Three rules, kept separate on purpose. A usable app_id claim wins. A claim
// that is present but unusable (empty, blank, not a string, nil) is refused:
// that session belongs to some tenant whose claim failed to resolve, and
// answering it with the default app's data is the cross-tenant bug this rule
// exists to stop. Only a session with no claim at all falls back, to the
// configured DefaultAppID, which is "" unless set. "" is a real app in
// Herald (an exact match in every store), not a wildcard.
//
// Nothing on the dashboard path populates Principal.Claims today, so the
// fallback is the behaviour every deployment actually runs.
func resolveApp(p contract.Principal, deps Deps) (string, error) {
	if raw, present := p.Claims["app_id"]; present {
		s, ok := raw.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return "", &contract.Error{Code: contract.CodePermissionDenied, Message: "the app on this session can't be read"}
		}
		return s, nil
	}
	return deps.DefaultAppID, nil
}

// appLabel is how the dashboard names an app in headers.
func appLabel(appID string) string {
	if appID == "" {
		return "Default app"
	}
	return appID
}
